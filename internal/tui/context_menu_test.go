package tui

import (
	"context"
	"errors"
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

type fakeUpdateAction struct {
	err   error
	calls int
}

func (a *fakeUpdateAction) Run(context.Context) error {
	a.calls++
	return a.err
}

func TestContextMenuActionMatrix(t *testing.T) {
	tests := []struct {
		name     string
		state    installation.State
		want     []ContextAction
		contains string
	}{
		{"not installed", installation.StateNotInstalled, []ContextAction{ContextActionInstall}, "Install"},
		{"current", installation.StateCurrent, []ContextAction{ContextActionUpdate, ContextActionUninstall}, "Uninstall (not available in this version)"},
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
	m = updated.(Model)
	if m.state != StatePreflight {
		t.Fatalf("Install state = %v, want StatePreflight", m.state)
	}
}

func TestRootUpdateRunsOnceAndReportsFailure(t *testing.T) {
	deps := buildTestDeps()
	action := &fakeUpdateAction{err: errors.New("pull failed")}
	deps.UpdateAction = action
	m := NewModel(deps)
	m.state = StateContextMenu
	m.contextMenu = NewContextMenuModel(deps.Theme, installation.Detection{State: installation.StateCurrent})
	updated, cmd := m.Update(ContextActionSelectedMsg{Action: ContextActionUpdate})
	m = updated.(Model)
	if m.state != StateUpdating || cmd == nil {
		t.Fatalf("update state/cmd = %v/%v", m.state, cmd)
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if action.calls != 1 || m.state != StateActionResult || !strings.Contains(m.View(), "pull failed") {
		t.Fatalf("calls/state/view = %d/%v/%q", action.calls, m.state, m.View())
	}
}

func TestContextMenuUsesGlobalSmallTerminalGuard(t *testing.T) {
	m := NewModel(buildTestDeps())
	m.state = StateContextMenu
	m.contextMenu = NewContextMenuModel(m.deps.Theme, installation.Detection{State: installation.StateCurrent})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 79, Height: 24})
	if !strings.Contains(updated.(Model).View(), "Terminal too small") {
		t.Fatal("contextual menu must use the global small-terminal guard")
	}
}

func TestContextMenuEscapeQuitsWithoutExecuting(t *testing.T) {
	m := NewModel(buildTestDeps())
	m.state = StateContextMenu
	m.contextMenu = NewContextMenuModel(m.deps.Theme, installation.Detection{State: installation.StateCurrent})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("Escape should exit safely from the contextual menu")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("Escape command = %T, want tea.QuitMsg", cmd())
	}
}

func TestBlockedOperationReturnsToMenuWithoutExecuting(t *testing.T) {
	deps := buildTestDeps()
	m := NewModel(deps)
	m.state = StateContextMenu
	m.contextMenu = NewContextMenuModel(deps.Theme, installation.Detection{State: installation.StateLegacyPM2})
	updated, _ := m.Update(ContextActionSelectedMsg{Action: ContextActionMigration})
	m = updated.(Model)
	if m.state != StateBlockedOperation || !strings.Contains(m.View(), "not available") {
		t.Fatalf("blocked state/view = %v/%q", m.state, m.View())
	}
	updated, _ = m.Update(BlockedOperationDismissedMsg{})
	m = updated.(Model)
	if m.state != StateContextMenu {
		t.Fatalf("dismissed state = %v", m.state)
	}
}

func TestMigrationAvailabilityReflectsExecutableDependencies(t *testing.T) {
	detection := installation.Detection{
		State: installation.StateLegacyPM2,
		Evidence: []installation.Evidence{{
			Kind: installation.EvidencePM2AliceProcess,
		}},
	}
	tests := []struct {
		name          string
		configure     func(*Dependencies)
		wantAvailable bool
	}{
		{
			name: "complete migration capability",
			configure: func(deps *Dependencies) {
				deps.LegacyBackupAction = &fakeLegacyBackupAction{}
				deps.LegacyRestoreAction = &fakeLegacyRestoreAction{}
				deps.MigrationHandoff = &fakeMigrationHandoff{}
			},
			wantAvailable: true,
		},
		{
			name: "incomplete migration capability",
			configure: func(deps *Dependencies) {
				deps.LegacyBackupAction = &fakeLegacyBackupAction{}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := buildTestDeps()
			tt.configure(&deps)
			m := NewModel(deps)
			updated, _ := m.Update(DetectionCompletedMsg{Detection: detection})
			m = updated.(Model)

			view := m.View()
			if strings.Contains(view, "Migration (not available in this version)") == tt.wantAvailable {
				t.Fatalf("migration availability label mismatch: %q", view)
			}
			if !strings.Contains(view, "Configured legacy PM2 deployment found") {
				t.Fatalf("detection evidence missing from migration menu: %q", view)
			}

			updated, cmd := m.Update(ContextActionSelectedMsg{Action: ContextActionMigration})
			m = updated.(Model)
			if tt.wantAvailable {
				if m.state != StateBackupPreflight || cmd == nil {
					t.Fatalf("available migration state/cmd = %v/%v", m.state, cmd)
				}
				return
			}
			if m.state != StateBlockedOperation || cmd != nil {
				t.Fatalf("incomplete migration state/cmd = %v/%v", m.state, cmd)
			}
		})
	}
}

