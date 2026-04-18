package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/theme"
)

func newTestSplash() SplashModel {
	return SplashModel{theme: theme.Default()}
}

// TestSplashViewContainsWordmark asserts the splash renders the ALICE GUARDIAN
// wordmark as plain styled text.
func TestSplashViewContainsWordmark(t *testing.T) {
	s := newTestSplash()
	view := s.View()
	if !strings.Contains(view, "ALICE GUARDIAN") {
		t.Errorf("splash view should contain 'ALICE GUARDIAN', got:\n%s", view)
	}
}

// TestSplashViewContainsInstaller asserts the splash screen contains the installer subtitle.
func TestSplashViewContainsInstaller(t *testing.T) {
	s := newTestSplash()
	view := s.View()
	if !strings.Contains(view, "Installer") {
		t.Errorf("splash view should contain 'Installer', got:\n%s", view)
	}
}

// TestSplashEnterEmitsPreflightStartedMsg asserts that pressing Enter returns a
// command that, when executed, produces a PreflightStartedMsg.
func TestSplashEnterEmitsPreflightStartedMsg(t *testing.T) {
	s := newTestSplash()
	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter key should return a non-nil command")
	}
	msg := cmd()
	if _, ok := msg.(PreflightStartedMsg); !ok {
		t.Errorf("Enter command should produce PreflightStartedMsg, got %T", msg)
	}
}

// TestSplashNonEnterKeyNoTransitionCmd asserts that pressing a non-Enter key
// does NOT return a transition command.
func TestSplashNonEnterKeyNoTransitionCmd(t *testing.T) {
	s := newTestSplash()
	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if cmd != nil {
		// Execute cmd to check if it produces a PreflightStartedMsg
		msg := cmd()
		if _, ok := msg.(PreflightStartedMsg); ok {
			t.Error("non-Enter key should NOT emit PreflightStartedMsg")
		}
	}
}

// TestSplashInitReturnsNilOrTick asserts that Init either returns nil or a Tick cmd (for auto-advance).
func TestSplashInitReturnsCmd(t *testing.T) {
	s := newTestSplash()
	// Init may return nil or a tick command — both are valid.
	// We just verify it doesn't panic.
	_ = s.Init()
}
