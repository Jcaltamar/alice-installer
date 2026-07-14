package migration

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/workspace"
)

var ErrRestoreAdapterPrecondition = errors.New("restore adapter precondition failed")

// TargetRollbackBackupAdapter creates the actual target dump through the private
// pgpass bridge. It rejects caller-selected publication roots.
type stagingFile interface {
	io.Writer
	Chmod(os.FileMode) error
	Close() error
}

type stagingWriter struct {
	file stagingFile
	err  error
}

func (w *stagingWriter) Write(data []byte) (int, error) {
	written, err := w.file.Write(data)
	if err != nil {
		w.err = err
	}
	return written, err
}

type TargetRollbackBackupAdapter struct {
	Validator   ArchiveValidator
	Executor    BinaryExecutor
	Credentials CredentialTransport
	OperationID func() (string, error)
	OpenStaging func(string, int, os.FileMode) (stagingFile, error)
}

func (a TargetRollbackBackupAdapter) CreateValidated(ctx context.Context, cfg workspace.TargetDatabaseConfig, destination string) (ValidatedBackup, error) {
	if filepath.Clean(destination) != filepath.Clean(authoritativeBackupRoot()) || a.Validator == nil || a.Executor == nil || a.OperationID == nil {
		return ValidatedBackup{}, ErrRestoreBackupGate
	}
	creator := TargetRollbackBackupCreator{Validator: a.Validator, OperationID: a.OperationID,
		Stage: func(ctx context.Context, cfg workspace.TargetDatabaseConfig, destination, operationID string) (string, error) {
			return a.stage(ctx, cfg, destination, operationID)
		}}
	return creator.CreateValidated(ctx, cfg, authoritativeBackupRoot())
}

func (a TargetRollbackBackupAdapter) stage(ctx context.Context, cfg workspace.TargetDatabaseConfig, destination, operationID string) (string, error) {
	if !validOperationID(operationID) {
		return "", ErrRestoreBackupGate
	}
	credential, err := PrepareTargetCredential(a.Credentials, cfg)
	if err != nil {
		return "", ErrRestoreBackupGate
	}
	defer credential.Cleanup()
	staged := filepath.Join(destination, ".target-rollback-"+operationID+".part")
	file, err := a.openStaging(staged)
	if err != nil {
		return "", ErrRestoreBackupGate
	}
	cleanup := func() {
		_ = file.Close()
		_ = os.Remove(staged)
	}
	spec, err := BuildTargetBackupDump(cfg, credential, operationID)
	writer := &stagingWriter{file: file}
	if err != nil || a.Executor.Run(ctx, spec, writer).Outcome != ProcessSucceeded || writer.err != nil || ctx.Err() != nil {
		cleanup()
		return "", ErrRestoreBackupGate
	}
	if err := file.Chmod(0o600); err != nil {
		cleanup()
		return "", ErrRestoreBackupGate
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(staged)
		return "", ErrRestoreBackupGate
	}
	return staged, nil
}

func (a TargetRollbackBackupAdapter) openStaging(path string) (stagingFile, error) {
	if a.OpenStaging != nil {
		return a.OpenStaging(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	}
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
}

// ComposeBackendProbe turns compose ps evidence into the narrow restore probes.
// It accepts exactly one backend record; absent, duplicate, malformed, or failed
// compose output is deliberately not evidence.
type ComposeBackendProbe struct{ Compose compose.ComposeRunner }

// ComposePostgreSQLIdentityProbe accepts exactly one running record for the
// immutable database service/container pair. It uses Compose's bounded JSON probe.
type ComposePostgreSQLIdentityProbe struct{ Compose compose.ComposeRunner }

func (p ComposePostgreSQLIdentityProbe) Prove(ctx context.Context, files []string, envFile string) error {
	if ctx.Err() != nil || p.Compose == nil {
		return ErrRestoreAdapterPrecondition
	}
	statuses, err := p.Compose.HealthStatus(ctx, files, envFile)
	if err != nil {
		return ErrRestoreAdapterPrecondition
	}
	found := false
	for _, status := range statuses {
		if status.Service != compose.PostgreSQLService {
			continue
		}
		if found || status.Container != compose.PostgreSQLContainer || status.State != "running" {
			return ErrRestoreAdapterPrecondition
		}
		found = true
	}
	if !found {
		return ErrRestoreAdapterPrecondition
	}
	return nil
}

func (p ComposeBackendProbe) BackendStopped(ctx context.Context, files []string, envFile string) (bool, error) {
	backend, err := p.backend(ctx, files, envFile)
	if err != nil || backend.State == "" {
		return false, ErrRestoreAdapterPrecondition
	}
	return backend.State == "exited" || backend.State == "created", nil
}

func (p ComposeBackendProbe) WaitHealthy(ctx context.Context, files []string, envFile string) error {
	backend, err := p.backend(ctx, files, envFile)
	if err != nil || !compose.IsReady(backend) {
		return ErrRestoreAdapterPrecondition
	}
	return nil
}

func (p ComposeBackendProbe) backend(ctx context.Context, files []string, envFile string) (compose.ServiceHealth, error) {
	if ctx.Err() != nil || p.Compose == nil {
		return compose.ServiceHealth{}, ErrRestoreAdapterPrecondition
	}
	statuses, err := p.Compose.HealthStatus(ctx, files, envFile)
	if err != nil {
		return compose.ServiceHealth{}, ErrRestoreAdapterPrecondition
	}
	var backend compose.ServiceHealth
	for _, status := range statuses {
		if status.Service != compose.BackendService {
			continue
		}
		if backend.Service != "" {
			return compose.ServiceHealth{}, ErrRestoreAdapterPrecondition
		}
		backend = status
	}
	if backend.Service != compose.BackendService {
		return compose.ServiceHealth{}, ErrRestoreAdapterPrecondition
	}
	return backend, nil
}
