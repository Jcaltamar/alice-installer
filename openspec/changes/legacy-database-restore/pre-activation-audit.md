# Pre-Activation Technical Audit — Slices 1, 2, and 3A (Fresh Post-Remediation Rerun)

**Change:** `legacy-database-restore`  
**Audit type:** Focused PRE-ACTIVATION technical audit only  
**Status:** **PASS — zero technical CRITICAL findings; Slice 3B is GO**  
**Authority:** This report is not final change verification, does not mark incomplete tasks complete, and creates no formal review authority.

## Executive summary

Fresh uncached focused and full tests pass, as do build, vet, formatting, diff, activation, secret, and prohibited-operation scans. The latest 76-line C2/C3 remediation closes both previous technical blockers.

Production legacy revalidation is now immutably rooted at `/opt/alice/backups/`. Traversal, sibling paths, final symlinks, intermediate symlink escapes, non-directory intermediates, and a validator-window intermediate path swap are rejected and covered behaviorally. The replacement builder returns an immutable five-step plan; the production runner executes that plan in exact terminate/drop/create/restore/validate order, and selected failure tests capture every actually submitted `ProcessSpec` while proving truthful failed-step and mutation evidence.

No coordinator, runtime/TUI/installer activation, shell execution, secret-bearing argv, PostgreSQL service stop/restart, merge, or validated-backup deletion was found. `pg_restore` includes `--exit-on-error`, `--no-owner`, `--no-privileges`, and `--no-password`.

## Inputs, status, and action context

Read proposal, full spec, design, tasks, apply progress, previous verify report, previous audit, `openspec/config.yaml`, and implemented Slice 1/2/3A production/tests. Active change and repository ownership are unambiguous. Action context is repo-local at `/home/dev/Documents/works/ebenezer/alice-installer`; verification edited only this audit and `verify-report.md`.

Strict TDD is active. `apply-progress.md` contains TDD Cycle Evidence, referenced test files exist, fresh uncached tests remain GREEN, and the corrected assertions are behavioral rather than tautological, smoke-only, type-only, ghost-loop, or implementation-detail CSS checks.

## Remediation findings

### C2 — Immutable backup root and path confinement

**Verdict:** PASS; previous CRITICAL closed.

Concrete evidence:

- `internal/migration/restore_backup.go` defines `const defaultBackupRoot = "/opt/alice/backups/"`.
- `BackupGate` exposes only `Validator`; production `Revalidate` always passes `authoritativeBackupRoot()` to the private `revalidateBackupInRoot` seam. No caller-controlled production root remains.
- `safeRestoreFileInRoot` rejects lexical sibling/traversal escapes, `Lstat`s the root and every relative component, rejects final/intermediate symlinks, rejects non-directory intermediate components, and requires a positive-size regular final file.
- Revalidation runs the complete safe-path/checksum/manifest validation before archive validation and again afterward, detecting the tested validator-window path swap.
- `TestBackupGateUsesImmutableProductionRoot` and `TestRevalidateBackupInRootRejectsEscapesAndSwaps` cover immutable authority, sibling, traversal, final symlink, intermediate symlink, and intermediate path swap behavior.

### C3 — Exact executed plan and truthful evidence

**Verdict:** PASS; previous CRITICAL closed.

Concrete evidence:

- `BuildTargetReplacement` returns `ReplacementPlan` containing exactly five private specs in terminate sessions, drop database, create database, restore archive, validate order.
- `ReplacementPlan.Specs()` deep-copies specs and argv; mutation of the observable view cannot alter execution.
- `RunTargetReplacement` consumes the private plan directly. It marks `Mutated=true` immediately before submitting the drop step and reports the selected `ReplacementStep` from the exact loop index.
- `replacementExecutor` records each actual submitted `ProcessSpec`.
- `TestReplacementPlanSpecsCannotChangeExecutedPlan` proves view immutability.
- `TestRunTargetReplacementRecordsBuiltSpecsAtEveryFailureBoundary` injects failure at each of the five executed steps and compares all submitted specs through the failure against the exact builder plan. Expected mutation is false only for terminate and true for drop/create/restore/validate.

## Rechecked safety invariants

