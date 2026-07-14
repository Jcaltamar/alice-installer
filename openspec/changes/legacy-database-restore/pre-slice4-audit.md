# PRE-SLICE-4 Activation Audit — legacy-database-restore

**Status:** **PASS — 0 CRITICAL; Slice 4 is GO**  
**Scope:** Fresh read-only activation audit after the 107-line operation-ID/close-error remediation. No code edits and no formal review authority.

## Executive summary

The two prior activation blockers are closed. Operation IDs are validated against a bounded 1–64 character ASCII filename-safe grammar before the staging callback, staging path construction, or file open. The production adapter repeats that validation before credential preparation and path construction. The authoritative publication root remains immutable at `/opt/alice/backups/`, and caller-selected roots are rejected.

Target staging now captures writer failures, explicitly closes the staging file before validation/publication, and requires close success. Injected write and close failures return a bounded gate error, remove the operation-owned partial stage, invoke no validator, and cannot publish a dump or manifest. Fresh focused and full uncached tests, vet, build, formatting/diff checks, and safety scans pass.

The production adapter graph remains package-only and fail closed. No runtime activation, secret-bearing argv, shell execution, PostgreSQL service stop/start/restart, whole-stack destructive operation in the restore graph, merge, retry, or validated-backup deletion was found.

## Inputs, structured status, and action context

Read the active spec, tasks, apply progress, latest pre-Slice-4 audit, prior verify report, strict-TDD config, remediation code, tests, composition, and coordinator policy. The active change and implementation ownership are unambiguous at `/home/dev/Documents/works/ebenezer/alice-installer`.

The parent supplied no native structured status object, so readiness was resolved from the authoritative OpenSpec artifacts. Tasks and apply progress are present and non-empty. Action context is treated as `repo-local`; the authoritative workspace is the repository root, and this audit edited only the two requested OpenSpec report artifacts.

## Remediation verification

| Required invariant | Verdict | Evidence |
| --- | --- | --- |
| No filesystem side effect before operation-ID validation | PASS | `TargetRollbackBackupCreator.CreateValidated` calls `validOperationID` before `Stage`; hostile IDs produce zero staging calls. `TargetRollbackBackupAdapter.stage` repeats validation before credentials, path construction, and `openStaging`. |
| Immutable root containment | PASS | Production `CreateValidated` rejects any destination not cleaning to `authoritativeBackupRoot()` and invokes the creator with `/opt/alice/backups/`; staging name is derived only after ID validation and later checked by `safeTargetStaging`. |
| Explicit close success before validation/publication | PASS | `stage` calls `file.Close()` synchronously and returns the staged path only on success; creator validation/publication occurs only after `stage` returns successfully. |
| Write failure cleanup/no publication | PASS | `stagingWriter` records writer failures even if an executor reports success; failure calls cleanup, removes only the `.target-rollback-<id>.part` path, and validator call count remains zero. |
| Close failure cleanup/no publication | PASS | Explicit close failure removes the operation-owned stage and returns the gate error; injected close-failure test observes one close, no stage, and no validator invocation. |

## Production adapter graph and destructive invariants

| Capability/invariant | Verdict |
| --- | --- |
| `RealWaiter` and exact one `60*time.Second` wait | PASS |
| Generated env reader and private password | PASS |
| Protected pgpass transport; no `PGPASSWORD` | PASS |
| Legacy archive revalidation under immutable root | PASS |
| Target rollback staging/validation/atomic publication | PASS after remediation |
| Direct-argv PostgreSQL reachability with exact bounded evidence | PASS |
| Explicit five-step replacement; no merge or retry | PASS |
| Concrete Compose backend health/stopped probes | PASS |
| Backend-only service control | PASS |
| PostgreSQL service stop/start/restart | Absent — PASS |
| Shell construction/execution | Absent — PASS |
| Runtime activation in `cmd`, production TUI, or headless code | Absent — PASS |
| Validated-backup deletion | Absent — PASS; deletion sites are operation-owned credential/staging paths or rollback of incomplete publication |
| Concrete production composition | PASS — env, validator, target backup, replacement, PostgreSQL probe, health/stopped probe, service controller, executor, and waiter are concrete |

Existing Compose `Down`/`Restart` methods remain in the compose package, but the restore production graph does not call them; their mere pre-existing definitions are not activation evidence.

## Strict TDD and assertion quality

Strict TDD is active. `apply-progress.md` contains a `TDD Cycle Evidence` table for C1/C2, and the referenced `restore_backup_test.go` exists. Fresh GREEN was confirmed. The remediation assertions are behavioral: unsafe IDs prove zero stage calls; injected writer/close faults prove exact close count, no validator call, cleanup, and no publication path. No tautology, ghost loop, type-only assertion, smoke-only assertion, or implementation-detail CSS assertion was found.

One non-blocking evidence limitation remains: production composition is proven through concrete constructor types plus controlled adapter/coordinator tests rather than a single end-to-end execution against real Docker/PostgreSQL/filesystem infrastructure. This is acceptable before Slice 4 because default tests must not require destructive infrastructure, but isolated integration evidence remains assigned to Slice 5.

## Review workload / PR boundary

The `auto-chain` / `feature-branch-chain` boundary remains respected. The remediation is limited to C1/C2 and reports 107 authored lines. No Slice 4 activation, branch, commit, push, PR, or `size:exception` was introduced. The current audit makes no lifecycle or publication decision.

## Task completion and archive status

Exact unchecked implementation tasks remain:

- [ ] 4.1 RED — Lock route, state, and cancellation behavior
- [ ] 4.2 GREEN — Implement TUI and installer wiring
- [ ] 4.3 TRIANGULATE — Route and platform verification
- [ ] 4.4 REFACTOR — Keep UI policy thin
- [ ] 5.1 RED/GREEN — Add opt-in integration coverage
- [ ] 5.2 TRIANGULATE/REFACTOR — Document operations and recovery

These are remaining planned scope and CRITICAL archive-completeness blockers. They do not block beginning the dependency-ready Slice 4, but archive is not ready and this is not a clean final-change verification pass.

## Commands executed

- `go test -count=1 ./internal/migration -run 'TestTargetRollback(CreatorRejectsUnsafeOperationIDsBeforeStaging|BackupAdapterStagingFaultsBlockValidationAndPublication|BackupAdapter)' -v` — PASS
- `go test -count=1 ./internal/migration ./internal/workspace ./internal/compose` — PASS
- `go test -count=1 ./...` — PASS
- `go vet ./...` — PASS
- `go build ./...` — PASS
- `git diff --check` — PASS
- `gofmt -d internal/migration/restore.go internal/migration/restore_composition.go internal/migration/restore_adapters.go internal/migration/restore_backup.go internal/migration/restore_process.go internal/workspace/target_env.go` — PASS (no output)
- Production runtime activation scan across `cmd`, `internal/tui`, and `internal/headless` excluding tests — PASS (no matches)
- Production prohibited-operation scan for `PGPASSWORD`, shell, PostgreSQL service control, Compose down/restart use, merge, and retry — PASS for the restore graph
- Restore deletion-site scan — PASS; only operation-owned credentials/staging and incomplete publication rollback sites found
- Unchecked-task scan — six exact Slice 4–5 task lines remain

## Severity and gate

- **CRITICAL activation findings:** 0
- **WARNING:** 1 — isolated real-infrastructure integration remains deferred to Slice 5
- **SUGGESTION:** 0

**Slice 4: GO.** The zero-CRITICAL activation condition is met. This report grants no archive, commit, push, PR, release, or formal review authority.
