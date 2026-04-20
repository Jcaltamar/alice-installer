// Package headless implements the unattended (non-interactive) installation path.
// It orchestrates the same stages as the TUI — preflight, bootstrap, env-write,
// pull, deploy, verify — but runs them sequentially and writes all progress to
// an io.Writer instead of a terminal UI.
package headless

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jcaltamar/alice-installer/internal/bootstrap"
	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/envgen"
	"github.com/jcaltamar/alice-installer/internal/platform"
	"github.com/jcaltamar/alice-installer/internal/ports"
	"github.com/jcaltamar/alice-installer/internal/preflight"
)

// ErrReloginRequired is returned when the docker_group_add bootstrap action
// succeeds but the new group membership cannot take effect in the current
// process. The caller must re-run the installer after a login refresh
// (e.g. via `sg docker -c 'alice-installer --unattended'`).
//
// Exit code convention: EX_TEMPFAIL (75).
var ErrReloginRequired = errors.New("docker group membership added — please log out and back in (or run `newgrp docker`), then re-run the installer")

// ErrPreflightStillFailing is returned when preflight remains blocking after
// all fixable bootstrap actions completed successfully.
var ErrPreflightStillFailing = errors.New("preflight still failing after bootstrap")

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

// Config carries the headless-mode options passed from the CLI.
type Config struct {
	// WorkspaceName is written into the .env WORKSPACE variable.
	WorkspaceName string

	// AcceptAllBootstrap automatically runs fixable bootstrap actions without
	// asking for confirmation. Defaults to true in --unattended mode.
	AcceptAllBootstrap bool

	// Deploy controls whether `docker compose up` is executed.
	// When false, Run stops after writing env/compose files.
	Deploy bool

	// SkipPull skips `docker compose pull` when true.
	SkipPull bool

	// GPUDetected controls whether the GPU overlay compose file is included.
	GPUDetected bool

	// VerifyTimeout overrides the default 60s health-check timeout.
	// When zero, the production default (60s) is used.
	VerifyTimeout time.Duration

	// VerifyPollInterval overrides the default 3s poll interval.
	// When zero, the production default (3s) is used.
	VerifyPollInterval time.Duration
}

// ---------------------------------------------------------------------------
// Dependencies
// ---------------------------------------------------------------------------

// Dependencies holds the injectable collaborators for headless.Run.
// It mirrors the subset of tui.Dependencies actually needed — deliberately
// omitting Theme and TUI-only fields to keep the dependency surface minimal
// and to guarantee zero imports of the tui package.
type Dependencies struct {
	PreflightCoordinator preflight.Coordinator
	Ports                ports.PortScanner
	Envgen               *envgen.Templater
	Writer               envgen.FileWriter
	Assets               TemplateAssets
	Compose              compose.ComposeRunner
	Arch                 platform.ArchDetector
	Env                  bootstrap.BootstrapEnv

	// Directories
	WorkspaceDir string
	MediaDir     string
	ConfigDir    string

	// RequiredTCPPorts is the env-key → default port map used for port scanning.
	RequiredTCPPorts map[string]int

	// CmdExecutor is the test seam for running bootstrap actions.
	// When nil, the default OS-backed executor is used.
	CmdExecutor CmdExecutor
}

// TemplateAssets bundles the embedded installer assets needed by headless path.
type TemplateAssets struct {
	BaselineYAML []byte
	OverlayYAML  []byte
	EnvExample   []byte
}

// ---------------------------------------------------------------------------
// CmdExecutor seam
// ---------------------------------------------------------------------------

// CmdExecutor abstracts os/exec so tests can inject a fake without running real commands.
type CmdExecutor interface {
	// Run executes the command and returns combined stdout+stderr and an error.
	Run(name string, args ...string) ([]byte, error)
}

// osCmdExecutor is the production CmdExecutor that shells out via os/exec.
type osCmdExecutor struct{}

func (osCmdExecutor) Run(name string, args ...string) ([]byte, error) {
	c := exec.Command(name, args...) //nolint:gosec
	out, err := c.CombinedOutput()
	return out, err
}

// ---------------------------------------------------------------------------
// Run
// ---------------------------------------------------------------------------

