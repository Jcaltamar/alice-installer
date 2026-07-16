package tui

import (
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

type MigrationAuthenticator interface {
	Authenticate() tea.Cmd
}

type SudoMigrationAuthenticator struct{}

func (SudoMigrationAuthenticator) Authenticate() tea.Cmd {
	return tea.ExecProcess(exec.Command("sudo", "-v"), func(err error) tea.Msg {
		return MigrationAuthenticationCompletedMsg{Err: err}
	})
}

type DockerAuthenticationCompletedMsg struct{ Err error }

type SudoDockerAuthenticator struct{}

func (SudoDockerAuthenticator) Authenticate() tea.Cmd {
	return tea.ExecProcess(exec.Command("sudo", "-v"), func(err error) tea.Msg {
		return DockerAuthenticationCompletedMsg{Err: err}
	})
}
