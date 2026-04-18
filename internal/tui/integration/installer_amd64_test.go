//go:build integration

// Package integration contains end-to-end tests that require real system tools
// (docker, docker compose) and compile+run the alice-installer binary.
//
// These tests are skipped by default; run with:
//
//	go test -tags=integration ./internal/tui/integration/...
//
// In CI the test job installs all dependencies (docker, docker compose) before
// running this package. See .github/workflows/ci.yml (integration-amd64 job).
package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// buildBinary compiles the alice-installer binary for the current platform into dir.
// Returns the path to the built binary.
func buildBinary(t *testing.T, dir string) string {
	t.Helper()

	binaryName := "alice-installer"
	binaryPath := filepath.Join(dir, binaryName)

	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/installer")
	// Run from the repo root (two levels up from this package).
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build alice-installer: %v\n%s", err, out)
	}
	return binaryPath
}

// repoRoot walks up from this file's directory to find the go.mod root.
func repoRoot(t *testing.T) string {
	t.Helper()
	// This file is at internal/tui/integration/; repo root is 3 levels up.
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for i := 0; i < 4; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("could not find go.mod (repo root)")
	return ""
}

// checkDockerAvailable skips the test if docker is not in PATH or daemon is unreachable.
func checkDockerAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("docker not in PATH: %v", err)
	}
	if out, err := exec.Command("docker", "info").CombinedOutput(); err != nil {
		t.Skipf("docker daemon not reachable: %v\n%s", err, out)
	}
}

// TestInstallerAMD64_VersionFlag builds the binary and verifies --version exits 0.
// This is the primary smoke test for the amd64 distribution artifact.
//
// REQ-DIST-7: the installer binary must run on linux/amd64.
func TestInstallerAMD64_VersionFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	dir := t.TempDir()
	binary := buildBinary(t, dir)

	cmd := exec.Command(binary, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("--version failed: %v\noutput: %s", err, out)
	}

	output := strings.TrimSpace(string(out))
	if !strings.Contains(output, "alice-installer") {
		t.Errorf("--version output %q does not contain 'alice-installer'", output)
	}
}

// TestInstallerAMD64_DryRun builds the binary, runs --dry-run, and asserts the
// preflight report contains expected check names.
//
// This test requires docker to be available (preflight checks docker daemon).
// It skips if docker is not running or not in PATH.
//
// REQ-PF-1..REQ-PF-7: preflight coordinator runs all checks.
// REQ-DIST-7: binary runs on amd64.
func TestInstallerAMD64_DryRun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	checkDockerAvailable(t)

	dir := t.TempDir()
	binary := buildBinary(t, dir)

	cmd := exec.Command(binary, "--dry-run",
		"--media-dir", dir,
		"--config-dir", dir)
	out, err := cmd.CombinedOutput()
	// exit 0 (all pass) or exit 1 (failures) are both valid here;
	// exit 2 would indicate a flag parsing error, which is a bug.
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 2 {
		t.Fatalf("--dry-run exited 2 (flag error): %s", out)
	}

	output := string(out)

	// Verify preflight report contains OS and Docker check lines.
	if !strings.Contains(output, "Operating system") {
		t.Errorf("dry-run output missing Operating system check: %s", output)
	}
	if !strings.Contains(output, "Docker") {
		t.Errorf("dry-run output missing Docker check: %s", output)
	}
}

// TestInstallerAMD64_FullDeployNote documents why a full up/down smoke test
// is not included in this package.
//
// A real `docker compose up` + health-check + `docker compose down` cycle
// requires:
//  1. Valid container registry access (private images).
//  2. Sufficient disk space and memory for all services.
//  3. A stable network connection.
//  4. Ports 5432, 6379, 9090, etc. to be free.
//
// These conditions are not reliable in standard GitHub Actions runners and would
// make the test flaky. The full-deploy scenario is covered by manual QA on
// staging machines.
//
// If you want to run a full smoke locally:
//
//	make build && ./bin/alice-installer --dry-run   # verify preflight
//	make build && ./bin/alice-installer              # full TUI (needs TTY)
func TestInstallerAMD64_FullDeployNote(t *testing.T) {
	t.Log("Full deploy smoke test is intentionally not automated; see comment in this file.")
}
