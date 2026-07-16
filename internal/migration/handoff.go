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
	Container       LegacyContainerController
	RecoveryContext func() (context.Context, context.CancelFunc)
}
type PreInstallMigrationLease struct {
	owner       *PreInstallMigrationCoordinator
	quiescence  installation.PM2Quiescence
	containerID string
	disposition ContainerDisposition
	pm2Only     bool
	mu          sync.Mutex
	consumed    bool
}

type PM2Target struct {
	PMID int64
	Name string
	Port uint16
}

func (l *PreInstallMigrationLease) PM2Targets() []PM2Target {
	if l == nil {
		return nil
	}
	targets := make([]PM2Target, 0, len(l.quiescence.Processes))
	for _, process := range l.quiescence.Processes {
		targets = append(targets, PM2Target{PMID: process.PMID, Name: process.Name, Port: process.Port})
	}
	return targets
}

func (c *PreInstallMigrationCoordinator) Begin(ctx context.Context, backup BackupRef, containerID string, disposition ContainerDisposition) (*PreInstallMigrationLease, error) {
	if c == nil || c.Legacy == nil || c.PM2 == nil || c.Container == nil || !fullContainerID.MatchString(containerID) || disposition > DispositionRemove || ctx.Err() != nil {
		return nil, errors.New("pre-install migration is unavailable")
	}
	if _, err := c.Legacy.Revalidate(ctx, backup); err != nil || ctx.Err() != nil {
		return nil, errors.New("pre-install migration backup is invalid")
	}
	quiescence, err := c.PM2.Quiesce(ctx)
	if err != nil || ctx.Err() != nil || !completeQuiescence(quiescence) {
		if len(quiescence.Evidence) > 0 {
			recovery, recoveryErr := c.recover(quiescence)
			if recoveryErr != nil || !recovery.Verified {
				code := recovery.Code
				if code == "" {
					code = "pm2-recovery-unproven"
				}
				lease := &PreInstallMigrationLease{owner: c, quiescence: cloneQuiescence(quiescence), pm2Only: true}
				return lease, errors.Join(installation.QuiescenceError{Code: code, Diagnostic: recovery.Diagnostic}, err, recoveryErr)
			}
		}
		return nil, errors.Join(errors.New("pre-install migration quiescence is incomplete"), err)
	}
	if result, err := c.Container.Apply(ctx, containerID, disposition); err != nil || ctx.Err() != nil {
		var containerErr error
		if disposition == DispositionRemove && result.Code == DispositionStoppedCode {
			recoveryCtx, cancel := c.recoveryContext()
			recovery, recoveryErr := c.Container.Recover(recoveryCtx, containerID, DispositionStop)
			cancel()
			if recoveryErr != nil || !recovery.Verified {
				containerErr = errors.Join(errors.New(DispositionRecoveryUnprovenCode), recoveryErr)
			}
		}
		_, pm2Err := c.recover(quiescence)
		if containerErr != nil {
			return nil, errors.Join(errors.New("legacy container disposition failed"), containerErr, pm2Err)
		}
		if errors.Is(err, ErrSudoDockerPermission) {
			return nil, ErrSudoDockerPermission
		}
		return nil, errors.New("legacy container disposition failed")
	}
	return &PreInstallMigrationLease{owner: c, quiescence: cloneQuiescence(quiescence), containerID: containerID, disposition: disposition}, nil
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
	container, containerErr := ContainerDispositionResult{Verified: true}, error(nil)
	if !lease.pm2Only {
		containerCtx, cancelContainer := c.recoveryContext()
		container, containerErr = c.Container.Recover(containerCtx, lease.containerID, lease.disposition)
		cancelContainer()
	}
	pm2Ctx, cancelPM2 := c.recoveryContext()
	pm2, pm2Err := c.PM2.Recover(pm2Ctx, quiescence)
	cancelPM2()
	if !lease.pm2Only && lease.disposition == DispositionRemove {
		pm2.Code = DispositionManualRecoveryCode
	} else if containerErr != nil || !container.Verified {
		pm2.Code = DispositionRecoveryUnprovenCode
	}
	return pm2, errors.Join(containerErr, pm2Err)
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
	if quiescence.NoopVerified {
		return len(quiescence.Processes) == 0 && len(quiescence.Evidence) == 0
	}
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
	return installation.PM2Quiescence{Processes: append([]installation.PM2ProcessIdentity(nil), source.Processes...), Evidence: append([]installation.PM2StoppedEvidence(nil), source.Evidence...), NoopVerified: source.NoopVerified}
}
