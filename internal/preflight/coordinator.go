package preflight

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/docker"
	"github.com/jcaltamar/alice-installer/internal/platform"
	"github.com/jcaltamar/alice-installer/internal/ports"
)

// ---------------------------------------------------------------------------
// DirectoryChecker
// ---------------------------------------------------------------------------

// DirectoryChecker tests whether the installer can create/write a path.
type DirectoryChecker interface {
	// IsWritable returns (true, "") if the path is writable/creatable,
	// or (false, reason) describing why it isn't.
	IsWritable(path string) (bool, string)
}

// OSDirChecker implements DirectoryChecker using the real filesystem.
// It does NOT create the target directory — it checks whether the installer
// would be able to create/write to it at install time.
//
// Logic:
//  1. If path exists → attempt a probe file write; remove probe; return result.
//  2. If path doesn't exist → check parent directory writability instead.
//  3. If parent doesn't exist → return (false, reason).
type OSDirChecker struct{}

// IsWritable checks whether path (or its parent, if path doesn't exist) is
// writable by the current process.
func (OSDirChecker) IsWritable(path string) (bool, string) {
	info, err := os.Stat(path)
	if err == nil {
		// Path exists — check if it is a directory and probe it.
		if !info.IsDir() {
			return false, fmt.Sprintf("%s exists but is not a directory", path)
		}
		return probeDir(path)
	}

	// Path doesn't exist — check the parent.
	parent := filepath.Dir(path)
	pInfo, pErr := os.Stat(parent)
	if pErr != nil {
		return false, fmt.Sprintf("parent directory %s does not exist: %v", parent, pErr)
	}
	if !pInfo.IsDir() {
		return false, fmt.Sprintf("parent %s is not a directory", parent)
	}
	return probeDir(parent)
}

// probeDir writes a tiny probe file to dir and removes it immediately.
func probeDir(dir string) (bool, string) {
	probe := filepath.Join(dir, ".alice_probe")
	if err := os.WriteFile(probe, []byte{}, 0600); err != nil {
		return false, fmt.Sprintf("cannot write to %s: %v", dir, err)
	}
	_ = os.Remove(probe)
	return true, ""
}

// ---------------------------------------------------------------------------
// Coordinator
// ---------------------------------------------------------------------------

// Coordinator orchestrates all preflight checks and aggregates results into a Report.
type Coordinator struct {
	OS               platform.OSGuard
	Arch             platform.ArchDetector
	Docker           docker.DockerClient
	Compose          compose.ComposeRunner
	GPU              platform.GPUDetector
	Ports            ports.PortScanner
	Dirs             DirectoryChecker
	MediaDir         string
	ConfigDir        string
	RequiredTCPPorts []int
	RequiredUDPPorts []int
	MinDockerVersion  string
	MinComposeVersion string
}

// Run executes all preflight checks in order and returns a Report.
// If the OS or architecture check fails, it short-circuits and returns
// immediately with only those check results.
func (c Coordinator) Run(ctx context.Context) Report {
	var items []CheckResult

	// 1. OS check — blocking short-circuit on failure.
	osResult := c.checkOS()
	items = append(items, osResult)
	if osResult.Status == StatusFail {
		return Report{Items: items}
	}

	// 2. Arch check — blocking short-circuit on failure.
	archResult := c.checkArch()
	items = append(items, archResult)
	if archResult.Status == StatusFail {
		return Report{Items: items}
	}

	// 3. Docker daemon.
	items = append(items, c.checkDockerDaemon(ctx))

	// 4. Docker version.
	items = append(items, c.checkDockerVersion(ctx))

	// 5. Compose version.
	items = append(items, c.checkComposeVersion(ctx))

	// 6. GPU (never FAIL — only WARN or PASS).
	items = append(items, c.checkGPU(ctx))

	// 7. Directory writability.
	items = append(items, c.checkDir(CheckMediaWritable, c.MediaDir))
	items = append(items, c.checkDir(CheckConfigWritable, c.ConfigDir))

	// 8. Ports.
	items = append(items, c.checkPorts(ctx))

	return Report{Items: items}
}

