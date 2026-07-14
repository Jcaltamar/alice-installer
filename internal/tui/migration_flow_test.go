package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/installation"
	"github.com/jcaltamar/alice-installer/internal/migration"
)

type fakeLegacyBackupAction struct {
	preflightCalls int
	runCalls       int
	preflightErr   error
	result         migration.BackupResult
}

func (a *fakeLegacyBackupAction) Preflight(context.Context, migration.BackupRequest) (migration.BackupPlan, error) {
	a.preflightCalls++
	return migration.BackupPlan{}, a.preflightErr
}
func (a *fakeLegacyBackupAction) Run(context.Context, migration.BackupPlan) migration.BackupResult {
	a.runCalls++
	return a.result
}

func TestMigrationRequiresConfirmationBeforeRun(t *testing.T) {
	deps := buildTestDeps()
	action := &fakeLegacyBackupAction{result: migration.BackupResult{Outcome: migration.BackupValidated, DumpPath: "/safe/backup.dump", ManifestPath: "/safe/backup.json", SHA256: "abc", Size: 42}}
	deps.LegacyBackupAction = action
	deps.LegacyRestoreAction = &fakeLegacyRestoreAction{result: migration.RestoreResult{Outcome: migration.RestoreSucceeded}}
	deps.MigrationHandoff = &fakeMigrationHandoff{}
	deps.LegacyBackupRequest = migration.BackupRequest{Destination: "/safe"}
	m := NewModel(deps)
	m.state = StateContextMenu
	m.contextMenu = NewContextMenuModel(deps.Theme, installation.Detection{State: installation.StateLegacyPM2})

	updated, cmd := m.Update(ContextActionSelectedMsg{Action: ContextActionMigration})
	m = updated.(Model)
	if m.state != StateBackupPreflight || cmd == nil || action.runCalls != 0 {
		t.Fatalf("state/cmd/run = %v/%v/%d", m.state, cmd, action.runCalls)
	}
	updated, cmd = m.Update(cmd())
	m = updated.(Model)
	if m.state != StateBackupConfirm || action.runCalls != 0 {
		t.Fatalf("preflight must only review; state/run = %v/%d", m.state, action.runCalls)
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != StateBackupRunning || cmd == nil || action.runCalls != 0 {
		t.Fatalf("confirmation state/cmd/run = %v/%v/%d", m.state, cmd, action.runCalls)
	}
	updated, cmd = m.Update(cmd())
	m = updated.(Model)
	if m.state != StateMigrationConfirm || cmd != nil {
		t.Fatalf("validated backup must wait before PM2 quiescence: state/cmd = %v/%v", m.state, cmd)
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != StateMigrationQuiescence || cmd == nil {
		t.Fatalf("confirmed migration must acquire a PM2 lease before preflight: state/cmd = %v/%v", m.state, cmd)
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.state != StatePreflight || !m.migrationPending || action.runCalls != 1 {
		t.Fatalf("validated backup must hand off to unchanged install path: state/pending/run = %v/%t/%d", m.state, m.migrationPending, action.runCalls)
	}
}

func TestMigrationFailuresAndCancellationStayBlocked(t *testing.T) {
	for _, result := range []migration.BackupResult{{Outcome: migration.BackupCancelled}, {Outcome: migration.BackupDumpFailed}, {Outcome: migration.BackupValidationFailed}} {
		t.Run(result.Message, func(t *testing.T) {
			deps := buildTestDeps()
			action := &fakeLegacyBackupAction{result: result}
			deps.LegacyBackupAction = action
			m := NewModel(deps)
			m.state = StateBackupRunning
			m.backupCancel = func() {}
			updated, _ := m.Update(BackupCompletedMsg{Result: result})
			if updated.(Model).state != StateBackupResult {
				t.Fatalf("failure state = %v", updated.(Model).state)
			}
		})
	}
}

func TestMigrationRunningPreventsDuplicateSubmitAndCancels(t *testing.T) {
	m := NewModel(buildTestDeps())
	m.state = StateBackupRunning
	cancelled := 0
	m.backupCancel = func() { cancelled++ }
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.(Model).state != StateBackupRunning || cmd != nil {
		t.Fatal("running backup must ignore duplicate confirmation")
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cancelled != 1 || cmd != nil {
		t.Fatalf("cancellation = %d, cmd=%v", cancelled, cmd)
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cancelled != 2 || cmd != nil {
		t.Fatalf("q cancellation = %d, cmd=%v", cancelled, cmd)
	}
}

func TestMigrationConfirmationRendersRedactedImmutableReview(t *testing.T) {
	m := NewModel(buildTestDeps())
	m.state = StateBackupConfirm
	view := m.View()
	for _, label := range []string{
		"Selected environment", "Endpoint", "Database", "User",
		"Container ID", "Image", "Destination",
	} {
		if !strings.Contains(view, label) {
			t.Fatalf("confirmation review missing %q: %q", label, view)
		}
	}
	for _, forbidden := range []string{"synthetic-secret", "PGPASS", "password", "config.js"} {
		if strings.Contains(strings.ToLower(view), strings.ToLower(forbidden)) {
			t.Fatalf("confirmation review leaked %q: %q", forbidden, view)
		}
	}
}

func TestMigrationConfirmationEscapeDoesNotRun(t *testing.T) {
	m := NewModel(buildTestDeps())
	m.state = StateBackupConfirm
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.(Model).state != StateBackupConfirm || cmd != nil {
		t.Fatalf("escape changed confirmation state or started run: state=%v cmd=%v", updated.(Model).state, cmd)
	}
}

func TestMigrationConfirmationHonorsSmallTerminalGuard(t *testing.T) {
	m := NewModel(buildTestDeps())
	m.state = StateBackupConfirm
	m.width, m.height = 79, 24
	if got := m.View(); !strings.Contains(got, "Terminal too small") {
		t.Fatalf("small terminal view = %q", got)
	}
}

func TestMigrationIgnoresStalePreflightCompletion(t *testing.T) {
	m := NewModel(buildTestDeps())
	m.state = StateContextMenu
	updated, cmd := m.Update(BackupPreflightCompletedMsg{Err: context.Canceled})
	if updated.(Model).state != StateContextMenu || cmd != nil {
		t.Fatalf("stale preflight completion changed state or scheduled work: state=%v cmd=%v", updated.(Model).state, cmd)
	}
}

func TestMigrationFailClosedTransitionsDoNotScheduleUnsafeWork(t *testing.T) {
	t.Run("missing action remains informationally blocked", func(t *testing.T) {
		m := NewModel(buildTestDeps())
		m.state = StateContextMenu
		m.contextMenu = NewContextMenuModel(m.deps.Theme, installation.Detection{State: installation.StateLegacyPM2})
		updated, cmd := m.Update(ContextActionSelectedMsg{Action: ContextActionMigration})
		if updated.(Model).state != StateBlockedOperation || cmd != nil {
			t.Fatalf("missing action state/cmd = %v/%v", updated.(Model).state, cmd)
		}
	})
	t.Run("preflight failure cannot reach confirmation", func(t *testing.T) {
		m := NewModel(buildTestDeps())
		m.state = StateBackupPreflight
		updated, cmd := m.Update(BackupPreflightCompletedMsg{Err: context.DeadlineExceeded})
		m = updated.(Model)
		if m.state != StateBackupResult || cmd != nil || strings.Contains(m.View(), "DeadlineExceeded") {
			t.Fatalf("preflight failure state/view/cmd = %v/%q/%v", m.state, m.View(), cmd)
		}
	})
	t.Run("stale completion cannot unlock later migration", func(t *testing.T) {
		m := NewModel(buildTestDeps())
		m.state = StateBackupConfirm
		updated, cmd := m.Update(BackupCompletedMsg{Result: migration.BackupResult{Outcome: migration.BackupValidated}})
		if updated.(Model).state != StateBackupConfirm || cmd != nil {
			t.Fatalf("stale completion state/cmd = %v/%v", updated.(Model).state, cmd)
		}
	})
	t.Run("ctrl c requests cancellation while the backup remains running", func(t *testing.T) {
		m := NewModel(buildTestDeps())
		m.state = StateBackupRunning
		cancelled := 0
		m.backupCancel = func() { cancelled++ }
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		if updated.(Model).state != StateBackupRunning || cancelled != 1 || cmd != nil {
			t.Fatalf("ctrl-c state/cancel/cmd = %v/%d/%v", updated.(Model).state, cancelled, cmd)
		}
	})
}

func TestMigrationPreconfirmationQuitAndNilDependencyNeverRun(t *testing.T) {
	t.Run("quit before confirmation never runs", func(t *testing.T) {
		deps := buildTestDeps()
		action := &fakeLegacyBackupAction{}
		deps.LegacyBackupAction = action
		m := NewModel(deps)
		m.state = StateBackupConfirm
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
		if updated.(Model).state != StateBackupConfirm || cmd == nil || action.runCalls != 0 {
			t.Fatalf("quit state/cmd/run = %v/%v/%d", updated.(Model).state, cmd, action.runCalls)
		}
	})
	t.Run("nil action cannot confirm", func(t *testing.T) {
		m := NewModel(buildTestDeps())
		m.state = StateBackupConfirm
		updated, cmd := m.Update(BackupConfirmedMsg{})
		if updated.(Model).state != StateBackupConfirm || cmd != nil {
			t.Fatalf("nil action state/cmd = %v/%v", updated.(Model).state, cmd)
		}
	})
}

func TestMigrationCancelledCompletionIsTerminalAndRedacted(t *testing.T) {
	m := NewModel(buildTestDeps())
	m.state = StateBackupRunning
	m.backupCancel = func() {}
	updated, cmd := m.Update(BackupCompletedMsg{Result: migration.BackupResult{
		Outcome:  migration.BackupCancelled,
		DumpPath: "/unsafe/partial.dump",
		Message:  "synthetic-secret-cancelled",
	}})
	m = updated.(Model)
	if m.state != StateBackupResult || cmd != nil || strings.Contains(m.View(), "synthetic-secret-cancelled") || strings.Contains(m.View(), "/unsafe/partial.dump") {
		t.Fatalf("cancelled completion state/view/cmd = %v/%q/%v", m.state, m.View(), cmd)
	}
}

func TestModelInitStartsSplashWithoutMigrationWork(t *testing.T) {
	m := NewModel(buildTestDeps())
	if cmd := m.Init(); cmd != nil {
		t.Fatal("splash initialization must wait for input without migration work")
	}
}
