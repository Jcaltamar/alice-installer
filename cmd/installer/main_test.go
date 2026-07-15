package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/jcaltamar/alice-installer/internal/asterisk"
	"github.com/jcaltamar/alice-installer/internal/bootstrap"
	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/headless"
	"github.com/jcaltamar/alice-installer/internal/installation"
	"github.com/jcaltamar/alice-installer/internal/migration"
	"github.com/jcaltamar/alice-installer/internal/platform"
	"github.com/jcaltamar/alice-installer/internal/restart"
	"github.com/jcaltamar/alice-installer/internal/tui"
	"github.com/jcaltamar/alice-installer/internal/update"
)

func TestParseCLI_CommandModeAndArgNormalization(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantMode    cliMode
		wantArgs    []string
		wantErrText string
	}{
		{
			name:     "update as first positional",
			args:     []string{"update", "--workspace-dir", "/tmp/ws"},
			wantMode: modeUpdate,
			wantArgs: []string{"--workspace-dir", "/tmp/ws"},
		},
		{
			name:     "restart as first positional",
			args:     []string{"restart", "--workspace-dir", "/tmp/ws"},
			wantMode: modeRestart,
			wantArgs: []string{"--workspace-dir", "/tmp/ws"},
		},
		{
			name:     "update after flags",
			args:     []string{"--workspace-dir", "/tmp/ws", "update"},
			wantMode: modeUpdate,
			wantArgs: []string{"--workspace-dir", "/tmp/ws"},
		},
		{
			name:     "restart after flags",
			args:     []string{"--workspace-dir", "/tmp/ws", "restart"},
			wantMode: modeRestart,
			wantArgs: []string{"--workspace-dir", "/tmp/ws"},
		},
		{
			name:     "legacy unattended unchanged",
			args:     []string{"--unattended", "--workspace-name", "site-a"},
			wantMode: modeInstall,
			wantArgs: []string{"--unattended", "--workspace-name", "site-a"},
		},
		{
			name:        "duplicate update positional",
			args:        []string{"update", "--workspace-dir", "/tmp/ws", "update"},
			wantErrText: "duplicate",
		},
		{
			name:        "duplicate restart positional",
			args:        []string{"restart", "--workspace-dir", "/tmp/ws", "restart"},
			wantErrText: "duplicate",
		},
		{
			name:        "mixed commands update and restart",
			args:        []string{"update", "--workspace-dir", "/tmp/ws", "restart"},
			wantErrText: "multiple commands",
		},
		{
			name:        "unknown positional",
			args:        []string{"upgrade"},
			wantErrText: "unknown positional",
		},
		{
			name:     "workspace name value update is not positional command",
			args:     []string{"--workspace-name", "update", "--unattended"},
			wantMode: modeInstall,
			wantArgs: []string{"--workspace-name", "update", "--unattended"},
		},
		{
			name:     "workspace name value restart is not positional command",
			args:     []string{"--workspace-name", "restart", "--unattended"},
			wantMode: modeInstall,
			wantArgs: []string{"--workspace-name", "restart", "--unattended"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMode, gotArgs, err := parseCLI(tt.args)
			if tt.wantErrText != "" {
				if err == nil {
					t.Fatalf("parseCLI() expected error containing %q, got nil", tt.wantErrText)
				}
				if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.wantErrText)) {
					t.Fatalf("parseCLI() error = %q, want contains %q", err.Error(), tt.wantErrText)
				}
				return
			}

			if err != nil {
				t.Fatalf("parseCLI() unexpected error: %v", err)
			}
			if gotMode != tt.wantMode {
				t.Fatalf("parseCLI() mode = %q, want %q", gotMode, tt.wantMode)
			}
			if len(gotArgs) != len(tt.wantArgs) {
				t.Fatalf("parseCLI() args len = %d, want %d; got=%v want=%v", len(gotArgs), len(tt.wantArgs), gotArgs, tt.wantArgs)
			}
			for i := range tt.wantArgs {
				if gotArgs[i] != tt.wantArgs[i] {
					t.Fatalf("parseCLI() args[%d] = %q, want %q", i, gotArgs[i], tt.wantArgs[i])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseFlags tests
// ---------------------------------------------------------------------------

func TestParseFlags_Version(t *testing.T) {
	f, err := parseFlags([]string{"--version"})
	if err != nil {
		t.Fatalf("parseFlags(--version) error: %v", err)
	}
	if !f.ShowVersion {
		t.Error("expected ShowVersion=true")
	}
}

func TestParseFlags_DryRun(t *testing.T) {
	f, err := parseFlags([]string{"--dry-run"})
	if err != nil {
		t.Fatalf("parseFlags(--dry-run) error: %v", err)
	}
	if !f.DryRun {
		t.Error("expected DryRun=true")
	}
}

func TestParseFlags_Defaults(t *testing.T) {
	f, err := parseFlags([]string{})
	if err != nil {
		t.Fatalf("parseFlags([]) error: %v", err)
	}
	if f.ShowVersion {
		t.Error("expected ShowVersion=false")
	}
	if f.DryRun {
		t.Error("expected DryRun=false")
	}
	if f.EnvOutput != "./.env" {
		t.Errorf("EnvOutput default = %q, want ./.env", f.EnvOutput)
	}
	if f.MediaDir != "/opt/alice-media" {
		t.Errorf("MediaDir default = %q, want /opt/alice-media", f.MediaDir)
	}
	if f.ConfigDir != "/opt/alice-config" {
		t.Errorf("ConfigDir default = %q, want /opt/alice-config", f.ConfigDir)
	}
}

func TestParseFlags_EnvOutput(t *testing.T) {
	f, err := parseFlags([]string{"--env-output", "/custom/.env"})
	if err != nil {
		t.Fatalf("parseFlags error: %v", err)
	}
	if f.EnvOutput != "/custom/.env" {
		t.Errorf("EnvOutput = %q, want /custom/.env", f.EnvOutput)
	}
}

func TestParseFlags_MediaAndConfig(t *testing.T) {
	f, err := parseFlags([]string{"--media-dir", "/mnt/media", "--config-dir", "/mnt/config"})
	if err != nil {
		t.Fatalf("parseFlags error: %v", err)
	}
	if f.MediaDir != "/mnt/media" {
		t.Errorf("MediaDir = %q, want /mnt/media", f.MediaDir)
	}
	if f.ConfigDir != "/mnt/config" {
		t.Errorf("ConfigDir = %q, want /mnt/config", f.ConfigDir)
	}
}

func TestParseFlags_UnknownFlagError(t *testing.T) {
	_, err := parseFlags([]string{"--unknown-flag"})
	if err == nil {
		t.Error("expected error for unknown flag, got nil")
	}
}

// ---------------------------------------------------------------------------
// newDependencies tests
// ---------------------------------------------------------------------------

func TestNewDependencies_AllFieldsNonNil(t *testing.T) {
	f := flags{
		MediaDir:  "/opt/alice-media",
		ConfigDir: "/opt/alice-config",
		EnvOutput: "./.env",
	}

	deps := newDependencies(context.Background(), f)

	if deps.OS == nil {
		t.Error("deps.OS is nil")
	}
	if deps.Arch == nil {
		t.Error("deps.Arch is nil")
	}
	if deps.GPU == nil {
		t.Error("deps.GPU is nil")
	}
	if deps.Ports == nil {
		t.Error("deps.Ports is nil")
	}
	if deps.Docker == nil {
		t.Error("deps.Docker is nil")
	}
	if deps.Compose == nil {
		t.Error("deps.Compose is nil")
	}
	if deps.Envgen == nil {
		t.Error("deps.Envgen is nil")
	}
	if deps.Writer == nil {
		t.Error("deps.Writer is nil")
	}
	// PreflightCoordinator is a struct (preflight.Coordinator), not a pointer.
	// Verify it has at least one non-nil interface field (OS).
	if deps.PreflightCoordinator.OS == nil {
		t.Error("deps.PreflightCoordinator.OS is nil")
	}
	if deps.MediaDir == "" {
		t.Error("deps.MediaDir is empty")
	}
	if deps.ConfigDir == "" {
		t.Error("deps.ConfigDir is empty")
	}
}

func TestNewOperationalDependenciesExcludeInteractiveActions(t *testing.T) {
	deps := newOperationalDependencies(context.Background(), flags{MediaDir: "/opt/alice-media", ConfigDir: "/opt/alice-config", WorkspaceDir: t.TempDir()})
	if deps.Detector != nil || deps.UpdateAction != nil || deps.LegacyBackupAction != nil || deps.MigrationHandoff != nil {
		t.Fatal("non-interactive dependencies must not construct contextual actions")
	}
}

func TestNewDependencies_WiresInteractiveDetectionAndUpdate(t *testing.T) {
	workspace := t.TempDir()
	deps := newDependencies(context.Background(), flags{MediaDir: "/opt/alice-media", ConfigDir: "/opt/alice-config", WorkspaceDir: workspace})
	if deps.Detector == nil {
		t.Fatal("interactive detector is nil")
	}
	if deps.UpdateAction == nil {
		t.Fatal("interactive update action is nil")
	}
	composite, ok := deps.Detector.(installation.CompositeDetector)
	if !ok {
		t.Fatalf("interactive detector = %T, want installation.CompositeDetector", deps.Detector)
	}
	if _, ok := composite.Legacy.(installation.LegacyFallbackProbe); !ok {
		t.Fatalf("legacy probe = %T, want installation.LegacyFallbackProbe with known directory policy", composite.Legacy)
	}
	if runtime.GOOS == "linux" && (runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64") {
		if deps.LegacyBackupAction == nil || deps.LegacyRestoreAction == nil || deps.MigrationHandoff == nil || deps.LegacyBackupRequest.ConfigEnvironment != "" || deps.LegacyBackupRequest.Destination != "/opt/alice/backups/" {
			t.Fatalf("supported interactive platforms must wire backup, restore, and PM2 handoff actions for /opt/alice/backups/, got %q", deps.LegacyBackupRequest.Destination)
		}
	} else if deps.LegacyBackupAction != nil || deps.LegacyRestoreAction != nil || deps.MigrationHandoff != nil {
		t.Fatal("unsupported platforms must not wire backup, restore, or PM2 handoff actions")
	}
	// Construction must be side-effect free; the detector is invoked only by the interactive TUI command.
}

func TestRestoreActionPlatformGate(t *testing.T) {
	for _, tt := range []struct {
		goos, goarch string
		want         bool
	}{
		{"linux", "amd64", true}, {"linux", "arm64", true},
		{"windows", "amd64", false}, {"darwin", "arm64", false}, {"linux", "386", false},
	} {
		if got := restoreSupportedPlatform(tt.goos, tt.goarch); got != tt.want {
			t.Fatalf("restoreSupportedPlatform(%q, %q) = %t, want %t", tt.goos, tt.goarch, got, tt.want)
		}
	}
}

func TestNewDependencies_WiresAsteriskInstallerOptionsAndAvailability(t *testing.T) {
	restore := replaceAsteriskHostDetector(func() asterisk.HostDetector {
		return &asterisk.FakeDetector{State: asterisk.SupportedHost(asterisk.PackageManagerAPT)}
	})
	t.Cleanup(restore)

	deps := newDependencies(context.Background(), flags{
		MediaDir:     "/opt/alice-media",
		ConfigDir:    "/opt/alice-config",
		EnvOutput:    "./.env",
		WorkspaceDir: t.TempDir(),
	})

	if deps.AsteriskInstaller == nil {
		t.Fatal("AsteriskInstaller should be wired for production CLI dependencies")
	}
	if deps.AsteriskAvailable == nil || !deps.AsteriskAvailable() {
		t.Fatal("AsteriskAvailable should report true for supported Linux hosts")
	}
	if deps.AsteriskOptions.ConfigRoot != "/opt/alice-config/asterisk" {
		t.Fatalf("AsteriskOptions.ConfigRoot = %q, want /opt/alice-config/asterisk", deps.AsteriskOptions.ConfigRoot)
	}
	if deps.AsteriskOptions.AMI.Username == "" || deps.AsteriskOptions.AMI.Password == "" {
		t.Fatalf("AsteriskOptions should carry generated AMI credentials, got %#v", deps.AsteriskOptions.AMI)
	}
}

func TestNewDependencies_HidesAsteriskWhenHostUnsupported(t *testing.T) {
	restore := replaceAsteriskHostDetector(func() asterisk.HostDetector {
		return &asterisk.FakeDetector{Err: asterisk.UnsupportedHostError{Reason: "unsupported Linux host: install apt, dnf, yum, or pacman before selecting Asterisk"}}
	})
	t.Cleanup(restore)

	deps := newDependencies(context.Background(), flags{
		MediaDir:     "/opt/alice-media",
		ConfigDir:    "/opt/alice-config",
		EnvOutput:    "./.env",
		WorkspaceDir: t.TempDir(),
	})

	if deps.AsteriskAvailable == nil {
		t.Fatal("AsteriskAvailable should be wired even when unsupported")
	}
	if deps.AsteriskAvailable() {
		t.Fatal("AsteriskAvailable should report false for unsupported package-manager hosts")
	}
}

func replaceAsteriskHostDetector(fn func() asterisk.HostDetector) func() {
	previous := newAsteriskHostDetector
	newAsteriskHostDetector = fn
	return func() {
		newAsteriskHostDetector = previous
	}
}

// fakeDepsFactory returns a depsFactoryFunc that produces a tui.Dependencies
// populated with fake implementations suitable for dry-run testing.
func fakeDepsFactory() depsFactoryFunc {
	return func(ctx context.Context, f flags) tui.Dependencies {
		return newDependencies(ctx, f)
	}
}

// ---------------------------------------------------------------------------
// run() testable-unit tests
// ---------------------------------------------------------------------------

func TestRun_VersionFlag(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := runWithoutStaleDockerGroup([]string{"--version"}, &out, &errOut, nil)

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}

	output := out.String()
	if !strings.Contains(output, "alice-installer") {
		t.Errorf("version output %q does not contain 'alice-installer'", output)
	}
	if !strings.Contains(output, version) {
		t.Errorf("version output %q does not contain version %q", output, version)
	}
}

func TestInjectedVersionIsSharedByCLIAndProductionTUI(t *testing.T) {
	previous := version
	version = "v9.8.7-test"
	t.Cleanup(func() { version = previous })

	var out bytes.Buffer
	if code := runWithoutStaleDockerGroup([]string{"--version"}, &out, &bytes.Buffer{}, nil); code != 0 {
		t.Fatalf("version exit code = %d", code)
	}
	deps := buildDependencies(context.Background(), flags{}, false)
	if !strings.Contains(out.String(), version) || deps.Version != version {
		t.Fatalf("CLI/TUI versions = %q/%q, want %q", out.String(), deps.Version, version)
	}
}

func TestRun_HelpFlag(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	// --help with ContinueOnError causes flag.ErrHelp; run() should return 0.
	code := run([]string{"--help"}, &out, &errOut, nil)

	if code != 0 {
		t.Errorf("--help exit code = %d, want 0", code)
	}
}

func TestRun_DryRun_PrintsPreflightReport(t *testing.T) {
	// --dry-run must always print a preflight report.
	// The report may contain failures (e.g., Docker not running in CI),
	// but the important contract is:
	//  1. Output contains the report header.
	//  2. Output contains at least one check result line.
	//  3. Exit code is 0 or 1 (not 2 — that is a flag error).
	var out bytes.Buffer
	var errOut bytes.Buffer

	fakeDeps := fakeDepsFactory()
	code := runWithoutStaleDockerGroup([]string{"--dry-run", "--media-dir", t.TempDir(), "--config-dir", t.TempDir()},
		&out, &errOut, fakeDeps)

	if code == 2 {
		t.Errorf("--dry-run exit code = 2, want 0 or 1 (flag error unexpected); stderr: %s", errOut.String())
	}

	output := out.String()
	// Must print the report header.
	if !strings.Contains(output, "dry-run") {
		t.Errorf("--dry-run output %q does not contain 'dry-run'", output)
	}
	// Must print at least one check result line (OS check always runs).
	if !strings.Contains(output, "OS") && !strings.Contains(output, "[PASS]") && !strings.Contains(output, "[FAIL]") {
		t.Errorf("--dry-run output %q does not contain any check results", output)
	}
}

func TestRun_UnknownFlag_ExitTwo(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--totally-unknown"}, &out, &errOut, nil)

	if code != 2 {
		t.Errorf("unknown flag exit code = %d, want 2", code)
	}
}

func runWithoutStaleDockerGroup(args []string, out, errOut io.Writer, factory depsFactoryFunc) int {
	return runWithStaleCheck(
		args,
		out,
		errOut,
		factory,
		func() (bootstrap.StaleGroupResult, error) {
			return bootstrap.StaleGroupResult{Stale: false}, nil
		},
		nil,
	)
}

// ---------------------------------------------------------------------------
// Stale-group gate tests (Phase 4)
// ---------------------------------------------------------------------------

// TestRunStaleGroupReexecSuccess verifies that when the stale-group detector
// returns Stale=true and the reexec helper succeeds (returns nil), run() returns
// the execFn-signalled exit code 0 without proceeding to the factory/TUI.
func TestRunStaleGroupReexecSuccess(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	// staleChecker: report stale=true
	staleChecker := func() (bootstrap.StaleGroupResult, error) {
		return bootstrap.StaleGroupResult{Stale: true, DockerGID: 999}, nil
	}
	// reexecFn: simulate success (returns nil — process would be replaced for real)
	reexecFn := func(argv []string, env []string) error {
		return nil
	}

	code := runWithStaleCheck(
		[]string{"--unattended", "--workspace-name=test"},
		&out, &errOut,
		nil, // factory — should not be called
		staleChecker,
		reexecFn,
	)

	// When reexec succeeds (nil), run should return 0 — process was "replaced".
	if code != 0 {
		t.Errorf("stale+reexec-ok exit code = %d, want 0; stderr: %s", code, errOut.String())
	}
}

// TestRunStaleGroupReexecFallback verifies that when the stale-group detector
// returns Stale=true but sg is not available (reexec returns an error), run()
// prints a fallback line containing "newgrp docker" and returns exit code 75.
func TestRunStaleGroupReexecFallback(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	staleChecker := func() (bootstrap.StaleGroupResult, error) {
		return bootstrap.StaleGroupResult{Stale: true, DockerGID: 999}, nil
	}
	reexecFn := func(argv []string, env []string) error {
		return &sgNotFoundError{}
	}

	code := runWithStaleCheck(
		[]string{"--unattended", "--workspace-name=test", "--deploy=false"},
		&out, &errOut,
		nil,
		staleChecker,
		reexecFn,
	)

	if code != 75 {
		t.Errorf("stale+no-sg exit code = %d, want 75; stderr: %s", code, errOut.String())
	}
	stderr := errOut.String()
	if !strings.Contains(stderr, "newgrp docker") {
		t.Errorf("expected stderr to contain 'newgrp docker', got: %s", stderr)
	}
}

// sgNotFoundError is a test-only error type simulating sg not being on PATH.
type sgNotFoundError struct{}

func (e *sgNotFoundError) Error() string { return "sg not found: exec: sg: not found in PATH" }

func TestRun_UpdateModeRoutesToUpdateFlow(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	updateCalled := false
	headlessCalled := false
	runUpdateFn = func(_ context.Context, cfg update.Config, deps update.Dependencies, _ io.Writer) error {
		updateCalled = true
		if cfg.WorkspaceDir == "" {
			t.Fatalf("update cfg workspace dir is empty")
		}
		if deps.Compose == nil {
			t.Fatalf("update deps compose is nil")
		}
		if deps.GPU == nil {
			t.Fatalf("update deps gpu is nil")
		}
		return nil
	}
	runHeadlessFn = func(context.Context, headless.Config, headless.Dependencies, io.Writer) error {
		headlessCalled = true
		return nil
	}
	t.Cleanup(func() {
		runUpdateFn = update.Run
		runHeadlessFn = headless.Run
	})

	staleChecker := func() (bootstrap.StaleGroupResult, error) {
		return bootstrap.StaleGroupResult{Stale: false}, nil
	}

	factoryCalls := 0
	factory := func(_ context.Context, f flags) tui.Dependencies {
		factoryCalls++
		return tui.Dependencies{
			Compose:      &compose.FakeComposeRunner{},
			GPU:          &platform.FakeGPUDetector{},
			WorkspaceDir: f.WorkspaceDir,
		}
	}

	code := runWithStaleCheck(
		[]string{"update", "--workspace-dir", t.TempDir()},
		&out,
		&errOut,
		factory,
		staleChecker,
		nil,
	)

	if code != 0 {
		t.Fatalf("update run exit code = %d, want 0; stderr=%s", code, errOut.String())
	}
	if !updateCalled {
		t.Fatalf("expected update flow to be called")
	}
	if headlessCalled {
		t.Fatalf("headless flow should not run in update mode")
	}
	if factoryCalls != 1 {
		t.Fatalf("factory calls = %d, want 1", factoryCalls)
	}
}

func TestRun_UpdateModeFailureReturnsNonZero(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	runUpdateFn = func(context.Context, update.Config, update.Dependencies, io.Writer) error {
		return errors.New("pull failed")
	}
	t.Cleanup(func() { runUpdateFn = update.Run })

	code := runWithStaleCheck(
		[]string{"update", "--workspace-dir", t.TempDir()},
		&out,
		&errOut,
		func(_ context.Context, f flags) tui.Dependencies {
			return tui.Dependencies{
				Compose:      &compose.FakeComposeRunner{},
				GPU:          &platform.FakeGPUDetector{},
				WorkspaceDir: f.WorkspaceDir,
			}
		},
		func() (bootstrap.StaleGroupResult, error) {
			return bootstrap.StaleGroupResult{Stale: false}, nil
		},
		nil,
	)

	if code != 1 {
		t.Fatalf("update error exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "pull failed") {
		t.Fatalf("stderr = %q, want compose error", errOut.String())
	}
}

func TestRun_UpdateModeUsesComposePullThenUp(t *testing.T) {
	workspace := t.TempDir()
	mustWriteTestFile(t, workspace+"/.env", "WORKSPACE=test\n")
	mustWriteTestFile(t, workspace+"/docker-compose.yml", "services: {}\n")

	composeFake := &compose.FakeComposeRunner{}
	code := runWithStaleCheck(
		[]string{"update", "--workspace-dir", workspace},
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(_ context.Context, f flags) tui.Dependencies {
			return tui.Dependencies{
				Compose:      composeFake,
				GPU:          &platform.FakeGPUDetector{},
				WorkspaceDir: f.WorkspaceDir,
			}
		},
		func() (bootstrap.StaleGroupResult, error) {
			return bootstrap.StaleGroupResult{Stale: false}, nil
		},
		nil,
	)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if len(composeFake.CallOrder) != 2 || composeFake.CallOrder[0] != "pull" || composeFake.CallOrder[1] != "up" {
		t.Fatalf("CallOrder = %v, want [pull up]", composeFake.CallOrder)
	}
	if len(composeFake.PullCalls) != 1 {
		t.Fatalf("PullCalls len = %d, want 1", len(composeFake.PullCalls))
	}
	if composeFake.PullCalls[0].EnvFile != workspace+"/.env" {
		t.Fatalf("PullCalls[0].EnvFile = %q, want %q", composeFake.PullCalls[0].EnvFile, workspace+"/.env")
	}
	if len(composeFake.PullCalls[0].Files) != 1 || composeFake.PullCalls[0].Files[0] != workspace+"/docker-compose.yml" {
		t.Fatalf("PullCalls[0].Files = %v, want [%s]", composeFake.PullCalls[0].Files, workspace+"/docker-compose.yml")
	}
}

func TestRun_RestartModeRoutesToRestartFlowBeforeUnattended(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	restartCalled := false
	updateCalled := false
	headlessCalled := false
	runRestartFn = func(_ context.Context, cfg restart.Config, deps restart.Dependencies, _ io.Writer) error {
		restartCalled = true
		if cfg.WorkspaceDir == "" {
			t.Fatalf("restart cfg workspace dir is empty")
		}
		if deps.Compose == nil {
			t.Fatalf("restart deps compose is nil")
		}
		if deps.GPU == nil {
			t.Fatalf("restart deps gpu is nil")
		}
		return nil
	}
	runUpdateFn = func(_ context.Context, _ update.Config, _ update.Dependencies, _ io.Writer) error {
		updateCalled = true
		return nil
	}
	runHeadlessFn = func(context.Context, headless.Config, headless.Dependencies, io.Writer) error {
		headlessCalled = true
		return nil
	}
	t.Cleanup(func() {
		runRestartFn = restart.Run
		runUpdateFn = update.Run
		runHeadlessFn = headless.Run
	})

	factoryCalls := 0
	code := runWithStaleCheck(
		[]string{"restart", "--workspace-dir", t.TempDir(), "--unattended"},
		&out,
		&errOut,
		func(_ context.Context, f flags) tui.Dependencies {
			factoryCalls++
			return tui.Dependencies{
				Compose:      &compose.FakeComposeRunner{},
				GPU:          &platform.FakeGPUDetector{},
				WorkspaceDir: f.WorkspaceDir,
			}
		},
		func() (bootstrap.StaleGroupResult, error) {
			return bootstrap.StaleGroupResult{Stale: false}, nil
		},
		nil,
	)

	if code != 0 {
		t.Fatalf("restart run exit code = %d, want 0; stderr=%s", code, errOut.String())
	}
	if !restartCalled {
		t.Fatalf("expected restart flow to be called")
	}
	if updateCalled {
		t.Fatalf("update flow should not run in restart mode")
	}
	if headlessCalled {
		t.Fatalf("headless flow should not run in restart mode")
	}
	if factoryCalls != 1 {
		t.Fatalf("factory calls = %d, want 1", factoryCalls)
	}
}

func TestRun_RestartModeFailureReturnsNonZero(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	runRestartFn = func(context.Context, restart.Config, restart.Dependencies, io.Writer) error {
		return errors.New("restart failed")
	}
	t.Cleanup(func() { runRestartFn = restart.Run })

	code := runWithStaleCheck(
		[]string{"restart", "--workspace-dir", t.TempDir()},
		&out,
		&errOut,
		func(_ context.Context, f flags) tui.Dependencies {
			return tui.Dependencies{
				Compose:      &compose.FakeComposeRunner{},
				GPU:          &platform.FakeGPUDetector{},
				WorkspaceDir: f.WorkspaceDir,
			}
		},
		func() (bootstrap.StaleGroupResult, error) {
			return bootstrap.StaleGroupResult{Stale: false}, nil
		},
		nil,
	)

	if code != 1 {
		t.Fatalf("restart error exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "restart failed") {
		t.Fatalf("stderr = %q, want restart error", errOut.String())
	}
}

func TestDefaultWorkspaceDirPrefersXDGConfigHome(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if got, want := defaultWorkspaceDir(), xdg+"/alice-guardian"; got != want {
		t.Fatalf("defaultWorkspaceDir() = %q, want %q", got, want)
	}
}

func TestRunUpdateUsesOperationalDefaultFactory(t *testing.T) {
	runUpdateFn = func(context.Context, update.Config, update.Dependencies, io.Writer) error { return nil }
	t.Cleanup(func() { runUpdateFn = update.Run })

	code := runWithStaleCheck(
		[]string{"update", "--workspace-dir", t.TempDir()},
		&bytes.Buffer{},
		&bytes.Buffer{},
		nil,
		func() (bootstrap.StaleGroupResult, error) { return bootstrap.StaleGroupResult{Stale: false}, nil },
		nil,
	)
	if code != 0 {
		t.Fatalf("update exit code = %d, want 0", code)
	}
}

func TestRunStaleCheckerFailureDoesNotBlockVersion(t *testing.T) {
	var out bytes.Buffer
	code := runWithStaleCheck(
		[]string{"--version"},
		&out,
		&bytes.Buffer{},
		nil,
		func() (bootstrap.StaleGroupResult, error) {
			return bootstrap.StaleGroupResult{}, errors.New("unavailable")
		},
		nil,
	)
	if code != 0 || !strings.Contains(out.String(), version) {
		t.Fatalf("version with unavailable stale check: code=%d output=%q", code, out.String())
	}
}

func TestReloginAndTTYGuards(t *testing.T) {
	if !isReloginRequired(headless.ErrReloginRequired) {
		t.Fatal("ErrReloginRequired must be recognized")
	}
	if isReloginRequired(errors.New("other")) {
		t.Fatal("unrelated error must not be recognized as relogin required")
	}
	file, err := os.CreateTemp(t.TempDir(), "regular-file")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	if isTTY(file) {
		t.Fatal("regular file must not be treated as a terminal")
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if isTTY(file) {
		t.Fatal("an unreadable file descriptor must not be treated as a terminal")
	}
}

func TestInteractiveProductionMigrationUsesOnlySudoDockerAdapters(t *testing.T) {
	if !restoreSupportedPlatform(runtime.GOOS, runtime.GOARCH) {
		t.Skip("migration production wiring is platform-gated")
	}
	deps := buildDependencies(context.Background(), flags{WorkspaceDir: t.TempDir(), MediaDir: t.TempDir(), ConfigDir: t.TempDir()}, true)
	action, ok := deps.LegacyBackupAction.(migration.BackupAction)
	if !ok {
		t.Fatalf("backup action type = %T", deps.LegacyBackupAction)
	}
	discovery, ok := action.Inspector.(migration.InspectorDiscovery)
	if !ok {
		t.Fatalf("discovery type = %T", action.Inspector)
	}
	inspector, ok := discovery.Inspector.(migration.DockerCLIInspector)
	if !ok || !strings.Contains(strings.ToLower(fmt.Sprintf("%T", inspector.Runner)), "sudodocker") {
		t.Fatalf("migration inspector runner = %T", inspector.Runner)
	}
	if _, ok := action.Executor.(migration.SudoDockerExecutor); !ok {
		t.Fatalf("migration helper executor = %T", action.Executor)
	}
	validator, ok := action.Validator.(migration.PG11ArchiveValidator)
	if !ok {
		t.Fatalf("archive validator = %T", action.Validator)
	}
	if _, ok := validator.Executor.(migration.SudoDockerExecutor); !ok {
		t.Fatalf("archive validator executor = %T", validator.Executor)
	}
}

func mustWriteTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%s): %v", path, err)
	}
}
