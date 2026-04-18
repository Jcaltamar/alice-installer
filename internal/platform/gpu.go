package platform

import (
	"context"
	"encoding/json"
	"fmt"
)

// GPUInfo holds the result of GPU detection.
type GPUInfo struct {
	// ToolkitInstalled is true when the NVIDIA container toolkit runtime is
	// registered in Docker (i.e. "nvidia" appears in docker info Runtimes).
	ToolkitInstalled bool

	// RuntimeAvailable is true when the GPU is generally usable.
	// For now it mirrors ToolkitInstalled; future probes (nvidia-smi) can
	// set them independently.
	RuntimeAvailable bool

	// Reason is empty when everything is fine; otherwise it contains a
	// human-readable explanation of why GPU is unavailable.
	Reason string
}

// GPUDetector detects NVIDIA GPU availability for Docker workloads.
type GPUDetector interface {
	Detect(ctx context.Context) GPUInfo
}

// CommandRunner abstracts shelling out to an external command.
// It is the seam used in unit tests to inject controlled stdout/stderr.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)
}

// dockerInfoResponse is the subset of `docker info --format '{{json .}}'` we care about.
type dockerInfoResponse struct {
	Runtimes map[string]json.RawMessage `json:"Runtimes"`
}

// DockerGPUDetector uses `docker info` to detect the NVIDIA container runtime.
type DockerGPUDetector struct {
	runner CommandRunner
}

// NewDockerGPUDetector creates a DockerGPUDetector.
// Pass nil to use the production os/exec runner.
func NewDockerGPUDetector(runner CommandRunner) *DockerGPUDetector {
	if runner == nil {
		runner = &OSCommandRunner{}
	}
	return &DockerGPUDetector{runner: runner}
}

// Detect shells out to `docker info --format '{{json .}}'` and checks for
// an "nvidia" key in the Runtimes map.
func (d *DockerGPUDetector) Detect(ctx context.Context) GPUInfo {
	stdout, _, err := d.runner.Run(ctx, "docker", "info", "--format", "{{json .}}")
	if err != nil {
		return GPUInfo{
			Reason: fmt.Sprintf("docker info failed: %v", err),
		}
	}

	var info dockerInfoResponse
	if err := json.Unmarshal(stdout, &info); err != nil {
		return GPUInfo{
			Reason: fmt.Sprintf("could not parse docker info output: %v", err),
		}
	}

	if _, ok := info.Runtimes["nvidia"]; ok {
		return GPUInfo{
			ToolkitInstalled: true,
			RuntimeAvailable: true,
		}
	}

	return GPUInfo{
		Reason: "nvidia runtime not found in docker runtimes; install nvidia-container-toolkit and restart docker",
	}
}
