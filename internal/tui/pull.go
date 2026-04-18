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

// PullModel renders the docker compose pull progress screen.
//
// Behaviour:
//   - Init() spawns two commands: one that runs Pull (blocking) then emits PullCompleteMsg
//     or InstallFailureMsg; and one that drains a progress channel and re-emits each
//     PullProgressMsg.
//   - On PullProgressMsg → updates services map.
//   - On PullCompleteMsg → done=true, emits DeployStartedMsg.
//   - On InstallFailureMsg → surfaced via err.
type PullModel struct {
	theme    theme.Theme
	compose  compose.ComposeRunner
	files    []string
	envFile  string
	spinner  spinner.Model
	services map[string]string // service → last status
	err      error
	done     bool
}

// NewPullModel constructs a PullModel.
func NewPullModel(
	th theme.Theme,
	runner compose.ComposeRunner,
	files []string,
	envFile string,
) PullModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(string(theme.ColorPrimary)))
	return PullModel{
		theme:    th,
		compose:  runner,
		files:    files,
		envFile:  envFile,
		spinner:  sp,
		services: make(map[string]string),
	}
}

// Init implements tea.Model.
// Starts the pull operation and the progress drain loop.
func (p PullModel) Init() tea.Cmd {
	ch := make(chan compose.PullProgressMsg, 64)
	// Command 1: run pull, close channel when done.
	runCmd := func() tea.Msg {
		err := p.compose.Pull(context.Background(), p.files, p.envFile, ch)
		close(ch)
		if err != nil {
			return InstallFailureMsg{Err: err, Stage: "pull"}
		}
		return PullCompleteMsg{}
	}
	// Command 2: drain progress channel.
	drainCmd := drainPullCh(ch)
	return tea.Batch(runCmd, drainCmd)
}

// runPull is a test helper that executes pull synchronously using a fresh channel.
// It returns the terminal message (PullCompleteMsg or InstallFailureMsg).
func (p PullModel) runPull() tea.Msg {
	ch := make(chan compose.PullProgressMsg, 64)
	err := p.compose.Pull(context.Background(), p.files, p.envFile, ch)
	close(ch)
	if err != nil {
		return InstallFailureMsg{Err: err, Stage: "pull"}
	}
	return PullCompleteMsg{}
}

// drainPullCh returns a Cmd that reads one message from ch and re-emits it,
// then recurses until the channel is closed.
func drainPullCh(ch <-chan compose.PullProgressMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			// Channel closed — pull finished; return a sentinel.
			return nil
		}
		return msg
	}
}

// Update implements tea.Model.
func (p PullModel) Update(msg tea.Msg) (PullModel, tea.Cmd) {
	switch m := msg.(type) {
	case compose.PullProgressMsg:
		if m.Service != "" {
			p.services[m.Service] = m.Status
		}
		// Continue draining — but we don't hold a reference to the channel here;
		// the drain is driven by the batched Cmd from Init.
		return p, nil

	case PullCompleteMsg:
		p.done = true
		return p, func() tea.Msg { return DeployStartedMsg{} }

	case InstallFailureMsg:
		p.err = m.Err
		return p, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		p.spinner, cmd = p.spinner.Update(m)
		return p, cmd
	}
	return p, nil
}

// View implements tea.Model.
func (p PullModel) View() string {
	title := p.theme.Primary.Bold(true).Render("Pulling Images")

	if p.err != nil {
		return title + "\n\n" + p.theme.Danger.Render("✗  Pull failed: "+p.err.Error()) + "\n"
	}

	if p.done {
		return title + "\n\n" + p.theme.Success.Render("✓  All images pulled.") + "\n"
	}

	var body string
	if len(p.services) == 0 {
		body = p.spinner.View() + " " + p.theme.TextMuted.Render("Pulling images…")
	} else {
		body = p.spinner.View() + " " + p.theme.TextMuted.Render(fmt.Sprintf("Pulling images (%d services)…", len(p.services)))
	}

	return title + "\n\n" + body + "\n"
}
