package docker_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jcaltamar/alice-installer/internal/docker"
	"github.com/jcaltamar/alice-installer/internal/platform"
)

// ---------------------------------------------------------------------------
// CLIDocker — Probe
// ---------------------------------------------------------------------------

func TestCLIDocker_Probe_Success(t *testing.T) {
	runner := &platform.FakeCommandRunner{
		Outputs: map[string]platform.FakeCmdOutput{
			"docker": {
				Stdout: []byte(`{"ServerVersion":"24.0.5","Architecture":"x86_64","OperatingSystem":"Ubuntu 22.04.3 LTS","Runtimes":{"runc":{},"nvidia":{}}}`),
			},
		},
	}
	client := docker.NewCLIDocker(runner)

	err := client.Probe(context.Background())
	if err != nil {
		t.Errorf("Probe() unexpected error: %v", err)
	}
}

func TestCLIDocker_Probe_Failure(t *testing.T) {
	runner := &platform.FakeCommandRunner{
		Outputs: map[string]platform.FakeCmdOutput{
			"docker": {Err: errors.New("docker daemon not running")},
		},
	}
	client := docker.NewCLIDocker(runner)

	err := client.Probe(context.Background())
	if err == nil {
		t.Fatal("Probe() expected error when daemon unreachable, got nil")
	}
}

// ---------------------------------------------------------------------------
// CLIDocker — Info
// ---------------------------------------------------------------------------

func TestCLIDocker_Info_ParsesJSON(t *testing.T) {
	jsonResp := `{"ServerVersion":"24.0.5","Architecture":"x86_64","OperatingSystem":"Ubuntu 22.04.3 LTS","Runtimes":{"runc":{},"nvidia":{}}}`
	runner := &platform.FakeCommandRunner{
		Outputs: map[string]platform.FakeCmdOutput{
			"docker": {Stdout: []byte(jsonResp)},
		},
	}
	client := docker.NewCLIDocker(runner)

	info, err := client.Info(context.Background())
	if err != nil {
		t.Fatalf("Info() unexpected error: %v", err)
	}
	if info.ServerVersion != "24.0.5" {
		t.Errorf("ServerVersion = %q, want %q", info.ServerVersion, "24.0.5")
	}
	if info.Architecture != "x86_64" {
		t.Errorf("Architecture = %q, want %q", info.Architecture, "x86_64")
	}
	if info.OperatingSystem != "Ubuntu 22.04.3 LTS" {
		t.Errorf("OperatingSystem = %q, want %q", info.OperatingSystem, "Ubuntu 22.04.3 LTS")
	}
	// Runtimes: both "runc" and "nvidia" should appear
	found := false
	for _, r := range info.Runtimes.Names {
		if r == "nvidia" {
			found = true
		}
	}
	if !found {
		t.Errorf("Runtimes.Names does not contain 'nvidia', got %v", info.Runtimes.Names)
	}
}

func TestCLIDocker_Info_ParseError(t *testing.T) {
	runner := &platform.FakeCommandRunner{
		Outputs: map[string]platform.FakeCmdOutput{
			"docker": {Stdout: []byte("not-json")},
		},
	}
	client := docker.NewCLIDocker(runner)

	_, err := client.Info(context.Background())
	if err == nil {
		t.Fatal("Info() expected error on malformed JSON, got nil")
	}
}

func TestCLIDocker_Info_RuntimesDefault(t *testing.T) {
	// Response with only runc — no nvidia; tests Runtimes.Default detection
	jsonResp := `{"ServerVersion":"20.10.0","Architecture":"aarch64","OperatingSystem":"Debian GNU/Linux 11","Runtimes":{"runc":{}}}`
	runner := &platform.FakeCommandRunner{
		Outputs: map[string]platform.FakeCmdOutput{
			"docker": {Stdout: []byte(jsonResp)},
		},
	}
	client := docker.NewCLIDocker(runner)

	info, err := client.Info(context.Background())
	if err != nil {
		t.Fatalf("Info() unexpected error: %v", err)
	}
	if len(info.Runtimes.Names) != 1 || info.Runtimes.Names[0] != "runc" {
		t.Errorf("Runtimes.Names = %v, want [runc]", info.Runtimes.Names)
	}
}

// ---------------------------------------------------------------------------
// CLIDocker — HasRuntime
// ---------------------------------------------------------------------------

func TestCLIDocker_HasRuntime_Present(t *testing.T) {
	jsonResp := `{"ServerVersion":"24.0.5","Architecture":"x86_64","OperatingSystem":"Linux","Runtimes":{"runc":{},"nvidia":{}}}`
	runner := &platform.FakeCommandRunner{
		Outputs: map[string]platform.FakeCmdOutput{
			"docker": {Stdout: []byte(jsonResp)},
		},
	}
	client := docker.NewCLIDocker(runner)

	ok, err := client.HasRuntime(context.Background(), "nvidia")
	if err != nil {
		t.Fatalf("HasRuntime() unexpected error: %v", err)
	}
	if !ok {
		t.Error("HasRuntime('nvidia') = false, want true")
	}
}