// ---------------------------------------------------------------------------
// Individual checks
// ---------------------------------------------------------------------------

func (c Coordinator) checkOS() CheckResult {
	if c.OS.IsLinux() {
		return CheckResult{
			ID:     CheckOS,
			Status: StatusPass,
			Title:  "Operating system",
			Detail: "Linux",
		}
	}
	return CheckResult{
		ID:          CheckOS,
		Status:      StatusFail,
		Title:       "Unsupported OS",
		Detail:      "detected: " + c.OS.OSName(),
		Remediation: "Run the installer on Linux (amd64 or arm64).",
	}
}

func (c Coordinator) checkArch() CheckResult {
	arch := c.Arch.Detect()
	switch arch {
	case platform.ArchAMD64, platform.ArchARM64:
		return CheckResult{
			ID:     CheckArch,
			Status: StatusPass,
			Title:  "CPU architecture",
			Detail: string(arch),
		}
	default:
		return CheckResult{
			ID:          CheckArch,
			Status:      StatusFail,
			Title:       "Unsupported architecture",
			Detail:      "detected: " + string(arch),
			Remediation: "Run the installer on amd64 or arm64.",
		}
	}
}

func (c Coordinator) checkDockerDaemon(ctx context.Context) CheckResult {
	if err := c.Docker.Probe(ctx); err != nil {
		return CheckResult{
			ID:          CheckDockerDaemon,
			Status:      StatusFail,
			Title:       "Docker daemon unreachable",
			Detail:      err.Error(),
			Remediation: "Start Docker (`sudo systemctl start docker`) and ensure your user is in the `docker` group.",
		}
	}
	return CheckResult{
		ID:     CheckDockerDaemon,
		Status: StatusPass,
		Title:  "Docker daemon",
		Detail: "reachable",
	}
}

func (c Coordinator) checkDockerVersion(ctx context.Context) CheckResult {
	ver, err := c.Docker.Version(ctx)
	if err != nil {
		return CheckResult{
			ID:     CheckDockerVersion,
			Status: StatusWarn,
			Title:  "Docker version",
			Detail: "could not determine version: " + err.Error(),
		}
	}

	// Use the lesser of client and server versions for comparison.
	effective := minVersion(ver.Client, ver.Server)
	if !semverGTE(effective, c.MinDockerVersion) {
		return CheckResult{
			ID:     CheckDockerVersion,
			Status: StatusWarn,
			Title:  "Docker version older than recommended",
			Detail: fmt.Sprintf("effective version %s < required %s", effective, c.MinDockerVersion),
			Remediation: fmt.Sprintf(
				"Upgrade Docker to %s or later: https://docs.docker.com/engine/install/",
				c.MinDockerVersion,
			),
		}
	}

	return CheckResult{
		ID:     CheckDockerVersion,
		Status: StatusPass,
		Title:  "Docker version",
		Detail: fmt.Sprintf("client %s / server %s", ver.Client, ver.Server),
	}
}

func (c Coordinator) checkComposeVersion(ctx context.Context) CheckResult {
	ver, err := c.Compose.Version(ctx)
	if err != nil {
		return CheckResult{
			ID:          CheckComposeVersion,
			Status:      StatusFail,
			Title:       "Compose v2 plugin required",
			Detail:      "could not determine compose version: " + err.Error(),
			Remediation: "Install the Docker Compose v2 plugin: https://docs.docker.com/compose/install/",
		}
	}

	if !ver.V2Plugin || !semverGTE(ver.Raw, c.MinComposeVersion) {
		detail := fmt.Sprintf("version %s, v2Plugin=%v", ver.Raw, ver.V2Plugin)
		return CheckResult{
			ID:          CheckComposeVersion,
			Status:      StatusFail,
			Title:       "Compose v2 plugin required",
			Detail:      detail,
			Remediation: "Install the Docker Compose v2 plugin: https://docs.docker.com/compose/install/",
		}
	}

	return CheckResult{
		ID:     CheckComposeVersion,
		Status: StatusPass,
		Title:  "Docker Compose version",
		Detail: ver.Raw,
	}
}

