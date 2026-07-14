package migration

import (
	"context"
	"errors"
	"github.com/jcaltamar/alice-installer/internal/installation"
	"sync"
	"time"
)

type PreInstallMigrationCoordinator struct {
	Legacy          BackupRevalidator
	PM2             installation.LegacyPM2Quiescer
	RecoveryContext func() (context.Context, context.CancelFunc)
}
type PreInstallMigrationLease struct {
	owner      *PreInstallMigrationCoordinator
	quiescence installation.PM2Quiescence
	mu         sync.Mutex
	consumed   bool
}

func (c *PreInstallMigrationCoordinator) Begin(ctx context.Context, backup BackupRef) (*PreInstallMigrationLease, error) {
	if c == nil || c.Legacy == nil || c.PM2 == nil || ctx.Err() != nil {
		return nil, errors.New("pre-install migration is unavailable")
	}
	if _, err := c.Legacy.Revalidate(ctx, backup); err != nil || ctx.Err() != nil {
		return nil, errors.New("pre-install migration backup is invalid")
	}
	quiescence, err := c.PM2.Quiesce(ctx)
	if err != nil || ctx.Err() != nil || !completeQuiescence(quiescence) {
		if len(quiescence.Evidence) > 0 {
			_, _ = c.recover(quiescence)
		}
		return nil, errors.New("pre-install migration quiescence is incomplete")
	}
	return &PreInstallMigrationLease{owner: c, quiescence: cloneQuiescence(quiescence)}, nil
}
func (c *PreInstallMigrationCoordinator) CompleteSuccess(lease *PreInstallMigrationLease) error {
	_, _, err := c.consume(lease)
	return err
}
func (c *PreInstallMigrationCoordinator) CompleteFailure(lease *PreInstallMigrationLease) (installation.PM2Recovery, error) {
	quiescence, first, err := c.consume(lease)
	if err != nil || !first {
		return installation.PM2Recovery{}, err
	}
	return c.recover(quiescence)
}
func (c *PreInstallMigrationCoordinator) recover(quiescence installation.PM2Quiescence) (installation.PM2Recovery, error) {
	ctx, cancel := c.recoveryContext()
	defer cancel()
	return c.PM2.Recover(ctx, quiescence)
}
func (c *PreInstallMigrationCoordinator) consume(lease *PreInstallMigrationLease) (installation.PM2Quiescence, bool, error) {
	if c == nil || lease == nil || lease.owner != c {
		return installation.PM2Quiescence{}, false, errors.New("pre-install migration lease is invalid")
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if lease.consumed {
		return installation.PM2Quiescence{}, false, nil
	}
	lease.consumed = true
	return cloneQuiescence(lease.quiescence), true, nil
}
func (c *PreInstallMigrationCoordinator) recoveryContext() (context.Context, context.CancelFunc) {
	parent, cancel := context.Background(), func() {}
	if c.RecoveryContext != nil {
		parent, cancel = c.RecoveryContext()
	}
	ctx, timeoutCancel := context.WithTimeout(parent, 10*time.Second)
	return ctx, func() { timeoutCancel(); cancel() }
}
func completeQuiescence(quiescence installation.PM2Quiescence) bool {
	if len(quiescence.Processes) == 0 || len(quiescence.Processes) != len(quiescence.Evidence) {
		return false
	}
	for index, evidence := range quiescence.Evidence {
		identity := quiescence.Processes[index]
		if !evidence.StopVerified || evidence.PMID != identity.PMID || evidence.OriginalPID != identity.PID || evidence.Port != identity.Port || evidence.StartTicks != identity.StartTicks {
			return false
		}
	}
	return true
}
func cloneQuiescence(source installation.PM2Quiescence) installation.PM2Quiescence {
	return installation.PM2Quiescence{Processes: append([]installation.PM2ProcessIdentity(nil), source.Processes...), Evidence: append([]installation.PM2StoppedEvidence(nil), source.Evidence...)}
}
