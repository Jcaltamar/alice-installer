package migration

import (
	"context"
	"errors"
	"github.com/jcaltamar/alice-installer/internal/installation"
	"strings"
	"testing"
)

func TestPreInstallMigrationCoordinatorBeginRequiresBackupCompleteQuiescenceAndCompensatesPartialStop(t *testing.T) {
	events := []string{}
	backup, quiescer := &handoffBackup{events: &events}, &handoffQuiescer{events: &events, stopped: acknowledgedQuiescence()}
	coordinator := PreInstallMigrationCoordinator{Legacy: backup, PM2: quiescer}
	lease, err := coordinator.Begin(context.Background(), BackupRef{DumpPath: "/opt/alice/backups/legacy.dump"})
	if err != nil || lease == nil {
		t.Fatalf("lease = %#v, err = %v", lease, err)
	}
	if got, want := strings.Join(events, ","), "backup,quiesce"; got != want {
		t.Fatalf("events = %q, want %q", got, want)
	}
	failedEvents := []string{}
	failedBackup := &handoffBackup{events: &failedEvents, err: errors.New("invalid")}
	if lease, err := (&PreInstallMigrationCoordinator{Legacy: failedBackup, PM2: quiescer}).Begin(context.Background(), BackupRef{}); err == nil || lease != nil || strings.Join(failedEvents, ",") != "backup" {
		t.Fatalf("backup failure lease = %#v, events = %q, err = %v", lease, strings.Join(failedEvents, ","), err)
	}
	quiescer.stopped.Evidence = nil
	if lease, err := coordinator.Begin(context.Background(), BackupRef{}); err == nil || lease != nil {
		t.Fatalf("incomplete lease = %#v, err = %v", lease, err)
	}
	quiescer.stopped, quiescer.err = acknowledgedQuiescence(), errors.New("partial stop")
	if lease, err := coordinator.Begin(context.Background(), BackupRef{}); err == nil || lease != nil || quiescer.recoverCalls != 1 {
		t.Fatalf("partial lease = %#v, recoveries = %d, err = %v", lease, quiescer.recoverCalls, err)
	}
}
func TestPreInstallMigrationCoordinatorCompletionConsumesLeaseOnce(t *testing.T) {
	backup, quiescer := &handoffBackup{}, &handoffQuiescer{stopped: acknowledgedQuiescence()}
	contexts := 0
	coordinator := PreInstallMigrationCoordinator{Legacy: backup, PM2: quiescer, RecoveryContext: func() (context.Context, context.CancelFunc) {
		contexts++
		return context.WithCancel(context.Background())
	}}
	success, err := coordinator.Begin(context.Background(), BackupRef{})
	if err != nil || coordinator.CompleteSuccess(success) != nil || coordinator.CompleteSuccess(success) != nil || quiescer.recoverCalls != 0 {
		t.Fatalf("success completion err = %v, recoveries = %d", err, quiescer.recoverCalls)
	}
	failure, err := coordinator.Begin(context.Background(), BackupRef{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.CompleteFailure(failure); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.CompleteFailure(failure); err != nil || quiescer.recoverCalls != 1 || contexts != 1 {
		t.Fatalf("duplicate failure err = %v, recoveries = %d, contexts = %d", err, quiescer.recoverCalls, contexts)
	}
}
func acknowledgedQuiescence() installation.PM2Quiescence {
	return installation.PM2Quiescence{Processes: []installation.PM2ProcessIdentity{{PMID: 1, PID: 10, CWD: "/opt/alice-guardian", ExecPath: "/opt/alice-guardian/app", Port: 8080, StartTicks: 1}}, Evidence: []installation.PM2StoppedEvidence{{PMID: 1, OriginalPID: 10, Port: 8080, StartTicks: 1, StopVerified: true}}}
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
	events       *[]string
	stopped      installation.PM2Quiescence
	recoverCalls int
	err          error
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
	return installation.PM2Recovery{Attempted: 1, Recovered: 1, Verified: true}, nil
}
