package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/installation"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

// ContextAction is an operator choice derived only from typed detection evidence.
type ContextAction uint8

const (
	ContextActionInstall ContextAction = iota
	ContextActionUpdate
	ContextActionUninstall
	ContextActionMigration
)

type ContextActionSelectedMsg struct{ Action ContextAction }
type DetectionStartedMsg struct{}
type DetectionCompletedMsg struct{ Detection installation.Detection }
type BlockedOperationDismissedMsg struct{}

type ContextMenuModel struct {
	theme     theme.Theme
	detection installation.Detection
	actions   []ContextAction
	cursor    int
}

func NewContextMenuModel(th theme.Theme, detection installation.Detection) ContextMenuModel {
	m := ContextMenuModel{theme: th, detection: detection}
	switch detection.State {
	case installation.StateNotInstalled:
		m.actions = []ContextAction{ContextActionInstall}
	case installation.StateCurrent:
		m.actions = []ContextAction{ContextActionUpdate, ContextActionUninstall}
	case installation.StateLegacyPM2:
		m.actions = []ContextAction{ContextActionMigration}
	}
	return m
}

func (m ContextMenuModel) hasAction(action ContextAction) bool {
	for _, available := range m.actions {
		if available == action {
			return true
		}
	}
	return false
}

func (m ContextMenuModel) Update(msg tea.Msg) (ContextMenuModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown:
			if m.cursor+1 < len(m.actions) {
				m.cursor++
			}
		case tea.KeyEnter:
			if len(m.actions) > 0 {
				action := m.actions[m.cursor]
				return m, func() tea.Msg { return ContextActionSelectedMsg{Action: action} }
			}
		}
	}
	return m, nil
}

func (m ContextMenuModel) View() string {
	var b strings.Builder
	b.WriteString(m.theme.Primary.Bold(true).Render("Installation options"))
	b.WriteString("\n\n")
	if len(m.actions) == 0 {
		b.WriteString(m.theme.TextMuted.Render("The installer cannot safely choose an action from the detected evidence."))
		b.WriteString("\nReview the installation artifacts and use an explicit CLI route when appropriate.\n")
	} else {
		for i, action := range m.actions {
			prefix := "  "
			if i == m.cursor {
				prefix = "> "
			}
			label := contextActionLabel(action)
			if action == ContextActionUninstall || action == ContextActionMigration {
				label += " (not available in this version)"
			}
			b.WriteString(prefix + label + "\n")
		}
	}
	for _, evidence := range m.detection.Evidence {
		if isNormalAbsenceEvidence(evidence.Kind) {
			continue
		}
		b.WriteString("- " + safeEvidenceLabel(evidence) + "\n")
	}
	b.WriteString("\nUse ↑/↓ and Enter. Press q or Ctrl+C to exit.\n")
	return b.String()
}

type blockedOperationModel struct {
	theme  theme.Theme
	action ContextAction
}

func (m blockedOperationModel) Update(msg tea.Msg) (blockedOperationModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEsc {
		return m, func() tea.Msg { return BlockedOperationDismissedMsg{} }
	}
	return m, nil
}

func (m blockedOperationModel) View() string {
	return m.theme.Primary.Bold(true).Render(contextActionLabel(m.action)+" is not available") + "\n\nThis operation has no approved safety contract and will not run. No files, processes, or services were changed.\n\nPress Escape to return to the menu, or q to exit.\n"
}

func contextActionLabel(action ContextAction) string {
	switch action {
	case ContextActionInstall:
		return "Install"
	case ContextActionUpdate:
		return "Update"
	case ContextActionUninstall:
		return "Uninstall"
	case ContextActionMigration:
		return "Migration"
	}
	return "Unknown action"
}

func isNormalAbsenceEvidence(kind installation.EvidenceKind) bool {
	switch kind {
	case installation.EvidenceWorkspaceAbsent,
		installation.EvidencePM2Absent,
		installation.EvidencePM2Unavailable,
		installation.EvidencePM2Unsupported:
		return true
	default:
		return false
	}
}

func safeEvidenceLabel(e installation.Evidence) string {
	labels := map[installation.EvidenceKind]string{
		installation.EvidenceWorkspaceComplete:   "Current workspace artifacts found",
		installation.EvidenceWorkspaceAbsent:     "No current workspace artifacts found",
		installation.EvidenceWorkspacePartial:    "Partial workspace artifacts found",
		installation.EvidenceWorkspaceInvalid:    "Invalid workspace artifacts found",
		installation.EvidenceWorkspaceUnreadable: "Workspace artifacts could not be read",
		installation.EvidencePM2AliceProcess:     "Configured legacy PM2 deployment found",
		installation.EvidencePM2Absent:           "No configured legacy PM2 deployment found",
		installation.EvidencePM2Unavailable:      "PM2 is not installed",
		installation.EvidencePM2Unsupported:      "Legacy PM2 probing is unsupported on this platform",
		installation.EvidencePM2Ambiguous:        "Legacy PM2 evidence needs manual verification",
		installation.EvidencePM2Failed:           "Legacy PM2 probe could not be verified",
	}
	if label, ok := labels[e.Kind]; ok {
		return label
	}
	return "Installation evidence requires manual verification"
}
