# Proposal: Stale Docker-Group Auto Re-exec

## Intent

When `usermod -aG docker $USER` runs, `/etc/group` is updated but already-running processes (including the user's shell) keep their pre-change supplementary GIDs until the next login. Users who re-run alice-installer in the SAME terminal hit `docker daemon unreachable (permission denied on /var/run/docker.sock)` and see a dead-end "Blocking issues found" screen — even though the underlying system is correctly configured. Eliminate this UX trap.

## Scope

### In Scope
- Detect at startup: user is in `/etc/group:docker` but docker GID is NOT in `syscall.Getgroups()`
- When stale → auto re-exec the installer via `syscall.Exec("/usr/bin/sg", ["sg", "docker", "-c", <shell-quoted argv>], env)` so the child process inherits the docker group
- Fallback path when `sg` is unavailable or re-exec fails: print an actionable message (exact `newgrp docker && alice-installer ...` command) and exit `75` (`EX_TEMPFAIL`, matching existing `ErrReloginRequired`)
- Applies equally to TUI and `--unattended` paths (single entry-point check in `main.go`)
- Strict-TDD coverage: unit tests for detection, shell-quoting, and re-exec dispatcher
- Extend `scripts/e2e/run.sh` to exercise the SAME-shell path (no fresh `docker exec`) on Ubuntu 22.04 in GitHub Actions `e2e.yml`

### Out of Scope
- Auto re-exec triggered from INSIDE the bootstrap flow (post-`docker_group_add`) — banner + exit stays as-is for now
- Remediation for distros without `sg` (Alpine/BusyBox) — surface clear message only
- NVIDIA Container Toolkit auto-install (unrelated warning from the original report)

## Capabilities

### New Capabilities
- `installer-stale-group-recovery`: startup-time detection of stale docker group membership + automatic `sg` re-exec + fallback exit 75 with actionable guidance

### Modified Capabilities
- None (specs are empty — `openspec/specs/` has no current sources of truth for the installer behavior; prior changes archived)

## Approach

Add a new file `internal/bootstrap/stale_group.go` with:
- `DetectStaleDockerGroup() (stale bool, dockerGID int, err error)` — parses `/etc/group` for docker membership, compares against `syscall.Getgroups()`
- `ReexecWithDockerGroup(argv []string, env []string) error` — calls `syscall.Exec("/usr/bin/sg", [...])` with a shell-quoted command string; returns error on `LookPath` or `Exec` failure

Hook in `cmd/installer/main.go::run()` right after `parseFlags`: if detection returns `stale=true`, call `ReexecWithDockerGroup`; on success the process is replaced, on failure log the fallback message and return exit 75.

Inject both helpers via an interface so tests can fake `/etc/group`, `getgroups`, `LookPath`, and `Exec`.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/bootstrap/stale_group.go` | New | Detection + re-exec helpers |
| `cmd/installer/main.go` | Modified | Early-startup check call site |
| `scripts/e2e/run.sh` | Modified | New assertion: same-shell invocation after usermod |
| `internal/bootstrap/stale_group_test.go` | New | Unit tests — table-driven |
| `cmd/installer/main_test.go` | Modified | New case for stale-group branch |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Shell-quoting bug in `sg -c` command | Med | Dedicated quoter + table-driven tests for spaces/quotes/backslashes |
| `sg` absent on minimal distros | Low | Fallback exit 75 with explicit manual command |
| `syscall.Exec` replaces process → early init loss | Low | Check fires BEFORE any resource allocation |

## Rollback Plan

Revert the commit. No schema / persistence / external-service migrations; change is purely process-startup behavior.

## Dependencies

- `sg` binary (shipped by `login` package on Ubuntu 22.04, baseline in E2E `Dockerfile`)

## Success Criteria

- [ ] Re-running installer in same terminal after `usermod -aG docker` succeeds without manual relogin (TUI + `--unattended`)
- [ ] E2E GitHub Action passes a new same-shell assertion on `ubuntu-latest`
- [ ] Unit tests cover stale detection, quoting, and re-exec dispatch
- [ ] When `sg` is missing, installer exits 75 with a copy-paste-ready command
