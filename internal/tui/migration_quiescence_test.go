package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/installation"
	"github.com/jcaltamar/alice-installer/internal/migration"
)

type fakeMigrationHandoff struct {
	beginCalls    int
	failureCalls  int
	successCalls  int
	beginErr      error
	recovery      installation.PM2Recovery
	recoveryErr   error
	lastBackupRef migration.BackupRef
}

type failingRootObservationRunner struct{}

func (failingRootObservationRunner) Run(context.Context, string, ...string) ([]byte, []byte, error) {
	return []byte(`[{"pm2_env":{"DATABASE_URL":"postgres://secret"}}]`), []byte("sudo: a password is required\n"), errors.New("exit status 1")
}

type unusedSocketSnapshot struct{}

func (unusedSocketSnapshot) Snapshot(context.Context) ([]installation.SocketOwner, error) {
	return nil, nil
}

type unusedProcIdentity struct{}

func (unusedProcIdentity) Read(context.Context, int) (installation.ProcIdentity, error) {
	return installation.ProcIdentity{}, nil
}

func (h *fakeMigrationHandoff) Begin(_ context.Context, ref migration.BackupRef, _ string, _ migration.ContainerDisposition) (*migration.PreInstallMigrationLease, error) {
	h.beginCalls++
	h.lastBackupRef = ref
	if h.beginErr != nil {
		return nil, h.beginErr
	}
	return &migration.PreInstallMigrationLease{}, nil
}

func (h *fakeMigrationHandoff) CompleteSuccess(*migration.PreInstallMigrationLease) error {
	h.successCalls++
	return nil
}

func (h *fakeMigrationHandoff) CompleteFailure(*migration.PreInstallMigrationLease) (installation.PM2Recovery, error) {
	h.failureCalls++
	return h.recovery, h.recoveryErr
}

func TestMigrationQuiescenceAcquiresLeaseBeforePreflight(t *testing.T) {
	deps := buildTestDeps()
	handoff := &fakeMigrationHandoff{}
	deps.LegacyRestoreAction = &fakeLegacyRestoreAction{result: migration.RestoreResult{Outcome: migration.RestoreSucceeded}}
	deps.MigrationHandoff = handoff
	m := NewModel(deps)
	m.state = StateBackupRunning

	backup := migration.BackupResult{Outcome: migration.BackupValidated, DumpPath: "/opt/alice/backups/legacy.dump", ManifestPath: "/opt/alice/backups/legacy.json", SHA256: "sum", Size: 1}
	updated, cmd := m.Update(BackupCompletedMsg{Result: backup})
	m = updated.(Model)
	if m.state != StateMigrationConfirm || cmd != nil || handoff.beginCalls != 0 {
		t.Fatalf("validated backup must wait for operator confirmation: state/cmd/calls = %v/%v/%d", m.state, cmd, handoff.beginCalls)
	}
	view := m.View()
	for _, text := range []string{"stopping confirmed legacy PM2 services", "installing new services", "Press Enter to continue", "preserve the backup"} {
		if !strings.Contains(view, text) {
			t.Fatalf("migration checkpoint view missing %q: %s", text, view)
		}
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != StateMigrationDisposition || cmd != nil || handoff.beginCalls != 0 {
		t.Fatalf("disposition choice state/cmd/calls = %v/%v/%d", m.state, cmd, handoff.beginCalls)
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	updated, cmd = m.Update(cmd())
	m = updated.(Model)
	if m.state != StateMigrationQuiescence || cmd == nil || handoff.beginCalls != 0 {
		t.Fatalf("acquisition state/cmd/calls = %v/%v/%d", m.state, cmd, handoff.beginCalls)
	}
	updated, cmd = m.Update(cmd())
	m = updated.(Model)
	if m.state != StatePreflight || !m.migrationPending || m.migrationLease == nil || handoff.beginCalls != 1 || cmd == nil {
		t.Fatalf("lease handoff state/pending/lease/calls/cmd = %v/%t/%t/%d/%v", m.state, m.migrationPending, m.migrationLease != nil, handoff.beginCalls, cmd)
	}
	if got := handoff.lastBackupRef.DumpPath; got != backup.DumpPath {
		t.Fatalf("handoff backup = %q, want %q", got, backup.DumpPath)
	}
}

func TestMigrationCheckpointCancellationPreservesBackupWithoutQuiescence(t *testing.T) {
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEsc}, {Type: tea.KeyRunes, Runes: []rune{'q'}}} {
		deps := buildTestDeps()
		handoff := &fakeMigrationHandoff{}
		deps.LegacyRestoreAction = &fakeLegacyRestoreAction{result: migration.RestoreResult{Outcome: migration.RestoreSucceeded}}
		deps.MigrationHandoff = handoff
		m := NewModel(deps)
		m.state = StateBackupRunning
		backup := migration.BackupResult{Outcome: migration.BackupValidated, DumpPath: "/opt/alice/backups/legacy.dump", ManifestPath: "/opt/alice/backups/legacy.json", SHA256: "sum", Size: 1}

		updated, _ := m.Update(BackupCompletedMsg{Result: backup})
		m = updated.(Model)
		updated, cmd := m.Update(key)
		m = updated.(Model)

		if m.state != StateMigrationCancelled || cmd != nil || handoff.beginCalls != 0 || m.migrationPending || m.migrationLease != nil {
			t.Fatalf("cancelled checkpoint state/cmd/calls/pending/lease = %v/%v/%d/%t/%t", m.state, cmd, handoff.beginCalls, m.migrationPending, m.migrationLease != nil)
		}
		if m.backupResult.DumpPath != backup.DumpPath || m.backupResult.ManifestPath != backup.ManifestPath {
			t.Fatalf("validated backup was not preserved: %#v", m.backupResult)
		}
	}
}

