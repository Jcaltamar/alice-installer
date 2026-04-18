package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

// TestResultModelSuccessViewContainsComplete verifies success view.
func TestResultModelSuccessViewContainsComplete(t *testing.T) {
	success := InstallSuccessMsg{
		WorkspaceDir: "/tmp/mysite",
		EnvPath:      "/tmp/.env",
		Services: []compose.ServiceHealth{
			{Service: "backend", Status: "healthy"},
			{Service: "web", Status: "healthy"},
		},
	}
	m := NewResultModel(theme.Default(), &success, nil)
	view := m.View()
	if !strings.Contains(strings.ToLower(view), "complete") && !strings.Contains(view, "✓") {
		t.Errorf("success view should contain 'complete' or ✓, got:\n%q", view)
	}
}

// TestResultModelFailureViewContainsFailedAndStage verifies failure view.
func TestResultModelFailureViewContainsFailedAndStage(t *testing.T) {
	failure := InstallFailureMsg{
		Err:   errors.New("network error"),
		Stage: "pull",
	}
	m := NewResultModel(theme.Default(), nil, &failure)
	view := m.View()
	if !strings.Contains(strings.ToLower(view), "fail") {
		t.Errorf("failure view should contain 'fail', got:\n%q", view)
	}
	if !strings.Contains(view, "pull") {
		t.Errorf("failure view should contain stage 'pull', got:\n%q", view)
	}
}

// TestResultModelQKeyEmitsQuit verifies q → tea.Quit.
func TestResultModelQKeyEmitsQuit(t *testing.T) {
	m := NewResultModel(theme.Default(), nil, nil)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("'q' key should return a non-nil Cmd")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("'q' → Cmd should produce tea.QuitMsg, got %T", msg)
	}
}

// TestResultModelEnterKeyEmitsQuit verifies Enter → tea.Quit.
func TestResultModelEnterKeyEmitsQuit(t *testing.T) {
	m := NewResultModel(theme.Default(), nil, nil)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter key should return a non-nil Cmd")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("Enter → Cmd should produce tea.QuitMsg, got %T", msg)
	}
}
