# Legacy Database Restore Implementation Tasks

## Review Workload Forecast

| Field | Value |
| ------- | ------- |
| Estimated changed lines | 1,800–2,240 total; PM2 delta: retained 7A-R foundation 349, 7A-1 PM2/ss adapters 278, 7A-2 proc/snapshot completion 349, 7B-1 recovery/lease 270–320, 7B-2 terminal routing/docs 250–320 (each child hard cap <350) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 safety/env → PR 2 service control/backup gates → PR 3A protected process/replacement builders → PR 3B coordinator/rollback → PR 3C-1 credential/backup bridge → PR 3C-2 probes/composition → PR 4 interactive wiring → PR 5 docs/integration → PR 7A-R retained foundation → PR 7A-1 PM2/ss adapters → PR 7A-2 proc/snapshot completion → PR 7B-1 recovery/lease → PR 7B-2 terminal routing/docs |
| Delivery strategy | force-chained |
| Chain strategy | feature-branch-chain |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High

Apply boundary: **Slices 1–5 and Final Verification Remediation 6.1 are complete. The prior 6.2 verification is invalidated by the PM2 delta: preserve retained 7A-R, execute 7A-1 then 7A-2, then 7B, then rerun final verification; archive remains blocked until every child and that rerun pass.** Preserve all completed checkboxes below; only the new Slice 7 tasks and the invalidated verification rerun remain unchecked.

## Delivery and TDD Rules

- Preserve all pre-existing working-tree changes; edit only the allowed roots listed for the active slice.
- Every implementation unit follows **RED → GREEN → TRIANGULATE → REFACTOR** and runs `go test ./....`
- Keep tests beside the behavior they verify. Use direct argv/process fakes; no Docker is required for default unit tests.
- Each slice must finish with a clean, bounded diff, acceptance checks, negative-route/security checks, and rollback verification before the next slice starts.
- Do not modify Compose identities, existing non-restore routes, or unrelated packages.

## Slice 1 — Safety Types and Generated `.env` Boundary

**Purpose:** Establish typed, secret-free contracts without enabling migration restore or any destructive executor.

**Dependency:** None. **Forecast:** 220–320 changed lines. **Rollback:** revert only the new contracts/parser and tests; existing routes remain byte-for-byte behaviorally unchanged.

**Exact allowed edit roots:**

- `internal/migration/restore.go` (types/contracts only; no runtime activation)
- `internal/migration/restore_test.go`
- `internal/migration/restore_types.go` and `internal/migration/restore_types_test.go` (if split is needed)
- `internal/workspace/target_env.go`
- `internal/workspace/target_env_test.go`

- [x] 1.1 RED — Define stage/outcome/evidence and capability contracts

- Add tests for all restore stages, outcomes, rollback statuses, `RestoreResult`, `BackupEvidence`, and capability interfaces from `design.md`.
- Assert result/evidence formatting and JSON cannot expose password, pgpass content/path, argv, raw stderr, or arbitrary wrapped errors.
- Add waiter contract tests recording exactly one duration and context cancellation behavior; no production route invokes it yet.

- [x] 1.2 GREEN — Implement typed safety contracts and waiter seam

- Implement typed enums/results and narrow interfaces with stable, allowlisted diagnostic codes.
- Keep private target password material unexported and without `String`, `GoString`, or marshal behavior.
- Implement the injectable waiter seam/fake only; do not add Compose, database, TUI, or command execution.

- [x] 1.3 RED — Specify the allowlisted generated `.env` parser

- Add table-driven tests for valid five-key input, blank/comment/unknown lines, missing/empty values, duplicate/conflicting keys, malformed syntax, `export`, quotes, escapes, substitutions, multiline/NUL/CR/inline-comment input, oversized files/lines, symlink/non-regular files, invalid host/port/user/database, and database `postgres` rejection.
- Add sentinel-password assertions across `%v`, `%+v`, JSON, returned errors, and evidence.
- Add negative tests proving credentials are not sourced from process environment, arbitrary files, container inspection, or defaults.

- [x] 1.4 GREEN — Implement `TargetEnvReader`

- Read only the exact workspace `.env` path with bounded size and `O_NOFOLLOW` where supported; require a regular file.
- Parse exactly the five allowlisted keys, preserve password only in private memory, validate host/port/identifiers, and return field-specific stable codes without values.

- [x] 1.5 TRIANGULATE — Verify safety boundary

- Run `go test ./...` and `go vet ./...`.
- Add/execute a repository diff check confirming no TUI activation, Compose calls, database commands, shell construction, or changes outside the allowed roots.
- Verify malformed/unsupported inputs have no side effects and temporary-file cleanup is not yet claimed by this slice.

- [x] 1.6 REFACTOR — Keep contracts reviewable

- Gofmt touched Go files, remove duplicate test helpers, preserve bounded error vocabulary, and keep the parser independent of the full installer model.
- Slice acceptance: all parser/secret tests pass; typed contracts compile; no restore route can be reached.

## Slice 2 — Backend-Only Service Control and Two-Backup Gates

**Purpose:** Add isolated Compose service control and backup lifecycle gates while still leaving interactive restore inactive.

**Dependency:** Slice 1. **Forecast:** 240–340 changed lines. **Rollback:** remove the new service methods/adapters and backup gate implementation; existing `Down`, `Restart`, startup, and migration-completion behavior remains intact.

**Exact allowed edit roots:**

- `internal/compose/runner.go`
- `internal/compose/fake.go`
- `internal/compose/*_test.go` (service-control tests only)
- `internal/migration/restore_backup.go`
- `internal/migration/restore_backup_test.go`
- `internal/migration/restore.go` (gate contracts only)

