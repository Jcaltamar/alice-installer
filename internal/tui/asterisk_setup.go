package tui

import (
	"context"
	"errors"
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jcaltamar/alice-installer/internal/asterisk"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

// AsteriskInstaller is the dependency seam for optional host Asterisk setup.
type AsteriskInstaller interface {
	Install(context.Context, asterisk.Options) (asterisk.Result, error)
}

// AsteriskSetupModel renders and runs the optional Asterisk setup step.
type AsteriskSetupModel struct {
	theme     theme.Theme
	installer AsteriskInstaller
	options   asterisk.Options
	spinner   spinner.Model
	done      bool
	err       error
	result    asterisk.Result
}

// NewAsteriskSetupModel constructs the optional Asterisk setup model.
func NewAsteriskSetupModel(th theme.Theme, installer AsteriskInstaller, options asterisk.Options) AsteriskSetupModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(string(theme.ColorPrimary)))
	return AsteriskSetupModel{
		theme:     th,
		installer: installer,
		options:   options,
		spinner:   sp,
	}
}

// Init implements tea.Model.
func (a AsteriskSetupModel) Init() tea.Cmd {
	return func() tea.Msg {
		if a.installer == nil {
			return InstallFailureMsg{Err: errors.New("asterisk installer dependency is not configured"), Stage: "asterisk-setup"}
		}
		result, err := a.installer.Install(context.Background(), a.options)
		if err != nil {
			return InstallFailureMsg{Err: err, Stage: "asterisk-setup"}
		}
		return AsteriskSetupCompleteMsg{Result: result}
	}
}

// Update implements tea.Model.
func (a AsteriskSetupModel) Update(msg tea.Msg) (AsteriskSetupModel, tea.Cmd) {
	switch m := msg.(type) {
	case AsteriskSetupCompleteMsg:
		a.done = true
		a.result = m.Result
		return a, func() tea.Msg { return PullStartedMsg{} }

	case InstallFailureMsg:
		a.err = m.Err
		return a, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		a.spinner, cmd = a.spinner.Update(m)
		return a, cmd
	}
	return a, nil
}

// View implements tea.Model.
func (a AsteriskSetupModel) View() string {
	title := a.theme.Primary.Bold(true).Render("Asterisk Setup")
	if a.err != nil {
		return title + "\n\n" + a.theme.Danger.Render("✗  Asterisk setup failed: "+a.err.Error()) + "\n"
	}
	if a.done {
		return title + "\n\n" + a.theme.Success.Render(fmt.Sprintf("✓  Asterisk ready at %s.", a.result.AMIEndpoint)) + "\n"
	}
	return title + "\n\n" + a.spinner.View() + " " + a.theme.TextMuted.Render("Preparing host Asterisk…") + "\n"
}
