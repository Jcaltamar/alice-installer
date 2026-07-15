package installation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestLinuxPM2InventoryUsesFixedBoundedRedactedAcquisition(t *testing.T) {
	const secret = "PM2_STDOUT_SECRET"
	valid := []byte(`[{"pm_id":7,"pid":70,"name":"guardian","pm_exec_path":"/opt/alice-guardian/app","pm2_env":{"cwd":"/opt/alice-guardian","status":"online"}}]`)
	for _, tt := range []struct {
		name    string
		runner  *adapterRunner
		limit   int
		wantErr string
	}{
		{name: "valid inventory", runner: &adapterRunner{stdout: valid}},
		{name: "mixed record evidence", runner: &adapterRunner{stdout: []byte(`[{"pm_id":7,"pid":70,"pm2_env":{"pid":71,"cwd":"/opt/alice-guardian","status":"online"},"name":"guardian","pm_exec_path":"/opt/alice-guardian/app"}]`)}, wantErr: "pm2 inventory output is invalid"},
		{name: "bounded output", runner: &adapterRunner{stdout: []byte(secret)}, limit: len(secret) - 1, wantErr: "pm2 inventory output exceeded limit"},
		{name: "tool failure is redacted", runner: &adapterRunner{stderr: []byte(secret), err: errors.New(secret)}, wantErr: "pm2 inventory command failed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			inventory := LinuxPM2Inventory{Runner: tt.runner, Timeout: time.Second, MaxOutput: tt.limit}
			records, err := inventory.Snapshot(context.Background())
			if errText := errorText(err); errText != tt.wantErr {
				t.Fatalf("error = %q, want %q", errText, tt.wantErr)
			}
			if strings.Contains(errorText(err), secret) {
				t.Fatal("raw command output leaked through error")
			}
			if tt.wantErr == "pm2 inventory output is invalid" || tt.wantErr == "pm2 inventory output exceeded limit" {
				var observation pm2ObservationError
				if !errors.As(err, &observation) {
					t.Fatalf("error = %v, want observation diagnostic", err)
				}
				wantCause := "output-invalid"
				if tt.wantErr == "pm2 inventory output exceeded limit" {
					wantCause = "output-too-large"
				}
				if observation.Diagnostic.Operation != "pm2-jlist" || observation.Diagnostic.Command != "sudo -n pm2 jlist" || observation.Diagnostic.Cause != wantCause {
					t.Fatalf("diagnostic = %#v", observation.Diagnostic)
				}
			}
			if tt.runner.name != "pm2" || tt.runner.args != "jlist" {
				t.Fatalf("command = %q %q", tt.runner.name, tt.runner.args)
			}
			if tt.wantErr == "" && (len(records) != 1 || records[0].ID != 7 || records[0].PID != 70) {
				t.Fatalf("records = %#v", records)
			}
		})
	}
}
func TestLinuxPM2InventoryHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := &adapterRunner{}
	_, err := (LinuxPM2Inventory{Runner: runner}).Snapshot(ctx)
	if got := errorText(err); got != "pm2 inventory cancelled" {
		t.Fatalf("error = %q", got)
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d", runner.calls)
	}
}
func TestLinuxPM2InventoryRejectsCompletedOutputAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	runner := &adapterRunner{stdout: []byte(`[{"pm_id":7,"pid":70,"name":"guardian","pm_exec_path":"/opt/alice-guardian/app","pm2_env":{"cwd":"/opt/alice-guardian","status":"online"}}]`), onRun: cancel}
	_, err := (LinuxPM2Inventory{Runner: runner, Timeout: time.Second}).Snapshot(ctx)
	if got := errorText(err); got != "pm2 inventory cancelled" {
		t.Fatalf("error = %q", got)
	}
}
func TestLinuxPM2InventoryRejectsCompletedOutputAfterTimeout(t *testing.T) {
	runner := &adapterRunner{stdout: []byte(`[{"pm_id":7,"pid":70,"name":"guardian","pm_exec_path":"/opt/alice-guardian/app","pm2_env":{"cwd":"/opt/alice-guardian","status":"online"}}]`), waitForContext: true}
	_, err := (LinuxPM2Inventory{Runner: runner, Timeout: time.Millisecond}).Snapshot(context.Background())
	if got := errorText(err); got != "pm2 inventory timed out" {
		t.Fatalf("error = %q", got)
	}
}
func TestPM2InventoryRejectsInvalidOrPartialRecords(t *testing.T) {
	for _, input := range []string{`[{"pm_id":1,"pid":0,"name":"a","pm_exec_path":"/a","pm2_env":{"cwd":"/a","status":"online"}}]`, `[{"pm_id":1,"pid":10,"name":"a","pm_exec_path":"/a","pm2_env":{"cwd":"/a","status":"online"}},{"pm_id":1,"pid":11,"name":"b","pm_exec_path":"/b","pm2_env":{"cwd":"/b","status":"online"}}]`, `[{"pm_id":1,"pid":10,"name":"a","pm_exec_path":"/a","pm2_env":{"cwd":"/a","status":"online"}},{}]`} {
		if _, err := ParsePM2Inventory([]byte(input)); err == nil {
			t.Fatal("unsafe inventory accepted")
		}
	}
	if owners, err := ParseSocketSnapshot([]byte("LISTEN 0 0 *:8080 *:* users:((\"node\",pid=12,fd=1))\n")); err != nil || len(owners) != 1 || owners[0].PID != 12 {
		t.Fatal("valid socket evidence rejected")
	}
	if _, err := ParseSocketSnapshot([]byte("LISTEN 0 0 *:8080 *:*\n")); err == nil {
		t.Fatal("missing owner accepted")
	}
	if ticks, err := ParseProcStartTicks([]byte("1 (node) S 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 42")); err != nil || ticks != 42 {
		t.Fatal("valid start ticks rejected")
	}
	if _, err := ParseProcStartTicks([]byte("1 (node) S")); err == nil {
		t.Fatal("invalid start ticks accepted")
	}
}
func TestPM2InventoryAcceptsPMCWDButRejectsConflict(t *testing.T) {
	valid := []byte(`[{"pm_id":1,"pid":1230,"name":"front-guardian","pm_exec_path":"/usr/bin/node","pm2_env":{"pm_cwd":"/opt/alice-guardian","status":"online"}}]`)
	records, err := ParsePM2Inventory(valid)
	if err != nil || len(records) != 1 || records[0].CWD != guardianRoot {
		t.Fatalf("records = %#v, %v", records, err)
	}
	conflict := []byte(`[{"pm_id":1,"pid":1230,"name":"front-guardian","pm_exec_path":"/usr/bin/node","pm2_env":{"cwd":"/opt/alice-guardian","pm_cwd":"/opt/backend_alice_guardian","status":"online"}}]`)
	if _, err := ParsePM2Inventory(conflict); err == nil {
		t.Fatal("conflicting cwd fields accepted")
	}
}

