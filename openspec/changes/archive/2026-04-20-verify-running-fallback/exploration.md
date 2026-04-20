# Exploration: verify-running-fallback

## Current State

`internal/compose/runner.go:177-201` runs `docker compose ps --format json` and parses each line into `psLine{Service, Health}` → `ServiceHealth{Service, Status}`. Only the `Health` field is captured. The `State` field (container lifecycle: `running`, `exited`, `restarting`, `paused`) is ignored.

### Verify loop in headless (`internal/headless/run.go:316-340`)

```go
for _, s := range statuses {
    if s.Status != "healthy" {
        allHealthy = false
        unhealthy = append(unhealthy, fmt.Sprintf("%s(%s)", s.Service, s.Status))
    }
}
```

Any service whose `Status` is not literally `"healthy"` — including services without a healthcheck (empty string) — is treated as failing. This is why `FULL_DEPLOY=1 make e2e` times out with `unhealthy: rtsp(), web(), websocket()`: three of six services in `docker-compose.yml` have no healthcheck block (lines 66 `websocket`, 82 `web`, 100 `rtsp`), so compose reports empty `Health` for them forever.

### Verify loop in TUI (`internal/tui/verify.go:74-84`)

```go
if s.Status != "healthy" && s.Status != "none" && s.Status != "" {
    allHealthy = false
    break
}
```

The TUI already accepts `"none"` and `""` as OK. **But that masks crash-loops**: a service with no healthcheck that is restarting in a loop (exit 1 → restart → exit 1) will still be reported as healthy because compose has no health info for it. The websocket container in the current compose is exactly that situation.

### E2E harness diagnostics (`scripts/e2e/run.sh:145-150`)

```bash
dump_diagnostics() {
  log "--- container logs (last 50 lines) ---"
  docker logs "$CID" 2>&1 | tail -50
  log "--- journalctl docker ---"
  docker exec "$CID" journalctl -u docker --no-pager 2>/dev/null | tail -50
}
```

`docker logs "$CID"` dumps the OUTER systemd container's logs, not the nested alice_* application containers. When a service crashes, we only see journalctl's "restarting container" lines — not the container's own stdout/stderr explaining WHY it crashed. The websocket failure in FULL_DEPLOY=1 proved this: we got 8+ restart entries in journalctl but zero clue about the actual crash reason.

### Compose JSON output

`docker compose ps --format json` returns per service (relevant fields):
- `Service`, `Name`, `Image`, `Command`
- `State`: `running` | `exited` | `restarting` | `paused` | `created` | `dead`
- `Health`: `healthy` | `unhealthy` | `starting` | `""` (no healthcheck configured)
- `Status`: human string (e.g. `Up 2 minutes (healthy)`)

`Health` alone is insufficient. `State` disambiguates "no healthcheck + running happily" from "no healthcheck + crash-looping".

## Affected Areas

- `internal/compose/runner.go` — `psLine` and `ServiceHealth` must carry `State`; `HealthStatus` must populate both fields
- `internal/compose/runner_test.go` — extend JSON parsing tests to cover the new field
- `internal/compose/fake.go` — `FakeComposeRunner.HealthStatus` return value gets `State` too
- `internal/headless/run.go` — verify loop accepts `Health=="healthy"` OR (`Health` empty/none AND `State=="running"`); reject otherwise
- `internal/headless/run_test.go` — new table cases for every combination
- `internal/tui/verify.go` — same logic change (replace the empty/none short-circuit with the State-aware check); propagate State to the view so users see the distinction
- `internal/tui/verify_test.go` — extend scenarios
- `scripts/e2e/run.sh` — on FULL_DEPLOY failure, before removing the container, dump `docker exec $CID docker compose -f ... logs <service>` for every unhealthy service

## Approaches

### 1. Add `State` to `ServiceHealth` + adopt State-aware verify + per-service log dump (RECOMMENDED)

Extend `ServiceHealth` with `State string`, update both parse sites and both verify loops to apply the two-step rule:

```
service is OK ⇔ Health == "healthy"
              ∨ (Health in {"", "none"} ∧ State == "running")
```

On E2E failure, the harness iterates over services that never reached healthy and dumps each one's compose logs using the workspace's env+compose files.

**Pros**:
- Correct semantics: services without healthcheck but running pass; crash-looping services without healthcheck fail
- Minimal interface change — `ServiceHealth` gets one extra field, no callers break
- Unblocks `FULL_DEPLOY=1` for the three services that legitimately have no healthcheck (rtsp, web, websocket-if-it-weren't-crashing)
- Diagnostic improvement turns the websocket crash from "silent timeout" into "clean error with container logs inline" — perfect signal for follow-up debug
- Both TUI and headless gain the same corrected behaviour — no drift

**Cons**:
- Requires updating test fixtures and mocks in three places (compose, headless, tui)
- The TUI view gets slightly busier (must render both Health and State when they disagree)

**Effort**: Low-Medium

### 2. Add healthcheck blocks to `web`, `rtsp`, `websocket` in `internal/assets/docker-compose.yml`

Instead of fixing verify semantics, make every service healthcheckable. Examples: `curl -f http://localhost:8080/` for web, TCP probe for rtsp, `wget --spider` for websocket.

**Pros**:
- Verify code stays unchanged
- Forces image owners to expose a health signal

**Cons**:
- Bandaid — doesn't fix the underlying verify bug (a future service without healthcheck reintroduces the issue)
- Requires knowing each service's internal health semantics — may add noise of its own if the probe is wrong (e.g., web hasn't finished warming up)
- Still doesn't solve the diagnostics gap — we STILL wouldn't know why websocket crashes
- The current crash-loop likely prevents any healthcheck from succeeding anyway, so we still time out, but now with `websocket(starting)` that eventually becomes `unhealthy`

**Effort**: Medium (per-service probe design) — AND still requires #1 to diagnose

### 3. Keep verify logic, expand timeout, hope services stabilise

Trivial: bump the 60s timeout to 10m.

**Pros**: zero code changes
**Cons**: doesn't actually fix anything, FULL_DEPLOY=1 still times out because websocket crash-loops indefinitely

**Effort**: None

## Recommendation

**Approach 1.** It's the most correct, smallest, and turns a bug (silent passing of no-healthcheck services, silent failing of crash-loops) into surfaced signal. Approach 2 is a duplicate effort layered on top that we can always add later if we want defense in depth. Approach 3 solves nothing.

## Risks

- **`docker compose ps --format json` version skew**: older compose versions may omit `State` or spell it differently. Mitigation: default to treating absent/unknown State as `""`, which with the new rule falls through to "not OK" — safe bias; but verify against compose v2 on Ubuntu 22.04 + macOS + Windows to be sure. Quick local test on the e2e container will confirm.
- **TUI view breakage**: the verify screen's dots and counters key off `Status`; adding State-aware logic must not break golden tests. Mitigation: update golden files as part of the change.
- **Race with just-started services**: during the first 3s after `docker compose up`, a container is `starting` or `created` — not yet `running`. We already poll every 3s and allow up to 60s; the first one-or-two ticks may falsely report unhealthy. Mitigation: no code change needed — the retry loop covers it.
- **Per-service log dump could flood the E2E log on real CI failures**: cap at 50 lines per service, matching the existing `| tail -50` convention.

## Ready for Proposal

**Yes** — approach 1 is concrete, scoped to 4 files + test updates + 1 E2E script change. Proceeding to `/sdd-propose`.
