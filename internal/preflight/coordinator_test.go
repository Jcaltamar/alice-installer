package preflight_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/docker"
	"github.com/jcaltamar/alice-installer/internal/platform"
	"github.com/jcaltamar/alice-installer/internal/ports"
	"github.com/jcaltamar/alice-installer/internal/preflight"
)

// ---------------------------------------------------------------------------
// FakeDirChecker — local test double
// ---------------------------------------------------------------------------

type FakeDirChecker struct {
	WritableOverride map[string]bool
	ReasonOverride   map[string]string
}

func (f FakeDirChecker) IsWritable(path string) (bool, string) {
	if v, ok := f.WritableOverride[path]; ok {
		return v, f.ReasonOverride[path]
	}
	return true, ""
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// happyCoordinator builds a Coordinator where all fakes return success.
func happyCoordinator() preflight.Coordinator {
	return preflight.Coordinator{
		OS:   &platform.FakeOSGuard{Linux: true, Name: "linux"},
		Arch: &platform.FakeArchDetector{Arch: platform.ArchAMD64},
		Docker: &docker.FakeDockerClient{
			ProbeErr: nil,
			VersionVal: docker.Version{
				Client: "24.0.5",
				Server: "24.0.5",
			},
		},
		Compose: &compose.FakeComposeRunner{
			VersionVal: compose.Version{V2Plugin: true, Raw: "2.21.0"},
		},
		GPU: &platform.FakeGPUDetector{
			Info: platform.GPUInfo{ToolkitInstalled: true, RuntimeAvailable: true},
		},
		Ports:             &ports.FakePortScanner{},
		Dirs:              FakeDirChecker{},
		MediaDir:          "/opt/alice-media",
		ConfigDir:         "/opt/alice-config",
		RequiredTCPPorts:  []int{8080, 5432},
		RequiredUDPPorts:  []int{8189},
		MinDockerVersion:  "20.10.0",
		MinComposeVersion: "2.0.0",
	}
}

// findCheck returns the CheckResult with the given ID, or t.Fatal if not found.
func findCheck(t *testing.T, r preflight.Report, id preflight.CheckID) preflight.CheckResult {
	t.Helper()
	for _, item := range r.Items {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("check %q not found in report (items: %v)", id, r.Items)
	return preflight.CheckResult{}
}

// ---------------------------------------------------------------------------
// Coordinator.Run scenarios
// ---------------------------------------------------------------------------

func TestCoordinator_HappyPath(t *testing.T) {
	t.Parallel()
	c := happyCoordinator()
	r := c.Run(context.Background())

	if !r.CanContinue() {
		t.Errorf("happy path: CanContinue() = false, want true; failures: %v", r.Failures())
	}
	if r.HasBlockingFailure() {
		t.Errorf("happy path: HasBlockingFailure() = true, failures: %v", r.Failures())
	}

	// All primary checks must be PASS.
	for _, id := range []preflight.CheckID{
		preflight.CheckOS,
		preflight.CheckArch,
		preflight.CheckDockerDaemon,
		preflight.CheckDockerVersion,
		preflight.CheckComposeVersion,
		preflight.CheckGPU,
		preflight.CheckMediaWritable,
		preflight.CheckConfigWritable,
		preflight.CheckPortsAvailable,
	} {
		item := findCheck(t, r, id)
		if item.Status != preflight.StatusPass {
			t.Errorf("check %q = %v, want PASS", id, item.Status)
		}
	}
}

func TestCoordinator_NonLinuxOS_BlocksRemaining(t *testing.T) {
	t.Parallel()
	c := happyCoordinator()
	c.OS = &platform.FakeOSGuard{Linux: false, Name: "darwin"}

	r := c.Run(context.Background())

	// Report must have exactly one item: the OS FAIL.
	if len(r.Items) != 1 {
		t.Fatalf("expected 1 item after OS FAIL, got %d: %v", len(r.Items), r.Items)
	}
	os := r.Items[0]
	if os.ID != preflight.CheckOS {
		t.Errorf("item ID = %q, want %q", os.ID, preflight.CheckOS)
	}
	if os.Status != preflight.StatusFail {
		t.Errorf("OS check status = %v, want FAIL", os.Status)
	}
	if !r.HasBlockingFailure() {
		t.Error("HasBlockingFailure() = false, want true")
	}
}

func TestCoordinator_UnknownArch_BlocksRemaining(t *testing.T) {
	t.Parallel()
	c := happyCoordinator()
	c.Arch = &platform.FakeArchDetector{Arch: platform.ArchUnknown}

	r := c.Run(context.Background())

	// OS check passes, Arch FAIL, then short-circuit.
	if len(r.Items) != 2 {
		t.Fatalf("expected 2 items after Arch FAIL, got %d: %v", len(r.Items), r.Items)
	}
	arch := findCheck(t, r, preflight.CheckArch)
	if arch.Status != preflight.StatusFail {
		t.Errorf("Arch check status = %v, want FAIL", arch.Status)
	}
	if !r.HasBlockingFailure() {
		t.Error("HasBlockingFailure() = false, want true")
	}
}

func TestCoordinator_DockerDaemonDown(t *testing.T) {
	t.Parallel()
	c := happyCoordinator()
	c.Docker = &docker.FakeDockerClient{
		ProbeErr: errors.New("connection refused"),
	}

	r := c.Run(context.Background())

	daemon := findCheck(t, r, preflight.CheckDockerDaemon)
	if daemon.Status != preflight.StatusFail {
		t.Errorf("DockerDaemon check status = %v, want FAIL", daemon.Status)
	}
	if daemon.Remediation == "" {
		t.Error("DockerDaemon FAIL: Remediation must not be empty")
	}
	if !r.HasBlockingFailure() {
		t.Error("HasBlockingFailure() = false, want true")
	}
}

func TestCoordinator_ComposeV1Only(t *testing.T) {
	t.Parallel()
	c := happyCoordinator()
	c.Compose = &compose.FakeComposeRunner{
		VersionVal: compose.Version{V2Plugin: false, Raw: "1.29.2"},
	}

	r := c.Run(context.Background())

	cv := findCheck(t, r, preflight.CheckComposeVersion)
	if cv.Status != preflight.StatusFail {
		t.Errorf("ComposeVersion check status = %v, want FAIL", cv.Status)
	}
	if !r.HasBlockingFailure() {
		t.Error("HasBlockingFailure() = false, want true")
	}
}

func TestCoordinator_GPUAbsent_Warn(t *testing.T) {
	t.Parallel()
	c := happyCoordinator()
	c.GPU = &platform.FakeGPUDetector{
		Info: platform.GPUInfo{
			ToolkitInstalled: false,
			RuntimeAvailable: false,
			Reason:           "nvidia runtime not found",
		},
	}

	r := c.Run(context.Background())

	gpu := findCheck(t, r, preflight.CheckGPU)
	if gpu.Status != preflight.StatusWarn {
		t.Errorf("GPU check status = %v, want WARN", gpu.Status)
	}
	if gpu.Remediation == "" {
		t.Error("GPU WARN: Remediation must not be empty")
	}
	// Not blocking — TUI can continue.
	if !r.CanContinue() {
		t.Errorf("CanContinue() = false, want true when GPU absent")
	}
}

func TestCoordinator_PortsOccupied_Warn(t *testing.T) {
	t.Parallel()
	c := happyCoordinator()
	// Mark port 8080 as occupied.
	c.Ports = &ports.FakePortScanner{OccupiedPorts: []int{8080}}

	r := c.Run(context.Background())

	pa := findCheck(t, r, preflight.CheckPortsAvailable)
	if pa.Status != preflight.StatusWarn {
		t.Errorf("PortsAvailable check status = %v, want WARN", pa.Status)
	}
	// Port conflict is a WARN, not a FAIL — user can choose alternate ports.
	if !r.CanContinue() {
		t.Errorf("CanContinue() = false, want true with only port WARN")
	}
}

func TestCoordinator_MediaDirNotWritable_Fail(t *testing.T) {
	t.Parallel()
	c := happyCoordinator()
	c.Dirs = FakeDirChecker{
		WritableOverride: map[string]bool{
			"/opt/alice-media": false,
		},
		ReasonOverride: map[string]string{
			"/opt/alice-media": "permission denied",
		},
	}

	r := c.Run(context.Background())

	media := findCheck(t, r, preflight.CheckMediaWritable)
	if media.Status != preflight.StatusFail {
		t.Errorf("MediaWritable check status = %v, want FAIL", media.Status)
	}
	if media.Detail == "" {
		t.Error("MediaWritable FAIL: Detail must not be empty")
	}
	if media.Remediation == "" {
		t.Error("MediaWritable FAIL: Remediation must not be empty")
	}
	if !r.HasBlockingFailure() {
		t.Error("HasBlockingFailure() = false, want true")
	}
}

func TestCoordinator_DockerVersionTooOld_Warn(t *testing.T) {
	t.Parallel()
	c := happyCoordinator()
	c.Docker = &docker.FakeDockerClient{
		ProbeErr: nil,
		VersionVal: docker.Version{
			Client: "19.03.0",
			Server: "19.03.0",
		},
	}

	r := c.Run(context.Background())

	dv := findCheck(t, r, preflight.CheckDockerVersion)
	if dv.Status != preflight.StatusWarn {
		t.Errorf("DockerVersion check status = %v, want WARN", dv.Status)
	}
}
