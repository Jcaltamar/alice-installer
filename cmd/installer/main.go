// Package main is the entry point for alice-installer.
// It wires all real dependencies and launches the Bubbletea TUI.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/assets"
	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/docker"
	"github.com/jcaltamar/alice-installer/internal/envgen"
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
	ShowVersion bool
	DryRun      bool
	EnvOutput   string // default "./.env"
	MediaDir    string // default "/opt/alice-media"
	ConfigDir   string // default "/opt/alice-config"
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
	"QUEUE_PORT":         3000,
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
	fs.StringVar(&f.MediaDir, "media-dir", "/opt/alice-media", "media directory")
	fs.StringVar(&f.ConfigDir, "config-dir", "/opt/alice-config", "config directory")

	if err := fs.Parse(args); err != nil {
		return f, err
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
