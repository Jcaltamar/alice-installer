package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/migration"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

type MigrationEnvironmentSelectedMsg struct{ Environment migration.EnvironmentName }

type migrationEnvironmentModel struct {
	theme  theme.Theme
	cursor int
}

var migrationEnvironments = []migration.EnvironmentName{
	migration.EnvironmentDevelopment,
	migration.EnvironmentProduction,
}

func (m migrationEnvironmentModel) Update(msg tea.Msg) (migrationEnvironmentModel, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}
	case tea.KeyDown:
		if m.cursor+1 < len(migrationEnvironments) {
			m.cursor++
		}
	case tea.KeyEnter:
		environment := migrationEnvironments[m.cursor]
		return m, func() tea.Msg { return MigrationEnvironmentSelectedMsg{Environment: environment} }
	}
	return m, nil
}

func (m migrationEnvironmentModel) View() string {
	var b strings.Builder
	b.WriteString(m.theme.Primary.Bold(true).Render("Select legacy configuration environment"))
	b.WriteString("\n\nChoose the Sequelize configuration block used by the existing deployment.\n\n")
	for i, environment := range migrationEnvironments {
		prefix := "  "
		if i == m.cursor {
			prefix = "> "
		}
		label := "Production"
		if environment == migration.EnvironmentDevelopment {
			label = "Development"
		}
		b.WriteString(prefix + label + "\n")
	}
	b.WriteString("\nUse ↑/↓ and Enter. Press q or Ctrl+C to exit.\n")
	return b.String()
}
