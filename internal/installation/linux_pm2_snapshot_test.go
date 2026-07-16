package installation

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
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

func TestLinuxPM2SnapshotProviderValidatesStatusAwareProcessIdentity(t *testing.T) {
	online := PM2Record{ID: 1, PID: 41, Name: "front-guardian", CWD: guardianRoot, ExecPath: "/usr/bin/bash", Status: "online"}
	stopped := PM2Record{ID: 2, PID: 0, Name: "node", CWD: backendRoot + "/node", ExecPath: backendRoot + "/node/bin/www", Status: "stopped"}
	reads := 0
	provider := LinuxPM2SnapshotProvider{
		Inventory: pm2InventoryFunc(func(context.Context) ([]PM2Record, error) { return []PM2Record{online, stopped}, nil }),
		Sockets:   socketSnapshotFunc(func(context.Context) ([]SocketOwner, error) { return nil, nil }),
		Proc: procIdentityFunc(func(_ context.Context, pid int) (ProcIdentity, error) {
			reads++
			if pid != online.PID {
				t.Fatalf("proc read PID = %d, want only online PID %d", pid, online.PID)
			}
			return ProcIdentity{CWD: online.CWD, ExecPath: "/usr/local/bin/node", StartTicks: 9}, nil
		}),
	}
	snapshot, err := provider.Snapshot(context.Background())
	if err != nil || reads != 1 || len(snapshot.Proc) != 1 {
		t.Fatalf("Snapshot() = %#v, %v; proc reads = %d", snapshot, err, reads)
	}
	if _, exists := snapshot.Proc[0]; exists {
		t.Fatal("zero PID inserted into process identity map")
	}

	for _, records := range [][]PM2Record{
		{{ID: 1, PID: 0, Name: "front-guardian", CWD: guardianRoot, ExecPath: "/usr/bin/bash", Status: "online"}},
		{{ID: 1, PID: 41, Name: "front-guardian", CWD: guardianRoot, ExecPath: "/usr/bin/bash", Status: "stopped"}},
		{{ID: 1, PID: -1, Name: "front-guardian", CWD: guardianRoot, ExecPath: "/usr/bin/bash", Status: "stopped"}},
		{{ID: 1, PID: 41, Name: "front-guardian", CWD: guardianRoot, ExecPath: "/usr/bin/bash", Status: "launching"}},
		{{ID: 1, PID: 41, Name: "front-guardian", CWD: guardianRoot, ExecPath: "/usr/bin/bash", Status: "online"}, {ID: 2, PID: 41, Name: "node", CWD: backendRoot + "/node", ExecPath: backendRoot + "/node/bin/www", Status: "online"}},
		{{ID: 1, PID: 0, Name: "front-guardian", CWD: guardianRoot, ExecPath: "/usr/bin/bash", Status: "stopped"}, {ID: 1, PID: 0, Name: "node", CWD: backendRoot + "/node", ExecPath: backendRoot + "/node/bin/www", Status: "stopped"}},
	} {
		provider.Inventory = pm2InventoryFunc(func(context.Context) ([]PM2Record, error) { return records, nil })
		if _, err := provider.Snapshot(context.Background()); err == nil {
			t.Fatalf("unsafe records accepted: %#v", records)
		}
	}
}

func TestPM2QuiescerAcceptsStoppedZeroPIDLifecycle(t *testing.T) {
	runner := &pm2LifecycleRunner{}
	provider := LinuxPM2SnapshotProvider{
		Inventory: LinuxPM2Inventory{Runner: runner},
		Sockets: socketSnapshotFunc(func(context.Context) ([]SocketOwner, error) {
			if runner.stopped {
				return nil, nil
			}
			return []SocketOwner{{PID: 41, Port: 8080}}, nil
		}),
		Proc: procIdentityFunc(func(_ context.Context, pid int) (ProcIdentity, error) {
			if pid != 41 {
				return ProcIdentity{}, errors.New("unexpected proc read")
			}
			return ProcIdentity{CWD: guardianRoot, ExecPath: "/usr/local/bin/node", StartTicks: 9}, nil
		}),
	}
	stopped, err := (PM2Quiescer{Snapshots: provider, Controller: PM2Controller{Runner: runner}}).Quiesce(context.Background())
	if err != nil || len(stopped.Evidence) != 1 || !stopped.Evidence[0].StopVerified || runner.stopCalls != 1 {
		t.Fatalf("Quiesce() = %#v, %v; stop calls = %d", stopped, err, runner.stopCalls)
	}
}

func TestPM2QuiescerRequiresSocketEvidenceForAlreadyStoppedServices(t *testing.T) {
	runner := &stoppedPasswordRequiredRunner{}
	root := RootPM2Boundary{Runner: runner}
	provider := LinuxPM2SnapshotProvider{
		Inventory: LinuxPM2Inventory{Runner: root},
		Sockets:   LinuxSocketSnapshot{Runner: root},
		Proc:      root,
	}

	if _, err := (PM2Quiescer{Snapshots: provider, Controller: PM2Controller{Runner: root}}).Quiesce(context.Background()); err == nil {
		t.Fatal("password-required socket observation was accepted")
	}
	if runner.pm2Calls != 1 || runner.socketCalls != 1 {
		t.Fatalf("sudo calls = pm2:%d socket:%d, want one failed snapshot", runner.pm2Calls, runner.socketCalls)
	}
}

type stoppedPasswordRequiredRunner struct {
	pm2Calls, socketCalls int
}

func (r *stoppedPasswordRequiredRunner) Run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	if name != "sudo" || len(args) < 2 || args[0] != "-n" {
		return nil, nil, errors.New("unexpected command")
	}
	if len(args) == 3 && args[1] == "pm2" && args[2] == "jlist" {
		r.pm2Calls++
		output, _ := json.Marshal([]map[string]any{{"pm_id": 1, "pid": 0, "name": "front-guardian", "pm_exec_path": "/usr/bin/bash", "pm2_env": map[string]any{"cwd": guardianRoot, "status": "stopped"}}})
		return output, nil, nil
	}
	if len(args) == 4 && args[1] == "ss" && args[2] == "-H" && args[3] == "-ltnp" {
		r.socketCalls++
		return nil, []byte("sudo: a password is required\n"), errors.New("exit status 1")
	}
	return nil, nil, errors.New("unexpected command")
}

type pm2LifecycleRunner struct {
	stopped   bool
	stopCalls int
}

func (r *pm2LifecycleRunner) Run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	if name != "pm2" {
		return nil, nil, errors.New("unexpected command")
	}
	if len(args) == 1 && args[0] == "jlist" {
		pid, status := 41, "online"
		if r.stopped {
			pid, status = 0, "stopped"
		}
		output, _ := json.Marshal([]map[string]any{{"pm_id": 1, "pid": pid, "name": "front-guardian", "pm_exec_path": "/usr/bin/bash", "pm2_env": map[string]any{"cwd": guardianRoot, "status": status}}})
		return output, nil, nil
	}
	if len(args) == 2 && args[0] == "stop" && args[1] == strconv.Itoa(1) {
		r.stopCalls++
		r.stopped = true
		return nil, nil, nil
	}
	return nil, nil, errors.New("unexpected pm2 command")
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
