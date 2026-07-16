package tui

import (
	"errors"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

type MigrationAuthenticator interface {
	Authenticate() tea.Cmd
}

type MigrationAuthorizationValidator interface {
	Validate() tea.Cmd
}

type MigrationAuthorizationRefresher interface {
	Refresh() tea.Cmd
}

type MigrationAuthorizationCheckedMsg struct{ Err error }

type MigrationAuthorizationRefreshCompletedMsg struct{ Err error }

type SudoMigrationAuthenticator struct{}

func (SudoMigrationAuthenticator) Authenticate() tea.Cmd {
	return tea.ExecProcess(exec.Command("sudo", "-v"), func(err error) tea.Msg {
		return MigrationAuthenticationCompletedMsg{Err: err}
	})
}

func (SudoMigrationAuthenticator) Refresh() tea.Cmd {
	return tea.ExecProcess(exec.Command("sudo", "-v"), func(err error) tea.Msg {
		return MigrationAuthorizationRefreshCompletedMsg{Err: err}
	})
}

type SudoMigrationAuthorizationValidator struct{}

func (SudoMigrationAuthorizationValidator) Validate() tea.Cmd {
	return func() tea.Msg {
		if err := exec.Command("sudo", "-n", "true").Run(); err != nil {
			return MigrationAuthorizationCheckedMsg{Err: errors.New("migration authorization is unavailable")}
		}
		return MigrationAuthorizationCheckedMsg{}
	}
}

type DockerAuthenticationCompletedMsg struct{ Err error }

type SudoDockerAuthenticator struct{}

func (SudoDockerAuthenticator) Authenticate() tea.Cmd {
	return tea.ExecProcess(exec.Command("sudo", "-v"), func(err error) tea.Msg {
		return DockerAuthenticationCompletedMsg{Err: err}
	})
}
