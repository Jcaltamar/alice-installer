# Tasks: Verify Running-State Fallback + E2E Service Log Dump

> Strict TDD is active. Every implementation task is preceded by a failing-test task (RED → GREEN). No implementation is written before its test file exists and fails.

## Phase 1: Foundation — compose.IsReady predicate

- [x] 1.1 **RED** — Create `internal/compose/compose_ready_test.go` with `TestIsReady` table-driven over 7 spec scenarios: healthy+running, ""+running, none+running, ""+restarting, unhealthy+running, healthy+exited, ""+"". Ensure `go test ./internal/compose -run TestIsReady` fails (symbol missing).
- [x] 1.2 **GREEN** — In `internal/compose/runner.go`, add `IsReady(s ServiceHealth) bool` implementing `Status=="healthy" || (Status∈{"","none"} && State=="running")`. Run 1.1 → pass.

## Phase 2: Foundation — State capture in ServiceHealth

- [x] 2.1 **RED** — In `internal/compose/runner_test.go`, extend the `TestHealthStatus` (or add a new test) with two JSON fixtures: one row with `"State":"running"` present, one row omitting the State key. Assert the parsed `ServiceHealth.State` matches. Run → fails.
- [x] 2.2 **GREEN** — In `internal/compose/runner.go`: add `State string` to `psLine` (json:"State") and to `ServiceHealth`; populate `ServiceHealth.State = row.State` in `HealthStatus`. Run 2.1 → pass. `go build ./...` must still succeed.

## Phase 3: Headless verify adoption

- [x] 3.1 **RED** — Extend `internal/headless/run_test.go` with new table cases driving `FakeComposeRunner.Healths` through: all ready (healthy+running, ""+running) → exit nil; one crash-loop (""+restarting) → returns err containing service name and state; one healthy+exited → returns err. Run → fails.
- [x] 3.2 **GREEN** — In `internal/headless/run.go`, replace the two `s.Status != "healthy"` checks with `!compose.IsReady(s)`. Include `s.State` in the `unhealthy` message when it's not `"running"`. Run 3.1 → pass.

## Phase 4: TUI verify adoption

- [x] 4.1 **RED** — Extend `internal/tui/verify_test.go`: add scenarios where a service has `Status=""`, `State="restarting"` → `poll()` returns `HealthReportMsg` (not `InstallSuccessMsg`); and `Status=""`, `State="running"` → `InstallSuccessMsg`. Run → fails.
- [x] 4.2 **GREEN** — In `internal/tui/verify.go`, replace the `poll()` short-circuit with `!compose.IsReady(s)` and the `View()` healthy-count likewise. When rendering a service row, append the `State` in parentheses whenever it is NOT `"running"` or when `Status` is empty/none. Run 4.1 → pass.

- [x] 4.3 Regenerate any affected goldens in `internal/tui/testdata/` (if verify view has goldens). Run `go test ./internal/tui/... -run Golden -update` if such a flag exists, otherwise update by hand and re-run.

## Phase 5: Fake + cross-package sanity

- [x] 5.1 Add a short doc comment above `FakeComposeRunner.Healths` noting that `State` is also honoured. No behaviour change.
- [x] 5.2 Run `go test ./...` — confirm no regression elsewhere. Run `go vet ./...`.

## Phase 6: E2E per-service log dump

- [x] 6.1 In `scripts/e2e/run.sh`, change `dump_diagnostics` to accept an optional list of pending services. If args provided, for each service run: `docker exec "$CID" docker compose -f /home/testuser/.config/alice-guardian/docker-compose.yml --env-file /home/testuser/.config/alice-guardian/.env logs --no-color --tail=50 <service>` inside a clearly labeled `--- logs: <svc> ---` block.
- [x] 6.2 In the FULL_DEPLOY branch, after capturing `FULL_EXIT`, parse the installer's final error line for `unhealthy: a, b, c` using `sed`/`awk` to extract service names (strip `(state)` suffix), then pass them to `dump_diagnostics`.
- [x] 6.3 Guard: if parsing finds no services, fall back to calling `dump_diagnostics` with no args (existing journalctl-only behavior).

## Phase 7: Verify + Polish

- [x] 7.1 Run `go vet ./...` and `go test ./... -cover` — all packages green.
- [x] 7.2 `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/installer` succeeds.
- [ ] 7.3 Run `FULL_DEPLOY=1 make e2e` locally. Expected outcome: pull+deploy succeed; verify ALSO completes (web, rtsp pass by running-state rule) UNLESS websocket still crashes — in which case the new diagnostic MUST print the websocket container's logs so we can see why.