func TestCorrelatePM2SelectsOnlyProductionEligibleIdentities(t *testing.T) {
	records := []PM2Record{
		{ID: 0, PID: 20, Name: "pm2-logrotate", CWD: "/root/.pm2/modules/pm2-logrotate", ExecPath: "/usr/bin/node", Status: "online"},
		{ID: 1, PID: 1230, Name: "front-guardian", CWD: guardianRoot, ExecPath: "/usr/bin/bash", Status: "online"},
		{ID: 2, PID: 1231, Name: "ws", CWD: backendRoot + "/websocket", ExecPath: "/usr/bin/bash", Status: "online"},
		{ID: 5, PID: 11174, Name: "node", CWD: backendRoot + "/node", ExecPath: backendRoot + "/node/bin/www", Status: "online"},
		{ID: 3, PID: 30, Name: "queue", CWD: backendRoot + "/queue", ExecPath: "/usr/bin/node", Status: "online"},
		{ID: 4, PID: 40, Name: "frontend-tec", CWD: guardianRoot, ExecPath: "/usr/bin/node", Status: "online"},
		{ID: 6, PID: 60, Name: "ws", CWD: backendRoot + "/node", ExecPath: "/usr/bin/node", Status: "online"},
		{ID: 7, PID: 70, Name: "node", CWD: backendRoot + "/node", ExecPath: "/usr/bin/node", Status: "online"},
		{ID: 8, PID: 80, Name: "front-guardian", CWD: guardianRoot + "-old", ExecPath: "/usr/bin/node", Status: "online"},
		{ID: 9, PID: 90, Name: "other", CWD: guardianRoot, ExecPath: "/usr/bin/node", Status: "online"},
	}
	sockets := []SocketOwner{{1230, 8080}, {1231, 4550}, {11174, 9090}, {30, 10030}, {40, 10040}, {60, 10060}, {70, 10070}, {80, 10080}, {90, 10090}}
	proc := map[int]ProcIdentity{}
	for _, record := range records {
		proc[record.PID] = ProcIdentity{CWD: record.CWD, ExecPath: "/usr/local/bin/node", StartTicks: uint64(record.PID)}
	}
	got, err := CorrelatePM2(records, sockets, proc)
	if err != nil || len(got) != 3 || got[0].PMID != 1 || got[1].PMID != 2 || got[2].PMID != 5 {
		t.Fatalf("selected = %#v, %v", got, err)
	}
	records[0].ID, records[1].ID = -1, 0
	if got, err = CorrelatePM2(records, sockets, proc); err != nil || len(got) != 3 || got[0].PMID != 0 {
		t.Fatalf("zero ID contract = %#v, %v", got, err)
	}
}
func TestCorrelatePM2RejectsExecutableContractDrift(t *testing.T) {
	base := PM2Record{ID: 1, PID: 1230, Name: "front-guardian", CWD: guardianRoot, ExecPath: "/usr/bin/bash", Status: "online"}
	baseProc := ProcIdentity{CWD: guardianRoot, ExecPath: "/usr/local/bin/node", StartTicks: 42}
	for _, tt := range []struct {
		name   string
		record PM2Record
		proc   ProcIdentity
		port   uint16
	}{
		{"swapped PM2 path", PM2Record{ID: 1, PID: 1230, Name: "front-guardian", CWD: guardianRoot, ExecPath: backendRoot + "/node/bin/www", Status: "online"}, baseProc, 8080},
		{"generic PM2 node", PM2Record{ID: 1, PID: 1230, Name: "front-guardian", CWD: guardianRoot, ExecPath: "/usr/bin/node", Status: "online"}, baseProc, 8080},
		{"alternate node path", base, ProcIdentity{CWD: guardianRoot, ExecPath: "/usr/bin/node", StartTicks: 42}, 8080},
		{"alternate shell", PM2Record{ID: 1, PID: 1230, Name: "front-guardian", CWD: guardianRoot, ExecPath: "/bin/bash", Status: "online"}, baseProc, 8080},
		{"near-prefix script", PM2Record{ID: 1, PID: 1230, Name: "node", CWD: backendRoot + "/node", ExecPath: backendRoot + "/node/bin/www-old", Status: "online"}, ProcIdentity{CWD: backendRoot + "/node", ExecPath: "/usr/local/bin/node", StartTicks: 42}, 9090},
		{"wrong runtime", base, ProcIdentity{CWD: guardianRoot, ExecPath: "/usr/local/bin/nodejs", StartTicks: 42}, 8080},
		{"unrelated service", PM2Record{ID: 1, PID: 1230, Name: "guardian", CWD: guardianRoot, ExecPath: "/usr/bin/bash", Status: "online"}, baseProc, 8080},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := CorrelatePM2([]PM2Record{tt.record}, []SocketOwner{{PID: 1230, Port: tt.port}}, map[int]ProcIdentity{1230: tt.proc}); err == nil {
				t.Fatal("unsafe executable contract accepted")
			}
		})
	}
}
func TestCorrelatePM2SelectsExactContractWhenPIDOwnsMultipleApprovedPorts(t *testing.T) {
	record := PM2Record{ID: 1, PID: 1230, Name: "front-guardian", CWD: guardianRoot, ExecPath: "/usr/bin/bash", Status: "online"}
	proc := map[int]ProcIdentity{1230: {CWD: guardianRoot, ExecPath: "/usr/local/bin/node", StartTicks: 42}}
	got, err := CorrelatePM2([]PM2Record{record}, []SocketOwner{{PID: 1230, Port: 9090}, {PID: 1230, Port: 8080}}, proc)
	if err != nil || len(got) != 1 || got[0].Port != 8080 || got[0].PID != 1230 || got[0].PMID != 1 {
		t.Fatalf("selected = %#v, %v", got, err)
	}
}
func TestPM2QuiescerProvesFullStoppedSetAndRejectsRespawn(t *testing.T) {
	before := PM2Snapshot{Records: []PM2Record{{ID: 1, PID: 11, Name: "front-guardian", CWD: guardianRoot, ExecPath: "/usr/bin/bash", Status: "online"}, {ID: 2, PID: 22, Name: "node", CWD: backendRoot + "/node", ExecPath: backendRoot + "/node/bin/www", Status: "online"}}, Sockets: []SocketOwner{{PID: 11, Port: 8080}, {PID: 22, Port: 9090}}, Proc: map[int]ProcIdentity{11: {CWD: guardianRoot, ExecPath: "/usr/local/bin/node", StartTicks: 9}, 22: {CWD: backendRoot + "/node", ExecPath: "/usr/local/bin/node", StartTicks: 10}}}
	afterOne := PM2Snapshot{Records: append([]PM2Record(nil), before.Records...), Sockets: []SocketOwner{{PID: 22, Port: 9090}}, Proc: before.Proc}
	afterOne.Records[0].Status = "stopped"
	afterAll := PM2Snapshot{Records: append([]PM2Record(nil), afterOne.Records...), Proc: before.Proc}
	afterAll.Records[1].Status = "stopped"
	run := func(final PM2Snapshot) (PM2Quiescence, error, *recoveryRunner) {
		runner := &recoveryRunner{}
		stopped, err := (PM2Quiescer{Snapshots: &snapshotSequence{items: []PM2Snapshot{before, before, afterOne, afterOne, afterAll, final}}, Controller: PM2Controller{Runner: runner}}).Quiesce(context.Background())
		return stopped, err, runner
	}
	stopped, err, runner := run(afterAll)
	if err != nil || len(stopped.Evidence) != 2 || !stopped.Evidence[0].StopVerified || !stopped.Evidence[1].StopVerified || strings.Join(runner.commands, ",") != "stop:1,stop:2" {
		t.Fatalf("result = %#v, %v", stopped, err)
	}
	respawn := PM2Snapshot{Records: append([]PM2Record(nil), afterAll.Records...), Sockets: []SocketOwner{{PID: 111, Port: 8080}}, Proc: before.Proc}
	respawn.Records[0].PID, respawn.Records[0].Status = 111, "online"
	stopped, err, _ = run(respawn)
	if err == nil || len(stopped.Evidence) != 2 {
		t.Fatalf("stopped = %#v, err = %v", stopped, err)
	}
}
func TestPM2QuiescerWaitsForDelayedPortRelease(t *testing.T) {
	before := PM2Snapshot{Records: []PM2Record{{ID: 1, PID: 11, Name: "front-guardian", CWD: guardianRoot, ExecPath: "/usr/bin/bash", Status: "online"}}, Sockets: []SocketOwner{{PID: 11, Port: 8080}}, Proc: map[int]ProcIdentity{11: {CWD: guardianRoot, ExecPath: "/usr/local/bin/node", StartTicks: 9}}}
	lingering := PM2Snapshot{Records: append([]PM2Record(nil), before.Records...), Sockets: before.Sockets, Proc: before.Proc}
	lingering.Records[0].Status = "stopped"
	released := PM2Snapshot{Records: append([]PM2Record(nil), lingering.Records...), Proc: before.Proc}
	runner := &recoveryRunner{}

	stopped, err := (PM2Quiescer{Snapshots: &snapshotSequence{items: []PM2Snapshot{before, before, lingering, released, released}}, Controller: PM2Controller{Runner: runner}, StopProofTimeout: 50 * time.Millisecond, StopProofInterval: time.Millisecond}).Quiesce(context.Background())
	if err != nil || len(stopped.Evidence) != 1 || !stopped.Evidence[0].StopVerified {
		t.Fatalf("stopped = %#v, err = %v", stopped, err)
	}
}
func TestPM2QuiescerRecoversExactTargetAfterStopProofTimeout(t *testing.T) {
	target := PM2ProcessIdentity{PMID: 1, Name: "front-guardian", PID: 11, CWD: guardianRoot, ExecPath: "/usr/bin/bash", RuntimeExecPath: "/usr/local/bin/node", Port: 8080, StartTicks: 9}
	before := recoverySnapshot(target, "online", target.PID, target.StartTicks)
	lingering := recoverySnapshot(target, "stopped", target.PID, target.StartTicks)
	lingering.Sockets = []SocketOwner{{PID: target.PID, Port: target.Port}}
	runner := &recoveryRunner{}
	q := PM2Quiescer{Snapshots: &snapshotSequence{items: []PM2Snapshot{before, before, lingering, recoverySnapshot(target, "stopped", target.PID, target.StartTicks), recoverySnapshot(target, "online", 111, 90)}}, Controller: PM2Controller{Runner: runner}, StopProofTimeout: time.Millisecond, StopProofInterval: 10 * time.Millisecond}

	stopped, err := q.Quiesce(context.Background())
	var failure QuiescenceError
	if !errors.As(err, &failure) || failure.Code != "pm2-stop-unproven" || len(stopped.Evidence) != 1 || stopped.Evidence[0].StopVerified {
		t.Fatalf("stopped = %#v, err = %v", stopped, err)
	}
	recovery, err := q.Recover(context.Background(), stopped)
	if err != nil || !recovery.Verified || recovery.Recovered != 1 || strings.Join(runner.commands, ",") != "stop:1,start:1" {
		t.Fatalf("recovery = %#v, commands = %#v, err = %v", recovery, runner.commands, err)
	}
}