- [x] 2.1 RED — Lock service isolation and backup gate behavior

- Test exact direct argv/order for `stop backend` and `start backend`, preserving Compose files and env file.
- Test empty, whitespace, multiple-token, non-`backend`, container-name, PostgreSQL, `down`, and restart requests execute nothing and return bounded errors.
- Test legacy revalidation and target rollback backup staging, custom-format/checksum/manifest/size validation, mode `0600`/protected publication, distinct operation ID/role, atomic publication, no overwrite, and retention.
- Add failure and cancellation tests proving target drop is blocked when either backup is missing, invalid, incomplete, or unpublished; preserve already validated artifacts.

- [x] 2.2 GREEN — Implement narrow Compose and backup capabilities

- Add `ServiceController` methods that accept exactly one allowlisted service and use direct argv only.
- Add recording fake entries for service calls without changing existing whole-stack methods.
- Implement reusable legacy revalidation and target rollback backup creation/validation/publication adapters using existing backup conventions; expose only validated typed artifacts.

- [x] 2.3 TRIANGULATE — Verify pre-mutation gates

- Run `go test ./...` and `go vet ./...`.
- Assert fake command records contain no `postgresql-master`, container-wide operation, shell, password, or raw diagnostics.
- Test rollback before mutation: a stop/backup/precondition failure restarts backend only when target remains unchanged, checks health, preserves both validated backups, and removes only operation-owned staging files.

- [x] 2.4 REFACTOR — Preserve compatibility

- Keep existing Compose callers and command order unchanged; consolidate validation helpers without broadening allowed paths.
- Slice acceptance: service isolation and two-backup gate tests pass, and no coordinator/TUI route invokes these capabilities yet.

## Slice 3A — Protected Credential Transport and Direct-Argv Replacement Builders

**Purpose:** Deliver the package-level process boundary independently from coordinator policy. This slice may construct replacement requests and execute through injected fakes, but it must not add or activate the coordinator, TUI, installer wiring, or destructive runtime route.

**Dependency:** Slices 1–2. **Forecast:** 180–280 authored changed lines; hard cap `<400`. **Rollback:** revert only the process-builder/credential-boundary files and tests; existing migration, Compose, and backup behavior remains unchanged.

**Exact allowed edit roots (no other paths):**

- `internal/migration/restore_process.go`
- `internal/migration/restore_process_test.go`

- [x] 3A.1 RED — Specify protected credential transport and direct-argv process contracts

- Add process-focused tests for an operation-owned directory mode `0700`, one pgpass file mode `0600`, read-only mount, fixed in-container pgpass path, `PGPASSFILE` only, and cleanup on success, error, cancellation, timeout, and panic-recovery boundary.
- Add cleanup-failure classification tests that prove the failure is bounded and secret-free; assert the password sentinel is absent from argv, process records, errors, logs, Docker metadata, and formatted evidence.
- Snapshot exact direct argv for session termination, `dropdb`, `createdb`, `pg_restore`, and validation. Require `--no-password`, `--no-owner`, `--no-privileges`, `--exit-on-error`, `--pull=never`, host networking, fixed read-only SQL/dump mounts, and no shell.
- Reject invalid or unvalidated identifiers before process execution; prove no `sh -c`, `bash -c`, shell interpolation, `PGPASSWORD`, or PostgreSQL service control is possible. Keep tests package-level and fake-executor based.

- [x] 3A.2 GREEN — Implement the protected process boundary and replacement builders

- Implement operation-owned credential transport with private password handling, deferred cleanup on every exit path, fixed in-container paths, protected permissions, and redacted stable error codes.
- Implement direct-argv `psql` session termination, `dropdb`, `createdb`, `pg_restore`, and bounded validation process specifications using fixed SQL assets/server-side identifier quoting and the reviewed client image.
- Expose only typed process specifications/evidence through the migration package; do not add coordinator sequencing, backend service calls, TUI states, installer construction, or production activation.

- [x] 3A.3 TRIANGULATE — Verify the process boundary and secret safety

- Run `go test ./...` and `go vet ./...`; run the secret-sentinel scan over the touched package files and fake command records.
- Verify every process outcome, cancellation, timeout, and cleanup failure removes only operation-owned temporary material and never removes either validated backup.
- Verify exact argv/order and required flags remain stable, host paths are used only for mounts, identifiers are validated, and no password or pgpass content crosses an observable boundary.

- [x] 3A.4 REFACTOR — Keep builders deterministic and package-level

- Separate credential transport, process-spec construction, and bounded classifiers without broadening allowed paths or introducing coordinator policy.
- Gofmt touched files, remove duplicate fake helpers, and keep the final authored diff below 400 lines.
- **Slice 3A acceptance:** all protected-transport and direct-argv tests pass; no coordinator/TUI/installer route is changed; no PostgreSQL stop/restart or shell execution is reachable.

**Slice 3A acceptance criteria:**

- Protected credentials are private, mounted read-only at the fixed path, and cleaned on every tested exit path.
- Replacement process specifications are exact, direct-argv, fail-fast, host-networked, secret-free, and reject unvalidated identifiers.
- The package compiles and `go test ./...` plus `go vet ./...` pass with no runtime activation.

## Slice 3B — Coordinator Mutation Boundary, Evidence, and Automatic Rollback

**Purpose:** Add package-level coordinator policy on top of the completed 3A process boundary. This slice owns mutation sequencing, typed evidence, automatic rollback, cancellation, and stage transitions; it must not wire TUI or installer construction.

