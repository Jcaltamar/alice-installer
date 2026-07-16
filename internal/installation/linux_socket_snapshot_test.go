package installation

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLinuxSocketSnapshotUsesFixedBoundedRedactedAcquisition(t *testing.T) {
	const secret = "SS_STDOUT_SECRET"
	valid := []byte("LISTEN 0 4096 *:8080 *:* users:((\"node\",pid=12,fd=1))\n")
	for _, tt := range []struct {
		name    string
		runner  *adapterRunner
		limit   int
		wantErr string
	}{
		{name: "valid socket evidence", runner: &adapterRunner{stdout: valid}},
		{name: "malformed socket evidence", runner: &adapterRunner{stdout: []byte("LISTEN 0 4096 *:8080 *:*\n")}, wantErr: "socket snapshot output is invalid"},
		{name: "duplicate ownership", runner: &adapterRunner{stdout: []byte("LISTEN 0 4096 *:8080 *:* users:((\"node\",pid=12,fd=1))\nLISTEN 0 4096 *:8080 *:* users:((\"node\",pid=13,fd=1))\n")}, wantErr: "socket snapshot output is invalid"},
		{name: "multiple owners on approved line", runner: &adapterRunner{stdout: []byte("LISTEN 0 4096 *:8080 *:* users:((\"node\",pid=12,fd=1),(\"node\",pid=13,fd=2))\n")}, wantErr: "socket snapshot output is invalid"},
		{name: "bounded output", runner: &adapterRunner{stdout: []byte(secret)}, limit: len(secret) - 1, wantErr: "socket snapshot output exceeded limit"},
		{name: "tool failure is redacted", runner: &adapterRunner{stderr: []byte(secret), err: errors.New(secret)}, wantErr: "socket snapshot command failed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := LinuxSocketSnapshot{Runner: tt.runner, Timeout: time.Second, MaxOutput: tt.limit}
			owners, err := snapshot.Snapshot(context.Background())
			if errText := errorText(err); errText != tt.wantErr {
				t.Fatalf("error = %q, want %q", errText, tt.wantErr)
			}
			if strings.Contains(errorText(err), secret) {
				t.Fatal("raw command output leaked through error")
			}
			if tt.wantErr == "socket snapshot output is invalid" || tt.wantErr == "socket snapshot output exceeded limit" {
				var observation pm2ObservationError
				if !errors.As(err, &observation) {
					t.Fatalf("error = %v, want observation diagnostic", err)
				}
				wantCause := "output-invalid"
				if tt.wantErr == "socket snapshot output exceeded limit" {
					wantCause = "output-too-large"
				}
				if observation.Diagnostic.Operation != "socket-listeners" || observation.Diagnostic.Command != "sudo -n ss -H -ltnp" || observation.Diagnostic.Cause != wantCause {
					t.Fatalf("diagnostic = %#v", observation.Diagnostic)
				}
			}
			if tt.runner.name != "ss" || tt.runner.args != "-H -ltnp" {
				t.Fatalf("command = %q %q", tt.runner.name, tt.runner.args)
			}
			if tt.wantErr == "" && (len(owners) != 1 || owners[0] != (SocketOwner{PID: 12, Port: 8080})) {
				t.Fatalf("owners = %#v", owners)
			}
		})
	}
}

func TestParseSocketSnapshotAcceptsNoListeners(t *testing.T) {
	for _, input := range []string{"", " \n\t"} {
		owners, err := ParseSocketSnapshot([]byte(input))
		if err != nil || len(owners) != 0 {
			t.Fatalf("owners = %#v, err = %v", owners, err)
		}
	}
}