func TestPM2QuiescerStopProofDiagnostic(t *testing.T) {
	target := PM2ProcessIdentity{PMID: 1, Name: "front-guardian", PID: 11, CWD: guardianRoot, ExecPath: "/usr/bin/bash", RuntimeExecPath: "/usr/local/bin/node", Port: 8080, StartTicks: 9}
	before := recoverySnapshot(target, "online", target.PID, target.StartTicks)
	secret := "DATABASE_URL=postgres://secret"

	for _, tt := range []struct {
		name      string
		proof     PM2SnapshotProvider
		want      []string
		forbidden []string
	}{
		{
			name:      "observation command failure",
			proof:     &snapshotSequence{items: []PM2Snapshot{before, before, before}, errs: []error{nil, nil, observationCommandError(context.Background(), "socket-list", "sudo -n ss -H -ltnp", []byte(secret), errors.New("exit status 2"))}},
			want:      []string{"pm2-stop-unproven", "stage=stop-proof-snapshot", "operation=socket-list", "command=sudo -n ss -H -ltnp", "cause=exit-2"},
			forbidden: []string{secret, "postgres://secret"},
		},
		{
			name:  "proof timeout without observation failure",
			proof: &snapshotSequence{items: []PM2Snapshot{before, before, before}},
			want:  []string{"pm2-stop-unproven", "stop command succeeded", "proof timed out", "PM2 ID 1", "status stopped", "port release on 8080"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			q := PM2Quiescer{Snapshots: tt.proof, Controller: PM2Controller{Runner: &recoveryRunner{}}, StopProofTimeout: time.Millisecond, StopProofInterval: 10 * time.Millisecond}
			_, err := q.Quiesce(context.Background())
			text := errorText(err)
			for _, want := range tt.want {
				if !strings.Contains(text, want) {
					t.Fatalf("error %q does not contain %q", text, want)
				}
			}
			for _, forbidden := range tt.forbidden {
				if strings.Contains(text, forbidden) {
					t.Fatalf("error leaked %q: %q", forbidden, text)
				}
			}
		})
	}
}
func TestPM2QuiescerRecoverUsesOnlyAcknowledgedIdentitiesInReverseStopOrder(t *testing.T) {
	first, second := PM2ProcessIdentity{PMID: 0, PID: 11, CWD: guardianRoot, ExecPath: "/usr/bin/bash", RuntimeExecPath: "/usr/local/bin/node", Port: 8080, StartTicks: 10}, PM2ProcessIdentity{PMID: 2, PID: 22, CWD: backendRoot + "/node", ExecPath: backendRoot + "/node/bin/www", RuntimeExecPath: "/usr/local/bin/node", Port: 9090, StartTicks: 20}
	stopped := PM2Quiescence{Processes: []PM2ProcessIdentity{first, second}, Evidence: []PM2StoppedEvidence{{PMID: 0, OriginalPID: 11, Port: 8080, StartTicks: 10, StopVerified: true}, {PMID: 2, OriginalPID: 22, Port: 9090, StartTicks: 20, StopVerified: true}}}
	runner := &recoveryRunner{onRun: func() { stopped.Processes[0].PMID = 999 }}
	q := PM2Quiescer{Snapshots: &snapshotSequence{items: []PM2Snapshot{
		recoverySnapshot(second, "stopped", 22, 20), recoverySnapshot(second, "online", 222, 200),
		recoverySnapshot(first, "stopped", 11, 10), recoverySnapshot(first, "online", 111, 100),
	}}, Controller: PM2Controller{Runner: runner}}
	recovery, err := q.Recover(context.Background(), stopped)
	if err != nil || !recovery.Verified || recovery.Attempted != 2 || recovery.Recovered != 2 {
		t.Fatalf("recovery = %#v, %v", recovery, err)
	}
	if got, want := strings.Join(runner.commands, ","), "start:2,start:0"; got != want {
		t.Fatalf("start selectors = %q, want %q", got, want)
	}
}
func TestPM2QuiescerRecoverRejectsUnsafeRecoveryBoundaries(t *testing.T) {
	target := PM2ProcessIdentity{PMID: 2, Name: "node", PID: 22, CWD: backendRoot + "/node", ExecPath: backendRoot + "/node/bin/www", RuntimeExecPath: "/usr/local/bin/node", Port: 9090, StartTicks: 20}
	stopped := PM2Quiescence{Processes: []PM2ProcessIdentity{target}, Evidence: []PM2StoppedEvidence{{PMID: 2, OriginalPID: 22, Port: 9090, StartTicks: 20, StopVerified: true}}}
	zero := PM2ProcessIdentity{PMID: 0, PID: 10, CWD: guardianRoot, ExecPath: "/usr/bin/bash", RuntimeExecPath: "/usr/local/bin/node", Port: 8080, StartTicks: 10}
	if _, ok := acknowledgedRecoveryTargets(PM2Quiescence{Processes: []PM2ProcessIdentity{zero, zero}}); ok {
		t.Fatal("duplicate zero PM2 ID accepted")
	}
	drift := recoverySnapshot(target, "stopped", 22, 20)
	drift.Records[0].CWD = backendRoot + "/other"
	pm2ExecDrift := recoverySnapshot(target, "stopped", 22, 20)
	pm2ExecDrift.Records[0].ExecPath = target.ExecPath + "-old"
	nameDrift := recoverySnapshot(target, "stopped", 22, 20)
	nameDrift.Records[0].Name = "replacement"
	runtimeDrift := recoverySnapshot(target, "online", 222, 200)
	runtimeDrift.Proc[222] = ProcIdentity{CWD: target.CWD, ExecPath: "/usr/bin/node", StartTicks: 200}
	competing := recoverySnapshot(target, "stopped", 22, 20)
	competing.Sockets = []SocketOwner{{PID: 99, Port: 9090}}
	assertUnsafe := func(name string, snapshots []PM2Snapshot, runner *recoveryRunner, attempted int) {
		t.Run(name, func(t *testing.T) {
			q := PM2Quiescer{Snapshots: &snapshotSequence{items: snapshots}, Controller: PM2Controller{Runner: runner}}
			recovery, err := q.Recover(context.Background(), stopped)
			if err == nil || recovery.Verified || recovery.Attempted != attempted || len(runner.commands) != attempted {
				t.Fatalf("recovery = %#v, commands = %#v, err = %v", recovery, runner.commands, err)
			}
		})
	}
	assertUnsafe("selector config drift", []PM2Snapshot{drift}, &recoveryRunner{}, 0)
	assertUnsafe("PM2 executable drift", []PM2Snapshot{pm2ExecDrift}, &recoveryRunner{}, 0)
	assertUnsafe("PM2 name drift", []PM2Snapshot{nameDrift}, &recoveryRunner{}, 0)
	assertUnsafe("competing port owner", []PM2Snapshot{competing}, &recoveryRunner{}, 0)
	assertUnsafe("failed start has no retry", []PM2Snapshot{recoverySnapshot(target, "stopped", 22, 20)}, &recoveryRunner{err: errors.New("failed")}, 1)
	assertUnsafe("original pid is not a new process", []PM2Snapshot{recoverySnapshot(target, "stopped", 22, 20), recoverySnapshot(target, "online", 22, 20)}, &recoveryRunner{}, 1)
	assertUnsafe("reused start ticks", []PM2Snapshot{recoverySnapshot(target, "stopped", 22, 20), recoverySnapshot(target, "online", 222, 20)}, &recoveryRunner{}, 1)
	assertUnsafe("restarted runtime drift", []PM2Snapshot{recoverySnapshot(target, "stopped", 22, 20), runtimeDrift}, &recoveryRunner{}, 1)
}
func recoverySnapshot(identity PM2ProcessIdentity, status string, pid int, ticks uint64) PM2Snapshot {
	snapshot := PM2Snapshot{Records: []PM2Record{{ID: identity.PMID, PID: pid, Name: identity.Name, CWD: identity.CWD, ExecPath: identity.ExecPath, Status: status}}}
	if status == "online" {
		snapshot.Sockets = []SocketOwner{{PID: pid, Port: identity.Port}}
		snapshot.Proc = map[int]ProcIdentity{pid: {CWD: identity.CWD, ExecPath: identity.RuntimeExecPath, StartTicks: ticks}}
	}
	return snapshot
}

type recoveryRunner struct {
	commands []string
	onRun    func()
	err      error
}

func (r *recoveryRunner) Run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	if name != "pm2" || len(args) != 2 || (args[0] != "start" && args[0] != "stop") {
		return nil, nil, errors.New("unexpected recovery command")
	}
	r.commands = append(r.commands, args[0]+":"+args[1])
	if r.onRun != nil {
		r.onRun()
	}
	return nil, nil, r.err
}

type snapshotSequence struct {
	items []PM2Snapshot
	errs  []error
	next  int
}

func (s *snapshotSequence) Snapshot(context.Context) (PM2Snapshot, error) {
	value := s.items[s.next]
	var err error
	if s.next < len(s.errs) {
		err = s.errs[s.next]
	}
	s.next++
	return value, err
}

type adapterRunner struct {
	name, args     string
	stdout, stderr []byte
	err            error
	calls          int
	onRun          func()
	waitForContext bool
}

func (r *adapterRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	r.calls++
	r.name, r.args = name, strings.Join(args, " ")
	if r.waitForContext {
		<-ctx.Done()
	}
	if r.onRun != nil {
		r.onRun()
	}
	return r.stdout, r.stderr, r.err
}
func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
