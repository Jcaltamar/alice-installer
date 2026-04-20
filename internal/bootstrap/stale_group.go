// Package bootstrap — stale_group.go
// Detects the "just-added-to-docker-group, haven't re-logged-in" condition and
// auto re-execs the installer under `sg docker -c <argv>` so the child process
// inherits the docker group without requiring a full logout.
package bootstrap

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"
)

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

// errExecFailed is the sentinel returned by the fake execFn in tests to simulate
// a syscall.Exec failure.
var errExecFailed = errors.New("exec failed")

// lookPathError is a test-helper type that implements the error interface to
// simulate exec.LookPath failures.
type lookPathError struct{ name string }

func (e *lookPathError) Error() string { return "exec: " + e.name + ": not found in PATH" }

// ---------------------------------------------------------------------------
// Result type
// ---------------------------------------------------------------------------

// StaleGroupResult is the return value of DetectStaleDockerGroup.
type StaleGroupResult struct {
	// Stale is true when the user is in /etc/group:docker but the docker GID is
	// absent from the current process's supplementary group set.
	Stale bool
	// DockerGID is the numeric GID of the docker group.  Zero when Stale is
	// false or when no docker group exists.
	DockerGID int
}

// ---------------------------------------------------------------------------
// Detection — staleGroupDetector
// ---------------------------------------------------------------------------

// staleGroupDetector holds injectable function dependencies for environment
// detection.  The production path wires real stdlib; tests inject fakes.
// This mirrors the envDetector pattern in bootstrap.go exactly.
type staleGroupDetector struct {
	// readGroupFn returns the raw bytes of /etc/group.
	readGroupFn func() ([]byte, error)
	// getgroupsFn returns the supplementary GIDs of the current process.
	getgroupsFn func() ([]int, error)
	// currentFn returns the current OS username.
	currentFn func() (string, error)
}

// productionStaleDetector returns a staleGroupDetector wired to real stdlib.
func productionStaleDetector() staleGroupDetector {
	return staleGroupDetector{
		readGroupFn: func() ([]byte, error) {
			return os.ReadFile("/etc/group")
		},
		getgroupsFn: func() ([]int, error) {
			return syscall.Getgroups()
		},
		currentFn: func() (string, error) {
			u, err := user.Current()
			if err != nil {
				return "", err
			}
			return u.Username, nil
		},
	}
}

// detect runs the stale-group detection and returns a StaleGroupResult.
// It never returns an error for conditions that simply mean "not stale";
// errors are reserved for unexpected I/O failures.
func (d staleGroupDetector) detect() (StaleGroupResult, error) {
	username, err := d.currentFn()
	if err != nil {
		return StaleGroupResult{}, fmt.Errorf("get current user: %w", err)
	}

	raw, err := d.readGroupFn()
	if err != nil {
		// /etc/group unreadable — treat as not stale so normal flow handles it.
		return StaleGroupResult{}, nil
	}

	dockerGID, inGroup := parseDockerGroup(string(raw), username)
	if !inGroup {
		return StaleGroupResult{}, nil
	}

	processGIDs, err := d.getgroupsFn()
	if err != nil {
		// Cannot determine process groups — treat conservatively as not stale.
		return StaleGroupResult{}, nil
	}

	for _, gid := range processGIDs {
		if gid == dockerGID {
			// GID is already present in the process — NOT stale.
			return StaleGroupResult{Stale: false, DockerGID: dockerGID}, nil
		}
	}

	// User is in /etc/group:docker but GID is absent from the process.
	return StaleGroupResult{Stale: true, DockerGID: dockerGID}, nil
}

// parseDockerGroup scans the /etc/group content for a line beginning with
// "docker:" and returns (gid, true) when username appears in the member list.
// Returns (0, false) for any parse error or if username is not found.
func parseDockerGroup(groupContent, username string) (gid int, found bool) {
	for _, line := range strings.Split(groupContent, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 4)
		if len(parts) != 4 {
			continue
		}
		if parts[0] != "docker" {
			continue
		}
		// parts[2] is the GID field.
		gidVal, err := strconv.Atoi(parts[2])
		if err != nil {
			// Malformed GID — skip.
			continue
		}
		// parts[3] is the comma-separated member list.
		for _, member := range strings.Split(parts[3], ",") {
			if strings.TrimSpace(member) == username {
				return gidVal, true
			}
		}
		// Docker group found but user not in it.
		return 0, false
	}
	return 0, false
}

// DetectStaleDockerGroup detects the stale-group condition using the real host
// environment.  Safe to call at startup; never panics.
func DetectStaleDockerGroup() (StaleGroupResult, error) {
	return productionStaleDetector().detect()
}

// ---------------------------------------------------------------------------
// Shell quoting
// ---------------------------------------------------------------------------

// shellQuote wraps s in POSIX single quotes, escaping any embedded single
// quotes as '\''.  The result is safe to pass as a sh -c argument.
//
// Examples:
//
//	abc        → 'abc'
//	hello world → 'hello world'
//	it's       → 'it'\''s'
//	           → ''  (empty string)
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ShellQuote is the exported version of shellQuote for use in cmd/installer.
func ShellQuote(s string) string { return shellQuote(s) }

// ---------------------------------------------------------------------------
// Re-exec dispatcher
// ---------------------------------------------------------------------------

// reexecDispatcher holds injectable function dependencies for re-execution.
// The production path wires real exec.LookPath and syscall.Exec.
type reexecDispatcher struct {
	// lookPathFn resolves the full path of a binary name.
	lookPathFn func(name string) (string, error)
	// execFn replaces the current process image.  It returns nil only when the
	// real syscall.Exec succeeds — which means it never actually returns on
	// success (the process is replaced).  The fake in tests returns nil to
	// simulate success without replacing the process.
	execFn func(argv0 string, argv []string, envv []string) error
}

// productionReexecDispatcher returns a reexecDispatcher wired to real stdlib.
func productionReexecDispatcher() reexecDispatcher {
	return reexecDispatcher{
		lookPathFn: exec.LookPath,
		execFn: func(argv0 string, argv []string, envv []string) error {
			return syscall.Exec(argv0, argv, envv)
		},
	}
}

// reexec replaces the current process with:
//
//	/usr/bin/sg  sg  docker  -c  '<quoted argv>'
//
// On success the call does not return (process replaced).
// Returns an error if sg is not on PATH or if syscall.Exec itself fails.
func (d reexecDispatcher) reexec(argv []string, env []string) error {
	sgPath, err := d.lookPathFn("sg")
	if err != nil {
		return fmt.Errorf("sg not found: %w", err)
	}

	// Build the shell command string: each argv element single-quoted.
	quotedParts := make([]string, len(argv))
	for i, a := range argv {
		quotedParts[i] = shellQuote(a)
	}
	cmdString := strings.Join(quotedParts, " ")

	sgArgv := []string{"sg", "docker", "-c", cmdString}
	return d.execFn(sgPath, sgArgv, env)
}

// ReexecWithDockerGroup replaces the current process with a new invocation
// executed under `sg docker -c <argv>` so the child process inherits the
// docker group.  Returns an error if sg is not on PATH or if exec fails.
// On success, the call does not return.
func ReexecWithDockerGroup(argv []string, env []string) error {
	return productionReexecDispatcher().reexec(argv, env)
}