**Dependency:** Slice 3A and Slices 1–2. **Forecast:** 220–320 authored changed lines; hard cap `<400`. **Rollback:** revert coordinator/evidence/rollback orchestration only; retain existing Slice 1–2 behavior and never delete operator backups.

**Exact allowed edit roots (no other paths):**

- `internal/migration/restore.go`
- `internal/migration/restore_test.go`

- [x] 3B.1 RED — Define coordinator transition, mutation, and evidence tests

- Add a table-driven transition matrix for platform/request gates, wait, credential/legacy/target-backup gates, backend stop/recovery, replacement, validation, backend start/health, duplicate completion, and every bounded stable result code.
- Assert `Mutated` becomes true immediately before the drop boundary and that no pre-mutation failure can call replacement or report success.
- Add evidence tests for restore exit, target connection, non-system application-table count, PostgreSQL reachability, backend health, stage, outcome, rollback status, and redaction of password, argv, raw stderr, pgpass path, and arbitrary errors.
- Keep all tests package-level with injected capability fakes; prove PostgreSQL is never stopped/restarted and no TUI/installer route is involved.

- [x] 3B.2 GREEN — Implement the stage-aware coordinator and bounded evidence

- Implement ordered coordinator gates: exact existing wait seam, backend-only stop, credential/legacy/target-backup prerequisites, replacement invocation, validation evidence, backend start, and health-before-success semantics.
- Implement the mutation boundary immediately before destructive replacement and return typed stage-aware `RestoreResult` values for all pre-cutover, cancelled, unsupported, failed, and successful paths.
- Keep policy independent from Docker/CLI construction; consume the 3A package boundary and expose only allowlisted evidence and stable codes.

- [x] 3B.3 RED/GREEN — Add automatic rollback and cancellation semantics

- Add tests and implementation for every primary failure after mutation: revalidate and restore only the validated `TargetRollback` backup through the same replacement engine, with no retries, merges, unvalidated sources, or backup deletion.
- Assert rollback success returns `RestorePartialCutover` with `RollbackSucceeded`; rollback failure leaves PostgreSQL running, backend stopped, both backups retained, and bounded recovery guidance.
- Assert caller cancellation after mutation derives a bounded rollback context; a second shutdown cancellation may yield `RollbackCancelled`; pre-mutation cancellation reports the actual stage and does not mutate.
- Test backend recovery before mutation and after rollback, including no-success-before-health and no restart against an unvalidated database.

- [x] 3B.4 TRIANGULATE — Verify destructive transitions and failure containment

- Run `go test ./...`, `go vet ./...`, and the secret-sentinel scan.
- Inject failure at each coordinator capability boundary and verify exact outcome, failed stage, mutation flag, rollback status, evidence, service calls, and retained artifacts.
- Verify cancellation, timeout, duplicate completion, rollback cancellation, and panic-boundary cleanup are deterministic and never expose secrets or raw diagnostics.

- [x] 3B.5 REFACTOR — Isolate coordinator policy and preserve the boundary

- Consolidate transition helpers and stable classifiers without moving process construction into the coordinator or broadening package roots.
- Gofmt touched files, remove duplicate transition fakes, and keep the final authored diff below 400 lines.
- **Slice 3B acceptance:** package-level coordinator, evidence, rollback, and cancellation tests pass; both validated backups remain retained; no TUI/installer dependency construction or runtime activation is added.

**Slice 3B acceptance criteria:**

- Mutation is explicit and stage-aware; every post-mutation failure enters automatic rollback and remains partial cutover.
- Rollback uses only the validated target artifact, preserves both backups, and leaves backend stopped when recovery cannot be proven healthy.
- Success requires restore evidence, PostgreSQL reachability, backend start, and backend health; no false success or secret leakage is possible.
- The package compiles and `go test ./...` plus `go vet ./...` pass without changing non-restore routes.

## Slice 3C-1 — Credential Bridge and Target Rollback Backup Adapter

**Purpose:** Deliver the real target credential bridge and rollback-backup production adapter without composing or activating the coordinator. This slice owns the operation ID/private-secret boundary and the complete target backup lifecycle.

**Dependency:** Slices 1–3B and the retained 93-line Slice 3C Compose probe foundation; Slice 3C-2 depends on this slice. **Forecast:** 250–320 authored changed lines; hard cap `<400`. **Rollback:** revert only the credential/backup adapter and its tests; retain completed contracts and the 93-line probe foundation.

**Exact allowed edit roots (no other paths):**

- `internal/migration/restore_adapters.go` (credential/backup adapters only; retain the existing probe)
- `internal/migration/restore_adapters_test.go` (credential/backup adapter tests only)
- `internal/migration/restore_backup.go` (production staging/operation-ID integration only)
- `internal/migration/restore_backup_test.go` (adapter/staging tests only)
- `internal/migration/restore_process.go` (credential bridge integration only; preserve direct-argv boundary)
- `internal/migration/restore_process_test.go` (bridge/secret-boundary tests only)
- `internal/workspace/target_env.go` (reader integration only; preserve parser rules)
- `internal/workspace/target_env_test.go` (reader integration tests only)
- `internal/migration/restore_types.go` (adapter-facing types only)

- [x] 3C-1.1 RED — Specify the private credential bridge and target backup contract

- Add failing tests for target `.env` reader → private `TargetDatabaseConfig` → operation-owned `CredentialFile`, proving the password never crosses exported fields, argv, errors, evidence, or fake executor records.
- Add failing tests requiring one operation ID to flow into the protected `.target-rollback-<id>.part` staging names and typed `TargetRollback` role; reject fabricated IDs, caller-controlled roots, collisions, overwrite, and fallback credentials.
- Add failure-path tests for staging, regular-file/path confinement, checksum/manifest/archive/size validation, atomic publication, protected modes, cleanup, cancellation, and retained published artifacts.

