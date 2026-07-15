package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/migration"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

type MigrationDispositionSelectedMsg struct {
	Disposition migration.ContainerDisposition
}
type MigrationRemoveConfirmedMsg struct{}

type migrationDispositionModel struct {
	theme  theme.Theme
	cursor int
}

func (m migrationDispositionModel) Update(msg tea.Msg) (migrationDispositionModel, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.Type {
	case tea.KeyUp:
		m.cursor = 0
	case tea.KeyDown:
		m.cursor = 1
	case tea.KeyEnter:
		choice := migration.DispositionStop
		if m.cursor == 1 {
			choice = migration.DispositionRemove
		}
		return m, func() tea.Msg { return MigrationDispositionSelectedMsg{Disposition: choice} }
	}
	return m, nil
}

func (m migrationDispositionModel) View() string {
	var b strings.Builder
	b.WriteString(m.theme.Primary.Bold(true).Render("Choose legacy PostgreSQL container disposition"))
	b.WriteString("\n\nThe validated backup is complete. Volumes are always preserved.\n\n")
	choices := []string{"Stop legacy PostgreSQL container (recommended)", "Remove legacy PostgreSQL container (volumes preserved)"}
	for i, choice := range choices {
		prefix := "  "
		if i == m.cursor {
			prefix = "> "
		}
		b.WriteString(prefix + choice + "\n")
	}
	b.WriteString("\nUse up/down and Enter. Press Escape to stop here.\n")
	return b.String()
}

func (m Model) migrationRemoveConfirmationView() string {
	return m.deps.Theme.Danger.Bold(true).Render("Confirm irreversible container removal") +
		"\n\nOnly the corroborated legacy PostgreSQL container will be removed. Its volumes will be preserved. The installer cannot automatically recreate the removed container if later work fails.\n\nPress Enter to confirm removal or Escape to return without mutation.\n"
}
