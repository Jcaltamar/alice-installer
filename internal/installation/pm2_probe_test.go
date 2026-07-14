package installation

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"
)

type fakeRunner struct {
	stdout     []byte
	err        error
	name       string
	args       []string
	deadline   time.Time
	contextErr error
}

func (f *fakeRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	f.name, f.args = name, append([]string(nil), args...)
	f.deadline, _ = ctx.Deadline()
	f.contextErr = ctx.Err()
	return f.stdout, nil, f.err
}

func TestPM2Probe(t *testing.T) {
	policy := LegacyPolicy{ProcessNames: []string{"alice"}, DeploymentRoots: []string{"/opt/alice"}}
	tests := []struct {
		name, output string
		err          error
		want         Presence
		kind         EvidenceKind
	}{
		{"confirmed process and path", `[{"name":"alice","pm_exec_path":"/opt/alice/server.js","pm2_env":{"cwd":"/opt/alice"}}]`, nil, PresencePresent, EvidencePM2AliceProcess},
		{"unrelated process", `[{"name":"other","pm_exec_path":"/opt/other/server.js","pm2_env":{"cwd":"/opt/other"}}]`, nil, PresenceAbsent, EvidencePM2Absent},
		{"weak name without root", `[{"name":"alice","pm_exec_path":"/tmp/server.js","pm2_env":{"cwd":"/tmp"}}]`, nil, PresenceUncertain, EvidencePM2Ambiguous},
		{"malformed output", `{`, nil, PresenceUncertain, EvidencePM2Failed},
		{"null output", `null`, nil, PresenceUncertain, EvidencePM2Failed},
		{"not found", ``, exec.ErrNotFound, PresenceAbsent, EvidencePM2Unavailable},
		{"process uses deployment root as cwd", `[{"name":"alice","pm2_env":{"cwd":"/opt/alice"}}]`, nil, PresencePresent, EvidencePM2AliceProcess},
		{"prefix collision is not a deployment root", `[{"name":"alice","pm_exec_path":"/opt/alice-other/server.js","pm2_env":{"cwd":"/opt/alice-other"}}]`, nil, PresenceUncertain, EvidencePM2Ambiguous},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &fakeRunner{stdout: []byte(tt.output), err: tt.err}
			got := PM2Probe{Runner: runner, Platform: Platform{GOOS: "linux", GOARCH: "amd64"}, Policy: policy}.Probe(context.Background())
			if got.Presence != tt.want || got.Evidence[0].Kind != tt.kind {
				t.Fatalf("Probe() = %#v, want %v/%v", got, tt.want, tt.kind)
			}
			if runner.name != "pm2" || len(runner.args) != 1 || runner.args[0] != "jlist" {
				t.Fatalf("command = %q %v, want pm2 jlist", runner.name, runner.args)
			}
		})
	}
}

func TestPM2Probe_UnsupportedDoesNotRunCommand(t *testing.T) {
	runner := &fakeRunner{}
	got := PM2Probe{Runner: runner, Platform: Platform{GOOS: "windows", GOARCH: "amd64"}}.Probe(context.Background())
	if got.Presence != PresenceUnsupported || got.Evidence[0].Kind != EvidencePM2Unsupported || runner.name != "" {
		t.Fatalf("Probe() = %#v, runner = %#v", got, runner)
	}
}

func TestPM2Probe_CommandFailureIsUncertain(t *testing.T) {
	got := PM2Probe{Runner: &fakeRunner{err: errors.New("permission denied")}, Platform: Platform{GOOS: "linux", GOARCH: "arm64"}}.Probe(context.Background())
	if got.Presence != PresenceUncertain || got.Evidence[0].Kind != EvidencePM2Failed {
		t.Fatalf("Probe() = %#v", got)
	}
}

func TestPM2Probe_EmptyPolicyIsUncertain(t *testing.T) {
	got := PM2Probe{Runner: &fakeRunner{stdout: []byte(`[]`)}, Platform: Platform{GOOS: "linux", GOARCH: "amd64"}}.Probe(context.Background())
	if got.Presence != PresenceUncertain || got.Evidence[0].Kind != EvidencePM2Failed {
		t.Fatalf("Probe() = %#v, want configuration uncertainty", got)
	}
}

func TestPM2Probe_PropagatesTimeoutAndParentCancellation(t *testing.T) {
	policy := LegacyPolicy{ProcessNames: []string{"alice"}, DeploymentRoots: []string{"/opt/alice"}}
	runner := &fakeRunner{stdout: []byte(`[]`)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	PM2Probe{Runner: runner, Platform: Platform{GOOS: "linux", GOARCH: "amd64"}, Policy: policy, Timeout: time.Second}.Probe(ctx)
	remaining := time.Until(runner.deadline)
	if remaining <= 0 || remaining > time.Second || !errors.Is(runner.contextErr, context.Canceled) {
		t.Fatalf("deadline remaining = %v, context error = %v", remaining, runner.contextErr)
	}

	defaultRunner := &fakeRunner{stdout: []byte(`[]`)}
	PM2Probe{Runner: defaultRunner, Platform: Platform{GOOS: "linux", GOARCH: "amd64"}, Policy: policy}.Probe(context.Background())
	defaultRemaining := time.Until(defaultRunner.deadline)
	if defaultRemaining < 4*time.Second || defaultRemaining > 5*time.Second {
		t.Fatalf("default deadline remaining = %v, want approximately five seconds", defaultRemaining)
	}
}
