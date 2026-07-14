# Post-Slice-3B Technical Audit — legacy-database-restore

**Status:** **PASS — zero technical CRITICAL findings; Slice 4 is GO**  
**Scope:** Fresh uncached POST-SLICE-3B technical audit after the latest 134-line remediation. This is not final verification and creates no formal review authority.

## Executive summary

The two prior CRITICAL findings are closed. Production database evidence is now derived only from stdout captured from the actually executed validation step. The capture is bounded to 128 bytes; absent, zero-table, malformed, contradictory, or process-failed evidence returns zero `DatabaseEvidence` and a bounded replacement error. No raw stdout crosses the adapter boundary.

Rollback database replacement is now gated by both the backend-only stop attempt and a separate positive `BackendStoppedProbe` result obtained with a fresh bounded context. Stop error, cancellation, probe error, or false/ambiguous state returns explicit `RestorePartialCutover` / `restore-rollback-stop` and performs no rollback revalidation or second replacement.

Fresh focused, package, and full uncached tests pass, as do vet, build, formatting/diff diagnostics, and safety scans. No activation, secret-bearing argv, shell execution, PostgreSQL service control, merge, retry, or validated-backup deletion was found.

## Inputs, status, and action context

Read the proposal, exploration, full spec, design, tasks, apply progress, prior audits, prior verify report, `openspec/config.yaml`, and Slice 1–3B production/tests. Active change and repository ownership are unambiguous. Artifact store is OpenSpec; action context is repo-local at `/home/dev/Documents/works/ebenezer/alice-installer`, within the authoritative workspace. Verification edited only this audit and `verify-report.md`.

Strict TDD is active. `apply-progress.md` contains `TDD Cycle Evidence`; referenced test files exist; fresh uncached GREEN remains true. Assertions exercise outputs, executed process boundaries, replacement counts, backend state, error codes, and call order rather than tautologies, ghost loops, type-only checks, smoke-only checks, or CSS details.

## Prior CRITICAL remediation recheck

### C1 — Actual bounded production validation evidence

**Verdict:** PASS; prior CRITICAL closed.

- `BuildTargetReplacement` makes validation the fifth and final reviewed process and requests headerless, unaligned `psql` output.
- `RunTargetReplacement` sends stdout to `io.Discard` for the first four steps and only the actually submitted validation process to `boundedOutput`.
- Capture is limited to 128 bytes. `OSBinaryExecutor` treats writer short-write failure as process failure; process failure yields zero evidence.
- `TargetReplacementAdapter.Replace` parses only `ReplacementResult.validationOutput` after the complete real plan reports success.
- The protocol requires exactly three fields: target connection `t`, a positive unsigned non-system table count, and PostgreSQL reachability `t`.
- Empty output, zero, non-numeric count, contradictory reachability, extra/malformed fields, and failed process outcomes return zero evidence and `ErrReplacementPrecondition`.
- Raw validation output is private and never enters `RestoreResult`, errors, logs, or argv.

### C2 — Positive stopped-state proof before rollback replacement

**Verdict:** PASS; prior CRITICAL closed.

- `rollback` first attempts `StopService(..., "backend")` with a fresh bounded recovery context.
- It then obtains a separate fresh bounded context and calls `BackendStoppedProbe`.
- Replacement can proceed only when `stopErr == nil`, `proofErr == nil`, and `stopped == true`.
- Stop error, cancelled stop, probe error, or false/ambiguous state returns `RestorePartialCutover`, code `restore-rollback-stop`, and no rollback revalidation or second replacement.
- `TestRestoreCoordinatorRollbackRequiresProvenBackendStop` proves all three modeled failures retain exactly one replacement (the primary mutation only) and leave the backend state unproven/running rather than claiming recovery.

## Destructive invariant and failure-matrix recheck

| Check | Verdict |
| --- | --- |
| Exactly one cancellable 60-second wait | PASS |
| Backend-only stop/start identity | PASS |
| Two validated backups before mutation | PASS |
| Exact terminate/drop/create/restore/validate order | PASS |
| Mutation evidence tied to drop submission | PASS |
| Actual bounded validation output | PASS |
| Zero/malformed/contradictory evidence fails closed | PASS |
| Rollback source is validated target backup only | PASS |
| Positive stopped-state proof before rollback replacement | PASS |
| Stop error/cancellation/ambiguous state blocks replacement | PASS |
| Explicit partial-cutover result on rollback-stop uncertainty | PASS |
| Backend health required before success | PASS |
| No runtime/TUI/installer activation | PASS |
| No secrets or `PGPASSWORD` in production argv/evidence | PASS |
| No shell execution | PASS |
| No PostgreSQL service stop/start/restart | PASS |
| No merge or retry behavior | PASS |
| No validated-backup deletion | PASS |
| Required `pg_restore` flags | PASS |

The focused matrix covers wait, primary stop, primary start/health, rollback revalidation/replacement/recovery, cancellation, each database-evidence bit, zero tables, rollback stop error, rollback stop cancellation, and ambiguous stopped-state proof.

## Review workload and PR boundary

The planned `auto-chain` / `feature-branch-chain` boundary was respected: Slice 3B and its focused remediation remain package-only, with no Slice 4 activation, branch, commit, push, PR, or `size:exception`. Apply progress records 134 remediation additions under the `<=200` correction cap. Git cannot independently reconstruct that total because migration files remain untracked among pre-existing workspace work.

## Task completion and archive status

No Slice 3B implementation task remains unchecked. Exact future implementation tasks are:

- [ ] 4.1 RED — Lock route, state, and cancellation behavior
- [ ] 4.2 GREEN — Implement TUI and installer wiring
- [ ] 4.3 TRIANGULATE — Route and platform verification
- [ ] 4.4 REFACTOR — Keep UI policy thin
- [ ] 5.1 RED/GREEN — Add opt-in integration coverage
- [ ] 5.2 TRIANGULATE/REFACTOR — Document operations and recovery

These are CRITICAL final-completeness/archive blockers. Archive is not ready.

## Commands executed

- `go test -count=1 ./internal/migration -run 'Test(TargetReplacementAdapterCrossesProductionBoundary|TargetReplacementAdapterRejectsMalformedOrZeroValidationEvidence|RestoreCoordinatorRollbackRequiresProvenBackendStop|RestoreCoordinatorFailureMatrixKeepsBackendStopped)' -v` — PASS
- `go test -count=1 ./internal/migration ./internal/workspace ./internal/compose` — PASS
- `go test -count=1 ./...` — PASS
- `go vet ./...` — PASS
- `go build ./...` — PASS
- `git diff --check` — PASS
- `gofmt -d internal/migration/restore.go internal/migration/restore_test.go internal/migration/restore_process.go internal/migration/restore_process_test.go internal/migration/restore_types.go internal/migration/restore_backup.go` — PASS (no output)
- Production prohibited-operation scans across coordinator/process/backup files — PASS
- Activation scan across `cmd`, `internal/tui`, and `internal/headless` — PASS
- Required restore-flag scan — PASS
- Production secret-sentinel scan — PASS
- Unchecked-task scan — six exact Slice 4–5 lines remain

## Severity and gate

- **Technical CRITICAL:** 0
- **WARNING:** 1 — the reported 134-line remediation total cannot be independently reconstructed from Git because relevant migration files are untracked
- **SUGGESTION:** 0

**Slice 4: GO.** The package boundary is technically ready for activation wiring. Slice 4 must supply a real fail-closed `BackendStoppedProbe`; this audit does not authorize runtime activation, archive, commit, push, or PR.
