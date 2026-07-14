package installation

import (
	"context"
	"errors"
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
		{name: "duplicate process identity", runner: &adapterRunner{stdout: []byte("LISTEN 0 4096 *:8080 *:* users:((\"node\",pid=12,fd=1))\nLISTEN 0 4096 *:9090 *:* users:((\"node\",pid=12,fd=1))\n")}, wantErr: "socket snapshot output is invalid"},
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
			if tt.runner.name != "ss" || tt.runner.args != "-H -ltnp" {
				t.Fatalf("command = %q %q", tt.runner.name, tt.runner.args)
			}
			if tt.wantErr == "" && (len(owners) != 1 || owners[0] != (SocketOwner{PID: 12, Port: 8080})) {
				t.Fatalf("owners = %#v", owners)
			}
		})
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
