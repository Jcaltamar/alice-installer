package installation

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type rootCall struct {
	name string
	args []string
}
type rootRunner struct {
	calls   []rootCall
	outputs [][]byte
	failAt  int
	stderr  []byte
	err     error
}

func (r *rootRunner) Run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, rootCall{name, append([]string(nil), args...)})
	if r.failAt == len(r.calls) {
		return []byte("PM2_STDOUT_SECRET"), r.stderr, r.err
	}
	if len(r.outputs) == 0 {
		return nil, nil, nil
	}
	out := r.outputs[0]
	r.outputs = r.outputs[1:]
	return out, nil, nil
}

func TestRootPM2BoundaryReportsOnlyAllowlistedObservationDiagnostics(t *testing.T) {
	stat := []byte("1230 (node) S 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 42")
	for _, tt := range []struct {
		name      string
		operation string
		command   string
		stderr    []byte
		invoke    func(RootPM2Boundary) error
		runner    *rootRunner
	}{
		{name: "pm2 inventory", operation: "pm2-jlist", command: "sudo -n pm2 jlist", stderr: []byte("sudo: a password is required\n"), runner: &rootRunner{failAt: 1, err: errors.New("exit status 1")}, invoke: func(b RootPM2Boundary) error { _, _, err := b.Run(context.Background(), "pm2", "jlist"); return err }},
		{name: "socket listeners", operation: "socket-listeners", command: "sudo -n ss -H -ltnp", stderr: []byte("TOKEN=secret"), runner: &rootRunner{failAt: 1, err: errors.New("exit status 2")}, invoke: func(b RootPM2Boundary) error {
			_, _, err := b.Run(context.Background(), "ss", "-H", "-ltnp")
			return err
		}},
		{name: "proc cwd", operation: "proc-cwd", command: "sudo -n readlink /proc/1230/cwd", stderr: []byte("DATABASE_URL=postgres://secret"), runner: &rootRunner{failAt: 1, err: errors.New("failed")}, invoke: func(b RootPM2Boundary) error { _, err := b.Read(context.Background(), 1230); return err }},
		{name: "proc executable", operation: "proc-exe", command: "sudo -n readlink /proc/1230/exe", stderr: []byte(strings.Repeat("x", 257)), runner: &rootRunner{outputs: [][]byte{[]byte("/opt/alice-guardian")}, failAt: 2, err: errors.New("failed")}, invoke: func(b RootPM2Boundary) error { _, err := b.Read(context.Background(), 1230); return err }},
		{name: "proc stat", operation: "proc-stat", command: "sudo -n cat /proc/1230/stat", stderr: []byte("arbitrary process detail"), runner: &rootRunner{outputs: [][]byte{[]byte("/opt/alice-guardian"), []byte("/usr/bin/node"), stat}, failAt: 3, err: errors.New("failed")}, invoke: func(b RootPM2Boundary) error { _, err := b.Read(context.Background(), 1230); return err }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.runner.stderr = tt.stderr
			err := tt.invoke(RootPM2Boundary{Runner: tt.runner})
			var observation pm2ObservationError
			if !errors.As(err, &observation) {
				t.Fatalf("error = %v, want observation diagnostic", err)
			}
			if observation.Diagnostic.Operation != tt.operation || observation.Diagnostic.Command != tt.command {
				t.Fatalf("diagnostic = %#v", observation.Diagnostic)
			}
			text := err.Error()
			for _, forbidden := range []string{"PM2_STDOUT_SECRET", "TOKEN=", "DATABASE_URL", "postgres://", "arbitrary process detail", strings.Repeat("x", 257)} {
				if strings.Contains(text, forbidden) {
					t.Fatalf("diagnostic leaked %q: %q", forbidden, text)
				}
			}
		})
	}
}

func TestObservationDiagnosticClassifiesBoundedCauses(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	timedOut, timeout := context.WithTimeout(context.Background(), 0)
	defer timeout()
	for _, tt := range []struct {
		name string
		ctx  context.Context
		err  error
		want string
	}{
		{name: "exit", ctx: context.Background(), err: errors.New("exit status 17"), want: "exit-17"},
		{name: "cancelled", ctx: cancelled, err: context.Canceled, want: "cancelled"},
		{name: "timeout", ctx: timedOut, err: context.DeadlineExceeded, want: "timeout"},
		{name: "execution failure", ctx: context.Background(), err: errors.New("secret internal error"), want: "execution-failed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := observationCause(tt.ctx, tt.err); got != tt.want {
				t.Fatalf("cause = %q, want %q", got, tt.want)
			}
		})
	}
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
		if _, err := b.read(context.Background(), "proc-stat", "cat", path); err == nil {
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

func TestRootPM2BoundaryClassifiesUnusableProcOutput(t *testing.T) {
	stat := []byte("1230 (node) S 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 42")
	for _, tt := range []struct {
		name      string
		outputs   [][]byte
		operation string
		command   string
		cause     string
	}{
		{name: "invalid cwd", outputs: [][]byte{[]byte("relative-secret")}, operation: "proc-cwd", command: "sudo -n readlink /proc/1230/cwd", cause: "output-invalid"},
		{name: "oversized cwd", outputs: [][]byte{[]byte("/" + strings.Repeat("S", defaultProcStatLimit+1))}, operation: "proc-cwd", command: "sudo -n readlink /proc/1230/cwd", cause: "output-too-large"},
		{name: "invalid executable", outputs: [][]byte{[]byte("/opt/alice"), []byte("relative-secret")}, operation: "proc-exe", command: "sudo -n readlink /proc/1230/exe", cause: "output-invalid"},
		{name: "oversized stat", outputs: [][]byte{[]byte("/opt/alice"), []byte("/usr/bin/node"), []byte(strings.Repeat("S", defaultProcStatLimit+1))}, operation: "proc-stat", command: "sudo -n cat /proc/1230/stat", cause: "output-too-large"},
		{name: "invalid stat", outputs: [][]byte{[]byte("/opt/alice"), []byte("/usr/bin/node"), append(stat[:10], []byte(" DATABASE_URL=postgres://secret")...)}, operation: "proc-stat", command: "sudo -n cat /proc/1230/stat", cause: "output-invalid"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := (RootPM2Boundary{Runner: &rootRunner{outputs: tt.outputs}}).Read(context.Background(), 1230)
			var observation pm2ObservationError
			if !errors.As(err, &observation) {
				t.Fatalf("error = %v, want observation diagnostic", err)
			}
			diagnostic := observation.Diagnostic
			if diagnostic.Operation != tt.operation || diagnostic.Command != tt.command || diagnostic.Cause != tt.cause {
				t.Fatalf("diagnostic = %#v", diagnostic)
			}
			if strings.Contains(err.Error(), "DATABASE_URL") || strings.Contains(err.Error(), "postgres://secret") || strings.Contains(err.Error(), "relative-secret") {
				t.Fatalf("diagnostic leaked command output: %q", err)
			}
		})
	}
}
