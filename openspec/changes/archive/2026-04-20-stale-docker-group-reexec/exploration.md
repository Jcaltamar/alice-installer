# Exploration: stale-docker-group-reexec

## Current State

`usermod -aG docker $USER` mutates `/etc/group` but does NOT refresh the supplementary GIDs of already-running processes (including the shell that spawned alice-installer). Until the user logs out and back in (or enters a fresh login session via `newgrp docker` / `sg docker`), `getgroups(2)` returns the pre-change GID list and the docker daemon socket rejects the user with `permission denied`.

### What the installer does today

- `bootstrap.DetectEnv()` (`internal/bootstrap/bootstrap.go:87`) populates `UserInDockerGroup` from `user.Current().GroupIds()` — which reflects the **current process** group set, NOT `/etc/group`. If the shell predates `usermod`, this stays `false` even after a successful group add.
- `preflight.checkDockerDaemon` (`internal/preflight/coordinator.go:182`) just runs `docker info` and reports `daemon unreachable` on any non-zero exit — it cannot distinguish "daemon down" from "permission denied on /var/run/docker.sock".
- `ClassifyBlockers` (`internal/bootstrap/bootstrap.go:218`) re-offers `docker_group_add` when `UserInDockerGroup=false`, regardless of whether the user is already in `/etc/group`.
- **Interactive (TUI)**: after `docker_group_add` succeeds, the post-action banner says *"Log out and back in or run `newgrp docker`"* and calls `tea.Quit` (`internal/tui/bootstrap.go:137-157`). If the user ignores the banner and simply re-runs in the same terminal, they hit the same failure and either see "Blocking issues found" or are asked to run the same action again.
- **Headless (`--unattended`)**: `headless.Run` (`internal/headless/run.go:164-175`) detects `docker_group_add` in the fixable set, executes it, then returns `ErrReloginRequired` → exit code 75. A subsequent invocation in a fresh shell (or a fresh `docker exec` in E2E) picks up the new group set.
- **E2E harness** (`scripts/e2e/run.sh:193-216`) already asserts this: it re-invokes the installer with a fresh `docker exec -u testuser` after exit 75, which works because a fresh exec establishes new supplementary groups.

### The gap the user hit

An interactive user in the same terminal session:
1. Installer adds them to docker group
2. They re-run the installer in the SAME terminal (no logout)
3. `DetectEnv` still sees `UserInDockerGroup=false` because the process inherits the shell's stale GIDs
4. `ClassifyBlockers` re-queues `docker_group_add`; if the TUI loop guard already fired (attempted map), the screen shows "Blocking issues found"
5. The user is stuck, even though the underlying config is already correct

## Affected Areas

- `internal/bootstrap/bootstrap.go` — add stale-group detection helpers + re-exec dispatcher
- `cmd/installer/main.go` — call the stale-group check at the earliest point (after `parseFlags`, before any TUI/headless branching)
- `internal/headless/run.go` — benefits automatically from the early-startup check; no direct changes needed, but the `ErrReloginRequired` path remains as a safety net for shells without `sg` available
- `internal/tui/bootstrap.go` — no mandatory changes for Phase 1 (banner+quit still fires if the bootstrap flow itself runs `docker_group_add`)
- `scripts/e2e/run.sh` — add a new assertion pass that exercises the SAME-shell path (no fresh `docker exec`) and verifies the auto-reexec unblocks it
- `scripts/e2e/Dockerfile` — confirm `sg` is present (it ships in `login` on Ubuntu 22.04 — already baseline)
- `.github/workflows/e2e.yml` — no changes needed; already runs on `ubuntu-latest` and invokes `scripts/e2e/run.sh`

## Approaches

### 1. Early-startup stale-group detection + auto re-exec with `sg` (RECOMMENDED)

In `main()` (before TUI/headless branch):
- `bootstrap.DetectStaleDockerGroup()` compares `/etc/group:docker` membership (via `user.LookupGroup`/`user.LookupId` + reading `/etc/group`) vs `syscall.Getgroups()` for the docker GID
- If user IS in `/etc/group:docker` but docker GID is NOT in `getgroups()` → **stale**
- If stale, attempt `bootstrap.ReexecWithDockerGroup(argv)`:
  - `exec.LookPath("sg")` → if found, `syscall.Exec("/usr/bin/sg", ["sg", "docker", "-c", quotedArgv], os.Environ())` replaces the current process
  - New process inherits docker group via PAM group-set reset in `sg`
  - Installer re-enters `main()` normally — detection returns `not stale` → boots normally
