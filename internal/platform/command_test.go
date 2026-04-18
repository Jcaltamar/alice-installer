package platform_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jcaltamar/alice-installer/internal/platform"
)

// ---------------------------------------------------------------------------
// FakeCommandRunner (Run) tests
// ---------------------------------------------------------------------------

func TestFakeCommandRunner_RunReturnsConfiguredOutput(t *testing.T) {
	fake := &platform.FakeCommandRunner{
		Outputs: map[string]platform.FakeCmdOutput{
			"docker": {
				Stdout: []byte(`{"Runtimes":{"nvidia":{}}}`),
				Stderr: nil,
				Err:    nil,
			},
		},
	}

	stdout, stderr, err := fake.Run(context.Background(), "docker", "info", "--format", "{{json .}}")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if !strings.Contains(string(stdout), "nvidia") {
		t.Errorf("stdout missing 'nvidia', got %q", stdout)
	}
	if len(stderr) != 0 {
		t.Errorf("stderr should be empty, got %q", stderr)
	}
}

func TestFakeCommandRunner_RunReturnsError(t *testing.T) {
	wantErr := errors.New("exec: not found")
	fake := &platform.FakeCommandRunner{
		Outputs: map[string]platform.FakeCmdOutput{
			"docker": {Err: wantErr},
		},
	}

	_, _, err := fake.Run(context.Background(), "docker", "info")
	if !errors.Is(err, wantErr) {
		t.Errorf("Run() error = %v, want %v", err, wantErr)
	}
}

func TestFakeCommandRunner_RunUnknownCommandReturnsNil(t *testing.T) {
	fake := &platform.FakeCommandRunner{}

	stdout, stderr, err := fake.Run(context.Background(), "unknown-cmd")
	if err != nil {
		t.Errorf("Run() error = %v, want nil for unknown command", err)
	}
	if stdout != nil || stderr != nil {
		t.Errorf("Run() = %v/%v, want nil/nil for unknown command", stdout, stderr)
	}
}

// ---------------------------------------------------------------------------
// FakeStreamingCommandRunner tests
// ---------------------------------------------------------------------------

func TestFakeStreamingCommandRunner_StreamDeliversLines(t *testing.T) {
	fake := &platform.FakeStreamingCommandRunner{
		Lines: []string{"line1", "line2", "line3"},
		Err:   nil,
	}

	var got []string
	err := fake.Stream(context.Background(),
		func(line string) { got = append(got, line) },
		func(_ string) {},
		"docker", "compose", "pull",
	)
	if err != nil {
		t.Fatalf("Stream() unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d lines, want 3", len(got))
	}
	for i, want := range []string{"line1", "line2", "line3"} {
		if got[i] != want {
			t.Errorf("line[%d] = %q, want %q", i, got[i], want)
		}
	}
}

func TestFakeStreamingCommandRunner_StreamReturnsError(t *testing.T) {
	wantErr := errors.New("stream error")
	fake := &platform.FakeStreamingCommandRunner{
		Lines: nil,
		Err:   wantErr,
	}

	err := fake.Stream(context.Background(), func(_ string) {}, func(_ string) {}, "cmd")
	if !errors.Is(err, wantErr) {
		t.Errorf("Stream() error = %v, want %v", err, wantErr)
	}
}