func (c Coordinator) checkGPU(ctx context.Context) CheckResult {
	info := c.GPU.Detect(ctx)
	if info.ToolkitInstalled && info.RuntimeAvailable {
		return CheckResult{
			ID:     CheckGPU,
			Status: StatusPass,
			Title:  "NVIDIA Container Toolkit",
			Detail: "detected",
		}
	}
	return CheckResult{
		ID:     CheckGPU,
		Status: StatusWarn,
		Title:  "NVIDIA Container Toolkit not detected",
		Detail: "backend will run on CPU",
		Remediation: "Install from https://docs.nvidia.com/datacenter/cloud-native/" +
			"container-toolkit/install-guide.html",
	}
}

func (c Coordinator) checkDir(id CheckID, path string) CheckResult {
	title := fmt.Sprintf("Directory writable: %s", path)
	ok, reason := c.Dirs.IsWritable(path)
	if !ok {
		return CheckResult{
			ID:          id,
			Status:      StatusFail,
			Title:       title,
			Detail:      reason,
			Remediation: fmt.Sprintf("Run: sudo mkdir -p %s && sudo chown $USER %s", path, path),
		}
	}
	return CheckResult{
		ID:     id,
		Status: StatusPass,
		Title:  title,
		Detail: "writable",
	}
}

func (c Coordinator) checkPorts(ctx context.Context) CheckResult {
	var occupied []string

	for _, p := range c.RequiredTCPPorts {
		if !c.Ports.IsAvailable(ctx, p) {
			occupied = append(occupied, fmt.Sprintf("TCP %d", p))
		}
	}
	for _, p := range c.RequiredUDPPorts {
		if !c.Ports.IsUDPAvailable(ctx, p) {
			occupied = append(occupied, fmt.Sprintf("UDP %d", p))
		}
	}

	if len(occupied) > 0 {
		return CheckResult{
			ID:     CheckPortsAvailable,
			Status: StatusWarn,
			Title:  "Port conflicts",
			Detail: "occupied: " + strings.Join(occupied, ", "),
			Remediation: "The installer will allow you to choose alternate ports for each conflict.",
		}
	}

	return CheckResult{
		ID:     CheckPortsAvailable,
		Status: StatusPass,
		Title:  "Required ports",
		Detail: "all available",
	}
}

// ---------------------------------------------------------------------------
// Semver helpers (stdlib only — no external packages)
// ---------------------------------------------------------------------------

// semverGTE returns true if version a is >= version b (simple integer comparison
// per major.minor.patch component). Non-numeric components default to 0.
func semverGTE(a, b string) bool {
	aParts := parseSemver(a)
	bParts := parseSemver(b)

	for i := 0; i < 3; i++ {
		if aParts[i] > bParts[i] {
			return true
		}
		if aParts[i] < bParts[i] {
			return false
		}
	}
	return true // equal
}

// parseSemver splits a semver string into [major, minor, patch].
// Non-numeric segments (pre-release suffixes) are stripped.
func parseSemver(v string) [3]int {
	// Strip any leading 'v'.
	v = strings.TrimPrefix(v, "v")
	// Strip pre-release/metadata (e.g. "2.21.0-rc1" → "2.21.0").
	if idx := strings.IndexAny(v, "-+"); idx >= 0 {
		v = v[:idx]
	}

	parts := strings.SplitN(v, ".", 3)
	var result [3]int
	for i := 0; i < len(parts) && i < 3; i++ {
		n, _ := strconv.Atoi(parts[i])
		result[i] = n
	}
	return result
}

// minVersion returns the lesser of two semver strings.
func minVersion(a, b string) string {
	if semverGTE(b, a) {
		return a
	}
	return b
}