- [x] 3C-1.2 GREEN — Implement the real credential bridge and backup adapter

- Bridge the existing private target config into the existing protected pgpass transport without exporting password bytes; keep fixed `PGPASSFILE`, mode `0600`, operation-owned cleanup, and no `PGPASSWORD`.
- Extend target backup creation so the operation ID is explicit at the adapter boundary, staging is under the authoritative `/opt/alice/backups/` root, and only validated, atomically published, non-overwriting `TargetRollback` artifacts are returned.
- Reuse existing validation and cleanup mechanisms; do not add probes, coordinator construction, TUI wiring, runtime activation, or new credential sources.

- [x] 3C-1.3 TRIANGULATE — Verify credential secrecy and backup lifecycle

- Run RED → GREEN → TRIANGULATE with `go test ./...`; also run `go vet ./...`, `go build ./...`, `gofmt -d` on touched Go files, `git diff --check`, and secret/prohibited-operation scans.
- Verify every rejected or cancelled operation removes only operation-owned staging/credential material, while both validated backups remain retained and no PostgreSQL or backend service control occurs.

- [x] 3C-1.4 REFACTOR — Keep the bridge and adapter reversible

- Keep operation-ID ownership, backup-root authority, validation, publication, and private-secret handling explicit and deterministic; remove duplicate test wiring and keep this slice below 400 changed lines.
- **Slice 3C-1 acceptance:** real credential-to-pgpass bridging and target rollback staging/validation/publication pass strict-TDD tests; secrets and caller-controlled roots are rejected; no coordinator composition or runtime route changes exist.

## Slice 3C-2 — PostgreSQL/Backend Probes and Explicit Production Composition

**Purpose:** Complete production fail-closed composition and prove the coordinator crosses real adapters. The retained 93 authored lines in `internal/migration/restore_adapters.go` and `internal/migration/restore_composition_test.go` are explicitly attributed to this slice and count toward its budget; do not reimplement them.

**Dependency:** Slice 3C-1 plus the retained 93-line Compose stopped/health probe foundation; Slice 4 depends on both 3C-1 and 3C-2. **Forecast:** 300–380 total authored changed lines including the retained 93 lines; hard cap `<400`. **Rollback:** revert only PostgreSQL/probe/composition work and its tests; retain Slice 3C-1 and completed package contracts, with no TUI/runtime changes.

**Exact allowed edit roots (no other paths):**

- `internal/migration/restore_adapters.go` (PostgreSQL/health/stopped adapters; preserve the retained probe)
- `internal/migration/restore_adapters_test.go` (probe adapter tests)
- `internal/migration/restore_composition.go`
- `internal/migration/restore_composition_test.go` (explicit composition and cross-adapter tests; preserve retained foundation tests)
- `internal/migration/restore.go` (production constructor/capability wiring only; no route activation)
- `internal/migration/restore_process.go` (PostgreSQL probe process integration only; preserve direct argv)
- `internal/migration/restore_process_test.go` (probe evidence tests only)
- `internal/compose/runner.go` (probe/health seam only; preserve service argv)
- `internal/compose/*_test.go` (probe/health tests only)

- [x] 3C-2.1 RED — Specify real probes and explicit composition

- Add failing tests for a bounded direct-argv PostgreSQL reachability probe, backend health verifier, and positive `BackendStoppedProbe`; malformed, duplicate, missing, cancelled, timed-out, failed, or ambiguous evidence MUST fail closed.
- Add failing constructor tests proving production composition supplies concrete adapters for credentials, legacy revalidation, target backup, replacement, PostgreSQL probe, backend health, and stopped-state proof, while rejecting nils, package fakes, no-op probes, permissive health, and fake executors.
- Keep all tests package-level and prove no edits or references are needed in `internal/tui`, `cmd/installer`, or `internal/headless`.

- [x] 3C-2.2 GREEN — Implement probes and production coordinator construction

- Implement bounded, typed, secret-free PostgreSQL reachability through `BinaryExecutor` and the reviewed direct-argv process boundary; reject process failure, timeout, malformed output, and unreachable state.
- Retain and integrate the real Compose backend health/stopped probes, requiring positive unique backend evidence and blocking rollback on uncertainty.
- Add an explicit deterministic production `RestoreCoordinator` constructor that composes the real adapters from approved dependencies and fails closed when any dependency is absent; do not activate any route.

- [x] 3C-2.3 TRIANGULATE — Exercise the complete adapter graph

- Add controlled cross-adapter tests for target env → credential transport → process executor, backup staging/publication, PostgreSQL probe, backend health/stopped probes, and coordinator; Docker is not required for `go test ./...`.
- Assert exact one `Wait(ctx, 60*time.Second)` before backend stop, both validated backups before mutation, PostgreSQL never stopped, no shell, no fake production dependency, secret/raw-diagnostic absence, and fail-closed behavior for every adapter failure, cancellation, cleanup failure, and malformed evidence.
- Run `go test ./...`, `go vet ./...`, `go build ./...`, `gofmt -d`, `git diff --check`, and secret/prohibited-operation/runtime-activation scans.

- [x] 3C-2.4 REFACTOR — Preserve explicit, bounded composition

- Separate constructor wiring from policy and evidence classification, remove duplicate test-only composition, and keep the retained 93-line foundation plus new authored changes below 400 lines.
- **Slice 3C-2 acceptance:** all coordinator capabilities have real fail-closed adapters; PostgreSQL, backend health, and stopped-state probes and cross-adapter composition tests pass; Slice 4 may begin only after both 3C-1 and 3C-2 acceptance, with no TUI/runtime/non-restore route changes.

