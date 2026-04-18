package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/docker"
	"github.com/jcaltamar/alice-installer/internal/platform"
	"github.com/jcaltamar/alice-installer/internal/ports"
	"github.com/jcaltamar/alice-installer/internal/preflight"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

// buildTestCoordinator returns a Coordinator configured with fakes.
func buildTestCoordinator(osPass, archPass, dockerPass bool) preflight.Coordinator {
	osName := "linux"
	if !osPass {
		osName = "darwin"
	}
	arch := platform.ArchAMD64
	if !archPass {
		arch = platform.Arch("unsupported")
	}
	var dockerErr error
	if !dockerPass {
		dockerErr = context.DeadlineExceeded
	}
	return preflight.Coordinator{
		OS:   &platform.FakeOSGuard{Linux: osPass, Name: osName},
		Arch: &platform.FakeArchDetector{Arch: arch},
		Docker: &docker.FakeDockerClient{
			ProbeErr:   dockerErr,
			VersionVal: docker.Version{Client: "24.0.0", Server: "24.0.0"},
		},
		Compose: &compose.FakeComposeRunner{
			VersionVal: compose.Version{V2Plugin: true, Raw: "2.21.0"},
		},
		GPU: &platform.FakeGPUDetector{
			Info: platform.GPUInfo{ToolkitInstalled: true, RuntimeAvailable: true},
		},
		Ports:             &ports.FakePortScanner{},
		Dirs:              &fakeDirChecker{writable: true},
		MediaDir:          "/tmp/media",
		ConfigDir:         "/tmp/config",
		MinDockerVersion:  "20.10.0",
		MinComposeVersion: "2.0.0",
	}
}

// fakeDirChecker is a local test double for preflight.DirectoryChecker.
type fakeDirChecker struct {
	writable bool
	reason   string
}

func (f *fakeDirChecker) IsWritable(_ string) (bool, string) {
	return f.writable, f.reason
}

func newTestPreflight(coord preflight.Coordinator) PreflightModel {
	return PreflightModel{
		theme: theme.Default(),
		coord: coord,
	}
}

// TestPreflightInitReturnsNonNilCmd verifies that Init() returns a command
// (it starts the coordinator run).
func TestPreflightInitReturnsNonNilCmd(t *testing.T) {
	m := newTestPreflight(buildTestCoordinator(true, true, true))
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init should return a non-nil command to start preflight")
	}
}

// TestPreflightInitCmdProducesPreflightResultMsg verifies that executing the
// Init() command produces a PreflightResultMsg.
func TestPreflightInitCmdProducesPreflightResultMsg(t *testing.T) {
	m := newTestPreflight(buildTestCoordinator(true, true, true))
	cmd := m.Init()
	msg := cmd()
	if _, ok := msg.(PreflightResultMsg); !ok {
		t.Errorf("Init cmd should produce PreflightResultMsg, got %T", msg)
	}
}

// TestPreflightViewNilReportContainsSpinner verifies that the view before the
// report arrives shows a spinner/running indicator.
func TestPreflightViewNilReportContainsSpinner(t *testing.T) {
	m := newTestPreflight(buildTestCoordinator(true, true, true))
	view := m.View()
	// Should contain some indication that checks are running.
	if !strings.Contains(view, "preflight") && !strings.Contains(view, "Running") && !strings.Contains(view, "checking") {
		t.Errorf("view without report should contain running indicator, got:\n%s", view)
	}
}

// TestPreflightUpdateWithResultStoresReport verifies that Update with a
// PreflightResultMsg stores the report on the model.
func TestPreflightUpdateWithResultStoresReport(t *testing.T) {
	m := newTestPreflight(buildTestCoordinator(true, true, true))
	report := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckOS, Status: preflight.StatusPass, Title: "OS"},
		},
	}
	updated, _ := m.Update(PreflightResultMsg{Report: report})
	if updated.report == nil {
		t.Fatal("report should be stored after PreflightResultMsg")
	}
	if updated.report.Items[0].Title != "OS" {
		t.Errorf("stored report does not match sent report")
	}
}

// TestPreflightViewWithReportContainsTitles verifies that a report with three
// different statuses renders all three check titles.
func TestPreflightViewWithReportContainsTitles(t *testing.T) {
	m := newTestPreflight(buildTestCoordinator(true, true, true))
	report := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckOS, Status: preflight.StatusPass, Title: "OS check"},
			{ID: preflight.CheckGPU, Status: preflight.StatusWarn, Title: "GPU warning"},
			{ID: preflight.CheckDockerDaemon, Status: preflight.StatusFail, Title: "Docker failure"},
		},
	}
	m.report = &report
	view := m.View()
	for _, title := range []string{"OS check", "GPU warning", "Docker failure"} {
		if !strings.Contains(view, title) {
			t.Errorf("view should contain check title %q, got:\n%s", title, view)
		}
	}
}

// TestPreflightEnterWithFailReport verifies that pressing Enter when the report
// has a blocking failure does NOT emit a transition command.
func TestPreflightEnterWithFailReport(t *testing.T) {
	m := newTestPreflight(buildTestCoordinator(true, true, true))
	failReport := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckDockerDaemon, Status: preflight.StatusFail, Title: "Docker"},
		},
	}
	m.report = &failReport

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(WorkspaceEnteredMsg); ok {
			t.Error("Enter with blocking failure should not advance to workspace")
		}
		// Also check for PreflightPassedMsg or similar advance signal
		if _, ok := msg.(PreflightPassedMsg); ok {
			t.Error("Enter with blocking failure should not emit PreflightPassedMsg")
		}
	}
}

// TestPreflightEnterWithPassOnlyReport verifies that pressing Enter when there
// are no failures emits PreflightPassedMsg.
func TestPreflightEnterWithPassOnlyReport(t *testing.T) {
	m := newTestPreflight(buildTestCoordinator(true, true, true))
	passReport := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckOS, Status: preflight.StatusPass, Title: "OS"},
		},
	}
	m.report = &passReport

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter with no failures should return a command")
	}
	msg := cmd()
	if _, ok := msg.(PreflightPassedMsg); !ok {
		t.Errorf("Enter with no failures should emit PreflightPassedMsg, got %T", msg)
	}
}
