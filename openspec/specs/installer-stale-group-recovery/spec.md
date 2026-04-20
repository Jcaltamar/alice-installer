# installer-stale-group-recovery Specification

## Purpose

Define the installer's behavior when the current user is already a member of the `docker` group in `/etc/group` but the invoking process's supplementary GID set does not yet include the docker GID (classic "just-added-to-group, haven't relogged-in" state). The installer MUST detect this condition at startup and MUST recover automatically without requiring the user to log out, or MUST exit with an actionable manual command when automatic recovery is not possible.

## Requirements

### Requirement: Stale Docker Group Detection

The installer MUST detect at startup whether the invoking user is in `/etc/group:docker` but the docker GID is absent from the current process's supplementary groups.

The detection MUST run on both interactive (TUI) and `--unattended` paths and MUST fire before any preflight check or bootstrap action.

#### Scenario: User in /etc/group but GID missing from process

- GIVEN the current user's name appears in the `docker` entry of `/etc/group`
- AND `syscall.Getgroups()` does not include the docker GID
- WHEN the installer starts
- THEN detection MUST return `stale=true`
- AND detection MUST return the docker GID

#### Scenario: User in /etc/group and GID present in process

- GIVEN the user is in `/etc/group:docker`
- AND `syscall.Getgroups()` already includes the docker GID
- WHEN the installer starts
- THEN detection MUST return `stale=false`
- AND the installer MUST proceed to normal preflight

#### Scenario: User not in /etc/group

- GIVEN the user is NOT listed in `/etc/group:docker`
- WHEN the installer starts
- THEN detection MUST return `stale=false`
- AND the normal bootstrap flow MUST handle the missing-group case (existing behavior)

#### Scenario: No docker group exists on host

- GIVEN `/etc/group` has no `docker` entry
- WHEN the installer starts
- THEN detection MUST return `stale=false` without error
- AND the normal bootstrap flow MUST offer `docker_install` (existing behavior)

### Requirement: Automatic Re-exec via sg

When detection returns `stale=true`, the installer MUST attempt to replace the current process with a new invocation executed under `sg docker -c <argv>`, so that the replacement process inherits the docker group.

The re-exec helper MUST shell-quote each argv element before passing it to `sg -c`.

#### Scenario: sg present and exec succeeds

- GIVEN detection returned `stale=true`
- AND `/usr/bin/sg` is resolvable via PATH lookup
- WHEN the installer attempts re-exec
- THEN the installer MUST call `syscall.Exec("/usr/bin/sg", ["sg", "docker", "-c", <quoted argv>], env)`
- AND on success control MUST NOT return to the original process

#### Scenario: argv contains characters needing shell-quoting

- GIVEN an argv element contains spaces, single-quotes, or backslashes
- WHEN the re-exec helper builds the `-c` command string
- THEN each element MUST be emitted in single-quoted form with embedded single-quotes escaped as `'\''`
- AND the resulting command string MUST round-trip safely through `sh -c`

### Requirement: Fallback on Re-exec Failure

When detection returns `stale=true` but automatic re-exec is not possible, the installer MUST exit with status `75` (`EX_TEMPFAIL`) and MUST print a copy-paste-ready manual command.

#### Scenario: sg binary not available

- GIVEN detection returned `stale=true`
- AND `exec.LookPath("sg")` returns an error
- WHEN the installer attempts recovery
- THEN the installer MUST print a message including the literal command `newgrp docker && alice-installer <original flags>`
- AND the installer MUST return exit code `75`

#### Scenario: syscall.Exec itself returns an error

- GIVEN `sg` was found on PATH
- WHEN `syscall.Exec` returns a non-nil error (e.g., permission denied)
- THEN the installer MUST print the error
- AND MUST print the manual fallback command
- AND MUST return exit code `75`

### Requirement: Dependency Injection for Testability

The detection and re-exec helpers MUST accept injected seams for `/etc/group` reading, `syscall.Getgroups`, `exec.LookPath`, and `syscall.Exec`, so that tests can verify every scenario above without touching the real process state.

#### Scenario: Tests inject fakes

- GIVEN a test constructs a detector with fake readers
- WHEN the test invokes detection or re-exec
- THEN no real filesystem or process system calls MUST be made
- AND the fakes MUST record the exact arguments that would have been passed

### Requirement: End-to-End Validation on Ubuntu

The GitHub Actions E2E workflow running on `ubuntu-latest` MUST include an assertion that proves the auto re-exec path succeeds in the SAME shell session that performed `usermod -aG docker`, without a separate fresh `docker exec` to refresh group membership.

#### Scenario: Same-shell invocation after usermod

- GIVEN the E2E harness has just completed a pass that ran `docker_group_add`
- WHEN the harness re-invokes the installer inside the same shell session (e.g. via `bash -lc`)
- THEN the installer MUST detect stale, re-exec via `sg`, and exit `0`
