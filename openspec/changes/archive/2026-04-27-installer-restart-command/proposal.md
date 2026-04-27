# Proposal: Installer Restart Command

## Intent

Add `alice-installer restart` so operators can restart an existing deployment in place with the exact semantics of `docker compose restart`, reusing persisted compose/env artifacts and bypassing install, TUI, and unattended setup flows.

## Scope

### In Scope
- Add positional `restart` command routing before normal install flow selection.
- Reuse the same workspace artifact resolution contract as `update`, preferably from a shared resolver, against existing `.env`, `docker-compose.yml`, and optional GPU overlay artifacts.
- Execute restart through `compose.ComposeRunner` as `docker compose restart`, with non-interactive error propagation and docs for Linux amd64/arm64 and Windows amd64.

### Out of Scope
- Replacing restart with `down`/`up`, `pull`/`up`, or any install/bootstrap fallback.
- Regenerating workspace artifacts or changing current TUI/`--unattended` behavior.

## Capabilities

### New Capabilities
- `installer-restart-command`: Supports restarting an existing installation from persisted compose artifacts via a dedicated CLI command.

### Modified Capabilities
- None.

## Approach

Introduce a dedicated restart path in `cmd/installer` that recognizes `alice-installer restart` and exits before Bubbletea/headless install branches. Extract or share update’s artifact resolver so update and restart target the same files from one source of truth. Extend `ComposeRunner` and its fakes with `Restart`, implemented as exact `docker compose restart`.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `cmd/installer/main.go` | Modified | Route `restart` without breaking update/install entrypoints. |
| `cmd/installer/main_test.go` | Modified | Add table-driven routing/failure coverage and preserve legacy install behavior. |
| `internal/update/run.go` | Modified | Extract/shared artifact resolution baseline used by update and restart. |
| `internal/update/run_test.go` | Modified | Lock shared resolution behavior, including optional GPU overlay targeting. |
| `internal/compose/runner.go`, `internal/compose/fake.go` | Modified | Add/test exact restart abstraction. |
| `README.md`, `RUNBOOK.md` | Modified | Document restart semantics and persisted-artifact expectations. |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Parser regressions across positional commands | Med | Add parser/routing tests first. |
| Update/restart artifact drift | Med | Share one resolver instead of duplicating logic. |
| Semantic drift from exact restart behavior | Low | Spec and test `Restart` as `docker compose restart` only. |

## Rollback Plan

Remove the `restart` route, shared resolver changes, and docs; keep existing update/install paths unchanged.

## Dependencies

- Existing workspace artifacts from a prior install.
- Strict TDD with `go test ./...`.

## Success Criteria

- [ ] `alice-installer restart` runs exact compose restart against existing resolved artifacts.
- [ ] Update and restart share the same artifact targeting rules without drift.
- [ ] TUI, unattended install, and update behaviors remain unchanged when `restart` is not used.
