package tui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/migration"
)

type fakeMigrationAuthorizationValidator struct {
	err   error
	calls int
}

func (v *fakeMigrationAuthorizationValidator) Validate() tea.Cmd {
	v.calls++
	return func() tea.Msg { return MigrationAuthorizationCheckedMsg{Err: v.err} }
}

type fakeMigrationAuthorizationRefresher struct {
	err   error
	calls int
}

func (r *fakeMigrationAuthorizationRefresher) Refresh() tea.Cmd {
	r.calls++
	return func() tea.Msg { return MigrationAuthorizationRefreshCompletedMsg{Err: r.err} }
}

func migrationModelAfterPull(t *testing.T, validator *fakeMigrationAuthorizationValidator, refresher *fakeMigrationAuthorizationRefresher) (Model, *compose.FakeComposeRunner, *fakeMigrationHandoff) {
	t.Helper()
	deps := buildTestDeps()
	runner := &compose.FakeComposeRunner{}
	handoff := &fakeMigrationHandoff{}
	deps.Compose = runner
	deps.MigrationHandoff = handoff
	deps.MigrationAuthValidator = validator
	deps.MigrationAuthRefresher = refresher
	m := NewModel(deps)
	m.state = StatePull
	m.migrationPending = true
	m.migrationLease = &migration.PreInstallMigrationLease{}
	m.composeFiles = []string{"compose.yml"}
	m.envPath = ".env"
	m.pull = NewPullModel(deps.Theme, runner, m.composeFiles, m.envPath)
	if msg := m.pull.runPull(); msg != (PullCompleteMsg{}) {
		t.Fatalf("pull result = %#v, want PullCompleteMsg", msg)
	}
	return m, runner, handoff
}

func TestMigrationPullCompleteWithValidAuthorizationDeploysWithoutPrompt(t *testing.T) {
	validator := &fakeMigrationAuthorizationValidator{}
	refresher := &fakeMigrationAuthorizationRefresher{}
	m, runner, _ := migrationModelAfterPull(t, validator, refresher)

	updated, cmd := m.Update(PullCompleteMsg{})
	m = updated.(Model)
	if m.state != StateMigrationAuthCheck || cmd == nil || validator.calls != 1 {
		t.Fatalf("post-pull check state/cmd/calls = %v/%v/%d", m.state, cmd, validator.calls)
	}
	updated, cmd = m.Update(cmd())
	m = updated.(Model)
	if m.state != StateDeploy || cmd == nil || refresher.calls != 0 {
		t.Fatalf("valid authorization state/cmd/refreshes = %v/%v/%d", m.state, cmd, refresher.calls)
	}
	if len(runner.CallOrder) != 1 || runner.CallOrder[0] != "pull" {
		t.Fatalf("compose calls before deploy command = %v, want one pull", runner.CallOrder)
	}
}

func TestMigrationPullCompleteExpiredAuthorizationPromptsThenDeploys(t *testing.T) {
	validator := &fakeMigrationAuthorizationValidator{err: errors.New("authorization expired")}
	refresher := &fakeMigrationAuthorizationRefresher{}
	m, runner, _ := migrationModelAfterPull(t, validator, refresher)

	updated, cmd := m.Update(PullCompleteMsg{})
	m = updated.(Model)
	updated, cmd = m.Update(cmd())
	m = updated.(Model)
	if m.state != StateMigrationAuthRefresh || cmd == nil || refresher.calls != 1 {
		t.Fatalf("expired authorization state/cmd/refreshes = %v/%v/%d", m.state, cmd, refresher.calls)
	}
	updated, cmd = m.Update(cmd())
	m = updated.(Model)
	if m.state != StateDeploy || cmd == nil {
		t.Fatalf("refreshed authorization state/cmd = %v/%v", m.state, cmd)
	}
	if len(runner.CallOrder) != 1 || runner.CallOrder[0] != "pull" {
		t.Fatalf("refresh repeated prior work: %v", runner.CallOrder)
	}
}

func TestMigrationRecoveryRefreshesAuthorizationAndPreservesOriginalFailure(t *testing.T) {
	for _, refreshErr := range []error{nil, errors.New("cancelled")} {
		name := "success"
		if refreshErr != nil {
			name = "failure"
		}
		t.Run(name, func(t *testing.T) {
			deps := buildTestDeps()
			refresher := &fakeMigrationAuthorizationRefresher{err: refreshErr}
			handoff := &fakeMigrationHandoff{}
			deps.MigrationAuthRefresher = refresher
			deps.MigrationHandoff = handoff
			m := NewModel(deps)
			m.migrationPending = true
			m.migrationLease = &migration.PreInstallMigrationLease{}
			original := &InstallFailureMsg{Err: errors.New("restore failed"), Stage: "database-restore"}
			m.originalFailure = original

			m, cmd := m.beginMigrationRecovery()
			if m.state != StateMigrationAuthRefresh || cmd == nil || refresher.calls != 1 || handoff.failureCalls != 0 {
				t.Fatalf("refresh state/cmd/calls/recovery = %v/%v/%d/%d", m.state, cmd, refresher.calls, handoff.failureCalls)
			}
			updated, recoveryCmd := m.Update(cmd())
			m = updated.(Model)
			if m.state != StateMigrationRecovery || recoveryCmd == nil || m.originalFailure != original || handoff.failureCalls != 0 {
				t.Fatalf("recovery state/cmd/original/calls = %v/%v/%p/%d", m.state, recoveryCmd, m.originalFailure, handoff.failureCalls)
			}
			_ = recoveryCmd()
			if handoff.failureCalls != 1 {
				t.Fatalf("recovery calls = %d, want 1", handoff.failureCalls)
			}
		})
	}
}

func TestMigrationAuthorizationRefreshFailureRecoversWithoutDeploy(t *testing.T) {
	validator := &fakeMigrationAuthorizationValidator{err: errors.New("authorization expired")}
	refresher := &fakeMigrationAuthorizationRefresher{err: errors.New("cancelled")}
	m, runner, handoff := migrationModelAfterPull(t, validator, refresher)

	updated, cmd := m.Update(PullCompleteMsg{})
	m = updated.(Model)
	updated, cmd = m.Update(cmd())
	m = updated.(Model)
	updated, cmd = m.Update(cmd())
	m = updated.(Model)
	if m.state != StateMigrationRecovery || cmd == nil || handoff.failureCalls != 0 {
		t.Fatalf("refresh failure state/cmd/recovery calls = %v/%v/%d", m.state, cmd, handoff.failureCalls)
	}
	if m.originalFailure == nil || m.originalFailure.Stage != "migration-authorization-refresh" {
		t.Fatalf("refresh diagnostic = %#v", m.originalFailure)
	}
	if len(runner.CallOrder) != 1 || runner.CallOrder[0] != "pull" {
		t.Fatalf("refresh failure attempted deploy or repeated pull: %v", runner.CallOrder)
	}
}
