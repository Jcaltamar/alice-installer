package compose_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/platform"
)

// ---------------------------------------------------------------------------
// composeArgs helper
// ---------------------------------------------------------------------------

func TestComposeArgs_OneFile(t *testing.T) {
	got := compose.ComposeArgs([]string{"docker-compose.yml"})
	want := []string{"-f", "docker-compose.yml"}
	if len(got) != len(want) {
		t.Fatalf("ComposeArgs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ComposeArgs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestComposeArgs_TwoFiles(t *testing.T) {
	got := compose.ComposeArgs([]string{"docker-compose.yml", "docker-compose.gpu.yml"})
	want := []string{"-f", "docker-compose.yml", "-f", "docker-compose.gpu.yml"}
	if len(got) != len(want) {
		t.Fatalf("ComposeArgs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ComposeArgs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestComposeArgs_Empty(t *testing.T) {
	got := compose.ComposeArgs(nil)
	if len(got) != 0 {
		t.Errorf("ComposeArgs(nil) = %v, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// CLICompose — Version
// ---------------------------------------------------------------------------

func TestCLICompose_Version_V2Plugin(t *testing.T) {
	runner := &platform.FakeCommandRunner{
		Outputs: map[string]platform.FakeCmdOutput{
			"docker": {Stdout: []byte("2.24.0\n")},
		},
	}
	c := compose.NewCLICompose(runner, nil)

	ver, err := c.Version(context.Background())
	if err != nil {
		t.Fatalf("Version() error = %v, want nil", err)
	}
	if !ver.V2Plugin {
		t.Error("V2Plugin = false, want true")
	}
	if ver.Raw != "2.24.0" {
		t.Errorf("Raw = %q, want %q", ver.Raw, "2.24.0")
	}
}

func TestCLICompose_Version_ErrorOnCommandFail(t *testing.T) {
	runner := &platform.FakeCommandRunner{
		Outputs: map[string]platform.FakeCmdOutput{
			"docker": {Err: errors.New("docker: not found")},
		},
	}
	c := compose.NewCLICompose(runner, nil)

	_, err := c.Version(context.Background())
	if err == nil {
		t.Fatal("Version() expected error, got nil")
	}
}

func TestCLICompose_Version_EmptyOutputError(t *testing.T) {
	runner := &platform.FakeCommandRunner{
		Outputs: map[string]platform.FakeCmdOutput{
			"docker": {Stdout: []byte("")},
		},
	}
	c := compose.NewCLICompose(runner, nil)

	_, err := c.Version(context.Background())
	if err == nil {
		t.Fatal("Version() expected error for empty output, got nil")
	}
}

// ---------------------------------------------------------------------------
// CLICompose — Pull (streaming)
// ---------------------------------------------------------------------------

func TestCLICompose_Pull_StreamsProgress(t *testing.T) {
	lines := []string{
		"[+] Pulling 2 objects (2/2)",
		" ✔ backend Pulled",
		" ✔ websocket Pulled",
	}
	streamer := &platform.FakeStreamingCommandRunner{Lines: lines}
	c := compose.NewCLICompose(nil, streamer)

	ch := make(chan compose.PullProgressMsg, 10)
	err := c.Pull(context.Background(), []string{"docker-compose.yml"}, ".env", ch)
	if err != nil {
		t.Fatalf("Pull() error = %v, want nil", err)
	}

	close(ch)
	var msgs []compose.PullProgressMsg
	for m := range ch {
		msgs = append(msgs, m)
	}
	if len(msgs) != 3 {
		t.Errorf("Pull() sent %d messages, want 3", len(msgs))
	}
}

func TestCLICompose_Pull_ErrorPropagated(t *testing.T) {
	wantErr := errors.New("network error")
	streamer := &platform.FakeStreamingCommandRunner{Err: wantErr}
	c := compose.NewCLICompose(nil, streamer)

	ch := make(chan compose.PullProgressMsg, 10)
	err := c.Pull(context.Background(), []string{"docker-compose.yml"}, ".env", ch)
	if !errors.Is(err, wantErr) {
		t.Errorf("Pull() error = %v, want %v", err, wantErr)
	}
}

// ---------------------------------------------------------------------------
// CLICompose — Up (streaming)
// ---------------------------------------------------------------------------

func TestCLICompose_Up_StreamsProgress(t *testing.T) {
	lines := []string{
		"[+] Running 2/2",
		" ✔ Container backend  Started",
		" ✔ Container websocket  Started",
	}
	streamer := &platform.FakeStreamingCommandRunner{Lines: lines}
	c := compose.NewCLICompose(nil, streamer)

	ch := make(chan compose.UpProgressMsg, 10)
	err := c.Up(context.Background(), []string{"docker-compose.yml"}, ".env", ch)
	if err != nil {
		t.Fatalf("Up() error = %v, want nil", err)
	}

	close(ch)
	var msgs []compose.UpProgressMsg
	for m := range ch {
		msgs = append(msgs, m)
	}
	if len(msgs) != 3 {
		t.Errorf("Up() sent %d messages, want 3", len(msgs))
	}
}

func TestCLICompose_Up_ErrorPropagated(t *testing.T) {
	wantErr := errors.New("up failed")
	streamer := &platform.FakeStreamingCommandRunner{Err: wantErr}
	c := compose.NewCLICompose(nil, streamer)

	ch := make(chan compose.UpProgressMsg, 10)
	err := c.Up(context.Background(), []string{"docker-compose.yml"}, ".env", ch)
	if !errors.Is(err, wantErr) {
		t.Errorf("Up() error = %v, want %v", err, wantErr)
	}
}

// ---------------------------------------------------------------------------
// CLICompose — Down
// ---------------------------------------------------------------------------

func TestCLICompose_Down_Success(t *testing.T) {
	runner := &platform.FakeCommandRunner{
		Outputs: map[string]platform.FakeCmdOutput{
			"docker": {Stdout: []byte("")},
		},
	}
	c := compose.NewCLICompose(runner, nil)

	err := c.Down(context.Background(), []string{"docker-compose.yml"}, ".env")
	if err != nil {
		t.Errorf("Down() error = %v, want nil", err)
	}
}

func TestCLICompose_Down_ErrorPropagated(t *testing.T) {
	wantErr := errors.New("down failed")
	runner := &platform.FakeCommandRunner{
		Outputs: map[string]platform.FakeCmdOutput{
			"docker": {Err: wantErr},
		},
	}
	c := compose.NewCLICompose(runner, nil)

	err := c.Down(context.Background(), []string{"docker-compose.yml"}, ".env")
	if !errors.Is(err, wantErr) {
		t.Errorf("Down() error = %v, want %v", err, wantErr)
	}
}

// ---------------------------------------------------------------------------
// CLICompose — HealthStatus
// ---------------------------------------------------------------------------

func TestCLICompose_HealthStatus_ParsesJSONLines(t *testing.T) {
	// docker compose ps --format json returns one JSON object per line
	psOutput := `{"Service":"backend","Health":"healthy"}
{"Service":"websocket","Health":"starting"}
`
	runner := &platform.FakeCommandRunner{
		Outputs: map[string]platform.FakeCmdOutput{
			"docker": {Stdout: []byte(psOutput)},
		},
	}
	c := compose.NewCLICompose(runner, nil)

	statuses, err := c.HealthStatus(context.Background(), []string{"docker-compose.yml"}, ".env")
	if err != nil {
		t.Fatalf("HealthStatus() error = %v, want nil", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("HealthStatus() returned %d entries, want 2", len(statuses))
	}
	if statuses[0].Service != "backend" {
		t.Errorf("statuses[0].Service = %q, want backend", statuses[0].Service)
	}
	if statuses[0].Status != "healthy" {
		t.Errorf("statuses[0].Status = %q, want healthy", statuses[0].Status)
	}
	if statuses[1].Status != "starting" {
		t.Errorf("statuses[1].Status = %q, want starting", statuses[1].Status)
	}
}

func TestCLICompose_HealthStatus_ErrorPropagated(t *testing.T) {
	wantErr := errors.New("ps failed")
	runner := &platform.FakeCommandRunner{
		Outputs: map[string]platform.FakeCmdOutput{
			"docker": {Err: wantErr},
		},
	}
	c := compose.NewCLICompose(runner, nil)

	_, err := c.HealthStatus(context.Background(), []string{"docker-compose.yml"}, ".env")
	if !errors.Is(err, wantErr) {
		t.Errorf("HealthStatus() error = %v, want %v", err, wantErr)
	}
}

// ---------------------------------------------------------------------------
// FakeComposeRunner
// ---------------------------------------------------------------------------

func TestFakeComposeRunner_ImplementsInterface(t *testing.T) {
	var _ compose.ComposeRunner = &compose.FakeComposeRunner{}
	t.Log("FakeComposeRunner implements ComposeRunner")
}

func TestFakeComposeRunner_PullSendsProgressAndReturnsErr(t *testing.T) {
	msgs := []compose.PullProgressMsg{
		{Service: "backend", Status: "Pulling"},
		{Service: "backend", Status: "Pulled"},
	}
	wantErr := errors.New("pull error")
	fake := &compose.FakeComposeRunner{
		PullProgressMsgs: msgs,
		PullErr:          wantErr,
	}

	ch := make(chan compose.PullProgressMsg, 10)
	err := fake.Pull(context.Background(), nil, "", ch)
	if !errors.Is(err, wantErr) {
		t.Errorf("Pull() error = %v, want %v", err, wantErr)
	}
	close(ch)
	var got []compose.PullProgressMsg
	for m := range ch {
		got = append(got, m)
	}
	if len(got) != 2 {
		t.Errorf("Pull() sent %d messages, want 2", len(got))
	}
}

func TestFakeComposeRunner_UpSendsProgressAndReturnsErr(t *testing.T) {
	msgs := []compose.UpProgressMsg{
		{Service: "backend", Status: "Started"},
	}
	fake := &compose.FakeComposeRunner{
		UpProgressMsgs: msgs,
		UpErr:          nil,
	}

	ch := make(chan compose.UpProgressMsg, 10)
	err := fake.Up(context.Background(), nil, "", ch)
	if err != nil {
		t.Fatalf("Up() unexpected error: %v", err)
	}
	close(ch)
	var got []compose.UpProgressMsg
	for m := range ch {
		got = append(got, m)
	}
	if len(got) != 1 {
		t.Errorf("Up() sent %d messages, want 1", len(got))
	}
}

func TestFakeComposeRunner_HealthStatusReturnsConfigured(t *testing.T) {
	want := []compose.ServiceHealth{
		{Service: "backend", Status: "healthy"},
		{Service: "websocket", Status: "unhealthy"},
	}
	fake := &compose.FakeComposeRunner{Healths: want}

	got, err := fake.HealthStatus(context.Background(), nil, "")
	if err != nil {
		t.Fatalf("HealthStatus() unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("HealthStatus() returned %d entries, want 2", len(got))
	}
	if got[0].Status != "healthy" {
		t.Errorf("got[0].Status = %q, want healthy", got[0].Status)
	}
	if got[1].Status != "unhealthy" {
		t.Errorf("got[1].Status = %q, want unhealthy", got[1].Status)
	}
}
