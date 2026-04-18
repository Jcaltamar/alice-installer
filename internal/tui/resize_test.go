package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestTerminalTooSmallGuardBelow60Cols verifies that when width < 60,
// View() returns the compact "Terminal too small" message regardless of state.
func TestTerminalTooSmallGuardBelow60Cols(t *testing.T) {
	m := NewModel(buildTestDeps())
	// Set a terminal that is too narrow.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 50, Height: 30})
	m = updated.(Model)
	view := m.View()
	if !strings.Contains(view, "Terminal too small") {
		t.Errorf("View() at 50 cols should say 'Terminal too small', got: %q", view)
	}
}

// TestTerminalTooSmallGuardBelow24Rows verifies that when height < 24,
// View() returns the compact "Terminal too small" message.
func TestTerminalTooSmallGuardBelow24Rows(t *testing.T) {
	m := NewModel(buildTestDeps())
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 10})
	m = updated.(Model)
	view := m.View()
	if !strings.Contains(view, "Terminal too small") {
		t.Errorf("View() at 10 rows should say 'Terminal too small', got: %q", view)
	}
}

// TestTerminalTooSmallGuardExactMinSize verifies that 80x24 renders normally.
func TestTerminalTooSmallGuardExactMinSize(t *testing.T) {
	m := NewModel(buildTestDeps())
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	view := m.View()
	if strings.Contains(view, "Terminal too small") {
		t.Errorf("View() at 80×24 should NOT show too-small message, got: %q", view)
	}
}

// TestTerminalTooSmallGuardAboveMinSize verifies that large terminals render normally.
func TestTerminalTooSmallGuardAboveMinSize(t *testing.T) {
	m := NewModel(buildTestDeps())
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	view := m.View()
	if strings.Contains(view, "Terminal too small") {
		t.Errorf("View() at 120×40 should NOT show too-small message, got: %q", view)
	}
}

// TestResizeAtEachStateProducesNonEmptyView table-drives through every state,
// sets a valid terminal size, and asserts View() is non-empty.
func TestResizeAtEachStateProducesNonEmptyView(t *testing.T) {
	states := []struct {
		name  string
		state State
	}{
		{"splash", StateSplash},
		{"preflight", StatePreflight},
		{"workspace", StateWorkspaceInput},
		{"portscan", StatePortScan},
		{"envwrite", StateEnvWrite},
		{"pull", StatePull},
		{"deploy", StateDeploy},
		{"verify", StateVerify},
		{"result", StateResult},
	}

	for _, tt := range states {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(buildTestDeps())
			m.state = tt.state

			// Apply a valid window size.
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
			m = updated.(Model)

			view := m.View()
			if view == "" {
				t.Errorf("View() at state %s should not be empty", tt.name)
			}
			if strings.Contains(view, "Terminal too small") {
				t.Errorf("View() at state %s with 100×40 should NOT show too-small message", tt.name)
			}
		})
	}
}

// TestTooSmallAtEachStateShowsGuard verifies the too-small guard fires at any state.
func TestTooSmallAtEachStateShowsGuard(t *testing.T) {
	states := []struct {
		name  string
		state State
	}{
		{"splash", StateSplash},
		{"preflight", StatePreflight},
		{"workspace", StateWorkspaceInput},
		{"portscan", StatePortScan},
		{"envwrite", StateEnvWrite},
		{"pull", StatePull},
		{"deploy", StateDeploy},
		{"verify", StateVerify},
		{"result", StateResult},
	}

	for _, tt := range states {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(buildTestDeps())
			m.state = tt.state

			// Apply a too-small window.
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
			m = updated.(Model)

			view := m.View()
			if !strings.Contains(view, "Terminal too small") {
				t.Errorf("View() at state %s with 40×10 should show too-small message, got: %q", tt.name, view)
			}
		})
	}
}