func TestParseSocketSnapshotFiltersProductionListeners(t *testing.T) {
	lines := make([]string, 0, 59)
	for i := 0; i < 56; i++ {
		port := 10000 + i%25
		pid := 200 + i%5
		lines = append(lines, "LISTEN 0 4096 *:"+strconv.Itoa(port)+" *:* users:((\"other\",pid="+strconv.Itoa(pid)+",fd=1))")
	}
	lines = append(lines,
		"LISTEN 0 4096 0.0.0.0:8080 0.0.0.0:* users:((\"node\",pid=1230,fd=1))",
		"LISTEN 0 4096 [::]:8080 [::]:* users:((\"node\",pid=1230,fd=2))",
		"an unrelated shape without socket ownership",
	)
	owners, err := ParseSocketSnapshot([]byte(strings.Join(lines, "\n")))
	if err != nil || len(owners) != 1 || owners[0] != (SocketOwner{PID: 1230, Port: 8080}) {
		t.Fatalf("owners = %#v, %v", owners, err)
	}
}

func TestParseSocketSnapshotRejectsUnsafeApprovedEvidence(t *testing.T) {
	for _, input := range []string{
		"LISTEN 0 4096 *:8080 *:* users:((\"node\",pid=12,fd=1))\nLISTEN 0 4096 [::]:8080 [::]:* users:((\"node\",pid=13,fd=2))",
		"LISTEN 0 4096 *:8080 *:* users:((\"node\",pid=12,fd=1))\nLISTEN 0 4096 shifted *:8080 *:* users:((\"node\",pid=13,fd=1))",
		"LISTEN 0 4096 *:8080 *:*",
		"LISTEN 0 4096 *:8080 *:* users:((\"node\",pid=0,fd=1))",
		"LISTEN 0 4096 *:08080 *:* users:((\"node\",pid=12,fd=1))",
		"LISTEN 0 4096 *:8080x *:* users:((\"node\",pid=12,fd=1))",
		"LISTEN *:4550",
		"LISTEN 0 4096 *:9090 *:* users:((\"node\",pid=012,fd=1))",
	} {
		if _, err := ParseSocketSnapshot([]byte(input)); err == nil {
			t.Fatalf("unsafe approved evidence accepted: %q", input)
		}
	}
}

func TestParseSocketSnapshotIgnoresUnrelatedMalformedEvidence(t *testing.T) {
	input := "LISTEN 0 4096 *:8080 *:* users:((\"node\",pid=12,fd=1))\n" +
		"LISTEN 0 malformed unrelated evidence\n" +
		"LISTEN 0 shifted *:18080 *:* users:((\"other\",pid=13,fd=1))"
	owners, err := ParseSocketSnapshot([]byte(input))
	if err != nil || len(owners) != 1 || owners[0] != (SocketOwner{PID: 12, Port: 8080}) {
		t.Fatalf("owners = %#v, %v", owners, err)
	}
}

func TestLinuxSocketSnapshotHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := &adapterRunner{}
	_, err := (LinuxSocketSnapshot{Runner: runner}).Snapshot(ctx)
	if got := errorText(err); got != "socket snapshot cancelled" {
		t.Fatalf("error = %q", got)
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d", runner.calls)
	}
}

func TestLinuxSocketSnapshotRejectsCompletedOutputAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	runner := &adapterRunner{stdout: []byte("LISTEN 0 4096 *:8080 *:* users:((\"node\",pid=12,fd=1))\n"), onRun: cancel}
	_, err := (LinuxSocketSnapshot{Runner: runner, Timeout: time.Second}).Snapshot(ctx)
	if got := errorText(err); got != "socket snapshot cancelled" {
		t.Fatalf("error = %q", got)
	}
}

func TestLinuxSocketSnapshotRejectsCompletedOutputAfterTimeout(t *testing.T) {
	runner := &adapterRunner{stdout: []byte("LISTEN 0 4096 *:8080 *:* users:((\"node\",pid=12,fd=1))\n"), waitForContext: true}
	_, err := (LinuxSocketSnapshot{Runner: runner, Timeout: time.Millisecond}).Snapshot(context.Background())
	if got := errorText(err); got != "socket snapshot timed out" {
		t.Fatalf("error = %q", got)
	}
}
