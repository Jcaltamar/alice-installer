package migration

import (
	"context"
	"time"

	"github.com/jcaltamar/alice-installer/internal/workspace"
)

const (
	restoreWaitDuration   = 60 * time.Second
	rollbackTimeout       = 30 * time.Second
	restoreBackendService = "backend"
)

// RestoreCoordinator sequences package-level restore policy. It has no route,
// Docker command, or credential-transport construction responsibility.
type RestoreCoordinator struct {
	Waiter             Waiter
	EnvReader          workspace.TargetEnvReader
	Legacy             BackupRevalidator
	Target             TargetBackupCreator
	Services           ServiceController
	PostgreSQL         PostgreSQLProbe
	PostgreSQLIdentity PostgreSQLIdentityProbe
	Replacement        CoordinatorReplacement
	Health             BackendHealthVerifier
	BackendStopped     BackendStoppedProbe
	// RollbackContext supplies the bounded recovery context and permits a second
	// shutdown cancellation seam without reusing a cancelled caller context.
	RollbackContext func() (context.Context, context.CancelFunc)
}

func (c RestoreCoordinator) Run(ctx context.Context, request RestoreRequest) RestoreResult {
	result := RestoreResult{FailedStage: StagePlatformGate, Code: "restore-precondition"}
	if request.GOOS != "linux" || (request.GOARCH != "amd64" && request.GOARCH != "arm64") {
		result.Outcome, result.Code = RestoreUnsupported, "restore-unsupported"
		return result
	}
	if c.Waiter == nil || c.EnvReader == nil || c.Legacy == nil || c.Target == nil || c.Services == nil || c.PostgreSQL == nil || c.PostgreSQLIdentity == nil || c.Replacement == nil || c.Health == nil || c.BackendStopped == nil {
		return beforeCutover(result, StagePlatformGate, ctx, "restore-precondition")
	}
	if err := c.Waiter.Wait(ctx, restoreWaitDuration); err != nil {
		return beforeCutover(result, StageWait, ctx, "restore-wait")
	}
	if err := c.PostgreSQLIdentity.Prove(ctx, request.ComposeFiles, request.EnvFile); err != nil {
		return beforeCutover(result, StagePostgresCheck, ctx, "restore-postgres-identity")
	}
	if err := c.Services.StopService(ctx, request.ComposeFiles, request.EnvFile, restoreBackendService); err != nil {
		return beforeCutover(result, StageBackendStop, ctx, "restore-backend-stop")
	}
	stopped := true
	cfg, err := c.EnvReader.ReadTargetDatabase(ctx, request.EnvFile)
	if err != nil {
		return c.recoverBeforeCutover(ctx, request, result, StageCredentials, "restore-credentials", stopped)
	}
	defer cfg.Release()
	legacy, err := c.Legacy.Revalidate(ctx, request.Legacy)
	if err != nil {
		return c.recoverBeforeCutover(ctx, request, result, StageLegacyRevalidation, "restore-legacy", stopped)
	}
	result.LegacyBackup = backupEvidence(legacy)
	if err := c.PostgreSQLIdentity.Prove(ctx, request.ComposeFiles, request.EnvFile); err != nil || c.PostgreSQL.Reachable(ctx, cfg) != nil {
		return c.recoverBeforeCutover(ctx, request, result, StagePostgresCheck, "restore-postgres", stopped)
	}
	target, err := c.Target.CreateValidated(ctx, cfg, request.BackupDestination)
	if err != nil || !target.targetRollback {
		return c.recoverBeforeCutover(ctx, request, result, StageTargetBackup, "restore-target-backup", stopped)
	}
	result.TargetBackup = backupEvidence(target)
	result.Database, result.Mutated, err = c.Replacement.Replace(ctx, cfg, legacy)
	if err != nil || !databaseValid(result.Database) {
		if !result.Mutated {
			return c.recoverBeforeCutover(ctx, request, result, StageTargetReplacement, "restore-primary-failed", stopped)
		}
		return c.rollback(ctx, request, cfg, target, result, StageTargetReplacement, "restore-primary-failed")
	}
	if err := c.PostgreSQLIdentity.Prove(ctx, request.ComposeFiles, request.EnvFile); err != nil {
		return c.rollback(ctx, request, cfg, target, result, StageRestoreValidation, "restore-postgres-identity")
	}
	if err := c.Services.StartService(ctx, request.ComposeFiles, request.EnvFile, restoreBackendService); err != nil {
		return c.rollback(ctx, request, cfg, target, result, StageBackendStart, "restore-backend-start")
	}
	if err := c.Health.WaitHealthy(ctx, request.ComposeFiles, request.EnvFile); err != nil {
		return c.rollback(ctx, request, cfg, target, result, StageBackendHealth, "restore-backend-health")
	}
	result.Outcome, result.Rollback, result.BackendHealthy, result.Code = RestoreSucceeded, RollbackNotRequired, true, "restore-succeeded"
	return result
}

