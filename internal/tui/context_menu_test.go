package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/installation"
)

func TestContextMenuActionMatrix(t *testing.T) {
	tests := []struct {
		name     string
		state    installation.State
		want     []ContextAction
		contains string
	}{
		{"not installed", installation.StateNotInstalled, []ContextAction{ContextActionInstall}, "Install"},
		{"current", installation.StateCurrent, []ContextAction{ContextActionUpdate, ContextActionUninstall}, "Uninstall"},
		{"legacy", installation.StateLegacyPM2, []ContextAction{ContextActionMigration}, "Migration"},
		{"conflict", installation.StateConflict, nil, "cannot safely choose"},
		{"unknown", installation.StateUnknown, nil, "cannot safely choose"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewContextMenuModel(themeDefaultForTest(), installation.Detection{State: tt.state})
			if len(m.actions) != len(tt.want) {
				t.Fatalf("actions = %v, want %v", m.actions, tt.want)
			}
			for i := range tt.want {
				if m.actions[i] != tt.want[i] {
					t.Fatalf("actions = %v, want %v", m.actions, tt.want)
				}
			}
			if !strings.Contains(m.View(), tt.contains) {
				t.Fatalf("view = %q, want %q", m.View(), tt.contains)
			}
		})
	}
}

func TestContextMenuBlockedActionsDoNotExecuteCommands(t *testing.T) {
	m := NewContextMenuModel(themeDefaultForTest(), installation.Detection{State: installation.StateCurrent})
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatal("cursor movement must not emit a command")
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Uninstall selection must emit an informational destination message")
	}
	if got := cmd(); got != (ContextActionSelectedMsg{Action: ContextActionUninstall}) {
		t.Fatalf("selection = %#v", got)
	}
}

func TestContextMenuHidesNormalAbsenceEvidence(t *testing.T) {
	m := NewContextMenuModel(themeDefaultForTest(), installation.Detection{
		State: installation.StateNotInstalled,
		Evidence: []installation.Evidence{
			{Kind: installation.EvidenceWorkspaceAbsent},
			{Kind: installation.EvidencePM2Absent},
			{Kind: installation.EvidencePM2Unavailable},
			{Kind: installation.EvidencePM2Unsupported},
		},
	})
	view := m.View()
	for _, confusing := range []string{"No current workspace artifacts found", "No configured legacy PM2 deployment found", "PM2 is not installed", "Legacy PM2 probing is unsupported on this platform"} {
		if strings.Contains(view, confusing) {
			t.Fatalf("view includes normal absence evidence %q: %q", confusing, view)
		}
	}
}

func TestContextMenuEvidenceUsesSafeLabels(t *testing.T) {
	m := NewContextMenuModel(themeDefaultForTest(), installation.Detection{
		State:    installation.StateUnknown,
		Evidence: []installation.Evidence{{Kind: installation.EvidencePM2Failed, Detail: "token=secret", Path: "/private/deployment/.env"}},
	})
	view := m.View()
	if !strings.Contains(view, "Legacy PM2 probe could not be verified") {
		t.Fatalf("view = %q, want safe evidence category", view)
	}
	if strings.Contains(view, "token=secret") || strings.Contains(view, "/private/deployment/.env") {
		t.Fatalf("view exposes raw evidence: %q", view)
	}
}

func TestContextMenuEvidenceLabelsAreSanitized(t *testing.T) {
	for _, kind := range []installation.EvidenceKind{
		installation.EvidenceWorkspaceComplete,
		installation.EvidenceWorkspaceAbsent,
		installation.EvidenceWorkspacePartial,
		installation.EvidenceWorkspaceInvalid,
		installation.EvidenceWorkspaceUnreadable,
		installation.EvidencePM2AliceProcess,
		installation.EvidencePM2Absent,
		installation.EvidencePM2Unavailable,
		installation.EvidencePM2Unsupported,
		installation.EvidencePM2Ambiguous,
		installation.EvidencePM2Failed,
		installation.EvidenceKind(255),
	} {
		t.Run(fmt.Sprintf("kind-%d", kind), func(t *testing.T) {
			label := safeEvidenceLabel(installation.Evidence{Kind: kind, Detail: "password=secret", Path: "/private/.env"})
			if label == "" || strings.Contains(label, "secret") || strings.Contains(label, "/private") {
				t.Fatalf("safeEvidenceLabel(%d) = %q", kind, label)
			}
		})
	}
}

func TestContextMenuNavigationAndBlockedEscape(t *testing.T) {
	menu := NewContextMenuModel(themeDefaultForTest(), installation.Detection{State: installation.StateCurrent})
	menu, cmd := menu.Update(tea.KeyMsg{Type: tea.KeyUp})
	if menu.cursor != 0 || cmd != nil {
		t.Fatal("cursor must remain at the first action")
	}
	menu, _ = menu.Update(tea.KeyMsg{Type: tea.KeyDown})
	menu, _ = menu.Update(tea.KeyMsg{Type: tea.KeyDown})
	if menu.cursor != 1 {
		t.Fatalf("cursor = %d, want final action", menu.cursor)
	}
	blocked := blockedOperationModel{theme: themeDefaultForTest(), action: ContextActionUninstall}
	_, cmd = blocked.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("blocked operation Escape must return an informational dismissal")
	}
	if _, ok := cmd().(BlockedOperationDismissedMsg); !ok {
		t.Fatalf("Escape message = %T", cmd())
	}
}

func TestContextMenuWithoutActionsNeverEmitsSelection(t *testing.T) {
	menu := NewContextMenuModel(themeDefaultForTest(), installation.Detection{State: installation.StateConflict})
	_, cmd := menu.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("a blocked contextual menu must not emit a lifecycle action")
	}
}
