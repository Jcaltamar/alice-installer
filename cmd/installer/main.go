// Package main is the entry point for alice-installer.
// It wires all real dependencies and launches the Bubbletea TUI or, when
// --unattended is passed, the headless sequential installer.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/assets"
	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/docker"
	"github.com/jcaltamar/alice-installer/internal/envgen"
	"github.com/jcaltamar/alice-installer/internal/headless"
	"github.com/jcaltamar/alice-installer/internal/platform"
	"github.com/jcaltamar/alice-installer/internal/ports"
	"github.com/jcaltamar/alice-installer/internal/preflight"
	"github.com/jcaltamar/alice-installer/internal/secrets"
	"github.com/jcaltamar/alice-installer/internal/theme"
	"github.com/jcaltamar/alice-installer/internal/tui"
)

// version is overridden at build time:
//
//	-ldflags "-X main.version=v1.2.3"
var version = "dev"

// flags holds parsed command-line options.
type flags struct {
	ShowVersion  bool
	DryRun       bool
	EnvOutput    string // default "./.env"
	MediaDir     string // default "/opt/alice-media"
	ConfigDir    string // default "/opt/alice-config"
	WorkspaceDir string // default: ${XDG_CONFIG_HOME:-$HOME/.config}/alice-guardian

	// Headless / unattended mode
	Unattended          bool   // --unattended: skip TUI, run sequentially
	WorkspaceName       string // --workspace-name: WORKSPACE value in .env
	AcceptAllBootstrap  bool   // --accept-all-bootstrap: auto-run bootstrap actions
	Deploy              bool   // --deploy: run docker compose up after env-write
	SkipPull            bool   // --skip-pull: skip docker compose pull (env-write only)
}

// defaultWorkspaceDir resolves the default WorkspaceDir from XDG_CONFIG_HOME or $HOME/.config.
func defaultWorkspaceDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "alice-guardian")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "alice-guardian")
	}
	return filepath.Join(home, ".config", "alice-guardian")
}

// depsFactoryFunc is the type for dependency-factory functions.
// The factory accepts a context and parsed flags and returns a tui.Dependencies struct.
// It is injectable to allow tests to replace the factory without using os.Args.
type depsFactoryFunc func(ctx context.Context, f flags) tui.Dependencies

// defaultPorts returns the default TCP port assignments used across all services.
var defaultPorts = map[string]int{
	"POSTGRES_PORT":      5432,
	"BACKEND_PORT":       9090,
	"WEBSOCKET_PORT":     4550,
	"WEB_PORT":           8080,
	"RTSP_PORT":          8554,
	"REDIS_PORT":         6379,
	"HLS_PORT":           8888,
	"HLS_PORT2":          8889,
	"HLS_PORT3":          8890,
	"RTMP_PORT":          1935,
	"MILVUS_PORT":        19530,
	"MINIO_API_PORT":     9000,
	"MINIO_CONSOLE_PORT": 9001,
}

// parseFlags parses os.Args-style slice into a flags struct.
// Uses flag.ContinueOnError so tests can check return errors without os.Exit.
func parseFlags(args []string) (flags, error) {
	var f flags
	fs := flag.NewFlagSet("alice-installer", flag.ContinueOnError)
	fs.BoolVar(&f.ShowVersion, "version", false, "print version and exit")
	fs.BoolVar(&f.DryRun, "dry-run", false, "run preflight only; do not write or deploy")
	fs.StringVar(&f.EnvOutput, "env-output", "./.env", "path to write generated .env")
	fs.StringVar(&f.MediaDir, "media-dir", "/opt/alice-media", "media directory for Docker volume mounts")
	fs.StringVar(&f.ConfigDir, "config-dir", "/opt/alice-config", "config directory for Docker volume mounts")
	fs.StringVar(&f.WorkspaceDir, "workspace-dir", defaultWorkspaceDir(), "user-editable workspace directory for .env and compose files")

	// Headless flags
	fs.BoolVar(&f.Unattended, "unattended", false, "skip TUI and run installer sequentially (headless mode)")
	fs.StringVar(&f.WorkspaceName, "workspace-name", "alice", "WORKSPACE value written into .env (used with --unattended)")
	fs.BoolVar(&f.AcceptAllBootstrap, "accept-all-bootstrap", false, "auto-run all fixable bootstrap actions without prompting (default true when --unattended)")
	fs.BoolVar(&f.Deploy, "deploy", true, "run `docker compose up` after env-write (used with --unattended)")
	fs.BoolVar(&f.SkipPull, "skip-pull", false, "skip `docker compose pull`; write env/compose files only (used with --unattended)")

	if err := fs.Parse(args); err != nil {
		return f, err
	}

	// When --unattended is set, AcceptAllBootstrap defaults to true unless
	// the user explicitly passed --accept-all-bootstrap=false.
	if f.Unattended {
		// Check if --accept-all-bootstrap was explicitly provided.
		acceptAllExplicit := false
		fs.Visit(func(fg *flag.Flag) {
			if fg.Name == "accept-all-bootstrap" {
				acceptAllExplicit = true
			}
		})
		if !acceptAllExplicit {
			f.AcceptAllBootstrap = true
		}
	}

	return f, nil
}

