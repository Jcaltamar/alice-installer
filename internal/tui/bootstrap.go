package tui

import (
	"fmt"
	"os/user"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jcaltamar/alice-installer/internal/preflight"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

// ---------------------------------------------------------------------------
// Classification
// ---------------------------------------------------------------------------

// ClassifyBlockers splits failing items in report into fixable and non-fixable
// sets. Only CheckMediaWritable and CheckConfigWritable are considered fixable
// for v1.
func ClassifyBlockers(report preflight.Report, mediaDir, configDir string) (fixable []Action, nonFixable []preflight.CheckResult) {
	// Resolve current username for chown; fall back to "$USER" if lookup fails.
	username := "$USER"
	if u, err := user.Current(); err == nil && u.Username != "" {
		username = u.Username
	}

	for _, item := range report.Items {
		if item.Status != preflight.StatusFail {
			continue
		}
		switch item.ID {
		case preflight.CheckMediaWritable:
			fixable = append(fixable, buildDirAction(string(preflight.CheckMediaWritable), mediaDir, username))
		case preflight.CheckConfigWritable:
			fixable = append(fixable, buildDirAction(string(preflight.CheckConfigWritable), configDir, username))
		default:
			nonFixable = append(nonFixable, item)
		}
	}
	return fixable, nonFixable
}

// buildDirAction constructs the Action that creates dir and grants ownership.
func buildDirAction(id, dir, username string) Action {
	script := fmt.Sprintf("mkdir -p %s && chown -R %s:%s %s", dir, username, username, dir)
	return Action{
		ID:          id,
		Description: fmt.Sprintf("Create %s and grant ownership to %s", dir, username),
		Command:     "sudo",
		Args:        []string{"sh", "-c", script},
	}
}

// ---------------------------------------------------------------------------
// Executor interface
// ---------------------------------------------------------------------------

// Executor is the test seam for running bootstrap actions.
// The production implementation uses tea.ExecProcess so the sudo password
// prompt reaches the real TTY.
type Executor interface {
	ExecCmd(action Action) tea.Cmd
}

// teaExecutor is the production Executor.
type teaExecutor struct{}

// NewExecutor returns the production Executor.
func NewExecutor() Executor { return teaExecutor{} }

// ExecCmd wraps tea.ExecProcess so the program releases the alt-screen while
// sudo is running.
func (teaExecutor) ExecCmd(a Action) tea.Cmd {
	// Import exec lazily via os/exec at call time.
	return execProcessCmd(a)
}

// FakeExecutor is a test double for Executor.
// Callers pre-load Results; each ExecCmd call pops the next result.
type FakeExecutor struct {
	Results []BootstrapActionResultMsg
	calls   int
}

// ExecCmd returns a synchronous tea.Cmd that posts the next pre-set result.
func (f *FakeExecutor) ExecCmd(_ Action) tea.Cmd {
	idx := f.calls
	f.calls++
	return func() tea.Msg {
		return f.Results[idx]
	}
}

// ---------------------------------------------------------------------------
// BootstrapModel
// ---------------------------------------------------------------------------

// BootstrapModel manages the confirm/execute/progress TUI state for bootstrap.
type BootstrapModel struct {
	theme      theme.Theme
	executor   Executor
	actions    []Action
	currentIdx int
	results    []BootstrapActionResultMsg
	confirming bool // true = waiting for Y/N; false = executing
	done       bool
	failed     *BootstrapFailedMsg
	declined   bool
}

// NewBootstrapModel constructs a BootstrapModel ready to confirm.
func NewBootstrapModel(th theme.Theme, exec Executor, actions []Action) BootstrapModel {
	return BootstrapModel{
		theme:      th,
		executor:   exec,
		actions:    actions,
		confirming: true,
	}
}

// Init implements tea.Model. Nothing to start automatically — waiting for user input.
func (m BootstrapModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m BootstrapModel) Update(msg tea.Msg) (BootstrapModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.confirming {
			switch {
			case msg.Type == tea.KeyRunes && (strings.EqualFold(string(msg.Runes), "y")):
				m.confirming = false
				if len(m.actions) == 0 {
					return m, func() tea.Msg { return BootstrapCompleteMsg{} }
				}
				return m, m.executor.ExecCmd(m.actions[m.currentIdx])

			case msg.Type == tea.KeyEnter:
				m.confirming = false
				if len(m.actions) == 0 {
					return m, func() tea.Msg { return BootstrapCompleteMsg{} }
				}
				return m, m.executor.ExecCmd(m.actions[m.currentIdx])

			case msg.Type == tea.KeyRunes && (strings.EqualFold(string(msg.Runes), "n")):
				m.declined = true
				return m, func() tea.Msg { return BootstrapSkippedMsg{} }

			case msg.Type == tea.KeyEsc:
				m.declined = true
				return m, func() tea.Msg { return BootstrapSkippedMsg{} }
			}
		}

	case BootstrapActionResultMsg:
		m.results = append(m.results, msg)
		if msg.Err != nil {
			failed := BootstrapFailedMsg{ActionID: msg.ActionID, Err: msg.Err}
			m.failed = &failed
			return m, func() tea.Msg { return failed }
		}
		m.currentIdx++
		if m.currentIdx >= len(m.actions) {
			m.done = true
			return m, func() tea.Msg { return BootstrapCompleteMsg{} }
		}
		return m, m.executor.ExecCmd(m.actions[m.currentIdx])
	}

	return m, nil
}

// View implements tea.Model.
func (m BootstrapModel) View() string {
	var sb strings.Builder

	title := m.theme.Primary.Bold(true).Render("Bootstrap — elevated setup")
	sb.WriteString(title)
	sb.WriteString("\n\n")

	if m.confirming {
		sb.WriteString(m.theme.TextMuted.Render("The following commands require sudo to set up required directories:"))
		sb.WriteString("\n\n")
		for i, a := range m.actions {
			bullet := fmt.Sprintf("  %d. %s\n", i+1, a.Description)
			sb.WriteString(m.theme.TextPrimary.Render(bullet))
			cmdLine := fmt.Sprintf("     $ %s %s\n", a.Command, strings.Join(a.Args, " "))
			sb.WriteString(m.theme.TextMuted.Render(cmdLine))
		}
		sb.WriteString("\n")
		sb.WriteString(m.theme.Warning.Render("Press Y or Enter to execute, N or Esc to skip"))
		sb.WriteString("\n")
		return sb.String()
	}

	if m.done {
		sb.WriteString(m.theme.Success.Render("✓  All bootstrap actions completed. Re-running preflight…"))
		sb.WriteString("\n")
		return sb.String()
	}

	if m.failed != nil {
		sb.WriteString(m.theme.Danger.Render(fmt.Sprintf("✗  Action %q failed: %v", m.failed.ActionID, m.failed.Err)))
		sb.WriteString("\n")
		return sb.String()
	}

	// Executing: show progress.
	total := len(m.actions)
	current := m.currentIdx
	if current < total {
		a := m.actions[current]
		line := fmt.Sprintf("Executing %d/%d: %s…", current+1, total, a.Description)
		sb.WriteString(m.theme.TextPrimary.Render(line))
	}

	// Show completed.
	for i, res := range m.results {
		if res.Err == nil && i < len(m.actions) {
			done := fmt.Sprintf("\n  ✓  %s", m.actions[i].Description)
			sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(string(theme.ColorSuccess))).Render(done))
		}
	}
	sb.WriteString("\n")

	return sb.String()
}
