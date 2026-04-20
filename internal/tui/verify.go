package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

// VerifyModel renders the healthcheck polling screen.
//
// Behaviour:
//   - Init() starts the HealthTickMsg polling interval (every 3s).
//   - On each HealthTickMsg: calls HealthStatus; if all healthy → done=true + InstallSuccessMsg.
//   - If elapsed >= timeout → InstallFailureMsg{Stage: "verify"}.
//   - 'r' key → immediate tick for manual refresh.
type VerifyModel struct {
	theme    theme.Theme
	compose  compose.ComposeRunner
	files    []string
	envFile  string
	spinner  spinner.Model
	services []compose.ServiceHealth
	elapsed  time.Duration
	timeout  time.Duration
	tickInterval time.Duration
	err      error
	done     bool
}

// NewVerifyModel constructs a VerifyModel with a 3-minute timeout.
func NewVerifyModel(
	th theme.Theme,
	runner compose.ComposeRunner,
	files []string,
	envFile string,
) VerifyModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(string(theme.ColorPrimary)))
	return VerifyModel{
		theme:        th,
		compose:      runner,
		files:        files,
		envFile:      envFile,
		spinner:      sp,
		timeout:      3 * time.Minute,
		tickInterval: 3 * time.Second,
	}
}

// Init implements tea.Model.
// Starts the first health tick.
func (v VerifyModel) Init() tea.Cmd {
	return tea.Tick(v.tickInterval, func(_ time.Time) tea.Msg {
		return HealthTickMsg{}
	})
}

// poll calls HealthStatus and returns the result as a tea.Msg.
func (v VerifyModel) poll() tea.Msg {
	services, err := v.compose.HealthStatus(context.Background(), v.files, v.envFile)
	if err != nil {
		return InstallFailureMsg{Err: err, Stage: "verify"}
	}
	allReady := true
	for _, s := range services {
		if !compose.IsReady(s) {
			allReady = false
			break
		}
	}
	if allReady && len(services) > 0 {
		return InstallSuccessMsg{Services: services}
	}
	return HealthReportMsg{Services: services, Done: false}
}

// Update implements tea.Model.
func (v VerifyModel) Update(msg tea.Msg) (VerifyModel, tea.Cmd) {
	switch m := msg.(type) {
	case HealthTickMsg:
		// Check timeout first.
		if v.elapsed >= v.timeout {
			return v, func() tea.Msg {
				return InstallFailureMsg{
					Err:   fmt.Errorf("healthcheck timeout after %s", v.timeout),
					Stage: "verify",
				}
			}
		}

		// Poll health status.
		pollResult := v.poll()
		v.elapsed += v.tickInterval

		switch r := pollResult.(type) {
		case InstallSuccessMsg:
			v.done = true
			v.services = r.Services
			return v, func() tea.Msg { return r }

		case InstallFailureMsg:
			v.err = r.Err
			return v, func() tea.Msg { return r }

		case HealthReportMsg:
			v.services = r.Services
			// Schedule next tick.
			nextTick := tea.Tick(v.tickInterval, func(_ time.Time) tea.Msg {
				return HealthTickMsg{}
			})
			return v, nextTick
		}

	case tea.KeyMsg:
		if m.Type == tea.KeyRunes && string(m.Runes) == "r" {
			// Immediate manual refresh.
			return v, func() tea.Msg { return HealthTickMsg{} }
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		v.spinner, cmd = v.spinner.Update(m)
		return v, cmd
	}
	return v, nil
}

// View implements tea.Model.
func (v VerifyModel) View() string {
	title := v.theme.Primary.Bold(true).Render("Health Check")

	if v.err != nil {
		return title + "\n\n" + v.theme.Danger.Render("✗  "+v.err.Error()) + "\n"
	}

	if v.done {
		return title + "\n\n" + v.theme.Success.Render("✓  All services are healthy.") + "\n"
	}

	var sb strings.Builder
	sb.WriteString(title)
	sb.WriteString("\n\n")

	ready := 0
	for _, s := range v.services {
		if compose.IsReady(s) {
			ready++
		}
	}

	if len(v.services) > 0 {
		sb.WriteString(v.theme.TextMuted.Render(fmt.Sprintf("%d/%d healthy", ready, len(v.services))))
		sb.WriteString("\n")
		for _, s := range v.services {
			dot := v.theme.TextMuted.Render("●")
			if compose.IsReady(s) {
				dot = v.theme.Success.Render("●")
			} else if s.Status == "unhealthy" {
				dot = v.theme.Danger.Render("●")
			}
			// Show state when it adds information: empty/none health or non-running state.
			label := s.Status
			if s.Status == "" || s.Status == "none" {
				if s.State != "" {
					label = s.State
				}
			} else if s.State != "" && s.State != "running" {
				label = fmt.Sprintf("%s/%s", s.Status, s.State)
			}
			sb.WriteString(fmt.Sprintf("  %s  %s (%s)\n", dot, s.Service, label))
		}
	} else {
		sb.WriteString(v.spinner.View() + " " + v.theme.TextMuted.Render("Polling healthchecks…"))
	}

	sb.WriteString("\n")
	sb.WriteString(v.theme.TextMuted.Render("  Press [r] to refresh manually."))
	sb.WriteString("\n")

	return sb.String()
}
