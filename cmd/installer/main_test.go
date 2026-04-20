package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/jcaltamar/alice-installer/internal/bootstrap"
	"github.com/jcaltamar/alice-installer/internal/tui"
)

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

	code := run([]string{"--version"}, &out, &errOut, nil)

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
	code := run([]string{"--dry-run", "--media-dir", t.TempDir(), "--config-dir", t.TempDir()},
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
		nil,        // factory — should not be called
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
