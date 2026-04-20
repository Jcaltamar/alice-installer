# Design: Stale Docker-Group Auto Re-exec

## Technical Approach

Add a new `internal/bootstrap/stale_group.go` exposing two pure-stdlib helpers that operate on an injectable `staleGroupDetector` struct (same pattern as the existing `envDetector` at `internal/bootstrap/bootstrap.go:53`). Hook them in `cmd/installer/main.go::run()` immediately after `parseFlags` — before any factory construction or TTY check. When detection flags `stale=true`, dispatch `syscall.Exec` via an injected `execFn` seam; on any failure, fall through to the existing exit-75 convention (`ErrReloginRequired`) with a copy-paste-ready message.

This satisfies all five spec requirements in one place: the check runs once, before TUI/headless divergence, and all collaborators are injected → every scenario is unit-testable without touching the real process.

## Architecture Decisions

| Decision | Choice | Alternatives | Rationale |
|----------|--------|--------------|-----------|
| **Where the check lives** | `internal/bootstrap/` (existing package) | New `internal/reexec/` package | The package already owns OS-level preflight mitigation; a new package would fragment the surface |
| **Detection signal** | Parse `/etc/group` + compare to `syscall.Getgroups()` | Parse `id` output; call `user.Current().GroupIds()` | `/etc/group` is the canonical source; `user.Current().GroupIds()` goes through NSS and may hide/include groups differently; parsing is trivial (colon-separated fields) |
| **Re-exec mechanism** | `syscall.Exec("/usr/bin/sg", ["sg", "docker", "-c", quoted], env)` | `exec.Command(...).Run()`; `newgrp` | `syscall.Exec` replaces the current process → clean child, no zombie parent; `newgrp` requires a TTY |
| **Shell quoting** | POSIX single-quote wrapping with `'\''` embedded-quote escape | `%q` (Go escape rules differ from sh); hand-rolled backslash escaping | POSIX single-quote is shell-unambiguous and round-trips through `sh -c` |
| **Test seams** | Function fields on a struct (`getgroupsFn`, `readGroupFn`, `lookPathFn`, `execFn`) | Global vars; build tags | Matches existing `envDetector` pattern at `bootstrap.go:53`; trivial to fake |
| **Fallback exit code** | `75` (`EX_TEMPFAIL`, reuse `ErrReloginRequired`) | New exit code; `1` | Already handled in `main.go::isReloginRequired()`; semantics are identical ("temporary: re-login required") |
| **Call site** | `cmd/installer/main.go::run()` post-`parseFlags` | Inside `newDependencies`; inside headless.Run | Runs once for both paths; pre-dates factory allocations so Exec-safe |

## Data Flow

```
          main()  ──► run(args, out, errOut, factory)
                          │
                          ├─ parseFlags
                          │
                          ├─ NEW: bootstrap.DetectStaleDockerGroup()
                          │        │
                          │        ├─ readGroupFn("/etc/group") ─► parse "docker:x:GID:users..."
                          │        └─ getgroupsFn() ─► []int
                          │
                          │     stale?
                          │     ├─ no ──► continue (unchanged path)
                          │     └─ yes ─►  bootstrap.ReexecWithDockerGroup(argv, env)
                          │                  │
                          │                  ├─ lookPathFn("sg") ─► path or err
                          │                  ├─ shellQuote(argv) ─► cmdString
                          │                  └─ execFn(path, ["sg","docker","-c",cmdString], env)
                          │                         │
                          │                         ├─ success ──► PROCESS REPLACED
                          │                         └─ error ───► return err
                          │
                          │     on reexec err:
                          │       ├─ print manual recovery line
                          │       └─ return 75
                          │
                          └─ (continue to dryrun / headless / TUI as today)
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/bootstrap/stale_group.go` | Create | `DetectStaleDockerGroup`, `ReexecWithDockerGroup`, `shellQuote`, detector struct with injected fns |
| `internal/bootstrap/stale_group_test.go` | Create | Table-driven tests for every spec scenario — 10+ cases |
| `cmd/installer/main.go` | Modify | Add stale-group gate after `parseFlags`; inject factory for reexec fn so tests can stub |
| `cmd/installer/main_test.go` | Modify | New cases: stale + sg ok → execFn invoked; stale + no sg → exit 75 with fallback line |
| `scripts/e2e/run.sh` | Modify | New "pass 4" assertion: after usermod pass, run installer via `bash -lc` in same shell → expect exit 0 |
| `.github/workflows/e2e.yml` | No change | Already runs `scripts/e2e/run.sh` on `ubuntu-latest` |
| `scripts/e2e/Dockerfile` | Verify only | Confirm `sg` (from `login` pkg on Ubuntu 22.04) is present; add install line if missing |

## Interfaces / Contracts

```go
// internal/bootstrap/stale_group.go
package bootstrap

type StaleGroupResult struct {
    Stale     bool
    DockerGID int
}

// Detector holds injectable seams for testability.
type staleGroupDetector struct {
    readGroupFn func() ([]byte, error)            // default: os.ReadFile("/etc/group")
    getgroupsFn func() ([]int, error)             // default: syscall.Getgroups
    currentFn   func() (string, error)            // default: user.Current() → Username
}

// DetectStaleDockerGroup returns whether the current user is in /etc/group:docker
// but the docker GID is absent from syscall.Getgroups().
func DetectStaleDockerGroup() (StaleGroupResult, error)

// ReexecWithDockerGroup replaces the current process with `sg docker -c <quoted argv>`.
// Returns an error only if sg is not on PATH or syscall.Exec itself fails;
// on success the call does not return.
func ReexecWithDockerGroup(argv []string, env []string) error

// shellQuote returns s wrapped in single quotes, with embedded single quotes escaped.
func shellQuote(s string) string
```

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | Detection: 4 spec scenarios (stale true, stale false, user not in /etc/group, no docker group in /etc/group); quoting: spaces/quotes/backslashes round-trip; reexec: LookPath error → typed err, execFn invoked with expected argv | Table-driven with fake `readGroupFn`, `getgroupsFn`, `currentFn`, `lookPathFn`, `execFn` (records call) — `t.TempDir()` NOT needed since we don't touch disk |
| Integration | `cmd/installer/main.go` branches: stale + exec-ok → returned; stale + exec-err → exit 75 + stderr line contains `newgrp docker` | Extend `main_test.go`; inject detector + reexec fn via factory |
| E2E | Same-shell path succeeds on Ubuntu 22.04 in CI | New pass in `scripts/e2e/run.sh` invoked via `bash -lc` after `usermod` — asserts exit 0 without fresh `docker exec -u` |

## Migration / Rollout

No migration required. Change is pure startup behavior extension; no persisted state, no external API, no feature flag. Backward-compatible: users who already log out and back in see no difference.

## Open Questions

- None blocking. `sg` presence on Ubuntu 22.04 `login` package is verified; `syscall.Exec` on `CGO_ENABLED=0` builds is stdlib and stable.
