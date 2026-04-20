# Tasks: Stale Docker-Group Auto Re-exec

> Strict TDD is active for this project. Every implementation task is preceded by a failing-test task (RED → GREEN). No implementation MUST be written before its test file exists and fails.

## Phase 1: Foundation — Shell Quoting

- [x] 1.1 **RED** — Create `internal/bootstrap/stale_group_test.go` with a table-driven test `TestShellQuote` covering: plain `abc`, spaces, single-quote, double-quote, backslash, empty string, already-quoted input. Verify each wraps in `'...'` with embedded `'` escaped as `'\''`. Ensure `go test ./internal/bootstrap -run TestShellQuote` fails (symbol not defined).
- [x] 1.2 **GREEN** — Create `internal/bootstrap/stale_group.go` with `shellQuote(s string) string`. Make 1.1 pass.

## Phase 2: Foundation — Detection

- [x] 2.1 **RED** — Add `TestDetectStaleDockerGroup` table with fakes (`readGroupFn`, `getgroupsFn`, `currentFn`). Cover 4 spec scenarios: stale true, stale false (gid present), user not in `/etc/group:docker`, no `docker` entry. Also add a malformed-`/etc/group` case (expect `stale=false`, no error). Run → fails.
- [x] 2.2 **GREEN** — In `stale_group.go` add `StaleGroupResult`, `staleGroupDetector` struct with function seams, and public `DetectStaleDockerGroup()` that wires real `os.ReadFile`, `syscall.Getgroups`, `user.Current`. Make 2.1 pass.

## Phase 3: Foundation — Re-exec

- [x] 3.1 **RED** — Add `TestReexecWithDockerGroup` with fake `lookPathFn` and `execFn` (records call). Cover: sg found + exec-ok (records `/usr/bin/sg`, args `["sg","docker","-c",<quoted>]`, env pass-through); sg missing → typed error; execFn returns error → returned unchanged. Run → fails.
- [x] 3.2 **GREEN** — Implement `ReexecWithDockerGroup(argv, env []string) error` using injected seams (default: `exec.LookPath`, `syscall.Exec`). Build `sg -c` string via `shellQuote`. Make 3.1 pass.

## Phase 4: Wiring in Main

- [x] 4.1 **RED** — Extend `cmd/installer/main_test.go` with `TestRunStaleGroupReexecSuccess` (stale=true, execFn ok → `run` calls execFn and returns its exit) and `TestRunStaleGroupReexecFallback` (sg missing → stderr contains literal `newgrp docker`, returns 75). Inject detector + reexec via a new `depsFactoryFunc` field or thin wrapper. Run → fails.
- [x] 4.2 **GREEN** — In `cmd/installer/main.go::run()`, insert stale-group gate after `parseFlags` (before `ShowVersion`/`DryRun`/factory). On `stale=true` call re-exec helper; on error print fallback message (`newgrp docker && alice-installer <original args>`) and return 75. Thread injection seams so tests can stub. Make 4.1 pass.

## Phase 5: E2E on Ubuntu

- [x] 5.1 Verify `scripts/e2e/Dockerfile` has `sg` present (Ubuntu 22.04 `login` package is baseline). If missing, add `RUN apt-get install -y login` to the image.
- [x] 5.2 Extend `scripts/e2e/run.sh` with a new assertion pass: after the usermod pass that yields exit 75, re-invoke the installer via `docker exec -u testuser "$CID" bash -lc '/home/testuser/alice-installer --unattended ...'` inside the SAME shell (no fresh exec privilege boost). Assert exit 0. Leave the existing fresh-exec pass intact as a parallel safety net.
- [x] 5.3 Dry-run the E2E script locally if possible (`scripts/e2e/run.sh`). CI path: confirm `.github/workflows/e2e.yml` picks it up on `ubuntu-latest` with no workflow changes.

## Phase 6: Verify + Polish

- [x] 6.1 Run `go vet ./...` and `go test ./... -cover` — all packages pass, coverage for `internal/bootstrap` does not regress.
- [x] 6.2 Confirm `go build ./cmd/installer` succeeds with `CGO_ENABLED=0 GOOS=linux GOARCH=amd64`.
- [x] 6.3 Append to `RUNBOOK.md` a short "Stale docker-group recovery" entry describing the auto `sg` re-exec and the exit-75 fallback.