// newDependencies constructs all production implementations and returns a tui.Dependencies.
func newDependencies(_ context.Context, f flags) tui.Dependencies {
	th := theme.Default()
	osGuard := platform.NewRuntimeOSGuard(nil)
	archDetector := platform.NewRuntimeArchDetector(nil)
	portScanner := ports.NewNetPortScanner()
	dockerClient := docker.NewCLIDocker(nil)
	composeRunner := compose.NewCLICompose(nil, nil)
	gpuDetector := platform.NewDockerGPUDetector(nil)
	passwordGen := secrets.CryptoRandGenerator{}
	templater := &envgen.Templater{PasswordGen: passwordGen}
	writer := envgen.AtomicWriter{}

	coord := preflight.Coordinator{
		OS:                osGuard,
		Arch:              archDetector,
		Docker:            dockerClient,
		Compose:           composeRunner,
		GPU:               gpuDetector,
		Ports:             portScanner,
		Dirs:              preflight.OSDirChecker{},
		MediaDir:          f.MediaDir,
		ConfigDir:         f.ConfigDir,
		WorkspaceDir:      f.WorkspaceDir,
		RequiredTCPPorts:  tcpPortValues(defaultPorts),
		MinDockerVersion:  "24.0.0",
		MinComposeVersion: "2.21.0",
	}

	embeddedAssets := tui.TemplateAssets{
		BaselineYAML: assets.DockerComposeYAML,
		OverlayYAML:  assets.DockerComposeGPU,
		EnvExample:   assets.EnvExample,
	}

	return tui.Dependencies{
		Theme:                th,
		OS:                   osGuard,
		Arch:                 archDetector,
		GPU:                  gpuDetector,
		Ports:                portScanner,
		Docker:               dockerClient,
		Compose:              composeRunner,
		Envgen:               templater,
		Writer:               writer,
		Assets:               embeddedAssets,
		PreflightCoordinator: coord,
		Executor:             tui.NewExecutor(),
		Env:                  tui.DetectEnv(),
		MediaDir:             f.MediaDir,
		ConfigDir:            f.ConfigDir,
		WorkspaceDir:         f.WorkspaceDir,
		RequiredTCPPorts:     defaultPorts,
	}
}

// tcpPortValues extracts the integer port values from the port map.
func tcpPortValues(m map[string]int) []int {
	vals := make([]int, 0, len(m))
	for _, v := range m {
		vals = append(vals, v)
	}
	return vals
}

// run is the testable entrypoint. It accepts args, writers, and an optional
// depsFactory (nil → use newDependencies). Returns exit code.
func run(args []string, out, errOut io.Writer, factory depsFactoryFunc) int {
	f, err := parseFlags(args)
	if err != nil {
		if err == flag.ErrHelp {
			// --help: flag package already wrote usage to stdout
			return 0
		}
		fmt.Fprintln(errOut, "error:", err)
		return 2
	}

	if f.ShowVersion {
		fmt.Fprintln(out, "alice-installer", version)
		return 0
	}

	if factory == nil {
		factory = newDependencies
	}

	ctx := context.Background()

	if f.DryRun {
		deps := factory(ctx, f)
		report := deps.PreflightCoordinator.Run(ctx)
		fmt.Fprintln(out, "=== alice-installer --dry-run preflight report ===")
		for _, item := range report.Items {
			fmt.Fprintf(out, "  [%s] %s: %s\n", item.Status, item.Title, item.Detail)
		}
		if report.HasBlockingFailure() {
			fmt.Fprintln(out, "Result: FAIL — blocking issues found.")
			return 1
		}
		fmt.Fprintln(out, "Result: OK — no blocking issues.")
		return 0
	}

	// Headless / unattended mode — bypasses TUI entirely.
	if f.Unattended {
		tuiDeps := factory(ctx, f)

		cfg := headless.Config{
			WorkspaceName:      f.WorkspaceName,
			AcceptAllBootstrap: f.AcceptAllBootstrap,
			Deploy:             f.Deploy,
			SkipPull:           f.SkipPull || !f.Deploy, // skip pull when deploy is disabled
			GPUDetected:        tuiDeps.GPU.Detect(ctx).ToolkitInstalled,
		}
		hdeps := headless.Dependencies{
			PreflightCoordinator: tuiDeps.PreflightCoordinator,
			Ports:                tuiDeps.Ports,
			Envgen:               tuiDeps.Envgen,
			Writer:               tuiDeps.Writer,
			Assets: headless.TemplateAssets{
				BaselineYAML: tuiDeps.Assets.BaselineYAML,
				OverlayYAML:  tuiDeps.Assets.OverlayYAML,
				EnvExample:   tuiDeps.Assets.EnvExample,
			},
			Compose:          tuiDeps.Compose,
			Arch:             tuiDeps.Arch,
			Env:              tuiDeps.Env,
			WorkspaceDir:     tuiDeps.WorkspaceDir,
			MediaDir:         tuiDeps.MediaDir,
			ConfigDir:        tuiDeps.ConfigDir,
			RequiredTCPPorts: tuiDeps.RequiredTCPPorts,
		}

		if runErr := headless.Run(ctx, cfg, hdeps, out); runErr != nil {
			fmt.Fprintln(errOut, "error:", runErr)
			// EX_TEMPFAIL (75) for ErrReloginRequired
			if isReloginRequired(runErr) {
				return 75
			}
			return 1
		}
		return 0
	}

	// TTY check: abort if stdin is not a terminal (REQ-TUI-7).
	if !isTTY(os.Stdin) {
		fmt.Fprintln(errOut, "alice-installer: stdin is not a terminal. Run interactively in a TTY.")
		return 2
	}

	deps := factory(ctx, f)
	model := tui.NewModel(deps)
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(errOut, "fatal:", err)
		return 1
	}
	return 0
}

// isReloginRequired returns true when the error chain contains ErrReloginRequired.
func isReloginRequired(err error) bool {
	return errors.Is(err, headless.ErrReloginRequired)
}

// isTTY returns true when f is connected to a real terminal device.
// Uses stdlib syscall bits — no external deps.
func isTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, nil))
}
