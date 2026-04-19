package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jcaltamar/alice-installer/internal/bootstrap"
	"github.com/jcaltamar/alice-installer/internal/preflight"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

// ---------------------------------------------------------------------------
// Re-exports from bootstrap package (keep TUI callers unchanged)
// ---------------------------------------------------------------------------

// Action is an alias for bootstrap.Action so that TUI-internal code compiles
// without changes.
type Action = bootstrap.Action

// BootstrapEnv is an alias for bootstrap.BootstrapEnv.
type BootstrapEnv = bootstrap.BootstrapEnv

// Action ID constants — re-exported for backward compatibility with tests.
const (
	ActionIDDockerInstall = bootstrap.ActionIDDockerInstall
	ActionIDSystemdStart  = bootstrap.ActionIDSystemdStart
	ActionIDDockerGroup   = bootstrap.ActionIDDockerGroup
)

// ClassifyBlockers delegates to bootstrap.ClassifyBlockers.
// Signature is identical; all call-sites in model.go and tests continue to work.
func ClassifyBlockers(report preflight.Report, env BootstrapEnv, mediaDir, configDir, workspaceDir string) (fixable []Action, nonFixable []preflight.CheckResult) {
	return bootstrap.ClassifyBlockers(report, env, mediaDir, configDir, workspaceDir)
}

// DetectEnv delegates to bootstrap.DetectEnv.
func DetectEnv() BootstrapEnv {
	return bootstrap.DetectEnv()
}

// ---------------------------------------------------------------------------
// Package-level action constructors (unexported thin wrappers)
// These exist so that tui-internal tests can call them without importing bootstrap.
// ---------------------------------------------------------------------------

func dockerInstallAction() Action     { return bootstrap.DockerInstallAction() }
func systemdStartDockerAction() Action { return bootstrap.SystemdStartDockerAction() }
func dockerGroupAddAction(username string) Action { return bootstrap.DockerGroupAddAction(username) }
func buildDirAction(id, dir, username string) Action { return bootstrap.BuildDirAction(id, dir, username) }
func buildUserDirAction(id, dir string) Action { return bootstrap.BuildUserDirAction(id, dir) }

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
	theme         theme.Theme
	executor      Executor
	actions       []Action
	currentIdx    int
	results       []BootstrapActionResultMsg
	confirming    bool // true = waiting for Y/N; false = executing
	done          bool
	failed        *BootstrapFailedMsg
	declined      bool
	showingBanner bool     // true when displaying post-action banners, waiting for Enter
	banners       []string // accumulated PostActionBanner values from completed actions
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
		// Banner screen: wait for Enter. If banners were emitted (currently only
		// by docker_group_add), re-running preflight in the same session is
		// pointless — group membership doesn't apply to running processes until
		// the user re-logs in. Exit cleanly with the banner text printed to
		// scrollback so the instructions survive alt-screen teardown.
		if m.showingBanner {
			if msg.Type == tea.KeyEnter {
				if len(m.banners) > 0 {
					cmds := []tea.Cmd{
						tea.Println(""),
						tea.Println("Bootstrap complete. Action required before continuing:"),
					}
					for _, b := range m.banners {
						cmds = append(cmds, tea.Println("  • "+b))
					}
					cmds = append(cmds,
						tea.Println(""),
						tea.Println("Once done, re-run `alice-installer`."),
						tea.Quit,
					)
					return m, tea.Sequence(cmds...)
				}
				return m, func() tea.Msg { return BootstrapCompleteMsg{} }
			}
			return m, nil
		}

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
			// Collect banners from all successfully completed actions.
			var banners []string
			for _, a := range m.actions {
				if a.PostActionBanner != "" {
					banners = append(banners, a.PostActionBanner)
				}
			}
			if len(banners) > 0 {
				m.showingBanner = true
				m.banners = banners
				// Do NOT emit BootstrapCompleteMsg yet — wait for user Enter.
				return m, nil
			}
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

	// Banner screen: show post-action banners and wait for Enter.
	if m.showingBanner {
		sb.WriteString(m.theme.Success.Render("✓  All bootstrap actions completed."))
		sb.WriteString("\n\n")
		for _, banner := range m.banners {
			sb.WriteString(m.theme.Warning.Render("⚠  " + banner))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
		sb.WriteString(m.theme.TextMuted.Render("Press Enter to continue"))
		sb.WriteString("\n")
		return sb.String()
	}

	if m.confirming {
		sb.WriteString(m.theme.TextMuted.Render("The following commands require sudo to set up required directories:"))
		sb.WriteString("\n\n")
		for i, a := range m.actions {
			bullet := fmt.Sprintf("  %d. %s", i+1, a.Description)
			sb.WriteString(m.theme.TextPrimary.Render(bullet))
			sb.WriteString("\n")
			cmdLine := fmt.Sprintf("     $ %s %s", a.Command, strings.Join(a.Args, " "))
			sb.WriteString(m.theme.TextMuted.Render(cmdLine))
			sb.WriteString("\n")
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
	successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(string(theme.ColorSuccess)))
	for i, res := range m.results {
		if res.Err == nil && i < len(m.actions) {
			sb.WriteString("\n  ")
			sb.WriteString(successStyle.Render(fmt.Sprintf("✓  %s", m.actions[i].Description)))
		}
	}
	sb.WriteString("\n")

	return sb.String()
}