func TestCLIDocker_HasRuntime_Absent(t *testing.T) {
	jsonResp := `{"ServerVersion":"24.0.5","Architecture":"x86_64","OperatingSystem":"Linux","Runtimes":{"runc":{}}}`
	runner := &platform.FakeCommandRunner{
		Outputs: map[string]platform.FakeCmdOutput{
			"docker": {Stdout: []byte(jsonResp)},
		},
	}
	client := docker.NewCLIDocker(runner)

	ok, err := client.HasRuntime(context.Background(), "nvidia")
	if err != nil {
		t.Fatalf("HasRuntime() unexpected error: %v", err)
	}
	if ok {
		t.Error("HasRuntime('nvidia') = true, want false")
	}
}

func TestCLIDocker_HasRuntime_ErrorOnCommandFail(t *testing.T) {
	runner := &platform.FakeCommandRunner{
		Outputs: map[string]platform.FakeCmdOutput{
			"docker": {Err: errors.New("command not found")},
		},
	}
	client := docker.NewCLIDocker(runner)

	ok, err := client.HasRuntime(context.Background(), "nvidia")
	if err == nil {
		t.Fatal("HasRuntime() expected error on command fail, got nil")
	}
	if ok {
		t.Error("HasRuntime() = true on error, want false")
	}
}

// ---------------------------------------------------------------------------
// CLIDocker — Version
// ---------------------------------------------------------------------------

func TestCLIDocker_Version_ParsesClientAndServer(t *testing.T) {
	// docker version --format {{json .}} returns a Version struct
	versionJSON := `{"Client":{"Version":"24.0.5"},"Server":{"Components":[{"Version":"24.0.5"}]}}`
	runner := &platform.FakeCommandRunner{
		Outputs: map[string]platform.FakeCmdOutput{
			"docker": {Stdout: []byte(versionJSON)},
		},
	}
	client := docker.NewCLIDocker(runner)

	ver, err := client.Version(context.Background())
	if err != nil {
		t.Fatalf("Version() unexpected error: %v", err)
	}
	if ver.Client != "24.0.5" {
		t.Errorf("Client = %q, want %q", ver.Client, "24.0.5")
	}
	if ver.Server != "24.0.5" {
		t.Errorf("Server = %q, want %q", ver.Server, "24.0.5")
	}
}

func TestCLIDocker_Version_ErrorOnCommandFail(t *testing.T) {
	runner := &platform.FakeCommandRunner{
		Outputs: map[string]platform.FakeCmdOutput{
			"docker": {Err: errors.New("not found")},
		},
	}
	client := docker.NewCLIDocker(runner)

	_, err := client.Version(context.Background())
	if err == nil {
		t.Fatal("Version() expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// FakeDockerClient
// ---------------------------------------------------------------------------

func TestFakeDockerClient_ImplementsInterface(t *testing.T) {
	var _ docker.DockerClient = &docker.FakeDockerClient{}
	t.Log("FakeDockerClient implements DockerClient")
}

func TestFakeDockerClient_ProbeReturnsConfiguredError(t *testing.T) {
	wantErr := errors.New("daemon down")
	fake := &docker.FakeDockerClient{ProbeErr: wantErr}

	err := fake.Probe(context.Background())
	if !errors.Is(err, wantErr) {
		t.Errorf("Probe() = %v, want %v", err, wantErr)
	}
}

func TestFakeDockerClient_InfoReturnsConfiguredResult(t *testing.T) {
	want := docker.Info{
		ServerVersion: "24.0.5",
		Architecture:  "x86_64",
		Runtimes:      docker.Runtimes{Names: []string{"runc", "nvidia"}, Default: "runc"},
	}
	fake := &docker.FakeDockerClient{InfoResult: want}

	got, err := fake.Info(context.Background())
	if err != nil {
		t.Fatalf("Info() unexpected error: %v", err)
	}
	if got.ServerVersion != want.ServerVersion {
		t.Errorf("ServerVersion = %q, want %q", got.ServerVersion, want.ServerVersion)
	}
	if len(got.Runtimes.Names) != 2 {
		t.Errorf("Runtimes.Names = %v, want 2 entries", got.Runtimes.Names)
	}
}

func TestFakeDockerClient_HasRuntimeFromMap(t *testing.T) {
	tests := []struct {
		name    string
		runtime string
		present bool
	}{
		{"nvidia present", "nvidia", true},
		{"runc absent from map", "runc", false},
		{"unknown absent", "unknown", false},
	}

	fake := &docker.FakeDockerClient{
		RuntimesMap: map[string]bool{"nvidia": true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := fake.HasRuntime(context.Background(), tt.runtime)
			if err != nil {
				t.Fatalf("HasRuntime() unexpected error: %v", err)
			}
			if ok != tt.present {
				t.Errorf("HasRuntime(%q) = %v, want %v", tt.runtime, ok, tt.present)
			}
		})
	}
}
