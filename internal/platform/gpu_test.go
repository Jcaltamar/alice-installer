package platform_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jcaltamar/alice-installer/internal/platform"
)

// fakeCommandRunner records calls and returns pre-configured output.
// It implements platform.CommandRunner interface.
type fakeCommandRunner struct {
	outputs map[string]cmdOutput
}

type cmdOutput struct {
	stdout []byte
	stderr []byte
	err    error
}

func (f *fakeCommandRunner) Run(_ context.Context, name string, _ ...string) ([]byte, []byte, error) {
	if out, ok := f.outputs[name]; ok {
		return out.stdout, out.stderr, out.err
	}
	return nil, nil, nil
}

func TestDockerGPUDetector_NvidiaPresent(t *testing.T) {
	// docker info returns JSON with nvidia in Runtimes
	dockerInfoJSON := `{"Runtimes":{"nvidia":{"path":"nvidia-container-runtime"},"runc":{"path":"runc"}}}`

	runner := &fakeCommandRunner{
		outputs: map[string]cmdOutput{
			"docker": {stdout: []byte(dockerInfoJSON)},
		},
	}

	d := platform.NewDockerGPUDetector(runner)
	info := d.Detect(context.Background())

	if !info.ToolkitInstalled {
		t.Errorf("ToolkitInstalled = false, want true; Reason: %q", info.Reason)
	}
	if info.Reason != "" {
		t.Errorf("Reason should be empty when GPU present, got %q", info.Reason)
	}
}

func TestDockerGPUDetector_NvidiaAbsent(t *testing.T) {
	// docker info returns JSON without nvidia runtime
	dockerInfoJSON := `{"Runtimes":{"runc":{"path":"runc"}}}`

	runner := &fakeCommandRunner{
		outputs: map[string]cmdOutput{
			"docker": {stdout: []byte(dockerInfoJSON)},
		},
	}

	d := platform.NewDockerGPUDetector(runner)
	info := d.Detect(context.Background())

	if info.ToolkitInstalled {
		t.Errorf("ToolkitInstalled = true, want false")
	}
	if info.Reason == "" {
		t.Errorf("Reason should be non-empty when nvidia runtime absent")
	}
}

func TestDockerGPUDetector_DockerInfoFails(t *testing.T) {
	// docker info command fails entirely (daemon not running)
	runner := &fakeCommandRunner{
		outputs: map[string]cmdOutput{
			"docker": {err: errors.New("docker: command failed")},
		},
	}

	d := platform.NewDockerGPUDetector(runner)
	info := d.Detect(context.Background())

	if info.ToolkitInstalled {
		t.Errorf("ToolkitInstalled = true, want false when docker fails")
	}
	if info.RuntimeAvailable {
		t.Errorf("RuntimeAvailable = true, want false when docker fails")
	}
	if info.Reason == "" {
		t.Errorf("Reason should be non-empty when docker fails")
	}
}

func TestDockerGPUDetector_InvalidJSON(t *testing.T) {
	// docker info returns garbage (not JSON)
	runner := &fakeCommandRunner{
		outputs: map[string]cmdOutput{
			"docker": {stdout: []byte("not-json-at-all")},
		},
	}

	d := platform.NewDockerGPUDetector(runner)
	info := d.Detect(context.Background())

	if info.ToolkitInstalled {
		t.Errorf("ToolkitInstalled = true, want false on parse error")
	}
	if info.Reason == "" {
		t.Errorf("Reason should be non-empty on JSON parse error")
	}
}

func TestGPUDetector_Interface(t *testing.T) {
	// compile-time: DockerGPUDetector implements GPUDetector
	var _ platform.GPUDetector = platform.NewDockerGPUDetector(nil)
	t.Log("interface satisfied")
}

// TestFakeGPUDetector ensures FakeGPUDetector satisfies GPUDetector.
func TestFakeGPUDetector_ReturnsConfiguredInfo(t *testing.T) {
	fake := &platform.FakeGPUDetector{
		Info: platform.GPUInfo{ToolkitInstalled: true, RuntimeAvailable: true},
	}

	info := fake.Detect(context.Background())
	if !info.ToolkitInstalled {
		t.Errorf("FakeGPUDetector.Detect() ToolkitInstalled = false, want true")
	}
}