## Slice 4 — Interactive Linux Wiring, TUI States, Cancellation, and Route Regression

**Purpose:** Activate restore only for confirmed interactive Linux amd64/arm64 Migration after unchanged deployment completes.

**Dependency:** Slices 3C-1 and 3C-2, with Slices 1–3B complete and required risk/reliability/resilience review of the destructive engine. **Forecast:** 220–340 changed lines. **Rollback:** remove `LegacyRestoreAction` construction and return supported Migration to the existing validated-backup blocked/completion state; preserve all generated backups.

**Exact allowed edit roots:**

- `internal/tui/model.go`
- `internal/tui/*restore*go`
- `internal/tui/*_test.go` (migration route/state tests only)
- `cmd/installer/main.go`
- `cmd/installer/*_test.go` (route/wiring tests only)
- `internal/headless/*_test.go` (negative assertions only; do not edit production headless behavior)

- [x] 4.1 RED — Lock route, state, and cancellation behavior

- Test one exact `Wait(ctx, 60*time.Second)` after successful deploy and before backend stop; cancellation identifies `StageWait` and causes no mutation.
- Test migration pending state enters restore after deploy, while ordinary install continues to emit the existing `HealthTickMsg` path.
- Test only `RestoreSucceeded` rejoins verification; failed, unsupported, cancelled, and partial outcomes are explicit terminal states and cannot emit install success.
- Test Escape cancellation reports actual stage and preserves backend-stop/rollback rules.
- Test action wiring exists only for confirmed interactive Linux amd64/arm64; nil/unsupported for Windows, other platforms, unattended/headless, update, restart, dry-run, and ordinary install.

- [x] 4.2 GREEN — Implement TUI and installer wiring

- Add the private pending-migration handoff, restore state/messages, bounded progress/result rendering, and cancellation routing.
- Construct `LegacyRestoreAction` only inside the existing interactive Linux-supported branch; fail closed if a confirmed Migration action is nil.
- Keep pre-deploy installation, Compose startup, migrations, seeds, and all non-restore dependency factories unchanged.

- [x] 4.3 TRIANGULATE — Route and platform verification

- Run `go test ./...`, `go vet ./...`, and relevant Bubbletea/teatest coverage.
- Assert no restore command/service/database side effect is reachable from unsupported or unattended routes.
- Verify fixed service/container identities remain unchanged and only backend service methods are called during cutover.

- [x] 4.4 REFACTOR — Keep UI policy thin

- Move database policy out of TUI, keep messages based on typed redacted results, and preserve existing health verification sequencing after successful restore.
- Slice acceptance: interactive supported route is the sole activation path; all negative route tests pass.

## Slice 5 — Operational Documentation and Opt-In Integration Evidence (Conditional Split)

**Purpose:** Document destructive behavior and add isolated evidence without inflating Slice 4 beyond the practical review budget.

**Dependency:** Slice 4. **Forecast:** 180–300 changed lines; create this slice whenever docs/integration would push Slice 4 to 350 or more.

**Exact allowed edit roots:**

- `README.md`
- `RUNBOOK.md`
- `internal/migration/*_integration_test.go` (integration-tagged only)
- `testdata/legacy-database-restore/**` (sanitized fixtures only)

- [x] 5.1 RED/GREEN — Add opt-in integration coverage

- Add an explicit integration/build-tag test for sanitized PostgreSQL 11 custom archive restore into an isolated supported PostgreSQL helper.
- Prove non-system table evidence, forced post-drop failure, automatic rollback, backup retention, architecture recording, image digest recording, and secret-sentinel absence.
- Ensure default `go test ./...` does not require Docker or credentials; integration failure cannot mutate the developer workspace.

- [x] 5.2 TRIANGULATE/REFACTOR — Document operations and recovery

- Document Linux amd64/arm64 limits, interactive-only activation, backup paths/retention, exact wait, backend-only service control, partial-cutover outcomes, automatic rollback, and bounded operator recovery.
- Document feature rollback and explicitly state that Install, Update, Restart, dry-run, unattended, and Windows behavior remains unchanged.
- Run `go test ./...`, `go vet ./...`, and the opt-in integration command in isolated infrastructure where available; verify no secrets/raw dump output are recorded.

## Final Verification Remediation — Exact PostgreSQL Compose Identity Proof

**Purpose:** Remediate the failed final verification by making Compose identity evidence all-or-nothing and by re-proving the exact PostgreSQL service/container identity immediately before every destructive mutation. This is a bounded strict-TDD verification/composition slice only; preserve all completed implementation tasks and do not change TUI or non-restore routes. **Forecast: 80–120 changed lines; hard cap 120.** **Rollback:** revert only the new parser/identity guards, composition wiring, and tests; the existing restore path must then remain blocked rather than proceeding without proof.

**Dependency:** All implementation tasks and the failed final verification report. New remediation tasks 6.1.1–6.1.4 must complete before final verification rerun (6.2); 6.2 remains unchecked and dependent on all of them.

**Exact allowed edit roots (no other paths):**

- `internal/compose/runner.go` (identity query/proof seam only)
- `internal/compose/fake.go` (identity-proof fake records only)
- `internal/compose/*_test.go` (identity-proof tests only)
- `internal/migration/restore.go` (identity gates and coordinator ordering only)
- `internal/migration/restore_adapters.go` (PostgreSQL identity adapter only)
- `internal/migration/restore_adapters_test.go` (identity adapter tests only)
- `internal/migration/restore_composition.go` (production identity composition only)
- `internal/migration/restore_composition_test.go` (composition and fail-closed tests only)
- `internal/migration/restore_process.go` (identity evidence process boundary only, if required)
- `internal/migration/restore_process_test.go` (identity evidence tests only, if required)

