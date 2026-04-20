# Proposal: Verify Running-State Fallback + E2E Service Log Dump

## Intent

`FULL_DEPLOY=1 make e2e` times out because half the compose services have no healthcheck → `docker compose ps` reports `Health: ""` for them → `internal/headless/run.go:327` treats anything non-`"healthy"` as unhealthy → timeout is inevitable. Meanwhile, the TUI's existing loophole (`accept Status == "" || "none"`) silently passes crash-looping services. Fix the verify semantic in both paths AND surface real failures (like the websocket container's current crash-loop) by dumping nested container logs when a FULL_DEPLOY E2E fails.

## Scope

### In Scope
- Add `State` to `compose.ServiceHealth` and parse it from `docker compose ps --format json`
- Rewrite the verify acceptance rule in `internal/headless/run.go` AND `internal/tui/verify.go` to: `Health=="healthy" OR (Health in {"","none"} AND State=="running")`
- Extend `internal/compose/fake.go` with the new field so existing tests keep compiling
- Add unit tests covering: healthy+running, no-healthcheck+running, healthy+exited, no-healthcheck+restarting, unhealthy+running, unhealthy+exited
- Update TUI verify view so users can see `State` when it disagrees with `Health`
- Regenerate any affected golden files
- Extend `scripts/e2e/run.sh` `dump_diagnostics` to run `docker exec $CID docker compose -f ... logs <service>` for every service that never reached acceptance, capped at 50 lines per service

### Out of Scope
- Adding healthcheck blocks to `web`, `rtsp`, `websocket` in `internal/assets/docker-compose.yml` (separate concern — those services rely on the new State-based check)
- Diagnosing or fixing the `alice_websocket` crash-loop itself — this change only surfaces it cleanly; the fix happens in a follow-up change informed by the new diagnostic output
- Timeout tuning

## Capabilities

### New Capabilities
- `installer-service-verify`: the rules under which compose-managed services are accepted as "ready" during install verification (both TUI and unattended paths) + diagnostic surfacing when they aren't

### Modified Capabilities
- None — the existing specs directory has no verify-related capability yet

## Approach

Recommended approach #1 from exploration: extend `compose.ServiceHealth` with a `State string`, populate from `psLine.State`, and apply the two-condition acceptance rule in both verify call-sites. Harness diagnostics iterate over pending services and dump per-service compose logs via the same compose files/env the installer wrote.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/compose/runner.go` | Modified | Add `State` to `psLine` and `ServiceHealth`; populate both |
| `internal/compose/runner_test.go` | Modified | JSON-parse tests covering `State` |
| `internal/compose/fake.go` | Modified | Add `State` field (no behaviour) |
| `internal/headless/run.go` | Modified | Verify loop uses new rule |
| `internal/headless/run_test.go` | Modified | New table cases for all Health×State combos |
| `internal/tui/verify.go` | Modified | Verify loop and view use new rule |
| `internal/tui/verify_test.go` | Modified | Same coverage |
| `internal/tui/testdata/*.golden` | Modified | Regenerate verify view goldens if present |
| `scripts/e2e/run.sh` | Modified | Per-service compose log dump on timeout |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| `State` key absent in older compose JSON | Low | Default to `""` → falls through to "not OK" → conservative bias; unit-test both presence and absence |
| TUI golden files drift | Med | Regenerate deliberately in this change; commit updated goldens |
| Per-service log dump floods CI output on real failures | Low | Cap at 50 lines per service (matches existing convention) |
| Race: service momentarily in `created`/`starting` | Low | Existing 3 s poll + 60 s timeout handles it unchanged |

## Rollback Plan

Revert the commit. No persisted state, no external API, no data migration. Pre-existing callers of `ServiceHealth{Service,Status}` keep working because we only add a field.

## Dependencies

- `docker compose` v2 with `--format json` output (already required)

## Success Criteria

- [ ] `go test ./...` green; new verify-rule cases pass
- [ ] `FULL_DEPLOY=1 make e2e` surfaces the REAL `alice_websocket` failure with readable container logs (instead of silent timeout)
- [ ] Services that legitimately have no healthcheck but are running pass verify cleanly
- [ ] A service with no healthcheck that is crash-looping (State `restarting`) fails verify with clear error (NOT accidentally passed by the empty-Health loophole)
