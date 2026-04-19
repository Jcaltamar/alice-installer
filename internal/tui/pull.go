package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

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
//     or InstallFailureMsg; and one that drains the progress channel into PullProgressMsg
//     messages. Each progress msg received by Update re-schedules another drain read so
//     the channel keeps feeding messages until Pull() finishes and closes it.
//   - On PullProgressMsg → updates services map AND re-issues drain cmd.
//   - On PullCompleteMsg → done=true, emits DeployStartedMsg.
//   - On InstallFailureMsg → surfaced via err.
type PullModel struct {
	theme      theme.Theme
	compose    compose.ComposeRunner
	files      []string
	envFile    string
	spinner    spinner.Model
	services   map[string]string // service → last status
	err        error
	done       bool
	progressCh chan compose.PullProgressMsg
}

// NewPullModel constructs a PullModel. The progress channel is created here so
// Init and Update can both reference it (Init produces, Update drains).
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
		theme:      th,
		compose:    runner,
		files:      files,
		envFile:    envFile,
		spinner:    sp,
		services:   make(map[string]string),
		progressCh: make(chan compose.PullProgressMsg, 64),
	}
}

// Init implements tea.Model.
// Starts the pull operation and the first drain read.
func (p PullModel) Init() tea.Cmd {
	ch := p.progressCh
	runCmd := func() tea.Msg {
		err := p.compose.Pull(context.Background(), p.files, p.envFile, ch)
		close(ch)
		if err != nil {
			return InstallFailureMsg{Err: err, Stage: "pull"}
		}
		return PullCompleteMsg{}
	}
	return tea.Batch(runCmd, p.drainNext())
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

// drainNext returns a Cmd that reads ONE message from the progress channel.
// When the channel is closed (Pull finished), it returns nil so Update stops
// rescheduling. As long as a progress msg arrives, Update re-issues this Cmd.
func (p PullModel) drainNext() tea.Cmd {
	ch := p.progressCh
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
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
		// Reschedule another drain read so the next progress msg arrives.
		return p, p.drainNext()

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

	if len(p.services) == 0 {
		body := p.spinner.View() + " " + p.theme.TextMuted.Render("Pulling images…")
		return title + "\n\n" + body + "\n"
	}

	// Per-service progress block, sorted for stable rendering.
	names := make([]string, 0, len(p.services))
	for name := range p.services {
		names = append(names, name)
	}
	sort.Strings(names)

	var lines []string
	lines = append(lines, p.spinner.View()+" "+p.theme.TextMuted.Render(fmt.Sprintf("Pulling images (%d active)…", len(p.services))))
	for _, name := range names {
		lines = append(lines, fmt.Sprintf("  %s  %s", name, p.theme.TextMuted.Render(p.services[name])))
	}
	return title + "\n\n" + strings.Join(lines, "\n") + "\n"
}
