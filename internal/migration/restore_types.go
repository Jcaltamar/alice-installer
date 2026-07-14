package migration

import (
	"context"
	"github.com/jcaltamar/alice-installer/internal/workspace"
	"time"
)

type RestoreStage uint8

const (
	StagePlatformGate RestoreStage = iota
	StageWait
	StageCredentials
	StageLegacyRevalidation
	StageBackendStop
	StagePostgresCheck
	StageTargetBackup
	StageTargetReplacement
	StageRestoreValidation
	StageBackendStart
	StageBackendHealth
	StageRollback
)

type RestoreOutcome uint8

const (
	RestoreSucceeded RestoreOutcome = iota
	RestoreFailedBeforeCutover
	RestoreCancelledBeforeCutover
	RestorePartialCutover
	RestoreUnsupported
)

type RollbackStatus uint8

const (
	RollbackNotRequired RollbackStatus = iota
	RollbackSucceeded
	RollbackFailed
	RollbackCancelled
)

type BackupRef struct {
	DumpPath, ManifestPath, SHA256 string
	Size                           int64
}
type BackupEvidence struct {
	DumpPath, ManifestPath, SHA256 string
	Size                           int64
	Validated                      bool
}
type ValidatedBackup struct {
	ref            BackupRef
	targetRollback bool
}
type DatabaseEvidence struct {
	RestoreExitOK       bool
	ConnectionOK        bool
	ApplicationTables   uint64
	PostgreSQLReachable bool
}
type RestoreResult struct {
	Outcome        RestoreOutcome
	FailedStage    RestoreStage
	Mutated        bool
	Rollback       RollbackStatus
	LegacyBackup   BackupEvidence
	TargetBackup   BackupEvidence
	Database       DatabaseEvidence
	BackendHealthy bool
	Code           string
}
type Waiter interface {
	Wait(context.Context, time.Duration) error
}
type RealWaiter struct{}

func (RealWaiter) Wait(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type FakeWaiter struct {
	Durations []time.Duration
	Err       error
}

func (w *FakeWaiter) Wait(ctx context.Context, duration time.Duration) error {
	w.Durations = append(w.Durations, duration)
	if err := ctx.Err(); err != nil {
		return err
	}
	return w.Err
}

type BackupRevalidator interface {
	Revalidate(context.Context, BackupRef) (ValidatedBackup, error)
}
type TargetBackupCreator interface {
	CreateValidated(context.Context, workspace.TargetDatabaseConfig, string) (ValidatedBackup, error)
}
type DatabaseReplacement interface {
	Replace(context.Context, workspace.TargetDatabaseConfig, ValidatedBackup) (DatabaseEvidence, error)
}
type PostgreSQLProbe interface {
	Reachable(context.Context, workspace.TargetDatabaseConfig) error
}
type PostgreSQLIdentityProbe interface {
	Prove(context.Context, []string, string) error
}
type BackendStoppedProbe interface {
	BackendStopped(context.Context, []string, string) (bool, error)
}
type BackendHealthVerifier interface {
	WaitHealthy(context.Context, []string, string) error
}
type ServiceController interface {
	StopService(context.Context, []string, string, string) error
	StartService(context.Context, []string, string, string) error
}
type RestoreRequest struct {
	GOOS, GOARCH               string
	ComposeFiles               []string
	EnvFile, BackupDestination string
	Legacy                     BackupRef
}
type LegacyRestoreAction interface {
	Run(context.Context, RestoreRequest) RestoreResult
}
