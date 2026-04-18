package tui

import (
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

// execProcessCmd returns a tea.Cmd that runs the action via tea.ExecProcess.
// Separated so that the os/exec import is isolated and tests can avoid it.
func execProcessCmd(a Action) tea.Cmd {
	c := exec.Command(a.Command, a.Args...) //nolint:gosec // args are controlled by ClassifyBlockers
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return BootstrapActionResultMsg{ActionID: a.ID, Err: err}
	})
}