| Check | Verdict |
| --- | --- |
| Production legacy root authority | PASS — immutable `/opt/alice/backups/` |
| Traversal and sibling rejection | PASS |
| Final and intermediate symlink rejection | PASS |
| Tested validator-window intermediate path swap | PASS |
| Executed specs/order | PASS — exact terminate/drop/create/restore/validate plan |
| Selected failure and mutation evidence | PASS at all five submitted steps |
| Runtime activation in `cmd`, `internal/tui`, or `internal/headless` | PASS — no restore-boundary references |
| Secret leakage in audited argv/evidence | PASS — no password/`PGPASSWORD`; fixed `PGPASSFILE` path only |
| Shell execution | PASS — direct `docker` specs; no `sh -c`/`bash -c` |
| PostgreSQL service stop/restart | PASS — absent |
| Merge behavior | PASS — explicit drop/create/restore only |
| Validated-backup deletion | PASS — absent; cleanup remains operation-owned staging/credentials |
| Required restore flags | PASS — `--exit-on-error`, `--no-owner`, `--no-privileges`, `--no-password` |

## Review workload and boundary

The implementation respects the approved `auto-chain` / `feature-branch-chain` pre-activation boundary: only Slices 1, 2, 3A and the focused C2/C3 remediation are implemented. No Slice 3B coordinator, Slice 4 activation, branch, commit, push, PR, or `size:exception` was introduced. The reported remediation is 76 authored code/test additions, within its `<=200` correction cap. Git still cannot independently attribute all historical slice totals because migration files are untracked amid pre-existing workspace work.

## Remaining scope and archive blockers

The following implementation tasks remain unchecked. They are CRITICAL completeness blockers for final verification/archive, but are the explicitly planned future chain slices and do not invalidate the technical GO to begin Slice 3B:

- [ ] 3B.1 RED — Define coordinator transition, mutation, and evidence tests
- [ ] 3B.2 GREEN — Implement the stage-aware coordinator and bounded evidence
- [ ] 3B.3 RED/GREEN — Add automatic rollback and cancellation semantics
- [ ] 3B.4 TRIANGULATE — Verify destructive transitions and failure containment
- [ ] 3B.5 REFACTOR — Isolate coordinator policy and preserve the boundary
- [ ] 4.1 RED — Lock route, state, and cancellation behavior
- [ ] 4.2 GREEN — Implement TUI and installer wiring
- [ ] 4.3 TRIANGULATE — Route and platform verification
- [ ] 4.4 REFACTOR — Keep UI policy thin
- [ ] 5.1 RED/GREEN — Add opt-in integration coverage
- [ ] 5.2 TRIANGULATE/REFACTOR — Document operations and recovery

Archive is not ready.

## Commands executed

- `go test -count=1 ./internal/migration ./internal/workspace ./internal/compose` — PASS
- `go test -count=1 ./...` — PASS
- `go vet ./...` — PASS
- `go build ./...` — PASS
- `git diff --check` — PASS
- `gofmt -d internal/migration/restore_types.go internal/migration/restore_types_test.go internal/workspace/target_env.go internal/workspace/target_env_test.go internal/compose/runner.go internal/compose/fake.go internal/compose/runner_test.go internal/migration/restore_backup.go internal/migration/restore_backup_test.go internal/migration/restore_process.go internal/migration/restore_process_test.go` — PASS (no output)
- Activation reference scan across `cmd`, `internal/tui`, and `internal/headless` — PASS (no matches)
- Prohibited production-token scan across `restore_backup.go` and `restore_process.go` — PASS for shell, `PGPASSWORD`, PostgreSQL service operations, merge, Compose down/restart, and validated-backup deletion
- Required restore-flag scan — PASS
- Immutable-root/static authority scan — PASS

## Severity summary and go/no-go

- **Technical CRITICAL:** 0
- **WARNING:** 1 — historical slice line totals cannot be independently reconstructed from Git because relevant files remain untracked among pre-existing work
- **SUGGESTION:** 0
- **Completeness/archive blockers:** 11 exact unchecked future implementation tasks above

**Slice 3B: GO.** The two required pre-activation technical blockers are closed. This is not archive readiness or formal review authority.