// Run executes the full install sequence in headless (unattended) mode.
// All progress is written to out using "[stage] message" lines.
// Returns nil on success, or a descriptive error (possibly ErrReloginRequired
// or ErrPreflightStillFailing) on failure.
func Run(ctx context.Context, cfg Config, deps Dependencies, out io.Writer) error {
	exec := deps.CmdExecutor
	if exec == nil {
		exec = osCmdExecutor{}
	}

	// ------------------------------------------------------------------
	// 1. Preflight
	// ------------------------------------------------------------------
	logf(out, "preflight", "running checks…")
	report := deps.PreflightCoordinator.Run(ctx)
	for _, item := range report.Items {
		logf(out, "preflight", "[%s] %s: %s", item.Status, item.Title, item.Detail)
	}

	if report.HasBlockingFailure() {
		fixable, nonFixable := bootstrap.ClassifyBlockers(
			report, deps.Env, deps.MediaDir, deps.ConfigDir, deps.WorkspaceDir,
		)

		if len(nonFixable) > 0 {
			var msgs []string
			for _, nf := range nonFixable {
				msgs = append(msgs, fmt.Sprintf("%s: %s", nf.Title, nf.Detail))
			}
			return fmt.Errorf("preflight: non-fixable failures:\n  %s", strings.Join(msgs, "\n  "))
		}

		if len(fixable) == 0 {
			// Should not happen, but guard anyway.
			return fmt.Errorf("preflight: blocking failures with no known remediation")
		}

		if !cfg.AcceptAllBootstrap {
			return fmt.Errorf("preflight: fixable failures found; re-run with --accept-all-bootstrap to auto-fix")
		}

		// Check for docker_group_add before running — it requires a re-login.
		for _, a := range fixable {
			if a.ID == bootstrap.ActionIDDockerGroup {
				// Run the action first so the group is actually added.
				logf(out, "bootstrap", "executing: %s %s", a.Command, strings.Join(a.Args, " "))
				if out2, err := exec.Run(a.Command, a.Args...); err != nil {
					return fmt.Errorf("bootstrap: action %q failed: %w\n%s", a.ID, err, string(out2))
				}
				logf(out, "bootstrap", "action %q succeeded", a.ID)
				return fmt.Errorf("%w", ErrReloginRequired)
			}
		}

		// Run all other fixable actions.
		for _, a := range fixable {
			logf(out, "bootstrap", "executing: %s %s", a.Command, strings.Join(a.Args, " "))
			if out2, runErr := exec.Run(a.Command, a.Args...); runErr != nil {
				return fmt.Errorf("bootstrap: action %q failed: %w\n%s", a.ID, runErr, string(out2))
			}
			logf(out, "bootstrap", "action %q succeeded", a.ID)
		}

		// Re-run preflight.
		logf(out, "preflight", "re-running checks after bootstrap…")
		report = deps.PreflightCoordinator.Run(ctx)
		for _, item := range report.Items {
			logf(out, "preflight", "[%s] %s: %s", item.Status, item.Title, item.Detail)
		}
		if report.HasBlockingFailure() {
			var msgs []string
			for _, item := range report.Failures() {
				msgs = append(msgs, fmt.Sprintf("%s: %s", item.Title, item.Detail))
			}
			return fmt.Errorf("%w: %s", ErrPreflightStillFailing, strings.Join(msgs, "; "))
		}
	}

	logf(out, "preflight", "all checks passed")

	// ------------------------------------------------------------------
	// 2. Port scan (warn-only in v1 — no interactive resolution)
	// ------------------------------------------------------------------
	logf(out, "portscan", "scanning required ports…")
	tcpPorts := tcpPortValues(deps.RequiredTCPPorts)
	var conflicts []string
	for _, p := range tcpPorts {
		if !deps.Ports.IsAvailable(ctx, p) {
			conflicts = append(conflicts, fmt.Sprintf("TCP %d", p))
		}
	}
	if len(conflicts) > 0 {
		return fmt.Errorf("portscan: port conflicts (no interactive resolution in unattended mode): %s",
			strings.Join(conflicts, ", "))
	}
	logf(out, "portscan", "all ports available")

	// ------------------------------------------------------------------
	// 3. Env-write
	// ------------------------------------------------------------------
	logf(out, "env-write", "writing .env and compose files to %s…", deps.WorkspaceDir)
	if err := os.MkdirAll(deps.WorkspaceDir, 0o755); err != nil {
		return fmt.Errorf("env-write: create workspace dir: %w", err)
	}

	arch := deps.Arch.Detect()
	portsConfig := portsConfigFromMap(deps.RequiredTCPPorts)
	envInput := envgen.Input{
		Workspace:        cfg.WorkspaceName,
		Arch:             arch,
		Ports:            portsConfig,
		GeneratePassword: true,
	}

	rendered, err := deps.Envgen.Render(deps.Assets.EnvExample, envInput)
	if err != nil {
		return fmt.Errorf("env-write: render: %w", err)
	}

	envPath := filepath.Join(deps.WorkspaceDir, ".env")
	if err := deps.Writer.WriteEnv(envPath, rendered); err != nil {
		return fmt.Errorf("env-write: write .env: %w", err)
	}
	logf(out, "env-write", "wrote %s", envPath)

	composePath := filepath.Join(deps.WorkspaceDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, deps.Assets.BaselineYAML, 0o644); err != nil {
		return fmt.Errorf("env-write: write docker-compose.yml: %w", err)
	}
	logf(out, "env-write", "wrote %s", composePath)

	composeGPUPath := filepath.Join(deps.WorkspaceDir, "docker-compose.gpu.yml")
	if err := os.WriteFile(composeGPUPath, deps.Assets.OverlayYAML, 0o644); err != nil {
		return fmt.Errorf("env-write: write docker-compose.gpu.yml: %w", err)
	}
	logf(out, "env-write", "wrote %s", composeGPUPath)

	composeFiles := compose.ComposeFiles(cfg.GPUDetected, composePath, composeGPUPath)

	// ------------------------------------------------------------------
	// 4. Pull
	// ------------------------------------------------------------------
	if !cfg.SkipPull {
		logf(out, "pull", "pulling images…")
		progressCh := make(chan compose.PullProgressMsg, 64)
		pullDone := make(chan error, 1)
		go func() {
			pullDone <- deps.Compose.Pull(ctx, composeFiles, envPath, progressCh)
		}()
		go func() {
			for msg := range progressCh {
				if msg.Raw != "" {
					logf(out, "pull", "%s", msg.Raw)
				}
			}
		}()
		if pullErr := <-pullDone; pullErr != nil {
			return fmt.Errorf("pull: %w", pullErr)
		}
		logf(out, "pull", "images pulled successfully")
	}

	// ------------------------------------------------------------------
	// 5. Deploy
	// ------------------------------------------------------------------
	if cfg.Deploy {
		logf(out, "deploy", "starting services…")
		upCh := make(chan compose.UpProgressMsg, 64)
		upDone := make(chan error, 1)
		go func() {
			upDone <- deps.Compose.Up(ctx, composeFiles, envPath, upCh)
		}()
		go func() {
			for msg := range upCh {
				if msg.Raw != "" {
					logf(out, "deploy", "%s", msg.Raw)
				}
			}
		}()
		if upErr := <-upDone; upErr != nil {
			return fmt.Errorf("deploy: %w", upErr)
		}
		logf(out, "deploy", "services started")

		// ------------------------------------------------------------------
		// 6. Verify
		// ------------------------------------------------------------------
		logf(out, "verify", "waiting for services to become healthy…")
		healthPollInterval := cfg.VerifyPollInterval
		if healthPollInterval == 0 {
			healthPollInterval = 3 * time.Second
		}
		healthTimeout := cfg.VerifyTimeout
		if healthTimeout == 0 {
			healthTimeout = 60 * time.Second
		}
		deadline := time.Now().Add(healthTimeout)
		for time.Now().Before(deadline) {
			statuses, healthErr := deps.Compose.HealthStatus(ctx, composeFiles, envPath)
			if healthErr != nil {
				logf(out, "verify", "health check error: %v — retrying", healthErr)
				time.Sleep(healthPollInterval)
				continue
			}

			allReady := true
			var unhealthy []string
			for _, s := range statuses {
				if !compose.IsReady(s) {
					allReady = false
					label := fmt.Sprintf("%s(%s/%s)", s.Service, s.Status, s.State)
					unhealthy = append(unhealthy, label)
				}
			}

			if allReady && len(statuses) > 0 {
				logf(out, "verify", "all %d services healthy", len(statuses))
				return nil
			}

			logf(out, "verify", "not yet healthy: %s — retrying in %s", strings.Join(unhealthy, ", "), healthPollInterval)
			time.Sleep(healthPollInterval)
		}

		// Timeout: gather final state for error message.
		statuses, _ := deps.Compose.HealthStatus(ctx, composeFiles, envPath)
		var unhealthy []string
		for _, s := range statuses {
			if !compose.IsReady(s) {
				label := fmt.Sprintf("%s(%s/%s)", s.Service, s.Status, s.State)
				unhealthy = append(unhealthy, label)
			}
		}
		return fmt.Errorf("verify: timed out waiting for healthy services after %s; unhealthy: %s",
			healthTimeout, strings.Join(unhealthy, ", "))
	}

	logf(out, "done", "env files written; deploy skipped (--deploy=false)")
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// logf writes a "[stage] message" line to w.
func logf(w io.Writer, stage, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(w, "[%s] %s\n", stage, msg)
}

// tcpPortValues extracts the integer port values from the port map.
func tcpPortValues(m map[string]int) []int {
	vals := make([]int, 0, len(m))
	for _, v := range m {
		vals = append(vals, v)
	}
	return vals
}

// portsConfigFromMap converts the flat env-key → port map into envgen.PortsConfig.
func portsConfigFromMap(p map[string]int) envgen.PortsConfig {
	return envgen.PortsConfig{
		PostgresPort:     p["POSTGRES_PORT"],
		BackendPort:      p["BACKEND_PORT"],
		WebsocketPort:    p["WEBSOCKET_PORT"],
		WebPort:          p["WEB_PORT"],
		RTSPPort:         p["RTSP_PORT"],
		RedisPort:        p["REDIS_PORT"],
		HLSPort:          p["HLS_PORT"],
		HLSPort2:         p["HLS_PORT2"],
		HLSPort3:         p["HLS_PORT3"],
		RTMPPort:         p["RTMP_PORT"],
		MilvusPort:       p["MILVUS_PORT"],
		MinioAPIPort:     p["MINIO_API_PORT"],
		MinioConsolePort: p["MINIO_CONSOLE_PORT"],
	}
}
