package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/theme"
)

// SplashModel renders the initial branding screen.
//
// Behaviour:
//   - Displays the "ALICE GUARDIAN" ASCII art banner in Primary (cyan) colour.
//   - Displays "Installer v0.1.0" subtitle in TextMuted colour.
//   - Enter → emits PreflightStartedMsg to advance to the preflight state.
//   - Any other key → no-op.
type SplashModel struct {
	theme theme.Theme
}

// NewSplashModel constructs a SplashModel with the given theme.
func NewSplashModel(th theme.Theme) SplashModel {
	return SplashModel{theme: th}
}

// Init implements tea.Model.
// Returns nil — the splash screen waits for the user to press Enter.
func (s SplashModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
// Enter key returns a command that emits PreflightStartedMsg.
func (s SplashModel) Update(msg tea.Msg) (SplashModel, tea.Cmd) {
	switch m := msg.(type) {
	case tea.KeyMsg:
		switch m.Type {
		case tea.KeyEnter:
			return s, func() tea.Msg { return PreflightStartedMsg{} }
		}
	}
	return s, nil
}

// View implements tea.Model.
// Renders the ASCII-art banner and subtitle using theme colours.
func (s SplashModel) View() string {
	banner := `
     _    _     ___ ____ _____    ____ _   _    _    ____  ____ ___ ____  _   _
    / \  | |   |_ _/ ___| ____|  / ___| | | |  / \  |  _ \|  _ \_ _|  _ \| \ | |
   / _ \ | |    | | |   |  _|   | |  _| | | | / _ \ | |_) | | | | || |_) |  \| |
  / ___ \| |___ | | |___| |___  | |_| | |_| |/ ___ \|  _ <| |_| | ||  _ <| |\  |
 /_/   \_\_____|___\____|_____|  \____|\___//_/   \_\_| \_\____/___|_| \_\_| \_|

  ALICE GUARDIAN`

	title := s.theme.Primary.Bold(true).Render(banner)
	subtitle := s.theme.TextMuted.Render("  Installer v0.1.0  —  press Enter to start")

	return title + "\n\n" + subtitle + "\n"
}