- If `sg` not available or Exec fails → print clear actionable message and exit 75

**Pros**:
- Zero TTY teardown issues (runs before TUI initializes)
- Benefits both TUI and `--unattended` paths uniformly
- Single place to add / test / maintain
- E2E can validate the full "same-shell reexec" path in one assertion

**Cons**:
- `syscall.Exec` replaces the process — any global init before the check must be Exec-safe (currently there is none, but future code must respect this constraint)
- `sg docker -c '...'` requires careful arg quoting (we build the command string ourselves)
- `sg` behavior across distros: Ubuntu/Debian `login` package provides `/usr/bin/sg`; RHEL via `util-linux`. Not present on minimal Alpine — we fall back to message+exit in that case.

**Effort**: Medium (new package funcs + main.go hook + unit tests + E2E extension)

### 2. Auto re-exec inside the bootstrap flow (after `docker_group_add` action)

Instead of `tea.Quit` + banner, run `syscall.Exec("sg", ...)` directly from the TUI after the bootstrap completes.

**Pros**:
- Same session "just works" end-to-end

**Cons**:
- Complex: must tear down Bubbletea's alt-screen, flush pending tea.Cmds, then Exec
- Stdin/stdout/stderr state after alt-screen teardown is non-trivial; `sg` may inherit a half-restored TTY
- Duplicated effort: headless path still needs its own reexec (or continues to use exit 75)

**Effort**: High

### 3. Keep banner, improve detection + messaging

At startup, detect stale-group → print a very clear actionable message (with exact copy-paste command) and exit 75. No auto-reexec.

**Pros**:
- Simplest; zero new OS dependencies
- No Exec semantics to worry about

**Cons**:
- Still requires user action — doesn't "just work"
- The user explicitly asked for auto-mitigation

**Effort**: Low

## Recommendation

**Approach 1** (early-startup detection + `sg` reexec, fallback to message+exit 75).

Reasons:
- Matches the user's explicit ask ("auto-reexec via `sg`; fallback to clear instructions")
- Minimal blast radius: one new package file + one call site in `main()`
- Works for TUI and headless equally without duplicating logic
- The E2E harness can prove it end-to-end: currently the fresh `docker exec` masks the stale-session path. We add a NEW pass that runs the installer in the SAME shell session (e.g., via `bash -lc 'alice-installer ...'` after the group add) and asserts exit 0 without a separate relogin step.

Tests (Strict TDD, Go):
- `internal/bootstrap/stale_group_test.go` — table-driven cases covering: stale true, stale false, user not in /etc/group, missing docker group, malformed /etc/group
- `internal/bootstrap/reexec_test.go` — `ReexecWithDockerGroup` returns correct path/args; injected `execFn` verifies call without actually exec'ing
- `cmd/installer/main_test.go` — new case: factory returns a BootstrapEnv reporting stale; `run()` calls injected re-exec function and returns its exit code
- `scripts/e2e/run.sh` — new pass: after usermod, run `docker exec -u testuser "$CID" bash -lc '/home/testuser/alice-installer --unattended ...'` and assert exit 0 in that SAME shell. This exercises `sg` reexec for real on Ubuntu 22.04 in CI.

## Risks

- **`sg` argument escaping**: building a single command string for `sg -c` means os.Args must be properly shell-quoted. Mitigation: vendor a tiny `shellQuote()` helper (or use `%q` with Posix quoting rules). Unit-test against argv containing spaces, quotes, backslashes, and absolute paths.
- **`sg` unavailable on minimal images**: Alpine/BusyBox won't have it. Mitigation: fall back to exit 75 with a clear message listing `newgrp docker && alice-installer ...` as the manual path.
- **`syscall.Exec` and CGO_ENABLED**: We build with `CGO_ENABLED=0` for the E2E binary. `syscall.Exec` is a pure-Go wrapper — compatible.
- **Group password**: `sg` can prompt for a group password if `/etc/gshadow` sets one. Docker group typically has no password. Mitigation: document this edge case in RUNBOOK.md; if `sg` stalls, user can always `Ctrl+C` and fall back to manual relogin.
- **PAM integration**: `sg` via `-c` launches a shell that sources profile scripts. If the user's shell profile crashes, `sg` returns non-zero. Mitigation: on Exec failure, caller reports exit code 75 and guides the user to manual relogin.

## Ready for Proposal

**Yes** — Approach 1 is well-scoped, the file impacts are clear, Strict-TDD test vectors are concrete, and the E2E harness already runs on `ubuntu-latest` in GitHub Actions. Proceeding to `/sdd-propose`.
