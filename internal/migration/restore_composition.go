package migration

import (
	"context"
	"errors"

	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/workspace"
)

var ErrRestoreCompositionPrecondition = errors.New("restore composition precondition failed")

// ProductionRestoreDependencies admits only the operational Compose client and an
// operation-ID generator. Every other restore capability is constructed here.
type ProductionRestoreDependencies struct {
	Compose        *compose.CLICompose
	OperationID    func() (string, error)
	TempRoot       string
	DockerExecutor BinaryExecutor
}

// NewProductionRestoreCoordinator composes the package-only restore graph. It is
// deliberately not invoked by a TUI, command, or runtime route in this slice.
func NewProductionRestoreCoordinator(deps ProductionRestoreDependencies) (RestoreCoordinator, error) {
	if deps.Compose == nil || deps.OperationID == nil || deps.DockerExecutor == nil {
		return RestoreCoordinator{}, ErrRestoreCompositionPrecondition
	}
	executor := deps.DockerExecutor
	credentials := CredentialTransport{TempRoot: deps.TempRoot}
	validator := PG11ArchiveValidator{Executor: executor}
	probe := ComposeBackendProbe{Compose: deps.Compose}
	identity := ComposePostgreSQLIdentityProbe{Compose: deps.Compose}
	prepare := func(_ context.Context, cfg workspace.TargetDatabaseConfig) (CredentialFile, error) {
		return PrepareTargetCredential(credentials, cfg)
	}
	return RestoreCoordinator{
		Waiter:             RealWaiter{},
		EnvReader:          workspace.TargetEnvFileReader{},
		Legacy:             BackupGate{Validator: validator},
		Target:             TargetRollbackBackupAdapter{Validator: validator, Executor: executor, Credentials: credentials, OperationID: deps.OperationID},
		Services:           deps.Compose,
		PostgreSQL:         PostgreSQLReachabilityAdapter{Executor: executor, Prepare: prepare},
		PostgreSQLIdentity: identity,
		Replacement:        TargetReplacementAdapter{Executor: executor, Prepare: prepare},
		Health:             probe,
		BackendStopped:     probe,
	}, nil
}