// CoordinatorReplacement reports the exact drop-boundary mutation state from the
// 3A direct-argv executor; the coordinator never guesses that boundary.
type CoordinatorReplacement interface {
	Replace(context.Context, workspace.TargetDatabaseConfig, ValidatedBackup) (DatabaseEvidence, bool, error)
}

func (c RestoreCoordinator) recoverBeforeCutover(ctx context.Context, request RestoreRequest, result RestoreResult, stage RestoreStage, code string, stopped bool) RestoreResult {
	if stopped {
		recoveryCtx, cancel := c.recoveryContext()
		defer cancel()
		if c.Services.StartService(recoveryCtx, request.ComposeFiles, request.EnvFile, restoreBackendService) != nil || c.Health.WaitHealthy(recoveryCtx, request.ComposeFiles, request.EnvFile) != nil {
			return beforeCutover(result, stage, ctx, "restore-backend-recovery")
		}
	}
	return beforeCutover(result, stage, ctx, code)
}

func (c RestoreCoordinator) rollback(_ context.Context, request RestoreRequest, cfg workspace.TargetDatabaseConfig, target ValidatedBackup, result RestoreResult, stage RestoreStage, code string) RestoreResult {
	result.Outcome, result.FailedStage, result.Code = RestorePartialCutover, stage, code
	rollbackCtx, cancel := c.recoveryContext()
	defer cancel()
	if c.PostgreSQLIdentity.Prove(rollbackCtx, request.ComposeFiles, request.EnvFile) != nil || c.PostgreSQL.Reachable(rollbackCtx, cfg) != nil {
		result.Rollback, result.Code = rollbackStatus(rollbackCtx), "restore-rollback-postgres"
		return result
	}
	stopErr := c.Services.StopService(rollbackCtx, request.ComposeFiles, request.EnvFile, restoreBackendService)
	proofCtx, proofCancel := c.recoveryContext()
	stopped, proofErr := c.BackendStopped.BackendStopped(proofCtx, request.ComposeFiles, request.EnvFile)
	proofCancel()
	if stopErr != nil || proofErr != nil || !stopped {
		result.Rollback, result.Code = rollbackStatus(rollbackCtx), "restore-rollback-stop"
		if result.Rollback == RollbackCancelled && (proofErr != nil || !stopped) {
			result.Rollback = RollbackFailed
		}
		return result
	}
	if _, err := c.Legacy.Revalidate(rollbackCtx, target.ref); err != nil {
		result.Rollback, result.Code = rollbackStatus(rollbackCtx), "restore-rollback-revalidate"
		return result
	}
	if c.PostgreSQLIdentity.Prove(rollbackCtx, request.ComposeFiles, request.EnvFile) != nil || c.PostgreSQL.Reachable(rollbackCtx, cfg) != nil {
		result.Rollback, result.Code = rollbackStatus(rollbackCtx), "restore-rollback-postgres"
		return result
	}
	var err error
	result.Database, _, err = c.Replacement.Replace(rollbackCtx, cfg, target)
	if err != nil || !databaseValid(result.Database) {
		result.Rollback, result.Code = rollbackStatus(rollbackCtx), "restore-rollback-failed"
		return result
	}
	if c.Services.StartService(rollbackCtx, request.ComposeFiles, request.EnvFile, restoreBackendService) != nil || c.Health.WaitHealthy(rollbackCtx, request.ComposeFiles, request.EnvFile) != nil {
		_ = c.Services.StopService(rollbackCtx, request.ComposeFiles, request.EnvFile, restoreBackendService)
		result.Rollback, result.Code = rollbackStatus(rollbackCtx), "restore-rollback-recovery"
		return result
	}
	result.Rollback, result.BackendHealthy = RollbackSucceeded, true
	return result
}

func (c RestoreCoordinator) recoveryContext() (context.Context, context.CancelFunc) {
	if c.RollbackContext != nil {
		return c.RollbackContext()
	}
	return context.WithTimeout(context.Background(), rollbackTimeout)
}

func beforeCutover(result RestoreResult, stage RestoreStage, ctx context.Context, code string) RestoreResult {
	result.FailedStage, result.Code = stage, code
	if ctx.Err() != nil {
		result.Outcome, result.Code = RestoreCancelledBeforeCutover, "restore-cancelled"
	} else {
		result.Outcome = RestoreFailedBeforeCutover
	}
	return result
}
func rollbackStatus(ctx context.Context) RollbackStatus {
	if ctx.Err() != nil {
		return RollbackCancelled
	}
	return RollbackFailed
}
func databaseValid(e DatabaseEvidence) bool {
	return e.RestoreExitOK && e.ConnectionOK && e.ApplicationTables > 0 && e.PostgreSQLReachable
}
func backupEvidence(backup ValidatedBackup) BackupEvidence {
	return BackupEvidence{DumpPath: backup.ref.DumpPath, ManifestPath: backup.ref.ManifestPath, SHA256: backup.ref.SHA256, Size: backup.ref.Size, Validated: true}
}