func TestMigrationQuiescenceFailureBlocksBeforePreflight(t *testing.T) {
	deps := buildTestDeps()
	deps.LegacyRestoreAction = &fakeLegacyRestoreAction{}
	deps.MigrationHandoff = &fakeMigrationHandoff{beginErr: errors.New("unavailable")}
	m := NewModel(deps)
	m.state = StateBackupRunning

	updated, cmd := m.Update(BackupCompletedMsg{Result: migration.BackupResult{Outcome: migration.BackupValidated}})
	m = updated.(Model)
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	updated, cmd = m.Update(cmd())
	m = updated.(Model)
	updated, cmd = m.Update(cmd())
	m = updated.(Model)
	if m.state != StateMigrationResult || m.migrationPending || m.migrationLease != nil || cmd != nil {
		t.Fatalf("failed acquisition state/pending/lease/cmd = %v/%t/%t/%v", m.state, m.migrationPending, m.migrationLease != nil, cmd)
	}
}

func TestPM2ObservationFailureDiagnosticReachesMigrationTerminal(t *testing.T) {
	root := installation.RootPM2Boundary{Runner: failingRootObservationRunner{}}
	quiescer := installation.PM2Quiescer{
		Snapshots: installation.LinuxPM2SnapshotProvider{
			Inventory: installation.LinuxPM2Inventory{Runner: root},
			Sockets:   unusedSocketSnapshot{},
			Proc:      unusedProcIdentity{},
		},
		Controller: installation.PM2Controller{Runner: root},
	}
	_, err := quiescer.Quiesce(context.Background())
	if err == nil {
		t.Fatal("Quiesce() succeeded, want observation failure")
	}

	deps := buildTestDeps()
	deps.Debug = true
	m := NewModel(deps)
	m.state = StateMigrationQuiescence
	updated, _ := m.Update(MigrationQuiescenceCompletedMsg{Err: err})
	view := updated.(Model).View()
	for _, want := range []string{
		"pm2-observation-unavailable",
		"stage=initial-snapshot",
		"operation=pm2-jlist",
		"command=sudo -n pm2 jlist",
		"cause=exit-1",
		"stderr=sudo authentication required",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("terminal view %q does not contain %q", view, want)
		}
	}
	for _, forbidden := range []string{"DATABASE_URL", "postgres://secret", "pm2_env"} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("terminal view leaked %q: %q", forbidden, view)
		}
	}
}

