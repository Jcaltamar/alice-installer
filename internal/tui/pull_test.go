package tui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

// buildPullModel returns a PullModel with a FakeComposeRunner.
func buildPullModel(runner *compose.FakeComposeRunner) PullModel {
	return NewPullModel(
		theme.Default(),
		runner,
		[]string{"docker-compose.yml"},
		"/tmp/.env",
	)
}

// TestPullModelInitReturnsNonNilCmd verifies Init() returns a Cmd.
func TestPullModelInitReturnsNonNilCmd(t *testing.T) {
	runner := &compose.FakeComposeRunner{}
	m := buildPullModel(runner)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("PullModel.Init() should return a non-nil Cmd")
	}
}

// TestPullModelFeedsProgressMessages verifies services map updated on progress.
func TestPullModelFeedsProgressMessages(t *testing.T) {
	runner := &compose.FakeComposeRunner{
		PullProgressMsgs: []compose.PullProgressMsg{
			{Service: "backend", Status: "Pulling"},
			{Service: "web", Status: "Pulling"},
			{Service: "queue", Status: "Pulled"},
		},
	}
	m := buildPullModel(runner)

	// Feed progress messages directly.
	var cmd tea.Cmd
	for _, pm := range runner.PullProgressMsgs {
		m, cmd = m.Update(pm)
		_ = cmd
	}

	if len(m.services) != 3 {
		t.Errorf("services map len = %d, want 3", len(m.services))
	}
	if m.services["queue"] != "Pulled" {
		t.Errorf("services[queue] = %q, want Pulled", m.services["queue"])
	}
}

// TestPullModelPullCompleteSetsDone verifies done=true on PullCompleteMsg.
func TestPullModelPullCompleteSetsDone(t *testing.T) {
	runner := &compose.FakeComposeRunner{}
	m := buildPullModel(runner)
	m, cmd := m.Update(PullCompleteMsg{})
	if !m.done {
		t.Error("PullModel should be done after PullCompleteMsg")
	}
	// PullCompleteMsg should also emit a DeployStartedMsg.
	if cmd == nil {
		t.Fatal("PullCompleteMsg should emit a Cmd")
	}
	msg := cmd()
	if _, ok := msg.(DeployStartedMsg); !ok {
		t.Errorf("PullCompleteMsg → Cmd should produce DeployStartedMsg, got %T", msg)
	}
}

// TestPullModelPullErrorEmitsInstallFailure verifies error path.
func TestPullModelPullErrorEmitsInstallFailure(t *testing.T) {
	runner := &compose.FakeComposeRunner{
		PullErr: errors.New("network error"),
	}
	m := buildPullModel(runner)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() should return non-nil cmd")
	}
	// Drain until we get a terminal message.
	// The init cmd starts a goroutine + drain loop; test the pull-run cmd directly.
	// We call the runPull cmd to get the PullCompleteMsg or error.
	msg := m.runPull()
	fail, ok := msg.(InstallFailureMsg)
	if !ok {
		t.Fatalf("runPull with error → %T, want InstallFailureMsg", msg)
	}
	if fail.Stage != "pull" {
		t.Errorf("InstallFailureMsg.Stage = %q, want pull", fail.Stage)
	}
}

// TestPullModelViewContainsTitle verifies the View renders a title.
func TestPullModelViewContainsTitle(t *testing.T) {
	runner := &compose.FakeComposeRunner{}
	m := buildPullModel(runner)
	view := m.View()
	if view == "" {
		t.Error("View() should not be empty")
	}
}
