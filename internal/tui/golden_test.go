package tui

// golden_test.go: Golden-file snapshot framework for TUI View() output.
//
// Usage:
//   go test -run TestGolden -update ./internal/tui/   # regenerate golden files
//   go test -run TestGolden ./internal/tui/           # compare against golden files
//
// All snapshots live in testdata/golden/<name>.golden.
// The lipgloss color profile is pinned to NoTTY (no ANSI) so snapshots are
// deterministic in CI (no terminal attached).

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/envgen"
	"github.com/jcaltamar/alice-installer/internal/installation"
	"github.com/jcaltamar/alice-installer/internal/migration"
	"github.com/jcaltamar/alice-installer/internal/platform"
	"github.com/jcaltamar/alice-installer/internal/preflight"
	"github.com/jcaltamar/alice-installer/internal/secrets"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

var updateGolden = flag.Bool("update", false, "regenerate golden snapshot files")

// goldenAssert reads testdata/golden/<name>.golden and compares it byte-for-byte
// with got. If -update is set, it writes got to the golden file instead.
func goldenAssert(t *testing.T, name, got string) {
	t.Helper()

	dir := filepath.Join("testdata", "golden")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("could not create golden dir: %v", err)
	}
	path := filepath.Join(dir, name+".golden")

	if *updateGolden {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("could not write golden file %s: %v", path, err)
		}
		t.Logf("updated golden file: %s", path)
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden file %s not found — run with -update to create it: %v", path, err)
	}

	if got != string(want) {
		// Show a helpful diff-like message.
		gotLines := strings.Split(got, "\n")
		wantLines := strings.Split(string(want), "\n")
		t.Errorf("golden mismatch for %s:\n  got  %d lines\n  want %d lines\n  (run with -update to regenerate)", name, len(gotLines), len(wantLines))
		// Print first divergent line for quick diagnosis.
		for i := 0; i < len(gotLines) && i < len(wantLines); i++ {
			if gotLines[i] != wantLines[i] {
				t.Errorf("  first diff at line %d:\n    got:  %q\n    want: %q", i+1, gotLines[i], wantLines[i])
				break
			}
		}
	}
}

// init pins the lipgloss color profile to NoTTY so ANSI escape codes are
// stripped — without this, golden files would vary between TTY and non-TTY
// environments (CI vs local).
func init() {
	lipgloss.SetColorProfile(termenv.TrueColor)
}

// TestGoldenSplash snapshots the splash screen with the default theme.
func TestGoldenSplash(t *testing.T) {
	m := NewSplashModel(theme.Default(), "dev")
	view := m.View()
	goldenAssert(t, "splash", view)
}

func TestSplashUsesInjectedVersion(t *testing.T) {
	const injected = "v9.8.7-test"
	view := NewSplashModel(theme.Default(), injected).View()
	if !strings.Contains(view, "Installer "+injected) || strings.Contains(view, "v0.1.0") {
		t.Fatalf("splash version was not injected: %q", view)
	}
}

// TestGoldenPreflightBlocked snapshots the preflight screen after a result
// with docker_daemon fail + both dirs fail.
func TestGoldenPreflightBlocked(t *testing.T) {
	coord := buildTestCoordinator(true, true, false) // dockerPass=false
	m := newTestPreflight(coord)
	report := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckOS, Status: preflight.StatusPass, Title: "Operating system", Detail: "linux"},
			{ID: preflight.CheckArch, Status: preflight.StatusPass, Title: "CPU architecture", Detail: "amd64"},
			{ID: preflight.CheckDockerDaemon, Status: preflight.StatusFail, Title: "Docker daemon unreachable", Detail: "context deadline exceeded", Remediation: "Start Docker and ensure your user is in the docker group."},
			{ID: preflight.CheckMediaWritable, Status: preflight.StatusFail, Title: "Directory writable: /opt/alice-media", Detail: "permission denied", Remediation: "Run: sudo mkdir -p /opt/alice-media"},
			{ID: preflight.CheckConfigWritable, Status: preflight.StatusFail, Title: "Directory writable: /opt/alice-config", Detail: "permission denied", Remediation: "Run: sudo mkdir -p /opt/alice-config"},
		},
	}
	m.report = &report
	view := m.View()
	goldenAssert(t, "preflight_blocked", view)
}

// TestGoldenBootstrapConfirm snapshots the bootstrap confirming state with a
// fresh Ubuntu scenario (3 actions: docker_install, media_writable, config_writable).
func TestGoldenBootstrapConfirm(t *testing.T) {
	fe := &FakeExecutor{
		Results: []BootstrapActionResultMsg{
			{ActionID: ActionIDDockerInstall, Err: nil},
			{ActionID: string(preflight.CheckMediaWritable), Err: nil},
			{ActionID: string(preflight.CheckConfigWritable), Err: nil},
		},
	}
	actions := []Action{
		dockerInstallAction(),
		buildDirAction(string(preflight.CheckMediaWritable), "/opt/alice-media", "ubuntu"),
		buildDirAction(string(preflight.CheckConfigWritable), "/opt/alice-config", "ubuntu"),
	}
	m := NewBootstrapModel(theme.Default(), fe, actions)
	// Model starts in confirming=true.
	view := m.View()
	goldenAssert(t, "bootstrap_confirm", view)
}

