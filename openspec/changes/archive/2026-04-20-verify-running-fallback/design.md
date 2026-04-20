# Design: Verify Running-State Fallback + E2E Service Log Dump

## Technical Approach

Extend `compose.ServiceHealth` with a new `State string` field, populate it from the `State` key of `docker compose ps --format json`, and apply a two-condition acceptance rule at both verify sites (`internal/headless/run.go` and `internal/tui/verify.go`). Existing tests keep compiling because we only ADD a field. The E2E harness gains a per-service log dump stanza wired into the existing `dump_diagnostics` function.

## Architecture Decisions

| Decision | Choice | Alternatives | Rationale |
|----------|--------|--------------|-----------|
| **Carry State alongside Health** | Add `State` to `ServiceHealth` and `psLine` | Return a separate struct; query `docker ps` a second time | One struct keeps call sites simple; compose JSON already emits State → zero extra subprocess overhead |
| **Acceptance rule location** | Shared predicate function `compose.IsReady(ServiceHealth) bool` used by both headless and TUI | Duplicate the logic in each verify site | Single source of truth; easier to test exhaustively; avoids drift like the current `""` vs `"none"` mismatch |
| **Unknown / missing State default** | Empty string → treated as "not ready" | Assume running when State missing | Conservative bias prevents silent false-positives on older compose JSON; the unit-test covers both presence and absence |
| **TUI view surfacing State** | Render a secondary indicator when `Status != State`-implied readiness | Always show both fields | Only show State when it adds information → avoids UI noise for the common healthy case |
| **Backward compat of ServiceHealth** | Add field; never rename existing ones | Introduce a new HealthV2 type | Existing callers (`fullflow_test.go`, fakes) just get a zero value for State → no breakage |
| **E2E log dump invocation** | Derive the unhealthy-set from the `verify` log lines by parsing the LAST `error: verify: ... unhealthy: x, y, z` message | Re-query compose post-timeout | Cheaper; the installer's own error message already enumerates what failed; no extra docker call from the harness |
| **Log dump volume cap** | 50 lines per service with `docker compose logs --no-color --tail=50 <svc>` | Unlimited; stream to file | Matches existing `| tail -50` convention; keeps CI log readable |

## Data Flow

```
     docker compose ps --format json
              │
              ▼
  psLine { Service, State, Health }     (internal/compose/runner.go)
              │
              ▼
  ServiceHealth { Service, Status, State }
              │
              ├──────────────┬───────────────────┐
              ▼              ▼                   ▼
     headless.Run       tui.VerifyModel    compose.IsReady  (shared predicate)
     (run.go:316)       (verify.go:73)     Health=="healthy"
              │              │              ∨ (Health∈{"","none"} ∧ State=="running")
              ▼              ▼
        exit 0 / err    view + tick
              
     on E2E timeout  ──►  scripts/e2e/run.sh dump_diagnostics:
                           for each pending svc:
                             docker exec $CID docker compose \
                               -f $COMPOSE_FILES -p alice-guardian \
                               logs --no-color --tail=50 <svc>
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/compose/runner.go` | Modify | Add `State` to `psLine` + `ServiceHealth`; populate both; add exported `IsReady(s ServiceHealth) bool` predicate |
| `internal/compose/runner_test.go` | Modify | JSON-parse fixtures covering present/absent State |
| `internal/compose/compose_ready_test.go` | Create | Table-driven tests for `IsReady` across all 7 scenarios from spec |
| `internal/compose/fake.go` | Modify | Nothing structural (zero value picks up State=""); one comment referencing new field |
| `internal/headless/run.go` | Modify | Replace `s.Status != "healthy"` checks with `!compose.IsReady(s)` (2 sites: live loop + final diagnostic) |
| `internal/headless/run_test.go` | Modify | Add Healths now with explicit State values; add table cases for all spec combinations |
| `internal/tui/verify.go` | Modify | Replace the `Status != "" && != "none"` short-circuit with `!compose.IsReady(s)`; update `View()` to render State when relevant; fix the "healthy count" to use `IsReady` too |
| `internal/tui/verify_test.go` | Modify | Healths now include State; add crash-loop + exited scenarios |
| `internal/tui/testdata/*.golden` | Modify if present | Regenerate any verify-view goldens |
| `scripts/e2e/run.sh` | Modify | `dump_diagnostics` takes the pending services as args; iterates and runs `docker compose logs --tail=50 <svc>` via docker exec |

## Interfaces / Contracts

```go
// internal/compose/runner.go
type ServiceHealth struct {
    Service string
    Status  string // Health column: "healthy" | "unhealthy" | "starting" | "none" | ""
    State   string // Lifecycle: "running" | "exited" | "restarting" | "paused" | "created" | "dead" | ""
}

// IsReady returns true when a service is acceptable for the verify stage.
//
// Rule: Status=="healthy" ∨ (Status∈{"","none"} ∧ State=="running")
//
// Services without a healthcheck that are running → ready.
// Services without a healthcheck that are restarting/exited → NOT ready.
func IsReady(s ServiceHealth) bool

// psLine adds State:
type psLine struct {
    Service string `json:"Service"`
    State   string `json:"State"`
    Health  string `json:"Health"`
}
```

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit — compose | `IsReady` across 7 spec scenarios (healthy+running, ""+running, ""+restarting, unhealthy+running, healthy+exited, ""+""-unknown, none+running) | Table-driven in `compose_ready_test.go` |
| Unit — compose | psLine parses State from real-shape JSON; absent State → `""` | Extend `runner_test.go` with two JSON fixtures |
| Unit — headless | `Run` exits 0 when `IsReady` accepts all; non-0 with readable err when any fails; error message includes State when != running | Inject `FakeComposeRunner` with curated Healths |
| Unit — TUI | `VerifyModel.poll()` returns InstallSuccessMsg iff all ready; View renders State for mixed scenarios | teatest or direct Update |
| E2E | `FULL_DEPLOY=1 make e2e` surfaces the real websocket failure with container logs; services without healthcheck but running pass | Run locally + ubuntu-latest CI |

## Migration / Rollout

No migration. Pure code change. Backward-compatible struct extension — all existing callers get `State=""` automatically until they opt in.

## Open Questions

None blocking. The pending-services list in the E2E log dump is parseable from the installer's final error message, which always contains `unhealthy: svc1(state), svc2(state), ...`.
