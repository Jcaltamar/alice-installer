package migration

import (
	"context"
	"errors"
	"github.com/jcaltamar/alice-installer/internal/installation"
	"strings"
	"testing"
	"time"
)

func TestPreInstallMigrationCoordinatorBeginRequiresBackupCompleteQuiescenceAndCompensatesPartialStop(t *testing.T) {
	events := []string{}
	backup, quiescer := &handoffBackup{events: &events}, &handoffQuiescer{events: &events, stopped: acknowledgedQuiescence()}
	container := &handoffContainer{events: &events}
	coordinator := PreInstallMigrationCoordinator{Legacy: backup, PM2: quiescer, Container: container}
	id := strings.Repeat("a", 64)
	lease, err := coordinator.Begin(context.Background(), BackupRef{DumpPath: "/opt/alice/backups/legacy.dump"}, id, DispositionStop)
	if err != nil || lease == nil {
		t.Fatalf("lease = %#v, err = %v", lease, err)
	}
	if got, want := strings.Join(events, ","), "backup,quiesce,stop"; got != want {
		t.Fatalf("events = %q, want %q", got, want)
	}
	failedEvents := []string{}
	failedBackup := &handoffBackup{events: &failedEvents, err: errors.New("invalid")}
	if lease, err := (&PreInstallMigrationCoordinator{Legacy: failedBackup, PM2: quiescer, Container: container}).Begin(context.Background(), BackupRef{}, id, DispositionStop); err == nil || lease != nil || strings.Join(failedEvents, ",") != "backup" {
		t.Fatalf("backup failure lease = %#v, events = %q, err = %v", lease, strings.Join(failedEvents, ","), err)
	}
	quiescer.stopped.Evidence = nil
	if lease, err := coordinator.Begin(context.Background(), BackupRef{}, id, DispositionStop); err == nil || lease != nil {
		t.Fatalf("incomplete lease = %#v, err = %v", lease, err)
	}
	quiescer.stopped, quiescer.err = acknowledgedQuiescence(), errors.New("partial stop")
	quiescer.recoveryErr = errors.New("sudo authorization expired")
	lease, err = coordinator.Begin(context.Background(), BackupRef{}, id, DispositionStop)
	if err == nil || lease == nil || quiescer.recoverCalls != 1 {
		t.Fatalf("partial lease = %#v, recoveries = %d, err = %v", lease, quiescer.recoverCalls, err)
	}
	var recoveryFailure installation.QuiescenceError
	if !errors.As(err, &recoveryFailure) || recoveryFailure.Code != "pm2-recovery-unproven" {
		t.Fatalf("recovery failure = %#v, err = %v", recoveryFailure, err)
	}
	quiescer.recoveryErr = nil
	if recovery, retryErr := coordinator.CompleteFailure(lease); retryErr != nil || !recovery.Verified || quiescer.recoverCalls != 2 || container.events == nil || strings.Contains(strings.Join(*container.events, ","), "container-recover") {
		t.Fatalf("retry = %#v, recoveries = %d, events = %q, err = %v", recovery, quiescer.recoverCalls, strings.Join(*container.events, ","), retryErr)
	}
}
func TestPreInstallMigrationCoordinatorAcceptsVerifiedNoopQuiescence(t *testing.T) {
	quiescer := &handoffQuiescer{stopped: installation.PM2Quiescence{NoopVerified: true}}
	coordinator := PreInstallMigrationCoordinator{Legacy: &handoffBackup{}, PM2: quiescer, Container: &handoffContainer{}}
	lease, err := coordinator.Begin(context.Background(), BackupRef{}, strings.Repeat("a", 64), DispositionStop)
	if err != nil || lease == nil || len(lease.PM2Targets()) != 0 {
		t.Fatalf("lease = %#v, targets = %#v, err = %v", lease, lease.PM2Targets(), err)
	}
}

func TestPreInstallMigrationCoordinatorCompletionConsumesLeaseOnce(t *testing.T) {
	backup, quiescer := &handoffBackup{}, &handoffQuiescer{stopped: acknowledgedQuiescence()}
	contexts := 0
	coordinator := PreInstallMigrationCoordinator{Legacy: backup, PM2: quiescer, Container: &handoffContainer{}, RecoveryContext: func() (context.Context, context.CancelFunc) {
		contexts++
		return context.WithCancel(context.Background())
	}}
	id := strings.Repeat("a", 64)
	success, err := coordinator.Begin(context.Background(), BackupRef{}, id, DispositionStop)
	if err != nil || coordinator.CompleteSuccess(success) != nil || coordinator.CompleteSuccess(success) != nil || quiescer.recoverCalls != 0 {
		t.Fatalf("success completion err = %v, recoveries = %d", err, quiescer.recoverCalls)
	}
	failure, err := coordinator.Begin(context.Background(), BackupRef{}, id, DispositionStop)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.CompleteFailure(failure); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.CompleteFailure(failure); err != nil || quiescer.recoverCalls != 1 || contexts != 2 {
		t.Fatalf("duplicate failure err = %v, recoveries = %d, contexts = %d", err, quiescer.recoverCalls, contexts)
	}
}

