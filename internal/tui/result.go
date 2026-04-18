package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/theme"
)

// ResultModel renders the final installation result screen.
//
// Behaviour:
//   - Success: green banner, services list with ticks, .env path, next-steps.
//   - Failure: red banner, stage + error, remediation hints.
//   - q or Enter → tea.Quit.
//   - r → AbortMsg (reserved for v2 restart flow; currently exits).
type ResultModel struct {
	theme         theme.Theme
	success       bool
	successDetail *InstallSuccessMsg
	failure       *InstallFailureMsg
}

// NewResultModel constructs a ResultModel.
// Pass successDetail non-nil for success, failureDetail non-nil for failure.
func NewResultModel(
	th theme.Theme,
	successDetail *InstallSuccessMsg,
	failureDetail *InstallFailureMsg,
) ResultModel {
	return ResultModel{
		theme:         th,
		success:       successDetail != nil,
		successDetail: successDetail,
		failure:       failureDetail,
	}
}

// Init implements tea.Model.
func (r ResultModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (r ResultModel) Update(msg tea.Msg) (ResultModel, tea.Cmd) {
	switch m := msg.(type) {
	case tea.KeyMsg:
		switch {
		case m.Type == tea.KeyEnter:
			return r, tea.Quit
		case m.Type == tea.KeyRunes && string(m.Runes) == "q":
			return r, tea.Quit
		case m.Type == tea.KeyRunes && string(m.Runes) == "r":
			// Reserved for restart; quit for now (v1 scope).
			return r, tea.Quit
		}
	}
	return r, nil
}

// View implements tea.Model.
func (r ResultModel) View() string {
	if r.success && r.successDetail != nil {
		return r.viewSuccess()
	}
	if r.failure != nil {
		return r.viewFailure()
	}
	// Fallback — should not happen in production.
	return r.theme.TextMuted.Render("Installation complete.\n\nPress Enter or q to exit.\n")
}

func (r ResultModel) viewSuccess() string {
	var sb strings.Builder
	sb.WriteString(r.theme.Success.Bold(true).Render("╔══════════════════════════════════════╗"))
	sb.WriteString("\n")
	sb.WriteString(r.theme.Success.Bold(true).Render("║   Installation complete  ✓           ║"))
	sb.WriteString("\n")
	sb.WriteString(r.theme.Success.Bold(true).Render("╚══════════════════════════════════════╝"))
	sb.WriteString("\n\n")

	if r.successDetail != nil {
		if r.successDetail.EnvPath != "" {
			sb.WriteString(r.theme.TextPrimary.Render(fmt.Sprintf("  .env written to: %s", r.successDetail.EnvPath)))
			sb.WriteString("\n")
		}

		if len(r.successDetail.Services) > 0 {
			sb.WriteString("\n")
			sb.WriteString(r.theme.TextPrimary.Render("  Services:"))
			sb.WriteString("\n")
			for _, s := range r.successDetail.Services {
				sb.WriteString(r.theme.Success.Render(fmt.Sprintf("    ✓  %s", s.Service)))
				sb.WriteString("\n")
			}
		}

		sb.WriteString("\n")
		sb.WriteString(r.theme.TextPrimary.Render("  Next steps:"))
		sb.WriteString("\n")
		sb.WriteString(r.theme.TextMuted.Render("    • View logs:   docker compose logs -f"))
		sb.WriteString("\n")
		sb.WriteString(r.theme.TextMuted.Render("    • Stop stack:  docker compose down"))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(r.theme.TextMuted.Render("  Press Enter or q to exit."))
	sb.WriteString("\n")
	return sb.String()
}

func (r ResultModel) viewFailure() string {
	var sb strings.Builder
	sb.WriteString(r.theme.Danger.Bold(true).Render("╔══════════════════════════════════════╗"))
	sb.WriteString("\n")
	sb.WriteString(r.theme.Danger.Bold(true).Render("║   Installation failed  ✗             ║"))
	sb.WriteString("\n")
	sb.WriteString(r.theme.Danger.Bold(true).Render("╚══════════════════════════════════════╝"))
	sb.WriteString("\n\n")

	sb.WriteString(r.theme.TextPrimary.Render(fmt.Sprintf("  Stage: %s", r.failure.Stage)))
	sb.WriteString("\n")
	if r.failure.Err != nil {
		sb.WriteString(r.theme.Danger.Render(fmt.Sprintf("  Error: %s", r.failure.Err.Error())))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(r.theme.TextPrimary.Render("  Remediation:"))
	sb.WriteString("\n")
	sb.WriteString(r.theme.TextMuted.Render("    1. Check logs:         docker compose logs -f"))
	sb.WriteString("\n")
	sb.WriteString(r.theme.TextMuted.Render("    2. Clean up:           docker compose down"))
	sb.WriteString("\n")
	sb.WriteString(r.theme.TextMuted.Render("    3. Re-run installer to try again."))
	sb.WriteString("\n\n")
	sb.WriteString(r.theme.TextMuted.Render("  Press Enter or q to exit."))
	sb.WriteString("\n")
	return sb.String()
}
