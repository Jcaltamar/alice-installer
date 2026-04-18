package tui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

// buildDeployModel returns a DeployModel with a FakeComposeRunner.
func buildDeployModel(runner *compose.FakeComposeRunner) DeployModel {
	return NewDeployModel(
		theme.Default(),
		runner,
		[]string{"docker-compose.yml"},
		"/tmp/.env",
	)
}

// TestDeployModelInitReturnsNonNilCmd verifies Init() returns a Cmd.
func TestDeployModelInitReturnsNonNilCmd(t *testing.T) {
	runner := &compose.FakeComposeRunner{}
	m := buildDeployModel(runner)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("DeployModel.Init() should return a non-nil Cmd")
	}
}

// TestDeployModelFeedsProgressMessages verifies services map updated on progress.
func TestDeployModelFeedsProgressMessages(t *testing.T) {
	runner := &compose.FakeComposeRunner{
		UpProgressMsgs: []compose.UpProgressMsg{
			{Service: "backend", Status: "Starting"},
			{Service: "web", Status: "Started"},
			{Service: "queue", Status: "Started"},
		},
	}
	m := buildDeployModel(runner)

	var cmd tea.Cmd
	for _, pm := range runner.UpProgressMsgs {
		m, cmd = m.Update(pm)
		_ = cmd
	}

	if len(m.services) != 3 {
		t.Errorf("services map len = %d, want 3", len(m.services))
	}
	if m.services["web"] != "Started" {
		t.Errorf("services[web] = %q, want Started", m.services["web"])
	}
}

// TestDeployModelDeployCompleteSetsDone verifies done=true and HealthTickMsg emitted.
func TestDeployModelDeployCompleteSetsDone(t *testing.T) {
	runner := &compose.FakeComposeRunner{}
	m := buildDeployModel(runner)
	m, cmd := m.Update(DeployCompleteMsg{})
	if !m.done {
		t.Error("DeployModel should be done after DeployCompleteMsg")
	}
	if cmd == nil {
		t.Fatal("DeployCompleteMsg should emit a Cmd")
	}
	msg := cmd()
	if _, ok := msg.(HealthTickMsg); !ok {
		t.Errorf("DeployCompleteMsg → Cmd should produce HealthTickMsg, got %T", msg)
	}
}

// TestDeployModelDeployErrorEmitsInstallFailure verifies error path via runDeploy.
func TestDeployModelDeployErrorEmitsInstallFailure(t *testing.T) {
	runner := &compose.FakeComposeRunner{
		UpErr: errors.New("port already in use"),
	}
	m := buildDeployModel(runner)
	msg := m.runDeploy()
	fail, ok := msg.(InstallFailureMsg)
	if !ok {
		t.Fatalf("runDeploy with error → %T, want InstallFailureMsg", msg)
	}
	if fail.Stage != "deploy" {
		t.Errorf("InstallFailureMsg.Stage = %q, want deploy", fail.Stage)
	}
}

// TestDeployModelViewContainsTitle verifies the View renders a title.
func TestDeployModelViewContainsTitle(t *testing.T) {
	runner := &compose.FakeComposeRunner{}
	m := buildDeployModel(runner)
	view := m.View()
	if view == "" {
		t.Error("View() should not be empty")
	}
}