func TestPM2StopProofTerminalDiagnosticsRespectDebugMode(t *testing.T) {
	const secret = "DATABASE_URL=postgres://secret"
	for _, tt := range []struct {
		name       string
		debug      bool
		diagnostic *installation.PM2ObservationDiagnostic
		want       []string
	}{
		{
			name:       "debug command failure",
			debug:      true,
			diagnostic: &installation.PM2ObservationDiagnostic{Stage: "stop-proof-snapshot", Operation: "pm2-jlist", Command: "sudo -n pm2 jlist", Cause: "exit-1", Stderr: secret},
			want:       []string{"pm2-stop-unproven", "command=sudo -n pm2 jlist", "cause=exit-1"},
		},
		{
			name:       "debug timeout",
			debug:      true,
			diagnostic: &installation.PM2ObservationDiagnostic{StopProofTimedOut: true, PMID: 7, Port: 9090},
			want:       []string{"pm2-stop-unproven", "stop command succeeded", "PM2 ID 7", "port release on 9090"},
		},
		{
			name:       "normal mode remains terse",
			diagnostic: &installation.PM2ObservationDiagnostic{Stage: "stop-proof-snapshot", Operation: "pm2-jlist", Command: "sudo -n pm2 jlist", Cause: "exit-1", Stderr: secret},
			want:       []string{"pm2-stop-unproven"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			deps := buildTestDeps()
			deps.Debug = tt.debug
			m := NewModel(deps)
			m.state = StateMigrationQuiescence
			updated, _ := m.Update(MigrationQuiescenceCompletedMsg{Err: installation.QuiescenceError{Code: "pm2-stop-unproven", Diagnostic: tt.diagnostic}})
			view := updated.(Model).View()
			for _, want := range tt.want {
				if !strings.Contains(view, want) {
					t.Fatalf("view %q does not contain %q", view, want)
				}
			}
			for _, forbidden := range []string{secret, "postgres://secret", "DATABASE_URL"} {
				if strings.Contains(view, forbidden) {
					t.Fatalf("view leaked %q: %q", forbidden, view)
				}
			}
			if !tt.debug && strings.Contains(view, "command=") {
				t.Fatalf("normal mode exposed command diagnostic: %q", view)
			}
		})
	}
}

func TestBoundedQuiescenceFailureCodes(t *testing.T) {
	for _, tt := range []struct {
		err  error
		want string
	}{
		{installation.QuiescenceError{Code: "pm2-observation-unavailable"}, "pm2-observation-unavailable"},
		{installation.QuiescenceError{Code: "pm2-correlation-failed"}, "pm2-correlation-failed"},
		{installation.QuiescenceError{Code: "pm2-state-changed"}, "pm2-state-changed"},
		{installation.QuiescenceError{Code: "pm2-stop-failed"}, "pm2-stop-failed"},
		{installation.QuiescenceError{Code: "pm2-stop-unproven"}, "pm2-stop-unproven"},
		{installation.QuiescenceError{Code: "pm2-final-state-unproven"}, "pm2-final-state-unproven"},
		{installation.QuiescenceError{Code: "raw-secret"}, "pm2-quiescence-unavailable"},
		{errors.New("raw-secret"), "pm2-quiescence-unavailable"},
	} {
		if got := boundedQuiescenceCode(tt.err); got != tt.want {
			t.Fatalf("code = %q, want %q", got, tt.want)
		}
	}
}

func TestMigrationLiveLeaseTerminalPathsRecoverExceptInstallSuccess(t *testing.T) {
	for _, msg := range []tea.Msg{
		InstallFailureMsg{Stage: "deploy", Err: errors.New("failure")},
		InstallFailureMsg{Stage: "verify", Err: errors.New("verification failure")},
		RestoreCompletedMsg{Result: migration.RestoreResult{Outcome: migration.RestoreFailedBeforeCutover, FailedStage: migration.StageCredentials}},
		RestoreCompletedMsg{Result: migration.RestoreResult{Outcome: migration.RestoreUnsupported, FailedStage: migration.StagePlatformGate}},
		RestoreCompletedMsg{Result: migration.RestoreResult{Outcome: migration.RestoreCancelledBeforeCutover, FailedStage: migration.StageWait}},
		RestoreCompletedMsg{Result: migration.RestoreResult{Outcome: migration.RestorePartialCutover, FailedStage: migration.StageRollback}},
		tea.KeyMsg{Type: tea.KeyEsc},
		tea.KeyMsg{Type: tea.KeyCtrlC},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}},
		MigrationAbandonedMsg{},
	} {
		t.Run("terminal", func(t *testing.T) {
			deps := buildTestDeps()
			handoff := &fakeMigrationHandoff{recovery: installation.PM2Recovery{Verified: true, Code: "pm2-recovery-verified"}}
			deps.MigrationHandoff = handoff
			m := NewModel(deps)
			m.state, m.migrationPending, m.migrationLease = StateDeploy, true, &migration.PreInstallMigrationLease{}
			if _, ok := msg.(RestoreCompletedMsg); ok {
				m.state = StateDatabaseRestore
			}
			updated, cmd := m.Update(msg)
			m = updated.(Model)
			if m.state != StateMigrationRecovery || cmd == nil || handoff.failureCalls != 0 {
				t.Fatalf("recovery state/cmd/calls = %v/%v/%d", m.state, cmd, handoff.failureCalls)
			}
			updated, cmd = m.Update(cmd())
			m = updated.(Model)
			if m.state != StateMigrationResult || cmd != nil || handoff.failureCalls != 1 || m.migrationLease != nil {
				t.Fatalf("terminal state/cmd/calls/lease = %v/%v/%d/%t", m.state, cmd, handoff.failureCalls, m.migrationLease != nil)
			}
		})
	}

	deps := buildTestDeps()
	handoff := &fakeMigrationHandoff{}
	deps.MigrationHandoff = handoff
	m := NewModel(deps)
	m.state, m.migrationPending, m.migrationLease = StateVerify, true, &migration.PreInstallMigrationLease{}
	updated, cmd := m.Update(InstallSuccessMsg{})
	m = updated.(Model)
	if m.state != StateMigrationSuccess || cmd == nil || handoff.failureCalls != 0 {
		t.Fatalf("success state/cmd/recovery = %v/%v/%d", m.state, cmd, handoff.failureCalls)
	}
	updated, _ = m.Update(cmd())
	if updated.(Model).state != StateResult || handoff.successCalls != 1 || handoff.failureCalls != 0 {
		t.Fatalf("success completion state/success/recovery = %v/%d/%d", updated.(Model).state, handoff.successCalls, handoff.failureCalls)
	}
}