// TestGoldenBootstrapBanner snapshots the bootstrap banner screen (showingBanner=true)
// with the usermod relogin banner.
func TestGoldenBootstrapBanner(t *testing.T) {
	fe := &FakeExecutor{
		Results: []BootstrapActionResultMsg{
			{ActionID: ActionIDDockerGroup, Err: nil},
		},
	}
	actions := []Action{
		dockerGroupAddAction("alice"),
	}
	m := NewBootstrapModel(theme.Default(), fe, actions)
	m.showingBanner = true
	m.banners = []string{actions[0].PostActionBanner}
	m.done = true
	view := m.View()
	goldenAssert(t, "bootstrap_banner", view)
}

// TestGoldenEnvWriteDone snapshots the EnvWriteModel after EnvWrittenMsg.
func TestGoldenEnvWriteDone(t *testing.T) {
	fw := &envgen.FakeWriter{Written: make(map[string][]byte)}
	templater := &envgen.Templater{PasswordGen: &secrets.FakeGenerator{Val: "snap-password"}}
	assets := TemplateAssets{
		EnvExample:   []byte("WORKSPACE=\nPOSTGRES_PASSWORD=\n"),
		BaselineYAML: []byte("# compose baseline\n"),
		OverlayYAML:  []byte("# compose gpu\n"),
	}
	input := envgen.Input{
		Workspace:        "snap-site",
		Arch:             platform.ArchAMD64,
		GeneratePassword: true,
	}
	m := NewEnvWriteModel(theme.Default(), templater, fw, assets, "/tmp/snap-ws/.env", input)
	m.done = true
	m.writtenPath = "/tmp/snap-ws/.env"
	view := m.View()
	goldenAssert(t, "env_write_done", view)
}

// TestGoldenResultSuccess snapshots the result screen in success state.
func TestGoldenResultSuccess(t *testing.T) {
	success := &InstallSuccessMsg{
		EnvPath: "/tmp/golden-ws/.env",
		Services: []compose.ServiceHealth{
			{Service: "backend", Status: "healthy"},
			{Service: "web", Status: "healthy"},
		},
	}
	m := NewResultModel(theme.Default(), success, nil)
	view := m.View()
	goldenAssert(t, "result_success", view)
}

// TestGoldenResultFailure snapshots the result screen in failure state.
func TestGoldenResultFailure(t *testing.T) {
	failure := &InstallFailureMsg{
		Stage: "pull",
		Err:   goldenTestErr("manifest unknown: manifest unknown"),
	}
	m := NewResultModel(theme.Default(), nil, failure)
	view := m.View()
	goldenAssert(t, "result_failure", view)
}

func TestGoldenContextMenus(t *testing.T) {
	for _, tt := range []struct {
		name      string
		detection installation.Detection
	}{
		{"context_not_installed", installation.Detection{State: installation.StateNotInstalled, Evidence: []installation.Evidence{{Kind: installation.EvidenceWorkspaceAbsent}, {Kind: installation.EvidencePM2Unavailable}}}},
		{"context_current", installation.Detection{State: installation.StateCurrent, Evidence: []installation.Evidence{{Kind: installation.EvidenceWorkspaceComplete}, {Kind: installation.EvidencePM2Absent}}}},
		{"context_legacy", installation.Detection{State: installation.StateLegacyPM2, Evidence: []installation.Evidence{{Kind: installation.EvidenceWorkspaceAbsent}, {Kind: installation.EvidencePM2AliceProcess}}}},
		{"context_conflict", installation.Detection{State: installation.StateConflict, Evidence: []installation.Evidence{{Kind: installation.EvidenceWorkspaceComplete}, {Kind: installation.EvidencePM2AliceProcess}}}},
		{"context_unknown", installation.Detection{State: installation.StateUnknown, Evidence: []installation.Evidence{{Kind: installation.EvidenceWorkspaceUnreadable}, {Kind: installation.EvidencePM2Failed}}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			goldenAssert(t, tt.name, NewContextMenuModel(theme.Default(), tt.detection).View())
		})
	}
}

func TestGoldenBlockedOperation(t *testing.T) {
	goldenAssert(t, "context_blocked", blockedOperationModel{theme: theme.Default(), action: ContextActionUninstall}.View())
}

func TestGoldenMigrationViews(t *testing.T) {
	m := NewModel(buildTestDeps())
	m.state = StateBackupConfirm
	goldenAssert(t, "migration_confirm", m.View())
	m.state = StateBackupRunning
	goldenAssert(t, "migration_running", m.View())
	m.state = StateBackupResult
	m.backupResult = migration.BackupResult{Outcome: migration.BackupValidationFailed}
	goldenAssert(t, "migration_failed", m.View())
	m.state = StateMigrationBlocked
	m.backupResult = migration.BackupResult{Outcome: migration.BackupValidated, DumpPath: "/safe/backup.dump", SHA256: "abc", Size: 42}
	goldenAssert(t, "migration_blocked", m.View())
}

// goldenTestErr is a simple error type for golden file tests.
type goldenTestErr string

func (e goldenTestErr) Error() string { return string(e) }