- [x] 6.1 RED — Specify exact PostgreSQL Compose identity verification

- Add failing package-level tests for exact service identity `postgresql-master` and exact container identity `alice_postgresql-master`.
- Cover positive unique evidence and fail-closed rejection for absent, ambiguous, duplicate, changed, mismatched, malformed, timed-out, cancelled, and process-failed identity evidence.
- Prove the gate runs before `backend` stop and before any database mutation; identity failure must produce a bounded redacted result and execute no backend stop, drop, create, restore, or rollback mutation.

- [x] 6.1 GREEN — Implement production identity proof and cutover gate

- Add a concrete production adapter using the existing direct-argv/Compose boundary to establish both immutable identities without shell execution, secrets, permissive endpoint-only acceptance, or container-wide destructive operations.
- Compose the real adapter in `NewProductionRestoreCoordinator`; require the exact service/container pair before backend stop and again wherever PostgreSQL availability is required through cutover.
- Preserve existing backend-only service control and all completed tasks; missing or ambiguous production dependencies fail closed.

- [x] 6.1 TRIANGULATE — Verify ordering, identity rejection, and safety

- Run RED → GREEN → TRIANGULATE with `go test ./...`, `go vet ./...`, `go build ./...`, `gofmt -d` on touched Go files, `git diff --check`, and scoped secret/shell/prohibited-operation scans.
- Assert exact identity arguments/evidence, no password/raw diagnostics leakage, no PostgreSQL stop/restart, and no backend stop or database mutation on every identity failure path.
- Confirm the production constructor cannot silently substitute a package fake, endpoint-only `SELECT 1`, or permissive/no-op identity probe.

- [x] 6.1 REFACTOR — Keep identity verification explicit and bounded

- Separate identity evidence acquisition, validation, and coordinator policy; remove duplicate test helpers and keep the total remediation diff at or below 120 changed lines.
- **Slice acceptance:** production restore fails closed unless service `postgresql-master` uniquely maps to container `alice_postgresql-master`; identity failure blocks backend stop and all database mutation, while successful proof preserves the existing cutover sequence and non-restore routes.

### Strict-TDD follow-up remediation after failed 6.2

- [x] 6.1.1 RED — Reject partial Compose identity evidence

- Add failing table-driven tests for evidence containing multiple JSON objects/records where at least one record is malformed. The parser MUST reject the entire evidence set, even when another record is valid; no valid subset may be accepted.
- Cover malformed JSON syntax, wrong types, missing identity fields, duplicate/ambiguous records, trailing records, and mixed valid/malformed streams. Assert bounded redacted diagnostics and zero downstream identity acceptance.

- [x] 6.1.2 GREEN — Make identity parsing all-or-nothing

- Implement the smallest parser/validator change in the exact allowed roots so every JSON object/record is decoded and validated before any identity is returned. Any malformed, incomplete, duplicate, or ambiguous record rejects the complete evidence set; never return partial identities.
- Preserve exact `postgresql-master` → `alice_postgresql-master` matching, direct argv, secret-free evidence, and existing completed behavior outside this remediation.

- [x] 6.1.3 RED/GREEN — Re-prove identity immediately before every rollback mutation

- Add failing-then-passing tests that place a fresh exact service/container identity proof immediately before each rollback `drop`/`create`/`restore` mutation. Inject drift, malformed/partial evidence, absent identity, stopped identity, timeout, cancellation, and process failure between earlier proof and mutation.
- Assert fail-closed behavior: on any drift or invalid/absent/stopped proof, execute no rollback mutation, no replacement mutation, and no PostgreSQL or backend service mutation; retain validated backups and return bounded evidence.

- [x] 6.1.4 TRIANGULATE/REFACTOR — Prove final strict-TDD boundary

- Run RED → GREEN → TRIANGULATE → REFACTOR with `go test ./...`, plus `go vet ./...`, `go build ./...`, `gofmt -d`, `git diff --check`, and scoped secret/shell/prohibited-operation scans.
- Verify every rollback mutation path has an adjacent fresh identity proof, no proof is reused across a mutation boundary, and malformed evidence never permits partial acceptance. Keep the remediation diff at or below 120 changed lines.

- [x] 6.1.5 RED/GREEN — Reorder rollback behind identity and reachability proof (correction forecast: 40–60 changed lines; hard cap 60)

- In `internal/migration/restore.go`, `internal/migration/restore_test.go`, and the relevant Compose/adapter composition tests, make rollback ordering explicit: prove the exact `postgresql-master` → `alice_postgresql-master` identity and positive PostgreSQL reachability before any backend `StopService` call or database mutation; require fresh proof/reachability at each subsequent rollback mutation boundary.
- Add recording-fake tests asserting the exact call sequence for successful rollback and exact no-call behavior when identity or reachability is absent, ambiguous, changed, stopped, malformed, timed out, cancelled, or process-failed. On proof failure, assert no backend stop/start, `dropdb`, `createdb`, `pg_restore`, or other database mutation occurs, while validated backups remain retained and the result stays bounded and redacted.
- Run the focused rollback ordering tests plus `go test ./...`, `go vet ./...`, `go build ./...`, `gofmt -d`, `git diff --check`, and scoped secret/shell/prohibited-operation scans. Keep this correction at or below 60 changed lines and preserve all completed tasks.

## Final Verification Rerun

