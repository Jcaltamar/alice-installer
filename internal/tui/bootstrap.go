package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jcaltamar/alice-installer/internal/preflight"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

// ---------------------------------------------------------------------------
// Action ID constants
// ---------------------------------------------------------------------------

const (
	ActionIDDockerInstall = "docker_install"
	ActionIDSystemdStart  = "systemd_start_docker"
	ActionIDDockerGroup   = "docker_group_add"
)

// ---------------------------------------------------------------------------
// Action constructors
// ---------------------------------------------------------------------------

// dockerInstallAction returns the Action that installs Docker via get.docker.com.
func dockerInstallAction() Action {
	return Action{
		ID:          ActionIDDockerInstall,
		Description: "Install Docker engine via get.docker.com",
		Command:     "sudo",
		Args:        []string{"sh", "-c", "curl -fsSL https://get.docker.com | sh"},
	}
}

// systemdStartDockerAction returns the Action that enables and starts the Docker daemon via systemd.
func systemdStartDockerAction() Action {
	return Action{
		ID:          ActionIDSystemdStart,
		Description: "Enable and start Docker daemon (systemctl enable --now docker)",
		Command:     "sudo",
		Args:        []string{"systemctl", "enable", "--now", "docker"},
	}
}

// dockerGroupAddAction returns the Action that adds username to the docker group.
// It includes a mandatory PostActionBanner instructing the user to re-login.
func dockerGroupAddAction(username string) Action {
	return Action{
		ID:               ActionIDDockerGroup,
		Description:      fmt.Sprintf("Add %s to the 'docker' group", username),
		Command:          "sudo",
		Args:             []string{"usermod", "-aG", "docker", username},
		PostActionBanner: "Log out and back in (or run `newgrp docker`) for the new group membership to take effect.",
	}
}

// ---------------------------------------------------------------------------
// Classification
// ---------------------------------------------------------------------------

// ClassifyBlockers splits failing items in report into fixable and non-fixable sets.
// env provides host environment information used to decide which Docker actions to offer.
// Actions are returned in priority order:
//
//	1. docker_install (if Docker binary is missing)
//	2. dir-creation actions (media, config)
//	3. systemd_start_docker (if Docker present, user in group, systemd available)
//	4. docker_group_add (if Docker present but user not in docker group)
func ClassifyBlockers(report preflight.Report, env BootstrapEnv, mediaDir, configDir string) (fixable []Action, nonFixable []preflight.CheckResult) {
	username := env.UserName
	if username == "" {
		username = "$USER"
	}

	// Buckets for priority ordering.
	var dockerInstall []Action // priority 1
	var dirActions []Action    // priority 2
	var systemdActions []Action // priority 3
	var groupActions []Action  // priority 4

	for _, item := range report.Items {
		if item.Status != preflight.StatusFail {
			continue
		}
		switch item.ID {
		case preflight.CheckDockerDaemon:
			switch {
			case !env.DockerBinaryPresent:
				// Docker not installed at all → install it.
				dockerInstall = append(dockerInstall, dockerInstallAction())
			case !env.UserInDockerGroup:
				// Binary present but user not in docker group → add to group.
				// The daemon might be running fine once group is right.
				groupActions = append(groupActions, dockerGroupAddAction(username))
			case env.SystemdPresent:
				// Binary present, user in group, systemd available → start daemon.
				systemdActions = append(systemdActions, systemdStartDockerAction())
			default:
				// Binary present, user in group, no systemd → non-fixable.
				nonFixable = append(nonFixable, item)
			}
		case preflight.CheckMediaWritable:
			dirActions = append(dirActions, buildDirAction(string(preflight.CheckMediaWritable), mediaDir, username))
		case preflight.CheckConfigWritable:
			dirActions = append(dirActions, buildDirAction(string(preflight.CheckConfigWritable), configDir, username))
		default:
			nonFixable = append(nonFixable, item)
		}
	}

	// Assemble in priority order.
	fixable = append(fixable, dockerInstall...)
	fixable = append(fixable, dirActions...)
	fixable = append(fixable, systemdActions...)
	fixable = append(fixable, groupActions...)

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
		// Banner screen: wait for Enter to dismiss and emit BootstrapCompleteMsg.
		if m.showingBanner {
			if msg.Type == tea.KeyEnter {
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
