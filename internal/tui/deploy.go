package tui

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

// DeployModel renders the docker compose up progress screen.
//
// Behaviour:
//   - Init() spawns a Cmd that runs Up (blocking) then emits DeployCompleteMsg
//     or InstallFailureMsg; and a drain Cmd that re-emits UpProgressMsg ticks.
//   - On UpProgressMsg → updates services map.
//   - On DeployCompleteMsg → done=true, emits HealthTickMsg to trigger verify.
//   - On InstallFailureMsg → surfaced via err.
type DeployModel struct {
	theme    theme.Theme
	compose  compose.ComposeRunner
	files    []string
	envFile  string
	spinner  spinner.Model
	services map[string]string // service → last status
	err      error
	done     bool
}

// NewDeployModel constructs a DeployModel.
func NewDeployModel(
	th theme.Theme,
	runner compose.ComposeRunner,
	files []string,
	envFile string,
) DeployModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(string(theme.ColorPrimary)))
	return DeployModel{
		theme:    th,
		compose:  runner,
		files:    files,
		envFile:  envFile,
		spinner:  sp,
		services: make(map[string]string),
	}
}

// Init implements tea.Model.
// Starts the deploy operation and the progress drain loop.
func (d DeployModel) Init() tea.Cmd {
	ch := make(chan compose.UpProgressMsg, 64)
	runCmd := func() tea.Msg {
		err := d.compose.Up(context.Background(), d.files, d.envFile, ch)
		close(ch)
		if err != nil {
			return InstallFailureMsg{Err: err, Stage: "deploy"}
		}
		return DeployCompleteMsg{}
	}
	drainCmd := drainUpCh(ch)
	return tea.Batch(runCmd, drainCmd)
}

// runDeploy is a test helper that executes Up synchronously using a fresh channel.
func (d DeployModel) runDeploy() tea.Msg {
	ch := make(chan compose.UpProgressMsg, 64)
	err := d.compose.Up(context.Background(), d.files, d.envFile, ch)
	close(ch)
	if err != nil {
		return InstallFailureMsg{Err: err, Stage: "deploy"}
	}
	return DeployCompleteMsg{}
}

// drainUpCh returns a Cmd that reads one message from ch and re-emits it.
func drainUpCh(ch <-chan compose.UpProgressMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

// Update implements tea.Model.
func (d DeployModel) Update(msg tea.Msg) (DeployModel, tea.Cmd) {
	switch m := msg.(type) {
	case compose.UpProgressMsg:
		if m.Service != "" {
			d.services[m.Service] = m.Status
		}
		return d, nil

	case DeployCompleteMsg:
		d.done = true
		return d, func() tea.Msg { return HealthTickMsg{} }

	case InstallFailureMsg:
		d.err = m.Err
		return d, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		d.spinner, cmd = d.spinner.Update(m)
		return d, cmd
	}
	return d, nil
}

// View implements tea.Model.
func (d DeployModel) View() string {
	title := d.theme.Primary.Bold(true).Render("Deploying Stack")

	if d.err != nil {
		return title + "\n\n" + d.theme.Danger.Render("✗  Deploy failed: "+d.err.Error()) + "\n"
	}

	if d.done {
		return title + "\n\n" + d.theme.Success.Render("✓  Stack started. Waiting for healthchecks…") + "\n"
	}

	var body string
	if len(d.services) == 0 {
		body = d.spinner.View() + " " + d.theme.TextMuted.Render("Starting services…")
	} else {
		body = d.spinner.View() + " " + d.theme.TextMuted.Render(fmt.Sprintf("Starting services (%d running)…", len(d.services)))
	}

	return title + "\n\n" + body + "\n"
}