- [ ] 6.2 TRIANGULATE — Rerun final verification after identity remediation (invalidated by Slice 7)

- This prior verification result is superseded by the newly added PM2 scope; do not treat it as archive-ready. Re-run only after Slice 7 and use the final Slice 7 rerun task below as the authoritative completion gate.

## Apply Order and Handoff

1. Preserve completed Slices 1–5 and Final Verification Remediation 6.1; do not reapply them.
2. Preserve retained Slice 7A-R, then apply 7A-1 and 7A-2 in order; each child stays below `<350` authored lines. Do not start 7B until 7A-2 focused/full evidence and its clean child diff are recorded.
3. Do not treat prior 6.2 as archive-ready; complete Slice 7A and 7B, then run 7.6 as the authoritative final verification rerun.
4. Verify the full change with `go test ./...`, `go vet ./...`, route regression tests, secret scan, and available isolated integration evidence before archive.

## Slice 7A — PM2 Acquisition and Controller (Amended Chain)

### Review Workload Forecast — Slice 7A

| Field | Value |
| --- | --- |
| Estimated authored changed lines | Retained 7A-R: 349; 7A-1: 120–170; 7A-2: 140–210 (each child hard cap <350) |
| Focused test command | Per work unit below; each also runs `go test -count=1 ./...` before handoff |
| Runtime harness | N/A — package-only adapters use controlled command/file seams; no production route is permitted before 7B |
| Rollback boundary | Per work unit below; retained foundation and each child are independently reversible |
| PR base | 7A-R base = feature/tracker; 7A-1 base = 7A-R branch; 7A-2 base = 7A-1 branch |

**Purpose:** Preserve the retained parser/correlation/controller foundation, then add fixed-argv PM2/`ss` and bounded `/proc` acquisition in two child units. All remain package-only and unreachable from production until Slice 7B provides complete lease compensation.

**Dependency:** Completed Slices 1–5 and 6.1. **Rollback:** revert only the active work unit; no route can invoke PM2 mutation.

### Suggested Work Units

| Unit | Goal / forecast | Chain target | Focused test command | Runtime harness | Rollback boundary |
| --- | --- | --- | --- | --- | --- |
| 7A-R retained | Keep the existing 349-line parser/correlation/controller foundation; do not reapply it. | PR 7A-R base = feature/tracker branch | `go test -count=1 ./internal/installation` | N/A — existing fake seams; no route reference | `pm2_quiescence.go`, `pm2_quiescence_test.go`, `linux_socket_snapshot.go`, `proc_identity.go` |
| 7A-1 | Fixed direct-argv PM2 inventory and `ss` adapters, 120–170 lines. | PR 7A-1 base = PR 7A-R branch | `go test -count=1 ./internal/installation -run 'Test(LinuxPM2Inventory | LinuxSocketSnapshot)'` | N/A — recording `CommandRunner`; production wiring prohibited | `pm2_quiescence*.go`, `linux_socket_snapshot*.go` |
| 7A-2 | Bounded `/proc` adapter and complete snapshot provider, 140–210 lines. | PR 7A-2 base = PR 7A-1 branch | `go test -count=1 ./internal/installation -run 'Test(LinuxProcIdentity | LinuxPM2SnapshotProvider)'` | N/A — temporary proc-root/file seams; no route reference | `proc_identity*.go`, `linux_pm2_snapshot*.go` |

Retained foundation: the existing 349 authored lines are completed package-only work with recorded passing checks. They are not tasks to repeat, and the unchecked implementation tasks below remain unchecked until their new RED/GREEN/TRIANGULATE/REFACTOR work is applied.

**7A-1 exact allowed edit paths (no other paths):**

- `internal/installation/pm2_quiescence.go`
- `internal/installation/pm2_quiescence_test.go`
- `internal/installation/linux_socket_snapshot.go`
- `internal/installation/linux_socket_snapshot_test.go`

- [x] 7A.1 RED — Specify fixed PM2 and socket acquisition adapters

- Add failing tests for exact `pm2 jlist` and `ss -H -ltnp` argv, bounded output, timeout/cancellation/tool failure, malformed/mixed output, and redacted failures.
- Reuse retained parsers to reject duplicate IDs/PIDs/owners and no raw stdout/stderr; do not modify correlation, controller, or routes.

- [x] 7A.2 GREEN — Implement fixed-argv PM2 and socket adapters

- Implement package-only adapters around `CommandRunner` using only `pm2 jlist` and `ss -H -ltnp`; apply bounded reads, fixed timeouts, parser-only records, and stable errors.
- Keep the retained deterministic PMID recheck and `pm2 stop <id>` controller unchanged; add no migration/TUI/cmd/headless reference.

- [x] 7A.3 TRIANGULATE/REFACTOR — Close 7A-1 independently

- Run the 7A-1 focused command, `go test -count=1 ./...`, `go vet ./...`, `go build ./...`, `gofmt -d`, `git diff --check`, and secret/shell/broad-command/route scans.
- Refactor only the 7A-1 paths; record a clean child diff below 350 lines before 7A-2.

**7A-2 exact allowed edit paths (no other paths):**

- `internal/installation/proc_identity.go`
- `internal/installation/proc_identity_test.go`
- `internal/installation/linux_pm2_snapshot.go`
- `internal/installation/linux_pm2_snapshot_test.go`

- [x] 7A.4 RED — Specify bounded `/proc` identity and complete snapshot assembly

- Add failing tests for fixed `/proc/<pid>/cwd`, `/proc/<pid>/exe`, and bounded `/proc/<pid>/stat` acquisition, including missing/permission/changed/zero/invalid evidence and cancellation.
- Specify a provider that composes the 7A-1 PM2/socket adapters with `/proc` identity for every candidate and fails closed on incomplete or contradictory evidence before `Quiesce` can stop anything.

