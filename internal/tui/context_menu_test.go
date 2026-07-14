package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/installation"
)

type fakeDetector struct {
	detection installation.Detection
	calls     int
}

func (d *fakeDetector) Detect(context.Context) installation.Detection {
	d.calls++
	return d.detection
}

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

func TestRootDetectsBeforePreflightAndRoutesInstall(t *testing.T) {
	deps := buildTestDeps()
	detector := &fakeDetector{detection: installation.Detection{State: installation.StateNotInstalled}}
	deps.Detector = detector
	m := NewModel(deps)
	updated, cmd := m.Update(DetectionStartedMsg{})
	m = updated.(Model)
	if m.state != StateDetecting || cmd == nil {
		t.Fatalf("detection start state/cmd = %v/%v", m.state, cmd)
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.state != StateContextMenu || detector.calls != 1 {
		t.Fatalf("detection completion state/calls = %v/%d", m.state, detector.calls)
	}
	updated, _ = m.Update(ContextActionSelectedMsg{Action: ContextActionInstall})
	if updated.(Model).state != StatePreflight {
		t.Fatalf("Install state = %v, want StatePreflight", updated.(Model).state)
	}
}

func TestRootRejectsActionsNotOfferedByDetection(t *testing.T) {
	m := NewModel(buildTestDeps())
	m.state = StateContextMenu
	m.contextMenu = NewContextMenuModel(m.deps.Theme, installation.Detection{State: installation.StateUnknown})
	updated, cmd := m.Update(ContextActionSelectedMsg{Action: ContextActionInstall})
	if updated.(Model).state != StateContextMenu || cmd != nil {
		t.Fatal("an action absent from the menu must not change state or emit a command")
	}
}

func TestRootUsesUnknownWhenDetectorIsUnavailable(t *testing.T) {
	deps := buildTestDeps()
	deps.Detector = nil
	m := NewModel(deps)
	updated, cmd := m.Update(DetectionStartedMsg{})
	m = updated.(Model)
	if m.state != StateDetecting || cmd == nil {
		t.Fatalf("state/cmd = %v/%v", m.state, cmd)
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.state != StateContextMenu || m.contextMenu.detection.State != installation.StateUnknown || len(m.contextMenu.actions) != 0 {
		t.Fatalf("unavailable detector produced unsafe menu: state=%v detection=%v actions=%v", m.state, m.contextMenu.detection.State, m.contextMenu.actions)
	}
}

func TestBlockedOperationReturnsToMenuWithoutExecuting(t *testing.T) {
	m := NewModel(buildTestDeps())
	m.state = StateContextMenu
	m.contextMenu = NewContextMenuModel(m.deps.Theme, installation.Detection{State: installation.StateLegacyPM2})
	updated, cmd := m.Update(ContextActionSelectedMsg{Action: ContextActionMigration})
	m = updated.(Model)
	if m.state != StateBlockedOperation || cmd != nil {
		t.Fatalf("blocked state/cmd = %v/%v", m.state, cmd)
	}
	updated, _ = m.Update(BlockedOperationDismissedMsg{})
	if updated.(Model).state != StateContextMenu {
		t.Fatalf("dismissed state = %v", updated.(Model).state)
	}
}

func TestRootRejectsLateTransitionMessages(t *testing.T) {
	for _, tt := range []struct {
		name string
		msg  tea.Msg
	}{
		{"late detection start", DetectionStartedMsg{}},
		{"late detection completion", DetectionCompletedMsg{Detection: installation.Detection{State: installation.StateNotInstalled}}},
		{"late blocked dismissal", BlockedOperationDismissedMsg{}},
		{"forged preflight start", PreflightStartedMsg{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(buildTestDeps())
			m.state = StateWorkspaceInput
			updated, cmd := m.Update(tt.msg)
			if updated.(Model).state != StateWorkspaceInput || cmd != nil {
				t.Fatalf("late message changed state/cmd to %v/%v", updated.(Model).state, cmd)
			}
		})
	}
}

func TestContextualStatesUseGlobalGuards(t *testing.T) {
	m := NewModel(buildTestDeps())
	m.state = StateContextMenu
	m.contextMenu = NewContextMenuModel(m.deps.Theme, installation.Detection{State: installation.StateCurrent})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 79, Height: 24})
	if !strings.Contains(updated.(Model).View(), "Terminal too small") {
		t.Fatal("context menu must use the global small-terminal guard")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("Escape must exit safely from contextual menu")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("Escape command = %T, want tea.QuitMsg", cmd())
	}
}
