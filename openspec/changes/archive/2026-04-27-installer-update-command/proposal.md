# Proposal: Installer Update Command

## Intent

Add `alice-installer update` so operators can refresh an existing deployment in place using the already-written compose assets, without rerunning install, TUI, bootstrap, or env generation flows.

## Scope

### In Scope
- Add positional `update` command routing before normal install flow selection.
- Reuse `compose.ComposeRunner` to run `docker compose pull` and then `docker compose up -d` against the existing workspace compose files.
- Define non-interactive behavior, error propagation, and docs for update usage across supported targets (Linux amd64/arm64, Windows amd64).

### Out of Scope
- Rewriting `.env` or compose files during update.
- Changing install, TUI, `--unattended`, bootstrap, or verify semantics beyond preserving current behavior.

## Capabilities

### New Capabilities
- `installer-update-command`: Supports updating an existing installation from persisted compose artifacts via a dedicated CLI subcommand.

### Modified Capabilities
- None.

## Approach

Introduce a dedicated update path in `cmd/installer` that recognizes `alice-installer update` unambiguously, resolves the current workspace compose/env artifacts, and executes `ComposeRunner.Pull` followed by `ComposeRunner.Up`. Keep execution inside existing compose abstractions and fakes; do not add raw `exec.Command` calls in CLI parsing. The update path must bypass Bubbletea and headless install stages entirely.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `cmd/installer/main.go` | Modified | Route positional `update` without breaking current flags/install entrypoints. |
| `cmd/installer/main_test.go` | Modified | Add TDD coverage for routing, ordering, exit codes, and preserved legacy behavior. |
| `internal/compose/runner.go` | Modified/Validated | Reuse current pull/up APIs and confirm update inputs match existing compose contract. |
| `internal/compose/fake.go` | Modified/Validated | Ensure tests can assert pull-then-up sequencing through existing fake seams. |
| `README.md`, `RUNBOOK.md` | Modified | Document update command, workspace expectations, and non-interactive semantics. |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Positional parsing breaks current flag UX | Med | Add parser tests covering legacy flag-only invocations and subcommand forms first. |
| Wrong workspace or env file updated | Med | Specify deterministic artifact resolution contract in specs before implementation. |
| Update path drifts from compose error/reporting behavior | Low | Mandate `ComposeRunner` reuse and fake-based tests instead of direct exec. |

## Rollback Plan

Remove the `update` route and related docs, returning all invocations to the current install-only entrypoints; no persisted data migration is introduced.

## Dependencies

- Existing workspace artifacts (`docker-compose.yml`, optional overlay/env file) produced by a prior install.
- Strict TDD with `go test ./...`.

## Success Criteria

- [ ] `alice-installer update` runs `pull` then `up -d` through compose abstractions using existing workspace artifacts.
- [ ] Normal install, TUI, and `--unattended` flows behave exactly as before when `update` is not used.
- [ ] Tests cover routing, sequencing, and error handling with fakes; docs explain operator expectations.
