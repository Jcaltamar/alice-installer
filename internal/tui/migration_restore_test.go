package tui

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/migration"
)

type fakeLegacyRestoreAction struct {
	calls     int
	request   migration.RestoreRequest
	result    migration.RestoreResult
	cancelled bool
}

func (a *fakeLegacyRestoreAction) Run(ctx context.Context, request migration.RestoreRequest) migration.RestoreResult {
	a.calls++
	a.request = request
	<-ctx.Done()
	if a.cancelled {
		return migration.RestoreResult{Outcome: migration.RestoreCancelledBeforeCutover, FailedStage: migration.StageWait, Code: "restore-cancelled"}
	}
	return a.result
}

func TestMigrationDeployStartsRestoreAndOnlySuccessRejoinsHealth(t *testing.T) {
	for _, tt := range []struct {
		name       string
		result     migration.RestoreResult
		wantState  State
		wantHealth bool
	}{
		{"success", migration.RestoreResult{Outcome: migration.RestoreSucceeded}, StateDeploy, true},
		{"failed", migration.RestoreResult{Outcome: migration.RestoreFailedBeforeCutover, FailedStage: migration.StageCredentials}, StateMigrationResult, false},
		{"unsupported", migration.RestoreResult{Outcome: migration.RestoreUnsupported}, StateMigrationResult, false},
		{"cancelled", migration.RestoreResult{Outcome: migration.RestoreCancelledBeforeCutover, FailedStage: migration.StageWait}, StateMigrationResult, false},
		{"partial", migration.RestoreResult{Outcome: migration.RestorePartialCutover, FailedStage: migration.StageRollback}, StateMigrationResult, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			deps := buildTestDeps()
			action := &fakeLegacyRestoreAction{result: tt.result}
			deps.LegacyRestoreAction = action
			m := NewModel(deps)
			m.state, m.migrationPending = StateDeploy, true
			m.envPath = "/workspace/.env"
			m.composeFiles = []string{"/workspace/docker-compose.yml"}
			m.backupResult = migration.BackupResult{DumpPath: "/opt/alice/backups/legacy.dump", ManifestPath: "/opt/alice/backups/legacy.json", SHA256: "sum", Size: 1}

			updated, cmd := m.Update(DeployCompleteMsg{})
			m = updated.(Model)
			if m.state != StateDatabaseRestore || cmd == nil {
				t.Fatalf("deploy state/cmd = %v/%v", m.state, cmd)
			}
			if action.calls != 0 {
				t.Fatal("restore must not run before its command executes")
			}
			action.cancelled = false
			m.restoreCancel()
			updated, cmd = m.Update(cmd())
			m = updated.(Model)
			if tt.wantHealth {
				if m.state != StateDeploy || cmd == nil {
					t.Fatalf("success state/cmd = %v/%v", m.state, cmd)
				}
				if msg := cmd(); func() bool { _, ok := msg.(HealthTickMsg); return ok }() == false {
					t.Fatalf("success must rejoin HealthTickMsg, got %T", msg)
				}
			} else if m.state != tt.wantState || cmd != nil {
				t.Fatalf("terminal state/cmd = %v/%v", m.state, cmd)
			}
		})
	}
}

func TestMigrationRestoreEscapeCancelsWaitWithoutHealth(t *testing.T) {
	deps := buildTestDeps()
	action := &fakeLegacyRestoreAction{cancelled: true}
	deps.LegacyRestoreAction = action
	m := NewModel(deps)
	m.state, m.migrationPending = StateDeploy, true
	m.backupResult = migration.BackupResult{DumpPath: "/opt/alice/backups/legacy.dump"}
	updated, cmd := m.Update(DeployCompleteMsg{})
	m = updated.(Model)
	if m.state != StateDatabaseRestore || cmd == nil {
		t.Fatalf("restore state/cmd = %v/%v", m.state, cmd)
	}
	updated, cancelCmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.state != StateDatabaseRestore || cancelCmd != nil {
		t.Fatalf("escape state/cmd = %v/%v", m.state, cancelCmd)
	}
	updated, cmd = m.Update(cmd())
	m = updated.(Model)
	if m.state != StateMigrationResult || cmd != nil || m.restoreResult.FailedStage != migration.StageWait {
		t.Fatalf("cancelled result = state:%v cmd:%v stage:%v", m.state, cmd, m.restoreResult.FailedStage)
	}
}

func TestMigrationRestoreActionIsNotNeededForOrdinaryDeploy(t *testing.T) {
	m := NewModel(buildTestDeps())
	m.state = StateDeploy
	updated, cmd := m.Update(DeployCompleteMsg{})
	if updated.(Model).state != StateDeploy || cmd == nil {
		t.Fatalf("ordinary deploy must retain its health route: state=%v cmd=%v", updated.(Model).state, cmd)
	}
	if _, ok := cmd().(HealthTickMsg); !ok {
		t.Fatalf("ordinary deploy must emit HealthTickMsg, got %T", cmd())
	}
}

var _ = time.Second
