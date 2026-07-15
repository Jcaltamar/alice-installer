package installation

import (
	"context"
	"reflect"
	"testing"
)

type rootCall struct {
	name string
	args []string
}
type rootRunner struct {
	calls   []rootCall
	outputs [][]byte
}

func (r *rootRunner) Run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, rootCall{name, append([]string(nil), args...)})
	if len(r.outputs) == 0 {
		return nil, nil, nil
	}
	out := r.outputs[0]
	r.outputs = r.outputs[1:]
	return out, nil, nil
}

func TestRootPM2BoundaryUsesExactSudoArgv(t *testing.T) {
	runner := &rootRunner{}
	boundary := RootPM2Boundary{Runner: runner}
	_, _, _ = boundary.Run(context.Background(), "pm2", "jlist")
	_, _, _ = boundary.Run(context.Background(), "ss", "-H", "-ltnp")
	id := PM2ProcessIdentity{PMID: 0, PID: 11174, CWD: backendRoot + "/node", ExecPath: backendRoot + "/node/bin/www", RuntimeExecPath: "/usr/local/bin/node", Port: 9090, StartTicks: 9}
	_ = boundary.mutate(context.Background(), "stop", id)
	_ = boundary.mutate(context.Background(), "start", id)
	want := []rootCall{{"sudo", []string{"-n", "pm2", "jlist"}}, {"sudo", []string{"-n", "ss", "-H", "-ltnp"}}, {"sudo", []string{"-n", "pm2", "stop", "0"}}, {"sudo", []string{"-n", "pm2", "start", "0"}}}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v", runner.calls)
	}
}

func TestRootPM2BoundaryRejectsForbiddenCommandsAndPaths(t *testing.T) {
	runner := &rootRunner{}
	b := RootPM2Boundary{Runner: runner}
	for _, command := range []struct {
		name string
		args []string
	}{{"sh", []string{"-c", "pm2 jlist"}}, {"pm2", []string{"stop", "1"}}, {"pm2", []string{"jlist", "extra"}}, {"ss", []string{"-ltnp"}}, {"cat", []string{"/etc/passwd"}}} {
		if _, _, err := b.Run(context.Background(), command.name, command.args...); err == nil {
			t.Fatalf("accepted %#v", command)
		}
	}
	for _, path := range []string{"/proc/0/stat", "/proc/-1/stat", "/proc/01/stat", "/proc/1/../stat", "/proc/1/environ", "/etc/passwd"} {
		if _, err := b.read(context.Background(), "cat", path); err == nil {
			t.Fatalf("accepted %q", path)
		}
	}
	for _, id := range []PM2ProcessIdentity{{PMID: -1}, {PMID: 1, PID: 2}} {
		if err := b.mutate(context.Background(), "stop", id); err == nil {
			t.Fatalf("accepted %#v", id)
		}
	}
	if len(runner.calls) != 0 {
		t.Fatalf("executed forbidden calls: %#v", runner.calls)
	}
}

func TestRootPM2BoundaryReadsExactProcIdentity(t *testing.T) {
	stat := []byte("1230 (node) S 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 42")
	runner := &rootRunner{outputs: [][]byte{[]byte("/opt/alice-guardian\n"), []byte("/usr/bin/node\n"), stat}}
	identity, err := (RootPM2Boundary{Runner: runner}).Read(context.Background(), 1230)
	if err != nil || identity.StartTicks != 42 {
		t.Fatalf("identity = %#v, %v", identity, err)
	}
	want := []rootCall{{"sudo", []string{"-n", "readlink", "/proc/1230/cwd"}}, {"sudo", []string{"-n", "readlink", "/proc/1230/exe"}}, {"sudo", []string{"-n", "cat", "/proc/1230/stat"}}}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v", runner.calls)
	}
}
