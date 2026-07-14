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
func TestCorrelatePM2RequiresCanonicalRootAndPort(t *testing.T) {
	records := []PM2Record{{ID: 2, PID: 22, Name: "backend", CWD: backendRoot + "/api", ExecPath: backendRoot + "/api/server", Status: "online"}, {ID: 1, PID: 11, Name: "guardian", CWD: guardianRoot, ExecPath: guardianRoot + "/app", Status: "online"}}
	proc := map[int]ProcIdentity{11: {CWD: guardianRoot, ExecPath: guardianRoot + "/app", StartTicks: 10}, 22: {CWD: backendRoot + "/api", ExecPath: backendRoot + "/api/server", StartTicks: 20}}
	got, err := CorrelatePM2(records, []SocketOwner{{PID: 11, Port: 8080}, {PID: 22, Port: 9090}}, proc)
	if err != nil || len(got) != 2 || got[0].PMID != 1 || got[1].Port != 9090 {
		t.Fatalf("correlation = %#v, %v", got, err)
	}
	_, err = CorrelatePM2([]PM2Record{{ID: 3, PID: 33, CWD: guardianRoot + "-old", ExecPath: guardianRoot + "-old/app", Status: "online"}}, []SocketOwner{{PID: 33, Port: 8080}}, map[int]ProcIdentity{33: {CWD: guardianRoot + "-old", ExecPath: guardianRoot + "-old/app", StartTicks: 30}})
	if err == nil {
		t.Fatal("prefix collision qualified")
	}
}
func TestPM2QuiescerProvesFullStoppedSetAndRejectsRespawn(t *testing.T) {
	before := PM2Snapshot{Records: []PM2Record{{ID: 1, PID: 11, Name: "guardian", CWD: guardianRoot, ExecPath: guardianRoot + "/app", Status: "online"}, {ID: 2, PID: 22, Name: "backend", CWD: backendRoot, ExecPath: backendRoot + "/app", Status: "online"}}, Sockets: []SocketOwner{{PID: 11, Port: 8080}, {PID: 22, Port: 9090}}, Proc: map[int]ProcIdentity{11: {CWD: guardianRoot, ExecPath: guardianRoot + "/app", StartTicks: 9}, 22: {CWD: backendRoot, ExecPath: backendRoot + "/app", StartTicks: 10}}}
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
func TestPM2QuiescerRecoverUsesOnlyAcknowledgedIdentitiesInReverseStopOrder(t *testing.T) {
	first, second := PM2ProcessIdentity{PMID: 1, PID: 11, CWD: guardianRoot, ExecPath: guardianRoot + "/app", Port: 8080, StartTicks: 10}, PM2ProcessIdentity{PMID: 2, PID: 22, CWD: backendRoot, ExecPath: backendRoot + "/app", Port: 9090, StartTicks: 20}
	stopped := PM2Quiescence{Processes: []PM2ProcessIdentity{first, second}, Evidence: []PM2StoppedEvidence{{PMID: 1, OriginalPID: 11, Port: 8080, StartTicks: 10, StopVerified: true}, {PMID: 2, OriginalPID: 22, Port: 9090, StartTicks: 20, StopVerified: true}}}
	runner := &recoveryRunner{onRun: func() { stopped.Processes[0].PMID = 999 }}
	q := PM2Quiescer{Snapshots: &snapshotSequence{items: []PM2Snapshot{
		recoverySnapshot(second, "stopped", 22, 20), recoverySnapshot(second, "online", 222, 200),
		recoverySnapshot(first, "stopped", 11, 10), recoverySnapshot(first, "online", 111, 100),
	}}, Controller: PM2Controller{Runner: runner}}
	recovery, err := q.Recover(context.Background(), stopped)
	if err != nil || !recovery.Verified || recovery.Attempted != 2 || recovery.Recovered != 2 {
		t.Fatalf("recovery = %#v, %v", recovery, err)
	}
	if got, want := strings.Join(runner.commands, ","), "start:2,start:1"; got != want {
		t.Fatalf("start selectors = %q, want %q", got, want)
	}
}
func TestPM2QuiescerRecoverRejectsUnsafeRecoveryBoundaries(t *testing.T) {
	target := PM2ProcessIdentity{PMID: 2, PID: 22, CWD: backendRoot, ExecPath: backendRoot + "/app", Port: 9090, StartTicks: 20}
	stopped := PM2Quiescence{Processes: []PM2ProcessIdentity{target}, Evidence: []PM2StoppedEvidence{{PMID: 2, OriginalPID: 22, Port: 9090, StartTicks: 20, StopVerified: true}}}
	drift := recoverySnapshot(target, "stopped", 22, 20)
	drift.Records[0].CWD = backendRoot + "/other"
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
	assertUnsafe("competing port owner", []PM2Snapshot{competing}, &recoveryRunner{}, 0)
	assertUnsafe("failed start has no retry", []PM2Snapshot{recoverySnapshot(target, "stopped", 22, 20)}, &recoveryRunner{err: errors.New("failed")}, 1)
	assertUnsafe("original pid is not a new process", []PM2Snapshot{recoverySnapshot(target, "stopped", 22, 20), recoverySnapshot(target, "online", 22, 20)}, &recoveryRunner{}, 1)
	assertUnsafe("reused start ticks", []PM2Snapshot{recoverySnapshot(target, "stopped", 22, 20), recoverySnapshot(target, "online", 222, 20)}, &recoveryRunner{}, 1)
}
func recoverySnapshot(identity PM2ProcessIdentity, status string, pid int, ticks uint64) PM2Snapshot {
	snapshot := PM2Snapshot{Records: []PM2Record{{ID: identity.PMID, PID: pid, CWD: identity.CWD, ExecPath: identity.ExecPath, Status: status}}}
	if status == "online" {
		snapshot.Sockets = []SocketOwner{{PID: pid, Port: identity.Port}}
		snapshot.Proc = map[int]ProcIdentity{pid: {CWD: identity.CWD, ExecPath: identity.ExecPath, StartTicks: ticks}}
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
	next  int
}

func (s *snapshotSequence) Snapshot(context.Context) (PM2Snapshot, error) {
	value := s.items[s.next]
	s.next++
	return value, nil
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
