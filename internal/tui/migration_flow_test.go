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

type fakeLegacyBackupAction struct {
	preflightCalls int
	runCalls       int
	preflightErr   error
	result         migration.BackupResult
	requests       []migration.BackupRequest
}

type fakeMigrationAuthenticator struct{ err error }

func (a fakeMigrationAuthenticator) Authenticate() tea.Cmd {
	return func() tea.Msg { return MigrationAuthenticationCompletedMsg{Err: a.err} }
}

func (a *fakeLegacyBackupAction) Preflight(_ context.Context, request migration.BackupRequest) (migration.BackupPlan, error) {
	a.preflightCalls++
	a.requests = append(a.requests, request)
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
	deps.MigrationAuthenticator = fakeMigrationAuthenticator{}
	deps.LegacyBackupRequest = migration.BackupRequest{Destination: "/safe"}
	m := NewModel(deps)
	m.state = StateContextMenu
	m.contextMenu = NewContextMenuModel(deps.Theme, installation.Detection{State: installation.StateLegacyPM2})

	updated, cmd := m.Update(ContextActionSelectedMsg{Action: ContextActionMigration})
	m = updated.(Model)
	if m.state != StateMigrationEnv || cmd != nil || action.preflightCalls != 0 || action.runCalls != 0 {
		t.Fatalf("state/cmd/run = %v/%v/%d", m.state, cmd, action.runCalls)
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("environment choice must emit a typed selection")
	}
	updated, cmd = m.Update(cmd())
	m = updated.(Model)
	if m.state != StateMigrationAuth || cmd == nil || m.migrationEnv != migration.EnvironmentDevelopment {
		t.Fatalf("environment selection state/cmd/environment = %v/%v/%q", m.state, cmd, m.migrationEnv)
	}
	updated, cmd = m.Update(cmd())
	m = updated.(Model)
	if m.state != StateBackupPreflight || cmd == nil || action.preflightCalls != 0 {
		t.Fatalf("successful authentication must schedule preflight: state/cmd/preflight = %v/%v/%d", m.state, cmd, action.preflightCalls)
	}
	updated, cmd = m.Update(cmd())
	m = updated.(Model)
	if m.state != StateBackupConfirm || action.runCalls != 0 || len(action.requests) != 1 || action.requests[0].ConfigEnvironment != "development" {
		t.Fatalf("preflight must only review; state/run = %v/%d", m.state, action.runCalls)
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != StateBackupRunning || cmd == nil || action.runCalls != 0 {
		t.Fatalf("confirmation state/cmd/run = %v/%v/%d", m.state, cmd, action.runCalls)
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) == 0 {
		t.Fatalf("backup start command = %#v, want batch", cmd())
	}
	updated, cmd = m.Update(batch[0]())
	m = updated.(Model)
	if m.state != StateMigrationConfirm || cmd != nil {
		t.Fatalf("validated backup must wait before PM2 quiescence: state/cmd = %v/%v", m.state, cmd)
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != StateMigrationDisposition || cmd != nil {
		t.Fatalf("final confirmation must precede disposition: state/cmd = %v/%v", m.state, cmd)
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("disposition must emit a typed selection")
	}
	updated, cmd = m.Update(cmd())
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

func TestMigrationBackupProgressAnimatesAndShowsRealStages(t *testing.T) {
	m := NewModel(buildTestDeps())
	m.state = StateBackupRunning
	m.backupStage = migration.BackupProgressDumping
	before := m.View()
	if !strings.Contains(before, "Creating database dump") || !strings.Contains(before, "0s elapsed") {
		t.Fatalf("initial progress view = %q", before)
	}

	tick := m.backupSpinner.Tick()
	updated, cmd := m.Update(tick)
	m = updated.(Model)
	if cmd == nil || m.View() == before {
		t.Fatalf("spinner did not animate: before=%q after=%q cmd=%v", before, m.View(), cmd)
	}
	updated, cmd = m.Update(backupElapsedMsg{})
	m = updated.(Model)
	if cmd == nil || !strings.Contains(m.View(), "1s elapsed") {
		t.Fatalf("elapsed progress view/cmd = %q/%v", m.View(), cmd)
	}
	updated, cmd = m.Update(BackupProgressMsg{Stage: migration.BackupProgressValidating})
	m = updated.(Model)
	if cmd == nil || !strings.Contains(m.View(), "Validating archive") {
		t.Fatalf("stage progress view/cmd = %q/%v", m.View(), cmd)
	}

	updated, cmd = m.Update(BackupCompletedMsg{Result: migration.BackupResult{Outcome: migration.BackupValidationFailed}})
	m = updated.(Model)
	if m.state != StateBackupResult || cmd != nil {
		t.Fatalf("completion state/cmd = %v/%v", m.state, cmd)
	}
	updated, cmd = m.Update(tick)
	if updated.(Model).state != StateBackupResult || cmd != nil {
		t.Fatalf("spinner continued after completion: state/cmd = %v/%v", updated.(Model).state, cmd)
	}
}

func TestMigrationFailuresAndCancellationStayBlocked(t *testing.T) {
	for name, result := range map[string]migration.BackupResult{"cancelled": {Outcome: migration.BackupCancelled}, "dump failed": {Outcome: migration.BackupDumpFailed}, "validation failed": {Outcome: migration.BackupValidationFailed}} {
		t.Run(name, func(t *testing.T) {
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

func TestMigrationPreflightRendersSelectorSpecificSafeCodes(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
		code string
	}{
		{"no trusted image", migration.ErrNoExactImageCandidate, "backup-legacy-container-image-untrusted"},
		{"identity mismatch", migration.ErrContainerIdentity, "backup-legacy-container-identity-mismatch"},
		{"endpoint mismatch", migration.ErrContainerEndpoint, "backup-legacy-container-endpoint-mismatch"},
		{"unsafe state", migration.ErrContainerUnsafeState, "backup-legacy-container-unsafe"},
		{"stopped legacy master", migration.ErrContainerStopped, "backup-legacy-container-stopped"},
		{"ambiguity", migration.ErrAmbiguousContainer, "backup-legacy-container-ambiguous"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(buildTestDeps())
			m.state = StateBackupPreflight
			updated, _ := m.Update(BackupPreflightCompletedMsg{Err: tt.err})
			m = updated.(Model)
			if m.state != StateBackupResult || m.backupResult.FailureCode.String() != tt.code {
				t.Fatalf("state/code = %v/%q", m.state, m.backupResult.FailureCode.String())
			}
			view := m.View()
			if !strings.Contains(view, tt.code) {
				t.Fatalf("view missing safe code %q: %q", tt.code, view)
			}
			for _, forbidden := range []string{"postgres_postgresql-master_1", strings.Repeat("a", 64), "docker.io/bitnami"} {
				if strings.Contains(view, forbidden) {
					t.Fatalf("view leaked %q: %q", forbidden, view)
				}
			}
		})
	}
}

func TestMigrationBackupResultRendersBoundedStageDiagnostics(t *testing.T) {
	stages := []migration.BackupStageResult{
		{Stage: migration.BackupStagePreconditions, Status: migration.BackupStagePassed},
		{Stage: migration.BackupStageDestination, Status: migration.BackupStagePassed},
		{Stage: migration.BackupStageCredentials, Status: migration.BackupStagePassed},
		{Stage: migration.BackupStageDump, Status: migration.BackupStagePassed},
		{Stage: migration.BackupStageStagedFile, Status: migration.BackupStagePassed},
		{Stage: migration.BackupStageArchiveValidation, Status: migration.BackupStagePassed},
		{Stage: migration.BackupStagePublication, Status: migration.BackupStagePassed},
	}
	for _, tt := range []struct {
		name      string
		state     State
		result    migration.BackupResult
		want      []string
		forbidden []string
	}{
		{
			name:   "validated backup shows all stages passed",
			state:  StateMigrationConfirm,
			result: migration.BackupResult{Outcome: migration.BackupValidated, Stages: stages, DumpPath: "/safe/backup.dump", ManifestPath: "/safe/backup.json", SHA256: "abc", Size: 42},
			want:   []string{"Backup validated", "Destination preparation:", "Archive validation:", "Publication, checksum, and manifest:", "passed"},
		},
		{
			name:      "archive failure shows bounded failure details",
			state:     StateBackupResult,
			result:    migration.BackupResult{Outcome: migration.BackupValidationFailed, Stages: append([]migration.BackupStageResult(nil), stages...), FailureCode: migration.BackupFailureArchiveValidation, Remediation: migration.BackupRemediationArchive},
			want:      []string{"Backup did not validate", "Archive validation:", "failed", "Publication, checksum, and manifest:", "not run", "backup-archive-validation", "do not continue migration"},
			forbidden: []string{"synthetic-secret", "raw stderr", "password="},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.state == StateBackupResult {
				tt.result.Stages[migration.BackupStageArchiveValidation].Status = migration.BackupStageFailed
				tt.result.Stages[migration.BackupStagePublication].Status = migration.BackupStageNotRun
			}
			m := NewModel(buildTestDeps())
			m.state, m.backupResult = tt.state, tt.result
			view := m.View()
			for _, want := range tt.want {
				if !strings.Contains(view, want) {
					t.Fatalf("view missing %q: %q", want, view)
				}
			}
			for _, forbidden := range tt.forbidden {
				if strings.Contains(view, forbidden) {
					t.Fatalf("view exposed %q: %q", forbidden, view)
				}
			}
		})
	}
}

func TestMigrationAuthenticationFailureBlocksBeforePreflight(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
	}{{name: "failure", err: errors.New("sudo failed")}, {name: "cancel", err: context.Canceled}} {
		t.Run(tt.name, func(t *testing.T) {
			deps := buildTestDeps()
			action := &fakeLegacyBackupAction{}
			deps.LegacyBackupAction = action
			deps.LegacyRestoreAction = &fakeLegacyRestoreAction{}
			deps.MigrationHandoff = &fakeMigrationHandoff{}
			deps.MigrationAuthenticator = fakeMigrationAuthenticator{err: tt.err}
			m := NewModel(deps)
			m.state = StateContextMenu
			m.contextMenu = NewContextMenuModel(deps.Theme, installation.Detection{State: installation.StateLegacyPM2})

			updated, cmd := m.Update(ContextActionSelectedMsg{Action: ContextActionMigration})
			m = updated.(Model)
			updated, cmd = m.Update(MigrationEnvironmentSelectedMsg{Environment: migration.EnvironmentProduction})
			m = updated.(Model)
			updated, cmd = m.Update(cmd())
			m = updated.(Model)
			if m.state != StateMigrationAuthFailed || cmd != nil || action.preflightCalls != 0 || action.runCalls != 0 {
				t.Fatalf("auth failure state/cmd/preflight/run = %v/%v/%d/%d", m.state, cmd, action.preflightCalls, action.runCalls)
			}
			if view := m.View(); strings.Contains(view, tt.err.Error()) || !strings.Contains(view, "No backup preflight or filesystem mutation") {
				t.Fatalf("unsafe authentication failure view = %q", view)
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
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cancelled != 1 || cmd != nil {
		t.Fatalf("cancellation = %d, cmd=%v", cancelled, cmd)
	}
	m = updated.(Model)
	updated, cmd = m.Update(m.backupSpinner.Tick())
	if !updated.(Model).backupCancelling || cmd != nil {
		t.Fatalf("spinner continued after cancellation: cancelling/cmd = %t/%v", updated.(Model).backupCancelling, cmd)
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

func TestMigrationDispositionDefaultsToStopAndRemoveRequiresSecondConfirmation(t *testing.T) {
	t.Run("stop is default", func(t *testing.T) {
		deps := buildTestDeps()
		handoff := &fakeMigrationHandoff{}
		deps.MigrationHandoff = handoff
		m := NewModel(deps)
		m.state = StateMigrationDisposition
		m.migrationDisposition = migrationDispositionModel{theme: deps.Theme}
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = updated.(Model)
		updated, cmd = m.Update(cmd())
		m = updated.(Model)
		if m.state != StateMigrationQuiescence || m.containerDisposition != migration.DispositionStop || cmd == nil || handoff.beginCalls != 0 {
			t.Fatalf("default disposition state/choice/cmd/calls = %v/%v/%v/%d", m.state, m.containerDisposition, cmd, handoff.beginCalls)
		}
	})

	t.Run("remove requires explicit second confirmation", func(t *testing.T) {
		deps := buildTestDeps()
		handoff := &fakeMigrationHandoff{}
		deps.MigrationHandoff = handoff
		m := NewModel(deps)
		m.state = StateMigrationDisposition
		m.migrationDisposition = migrationDispositionModel{theme: deps.Theme}
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(Model)
		updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = updated.(Model)
		updated, cmd = m.Update(cmd())
		m = updated.(Model)
		if m.state != StateMigrationRemoveConfirm || cmd != nil || handoff.beginCalls != 0 || !strings.Contains(m.View(), "cannot automatically recreate") {
			t.Fatalf("remove checkpoint state/cmd/calls/view = %v/%v/%d/%q", m.state, cmd, handoff.beginCalls, m.View())
		}
		updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = updated.(Model)
		if m.state != StateMigrationQuiescence || cmd == nil || handoff.beginCalls != 0 {
			t.Fatalf("confirmed removal state/cmd/calls = %v/%v/%d", m.state, cmd, handoff.beginCalls)
		}
	})
}

func TestMigrationSudoDockerPermissionFailureIsRedacted(t *testing.T) {
	m := NewModel(buildTestDeps())
	m.state = StateBackupPreflight
	updated, _ := m.Update(BackupPreflightCompletedMsg{Err: migration.ErrSudoDockerPermission})
	m = updated.(Model)
	view := m.View()
	if m.backupResult.FailureCode != migration.BackupFailureSudoDockerPermission || !strings.Contains(view, "backup-sudo-docker-permission") {
		t.Fatalf("permission state/code/view = %v/%v/%q", m.state, m.backupResult.FailureCode, view)
	}
	for _, forbidden := range []string{"sudo:", "permission denied", strings.Repeat("a", 64)} {
		if strings.Contains(strings.ToLower(view), strings.ToLower(forbidden)) {
			t.Fatalf("permission view leaked %q: %q", forbidden, view)
		}
	}
}

func TestRemoveDownstreamFailureShowsBoundedManualRecovery(t *testing.T) {
	m := NewModel(buildTestDeps())
	m.state = StateMigrationResult
	m.migrationRecoveryCode = migration.DispositionManualRecoveryCode
	view := m.View()
	for _, want := range []string{migration.DispositionManualRecoveryCode, "cannot be recreated automatically", "preserved volumes", "validated backup"} {
		if !strings.Contains(view, want) {
			t.Fatalf("manual recovery view missing %q: %q", want, view)
		}
	}
	if strings.Contains(view, strings.Repeat("a", 64)) {
		t.Fatalf("manual recovery view leaked an ID: %q", view)
	}
}
