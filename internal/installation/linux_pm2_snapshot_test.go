package installation

import (
	"context"
	"errors"
	"testing"
)

func TestLinuxPM2SnapshotProvider(t *testing.T) {
	record := PM2Record{ID: 1, PID: 41, Name: "front-guardian", CWD: guardianRoot, ExecPath: "/usr/bin/bash", Status: "online"}
	identity := ProcIdentity{CWD: record.CWD, ExecPath: "/usr/local/bin/node", StartTicks: 9}

	t.Run("assembles each candidate", func(t *testing.T) {
		provider := snapshotProvider(record, identity)
		snapshot, err := provider.Snapshot(context.Background())
		if err != nil || len(snapshot.Records) != 1 || len(snapshot.Sockets) != 1 || snapshot.Proc[record.PID] != identity {
			t.Fatalf("Snapshot() = %#v, %v; want complete evidence", snapshot, err)
		}
	})

	for _, tt := range []struct {
		name string
		proc procIdentityFunc
	}{
		{"missing identity", func(context.Context, int) (ProcIdentity, error) { return ProcIdentity{}, errors.New("missing") }},
		{"changed cwd", func(context.Context, int) (ProcIdentity, error) {
			return ProcIdentity{CWD: "/changed", ExecPath: record.ExecPath, StartTicks: 9}, nil
		}},
	} {
		t.Run(tt.name+" prevents mutation", func(t *testing.T) {
			provider := snapshotProvider(record, identity)
			provider.Proc = tt.proc
			runner := &snapshotRunner{}
			if _, err := (PM2Quiescer{Snapshots: provider, Controller: PM2Controller{Runner: runner}}).Quiesce(context.Background()); err == nil || runner.calls != 0 {
				t.Fatalf("Quiesce() error/calls = %v/%d, want fail-closed/0", err, runner.calls)
			} else if diagnostic := withObservationStage(err, "initial-snapshot"); diagnostic == nil || diagnostic.Stage != "initial-snapshot" || diagnostic.Operation == "" || diagnostic.Cause == "" || diagnostic.Command != "" {
				t.Fatalf("diagnostic = %#v, want bounded non-command identity diagnostic", diagnostic)
			}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := snapshotProvider(record, identity).Snapshot(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Snapshot() error = %v, want context.Canceled", err)
	}
}

func snapshotProvider(record PM2Record, identity ProcIdentity) LinuxPM2SnapshotProvider {
	return LinuxPM2SnapshotProvider{
		Inventory: pm2InventoryFunc(func(context.Context) ([]PM2Record, error) { return []PM2Record{record}, nil }),
		Sockets:   socketSnapshotFunc(func(context.Context) ([]SocketOwner, error) { return []SocketOwner{{PID: record.PID, Port: 8080}}, nil }),
		Proc:      procIdentityFunc(func(context.Context, int) (ProcIdentity, error) { return identity, nil }),
	}
}

type pm2InventoryFunc func(context.Context) ([]PM2Record, error)

func (f pm2InventoryFunc) Snapshot(ctx context.Context) ([]PM2Record, error) { return f(ctx) }

type socketSnapshotFunc func(context.Context) ([]SocketOwner, error)

func (f socketSnapshotFunc) Snapshot(ctx context.Context) ([]SocketOwner, error) { return f(ctx) }

type procIdentityFunc func(context.Context, int) (ProcIdentity, error)

func (f procIdentityFunc) Read(ctx context.Context, pid int) (ProcIdentity, error) {
	return f(ctx, pid)
}

type snapshotRunner struct{ calls int }

func (r *snapshotRunner) Run(context.Context, string, ...string) ([]byte, []byte, error) {
	r.calls++
	return nil, nil, errors.New("must not run")
}
