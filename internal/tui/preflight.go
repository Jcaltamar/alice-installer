package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jcaltamar/alice-installer/internal/preflight"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

// preflightTimeout bounds the total coordinator run. Covers docker info + version
// + compose version + GPU probe + dir probes + port scans serially, on a cold host.
const preflightTimeout = 60 * time.Second

// PreflightModel renders the preflight-checks screen.
//
// Behaviour:
//   - Init() runs the coordinator in a tea.Cmd and returns a PreflightResultMsg.
//   - While running, shows a spinner + "Running preflight checks…".
//   - On PreflightResultMsg, populates the report and renders per-item status dots.
//   - Enter when report has blocking failures → no-op.
//   - Enter when report has no failures → emits PreflightPassedMsg.
type PreflightModel struct {
	theme   theme.Theme
	coord   preflight.Coordinator
	spinner spinner.Model
	report  *preflight.Report
	err     error
}

// NewPreflightModel constructs a PreflightModel.
func NewPreflightModel(th theme.Theme, coord preflight.Coordinator) PreflightModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(string(theme.ColorPrimary)))
	return PreflightModel{
		theme:   th,
		coord:   coord,
		spinner: sp,
	}
}

// Rearm resets the preflight state (clears report and error) and returns a
// fresh Init cmd. This makes re-running preflight idempotent — the root model
// calls this after a successful bootstrap to re-evaluate checks.
func (p *PreflightModel) Rearm() tea.Cmd {
	p.report = nil
	p.err = nil
	return p.Init()
}

// Init implements tea.Model.
// Returns a Cmd that runs the coordinator and emits PreflightResultMsg.
func (p PreflightModel) Init() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), preflightTimeout)
		defer cancel()
		report := p.coord.Run(ctx)
		return PreflightResultMsg{Report: report}
	}
}

// Update implements tea.Model.
func (p PreflightModel) Update(msg tea.Msg) (PreflightModel, tea.Cmd) {
	switch m := msg.(type) {
	case PreflightResultMsg:
		r := m.Report
		p.report = &r
		return p, nil

	case tea.KeyMsg:
		switch m.Type {
		case tea.KeyEnter:
			if p.report != nil && !p.report.HasBlockingFailure() {
				return p, func() tea.Msg { return PreflightPassedMsg{} }
			}
			// Blocking failure or no report yet — no-op.
			return p, nil
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		p.spinner, cmd = p.spinner.Update(m)
		return p, cmd
	}
	return p, nil
}

// View implements tea.Model.
func (p PreflightModel) View() string {
	title := p.theme.Primary.Bold(true).Render("Preflight Checks")
	if p.report == nil {
		// Still running.
		running := p.theme.TextMuted.Render("Running preflight checks…")
		return fmt.Sprintf("%s\n\n%s %s\n", title, p.spinner.View(), running)
	}

	// Report received — render per-item status.
	var sb strings.Builder
	sb.WriteString(title)
	sb.WriteString("\n\n")

	for _, item := range p.report.Items {
		dot := statusDot(p.theme, item.Status)
		sb.WriteString(fmt.Sprintf("  %s  %s\n", dot, item.Title))
		if item.Detail != "" {
			sb.WriteString(fmt.Sprintf("       %s\n", p.theme.TextMuted.Render(item.Detail)))
		}
	}

	sb.WriteString("\n")

	if p.report.HasBlockingFailure() {
		sb.WriteString(p.theme.Danger.Bold(true).Render("✗  Blocking issues found. Resolve the errors above and re-run the installer."))
		sb.WriteString("\n")
	} else if len(p.report.Warnings()) > 0 {
		sb.WriteString(p.theme.Warning.Render("⚠  Warnings detected (non-blocking). Press Enter to continue."))
		sb.WriteString("\n")
	} else {
		sb.WriteString(p.theme.Success.Render("✓  All checks passed. Press Enter to continue."))
		sb.WriteString("\n")
	}

	return sb.String()
}

// statusDot returns a coloured dot string for the given preflight status.
func statusDot(th theme.Theme, s preflight.Status) string {
	switch s {
	case preflight.StatusPass:
		return th.Success.Render("●")
	case preflight.StatusWarn:
		return th.Warning.Render("●")
	default: // StatusFail
		return th.Danger.Render("●")
	}
}