func TestCompleteFailureGivesPM2IndependentRecoveryBudget(t *testing.T) {
	quiescer := &handoffQuiescer{stopped: acknowledgedQuiescence(), requireBudget: 15 * time.Millisecond}
	container := &handoffContainer{consumeContext: true}
	coordinator := PreInstallMigrationCoordinator{Legacy: &handoffBackup{}, PM2: quiescer, Container: container, RecoveryContext: func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.Background(), 20*time.Millisecond)
	}}
	lease, err := coordinator.Begin(context.Background(), BackupRef{}, strings.Repeat("a", 64), DispositionStop)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := coordinator.CompleteFailure(lease)
	if err == nil || !recovery.Verified || quiescer.recoverCalls != 1 {
		t.Fatalf("recovery = %#v, calls = %d, err = %v", recovery, quiescer.recoverCalls, err)
	}
}
func acknowledgedQuiescence() installation.PM2Quiescence {
	return installation.PM2Quiescence{Processes: []installation.PM2ProcessIdentity{{PMID: 1, PID: 10, CWD: "/opt/alice-guardian", ExecPath: "/usr/bin/bash", RuntimeExecPath: "/usr/local/bin/node", Port: 8080, StartTicks: 1}}, Evidence: []installation.PM2StoppedEvidence{{PMID: 1, OriginalPID: 10, Port: 8080, StartTicks: 1, StopVerified: true}}}
}

type handoffBackup struct {
	events *[]string
	err    error
}

func (b *handoffBackup) Revalidate(context.Context, BackupRef) (ValidatedBackup, error) {
	if b.events != nil {
		*b.events = append(*b.events, "backup")
	}
	return ValidatedBackup{}, b.err
}

type handoffQuiescer struct {
	events        *[]string
	stopped       installation.PM2Quiescence
	recoverCalls  int
	err           error
	recoveryErr   error
	requireBudget time.Duration
}

type handoffContainer struct {
	events         *[]string
	applyResult    ContainerDispositionResult
	applyErr       error
	recoveryResult ContainerDispositionResult
	recoveryErr    error
	consumeContext bool
}

func (c *handoffContainer) Apply(_ context.Context, _ string, disposition ContainerDisposition) (ContainerDispositionResult, error) {
	if c.events != nil {
		*c.events = append(*c.events, map[ContainerDisposition]string{DispositionStop: "stop", DispositionRemove: "remove"}[disposition])
	}
	if c.applyErr != nil {
		return c.applyResult, c.applyErr
	}
	return ContainerDispositionResult{Verified: true}, nil
}

func (c *handoffContainer) Recover(ctx context.Context, _ string, disposition ContainerDisposition) (ContainerDispositionResult, error) {
	if c.events != nil {
		*c.events = append(*c.events, "container-recover")
	}
	if c.recoveryErr != nil || c.recoveryResult.Code != "" {
		return c.recoveryResult, c.recoveryErr
	}
	if c.consumeContext {
		<-ctx.Done()
		return ContainerDispositionResult{Code: DispositionRecoveryUnprovenCode}, errors.New("container recovery timed out")
	}
	if disposition == DispositionRemove {
		return ContainerDispositionResult{Code: DispositionManualRecoveryCode}, nil
	}
	return ContainerDispositionResult{Code: DispositionRestartedCode, Verified: true}, nil
}

func (q *handoffQuiescer) Quiesce(context.Context) (installation.PM2Quiescence, error) {
	if q.events != nil {
		*q.events = append(*q.events, "quiesce")
	}
	return q.stopped, q.err
}
func (q *handoffQuiescer) Recover(ctx context.Context, _ installation.PM2Quiescence) (installation.PM2Recovery, error) {
	if _, ok := ctx.Deadline(); !ok {
		return installation.PM2Recovery{}, errors.New("recovery context is unbounded")
	}
	q.recoverCalls++
	if q.recoveryErr != nil {
		return installation.PM2Recovery{Attempted: 1, Code: "pm2-recovery-unproven"}, q.recoveryErr
	}
	if deadline, ok := ctx.Deadline(); q.requireBudget > 0 && (!ok || time.Until(deadline) < q.requireBudget) {
		return installation.PM2Recovery{}, errors.New("PM2 recovery budget was consumed")
	}
	if q.events != nil {
		*q.events = append(*q.events, "pm2-recover")
	}
	return installation.PM2Recovery{Attempted: 1, Recovered: 1, Verified: true}, nil
}

func TestBeginRecoversPartiallyRemovedContainerBeforePM2(t *testing.T) {
	for _, tt := range []struct {
		name, recoveryCode string
		recoveryErr        error
	}{
		{name: "restart succeeds", recoveryCode: DispositionRestartedCode},
		{name: "restart fails", recoveryCode: DispositionRecoveryUnprovenCode, recoveryErr: errors.New("restart failed")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			events := []string{}
			container := &handoffContainer{events: &events, applyResult: ContainerDispositionResult{Code: DispositionStoppedCode, Verified: true}, applyErr: errors.New("rm failed"), recoveryResult: ContainerDispositionResult{Code: tt.recoveryCode, Verified: tt.recoveryErr == nil}, recoveryErr: tt.recoveryErr}
			coordinator := PreInstallMigrationCoordinator{Legacy: &handoffBackup{events: &events}, PM2: &handoffQuiescer{events: &events, stopped: acknowledgedQuiescence()}, Container: container}
			lease, err := coordinator.Begin(context.Background(), BackupRef{}, strings.Repeat("d", 64), DispositionRemove)
			if lease != nil || err == nil || strings.Join(events, ",") != "backup,quiesce,remove,container-recover,pm2-recover" {
				t.Fatalf("lease/events/error = %#v/%q/%v", lease, strings.Join(events, ","), err)
			}
			if tt.recoveryErr != nil && !strings.Contains(err.Error(), DispositionRecoveryUnprovenCode) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