func TestMigrationRecoveryPanicBecomesBoundedTerminalResult(t *testing.T) {
	deps := buildTestDeps()
	deps.MigrationHandoff = panicFailureHandoff{}
	m := NewModel(deps)
	m.state, m.migrationPending, m.migrationLease = StateDeploy, true, &migration.PreInstallMigrationLease{}

	updated, cmd := m.Update(MigrationAbandonedMsg{})
	m = updated.(Model)
	if m.state != StateMigrationRecovery || cmd == nil {
		t.Fatalf("panic recovery start = state:%v cmd:%v", m.state, cmd)
	}
	updated, cmd = m.Update(cmd())
	m = updated.(Model)
	if m.state != StateMigrationResult || cmd != nil || m.migrationRecoveryCode != "pm2-recovery-unproven" {
		t.Fatalf("panic recovery result = state:%v cmd:%v code:%q", m.state, cmd, m.migrationRecoveryCode)
	}
}

type panicFailureHandoff struct{}

func (panicFailureHandoff) Begin(context.Context, migration.BackupRef, string, migration.ContainerDisposition) (*migration.PreInstallMigrationLease, error) {
	return nil, errors.New("unused")
}

func (panicFailureHandoff) CompleteSuccess(*migration.PreInstallMigrationLease) error {
	return errors.New("unused")
}

func (panicFailureHandoff) CompleteFailure(*migration.PreInstallMigrationLease) (installation.PM2Recovery, error) {
	panic("synthetic recovery panic")
}

func TestMigrationRestoreCancellationWaitsForDatabaseResultBeforePM2Recovery(t *testing.T) {
	deps := buildTestDeps()
	handoff := &fakeMigrationHandoff{recovery: installation.PM2Recovery{Verified: true, Code: "pm2-recovery-verified"}}
	action := &fakeLegacyRestoreAction{cancelled: true}
	deps.MigrationHandoff, deps.LegacyRestoreAction = handoff, action
	m := NewModel(deps)
	m.state, m.migrationPending, m.migrationLease = StateDeploy, true, &migration.PreInstallMigrationLease{}

	updated, restoreCmd := m.Update(DeployCompleteMsg{})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.state != StateDatabaseRestore || cmd != nil || handoff.failureCalls != 0 {
		t.Fatalf("cancellation must wait for restore completion: state/cmd/recovery = %v/%v/%d", m.state, cmd, handoff.failureCalls)
	}
	updated, recoveryCmd := m.Update(restoreCmd())
	m = updated.(Model)
	if m.state != StateMigrationRecovery || recoveryCmd == nil || handoff.failureCalls != 0 {
		t.Fatalf("database result must precede PM2 recovery: state/cmd/recovery = %v/%v/%d", m.state, recoveryCmd, handoff.failureCalls)
	}
	_, _ = m.Update(recoveryCmd())
}
