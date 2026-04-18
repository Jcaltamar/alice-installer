package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jcaltamar/alice-installer/internal/envgen"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

// WorkspaceInputModel renders the workspace name input screen.
//
// Behaviour:
//   - On Init the textinput gets focus with placeholder "my-site-name".
//   - User types; on Enter the value is validated via envgen.ValidateWorkspace.
//   - Valid → emits WorkspaceEnteredMsg{Value: trimmed}.
//   - Invalid → sets err string; re-renders with error in Danger colour.
//   - "q" key is intentionally NOT intercepted here; the root model skips the
//     q-to-quit shortcut when state == StateWorkspaceInput.
type WorkspaceInputModel struct {
	theme theme.Theme
	input textinput.Model
	err   string
}

// NewWorkspaceInputModel constructs a focused WorkspaceInputModel.
func NewWorkspaceInputModel(th theme.Theme) WorkspaceInputModel {
	ti := textinput.New()
	ti.Placeholder = "my-site-name"
	ti.CharLimit = 64
	ti.Width = 40
	ti.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(string(theme.ColorTextPrimary)))
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(string(theme.ColorTextMuted)))
	ti.Focus()
	return WorkspaceInputModel{
		theme: th,
		input: ti,
	}
}

// Init implements tea.Model.
func (w WorkspaceInputModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update implements tea.Model.
func (w WorkspaceInputModel) Update(msg tea.Msg) (WorkspaceInputModel, tea.Cmd) {
	var cmd tea.Cmd

	switch m := msg.(type) {
	case tea.KeyMsg:
		switch m.Type {
		case tea.KeyEnter:
			val := strings.TrimSpace(w.input.Value())
			if err := envgen.ValidateWorkspace(val); err != nil {
				w.err = err.Error()
				return w, nil
			}
			w.err = ""
			return w, func() tea.Msg { return WorkspaceEnteredMsg{Value: val} }
		}
	}

	// Forward all other messages to the textinput.
	w.input, cmd = w.input.Update(msg)
	return w, cmd
}

// View implements tea.Model.
func (w WorkspaceInputModel) View() string {
	title := w.theme.Primary.Bold(true).Render("Workspace Name")
	hint := w.theme.TextMuted.Render("Enter a name for this installation workspace (alphanumeric, hyphens, underscores):")

	view := title + "\n\n" + hint + "\n\n  " + w.input.View() + "\n"

	if w.err != "" {
		view += "\n  " + w.theme.Danger.Render("✗  "+w.err) + "\n"
	}

	view += "\n" + w.theme.TextMuted.Render("  Press Enter to continue.")

	return view
}