func TestRootRejectsActionsNotOfferedByDetection(t *testing.T) {
	tests := []struct {
		name      string
		detection installation.Detection
		action    ContextAction
	}{
		{"unknown cannot install", installation.Detection{State: installation.StateUnknown}, ContextActionInstall},
		{"current cannot migrate", installation.Detection{State: installation.StateCurrent}, ContextActionMigration},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := buildTestDeps()
			m := NewModel(deps)
			m.state = StateContextMenu
			m.contextMenu = NewContextMenuModel(deps.Theme, tt.detection)

			updated, cmd := m.Update(ContextActionSelectedMsg{Action: tt.action})
			m = updated.(Model)
			if m.state != StateContextMenu {
				t.Fatalf("state = %v, want StateContextMenu", m.state)
			}
			if cmd != nil {
				t.Fatal("an action absent from the contextual menu must not emit a command")
			}
		})
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
	for _, confusing := range []string{
		"No current workspace artifacts found",
		"No configured legacy PM2 deployment found",
		"PM2 is not installed",
		"Legacy PM2 probing is unsupported on this platform",
	} {
		if strings.Contains(view, confusing) {
			t.Fatalf("view includes normal absence evidence %q: %q", confusing, view)
		}
	}
}

func TestContextMenuEvidenceUsesSafeLabels(t *testing.T) {
	m := NewContextMenuModel(themeDefaultForTest(), installation.Detection{
		State: installation.StateUnknown,
		Evidence: []installation.Evidence{{
			Kind:   installation.EvidencePM2Failed,
			Detail: "token=secret",
			Path:   "/private/deployment/.env",
		}},
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

func TestRootContextualViewsAndFallbacks(t *testing.T) {
	m := NewModel(buildTestDeps())
	updated, cmd := m.Update(DetectionStartedMsg{})
	m = updated.(Model)
	if !strings.Contains(m.View(), "Detecting existing installation") || cmd == nil {
		t.Fatal("detection state must render while its command is pending")
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.contextMenu.detection.State != installation.StateNotInstalled {
		t.Fatalf("detector state = %v", m.contextMenu.detection.State)
	}

	m.state = StateUpdating
	if !strings.Contains(m.View(), "Updating existing installation") {
		t.Fatal("updating state must render progress")
	}
	m.state = StateActionResult
	m.actionResult = actionResultModel{theme: m.deps.Theme}
	if !strings.Contains(m.View(), "Update completed") {
		t.Fatal("successful update result must be rendered")
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
		t.Fatalf("unavailable detector must produce a safe menu, got state=%v detection=%v actions=%v", m.state, m.contextMenu.detection.State, m.contextMenu.actions)
	}
}

func TestRootUnavailableUpdateReportsResultAndDoesNotRetry(t *testing.T) {
	deps := buildTestDeps()
	deps.UpdateAction = nil
	m := NewModel(deps)
	m.state = StateContextMenu
	m.contextMenu = NewContextMenuModel(deps.Theme, installation.Detection{State: installation.StateCurrent})

	updated, cmd := m.Update(ContextActionSelectedMsg{Action: ContextActionUpdate})
	m = updated.(Model)
	if m.state != StateUpdating || cmd == nil {
		t.Fatalf("state/cmd = %v/%v", m.state, cmd)
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.state != StateActionResult || !strings.Contains(m.View(), "update action is unavailable") {
		t.Fatalf("state/view = %v/%q", m.state, m.View())
	}
	updated, duplicate := m.Update(ContextActionSelectedMsg{Action: ContextActionUpdate})
	if updated.(Model).state != StateActionResult || duplicate != nil {
		t.Fatal("late action selection must not re-run an update")
	}
}

func TestContextMenuWithoutActionsNeverEmitsSelection(t *testing.T) {
	menu := NewContextMenuModel(themeDefaultForTest(), installation.Detection{State: installation.StateConflict})
	_, cmd := menu.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("a blocked contextual menu must not emit a lifecycle action")
	}
}

func TestDetectionAndBlockedOperationCancellationHaveNoLifecycleCommand(t *testing.T) {
	m := NewModel(buildTestDeps())
	m.state = StateDetecting
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("Escape during detection must quit safely")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("detection Escape = %T, want tea.QuitMsg", cmd())
	}

	m.state = StateBlockedOperation
	m.blockedOperation = blockedOperationModel{theme: m.deps.Theme, action: ContextActionUninstall}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("Escape on blocked operation must only dismiss the screen")
	}
	updated, cmd = m.Update(cmd())
	m = updated.(Model)
	if m.state != StateContextMenu || cmd != nil {
		t.Fatalf("blocked Escape state/cmd = %v/%v, want StateContextMenu/nil", m.state, cmd)
	}
}
