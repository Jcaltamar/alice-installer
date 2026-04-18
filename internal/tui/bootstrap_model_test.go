package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/theme"
)

func buildTestBootstrap(results []BootstrapActionResultMsg) (BootstrapModel, *FakeExecutor) {
	fe := &FakeExecutor{Results: results}
	actions := make([]Action, len(results))
	for i := range results {
		actions[i] = Action{
			ID:          results[i].ActionID,
			Description: "Test action " + results[i].ActionID,
			Command:     "sudo",
			Args:        []string{"sh", "-c", "echo ok"},
		}
	}
	m := NewBootstrapModel(theme.Default(), fe, actions)
	return m, fe
}

// T-BS-007a: NewBootstrapModel sets confirming=true.
func TestNewBootstrapModelConfirmingTrue(t *testing.T) {
	m, _ := buildTestBootstrap([]BootstrapActionResultMsg{{ActionID: "a", Err: nil}})
	if !m.confirming {
		t.Error("NewBootstrapModel should start in confirming=true state")
	}
}

// T-BS-007b: Init() returns nil cmd.
func TestBootstrapModelInitNilCmd(t *testing.T) {
	m, _ := buildTestBootstrap(nil)
	cmd := m.Init()
	if cmd != nil {
		t.Error("Init() should return nil — nothing to start automatically")
	}
}

// T-BS-007c: pressing 'y' sets confirming=false and returns non-nil cmd.
func TestBootstrapModelPressYStartsExecution(t *testing.T) {
	m, fe := buildTestBootstrap([]BootstrapActionResultMsg{{ActionID: "a", Err: nil}})
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if updated.confirming {
		t.Error("after pressing y, confirming should be false")
	}
	if cmd == nil {
		t.Fatal("pressing y should return a non-nil cmd (executor called)")
	}
	if fe.calls != 1 {
		t.Errorf("executor should have been called once, got %d", fe.calls)
	}
}

// T-BS-007d: pressing 'n' emits BootstrapSkippedMsg.
func TestBootstrapModelPressNEmitsSkipped(t *testing.T) {
	m, _ := buildTestBootstrap([]BootstrapActionResultMsg{{ActionID: "a", Err: nil}})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if cmd == nil {
		t.Fatal("pressing n should return a cmd")
	}
	msg := cmd()
	if _, ok := msg.(BootstrapSkippedMsg); !ok {
		t.Errorf("pressing n should emit BootstrapSkippedMsg, got %T", msg)
	}
}

// T-BS-007e: pressing Esc emits BootstrapSkippedMsg.
func TestBootstrapModelPressEscEmitsSkipped(t *testing.T) {
	m, _ := buildTestBootstrap([]BootstrapActionResultMsg{{ActionID: "a", Err: nil}})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("pressing Esc should return a cmd")
	}
	msg := cmd()
	if _, ok := msg.(BootstrapSkippedMsg); !ok {
		t.Errorf("pressing Esc should emit BootstrapSkippedMsg, got %T", msg)
	}
}

// T-BS-007f: BootstrapActionResultMsg Err=nil on last action → BootstrapCompleteMsg.
func TestBootstrapModelLastActionSuccessEmitsComplete(t *testing.T) {
	m, _ := buildTestBootstrap([]BootstrapActionResultMsg{{ActionID: "a", Err: nil}})
	m.confirming = false // skip confirm step for this test

	_, cmd := m.Update(BootstrapActionResultMsg{ActionID: "a", Err: nil})
	if cmd == nil {
		t.Fatal("last action success should return a cmd")
	}
	msg := cmd()
	if _, ok := msg.(BootstrapCompleteMsg); !ok {
		t.Errorf("last action success should emit BootstrapCompleteMsg, got %T", msg)
	}
}

// T-BS-007g: BootstrapActionResultMsg Err≠nil → BootstrapFailedMsg.
func TestBootstrapModelActionErrorEmitsFailed(t *testing.T) {
	sentinel := errors.New("sudo failed")
	m, _ := buildTestBootstrap([]BootstrapActionResultMsg{{ActionID: "a", Err: nil}})
	m.confirming = false

	_, cmd := m.Update(BootstrapActionResultMsg{ActionID: "a", Err: sentinel})
	if cmd == nil {
		t.Fatal("action error should return a cmd")
	}
	msg := cmd()
	fail, ok := msg.(BootstrapFailedMsg)
	if !ok {
		t.Fatalf("action error should emit BootstrapFailedMsg, got %T", msg)
	}
	if fail.Err != sentinel {
		t.Errorf("BootstrapFailedMsg.Err = %v, want sentinel", fail.Err)
	}
	if fail.ActionID != "a" {
		t.Errorf("BootstrapFailedMsg.ActionID = %q, want 'a'", fail.ActionID)
	}
}

// T-BS-007h: BootstrapActionResultMsg Err=nil when more actions remain → advances currentIdx.
func TestBootstrapModelAdvancesCurrentIdxOnSuccess(t *testing.T) {
	results := []BootstrapActionResultMsg{
		{ActionID: "a", Err: nil},
		{ActionID: "b", Err: nil},
	}
	m, fe := buildTestBootstrap(results)
	m.confirming = false

	updated, cmd := m.Update(BootstrapActionResultMsg{ActionID: "a", Err: nil})
	if updated.currentIdx != 1 {
		t.Errorf("currentIdx = %d, want 1 after first action", updated.currentIdx)
	}
	if cmd == nil {
		t.Fatal("should return cmd for next action")
	}
	if fe.calls != 1 {
		t.Errorf("executor calls = %d, want 1 (for second action)", fe.calls)
	}
}

// T-BS-007i: View confirming=true shows action descriptions and Y/N prompt.
func TestBootstrapModelViewConfirmingContainsActionsAndPrompt(t *testing.T) {
	m, _ := buildTestBootstrap([]BootstrapActionResultMsg{{ActionID: "a", Err: nil}})
	m.actions[0].Description = "Create /opt/alice-media and grant ownership"
	view := m.View()
	if !strings.Contains(view, "Create /opt/alice-media") {
		t.Errorf("view should contain action description, got:\n%s", view)
	}
	if !strings.Contains(view, "Y") {
		t.Errorf("view should contain Y prompt, got:\n%s", view)
	}
	if !strings.Contains(view, "N") {
		t.Errorf("view should contain N prompt, got:\n%s", view)
	}
}