- [x] 7A.5 GREEN — Implement `/proc` adapter and snapshot provider

- Implement a Linux-only package adapter with a testable proc root, bounded stat reads, canonical cwd/executable evidence, and start-tick parsing; assemble one complete `PM2Snapshot` from the fixed adapters.
- Keep it unreachable from `cmd`, `internal/tui`, `internal/migration`, and `internal/headless`; do not add recovery, handoff, or production route wiring.

- [x] 7A.6 TRIANGULATE/REFACTOR — Close 7A-2 and Slice 7A acquisition

- Run the 7A-2 focused command, then `go test -count=1 ./...`, `go vet ./...`, `go build ./...`, `gofmt -d`, `git diff --check`, and secret/shell/broad-command/route scans.
- Refactor only 7A-2 paths; prove the retained controller consumes a complete snapshot, no production route is reachable, and the 7A-2 child diff stays below 350 lines.

## Slice 7B — Amended PM2 Recovery/Lease and Terminal-Routing Chain

| Unit | Forecast / base | Focused test command | Runtime harness | Rollback boundary |
| --- | --- | --- | --- | --- |
| 7B-1 | 270–320; base = PR 7A-2 branch | `go test -count=1 ./internal/installation ./internal/migration -run 'Test(PM2.*Recover | PreInstallMigration)'` | N/A — package fakes/snapshot sequences only; no production route | `internal/installation/pm2_quiescence*.go`, `internal/migration/handoff*.go` |
| 7B-2 | 250–320; base = PR 7B-1 branch | `go test -count=1 ./internal/tui ./cmd/installer ./internal/headless` | Controlled TUI message-loop and fake-lease scenarios; no real PM2 command | TUI/cmd route files and PM2 additions in `README.md`, `RUNBOOK.md` |

**Dependency:** 7A-R, 7A-1, and 7A-2. Each child is `<350` authored lines; retarget/rebase if a child includes its parent. 7B-1 remains unreachable from production. 7B-2 is the first activation and MUST invoke database rollback before PM2 recovery; migration success consumes the lease and retains the stopped set.

### Child 7B-1 — Exact acknowledged recovery and one-owner lease

**Allowed paths:** `internal/installation/pm2_quiescence.go`, `internal/installation/pm2_quiescence_test.go`, `internal/migration/handoff.go`, `internal/migration/handoff_test.go`.

- [x] 7B-1.1 RED — Test partial-stop recovery uses only defensive-copy acknowledged identities; `pm2 start <same id>` rejects selector reuse, drift, competing owners, failed commands, or unverifiable new PID/start-ticks/cwd/exec/port without retry.
- [x] 7B-1.2 GREEN — Add bounded `Recover` and immutable acknowledged-set evidence; recover reverse stop order and stop further recovery on ambiguity.
- [x] 7B-1.3 RED — Test `PreInstallMigrationCoordinator` revalidates backup, requires complete quiescence/final release before installation, and gives one owner an opaque lease.
- [x] 7B-1.4 GREEN/TRIANGULATE/REFACTOR — Implement idempotent `CompleteSuccess` (no recovery) and `CompleteFailure` (one bounded recovery); run focused, `go test -count=1 ./...`, vet/build/format/diff and secret/shell/broad-command scans.

### Child 7B-2 — Interactive terminal routing and operations

**Allowed paths:** `internal/tui/model.go`, `internal/tui/migration_quiescence.go`, `internal/tui/migration_quiescence_test.go`, `internal/tui/migration_restore_test.go`, `internal/tui/migration_flow_test.go`, `cmd/installer/main.go`, `cmd/installer/main_test.go`, `internal/headless/*_test.go`, `README.md`, `RUNBOOK.md`.

- [x] 7B-2.1 RED — Test supported confirmed Migration acquires the 7B-1 lease before `StatePreflight`; nil/unsupported/failed acquisition blocks before install, while ordinary deploy still emits `HealthTickMsg`.
- [x] 7B-2.2 GREEN — Wire quiescer/handoff only in interactive Linux amd64/arm64 composition; keep Install, Update, Restart, dry-run, unattended/headless, Windows, unsupported platforms, `PM2Probe`, and `LegacyPolicy` unchanged.
- [x] 7B-2.3 RED — Test every live-lease terminal: install failure, restore failure/unsupported/cancel/partial, Escape/Ctrl-C/q, abandon/panic boundary, and verification failure; database rollback completes before exact PM2 recovery, and only `InstallSuccessMsg` retains stops.
- [x] 7B-2.4 GREEN/TRIANGULATE/REFACTOR — Route all terminals through one bounded completion path, render redacted recovery status, document roots/ports/tools/retention/unchanged routes, and run focused, full, vet/build/format/diff, route/secret/prohibited-command scans.

- [ ] 7.6 TRIANGULATE — Rerun authoritative final verification after Slice 7

- Re-run the full final verification against completed Slices 7A and 7B and apply state: uncached suite, vet/build/format/diff checks, route and prohibited-operation scans, coverage, and available isolated integration evidence.
- Confirm PM2 RED/GREEN/TRIANGULATE/REFACTOR evidence, exact identity capture, port-release proof, pre-install ordering, narrow compensation/recovery verification, success leaves legacy PM2 stopped, and all non-migration route regressions.
- Replace the superseded 6.2 status only after this rerun passes; archive remains blocked on any PM2 identity, tool/permission, race, compensation, route, or destructive-path failure.
