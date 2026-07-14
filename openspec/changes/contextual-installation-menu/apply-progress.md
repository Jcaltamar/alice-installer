
## Migration Slice 4.4 — Protected destination and streaming backup engine (2026-07-11)

**Status:** complete for the assigned Slice 4.4 boundary only. The implementation produces a protected, unvalidated staging dump; it does **not** validate, checksum, manifest, rename, or publish a completed backup. Those operations remain exclusively Slice 4.5. No TUI, PM2, source, legacy-container lifecycle, Docker/database integration test, or commit was added.

### Completed task and persisted checkbox update

- [x] 4.4 Start: 4.3 transport proof accepted; no TUI action wired.
- [x] 4.4 Finish: an explicitly confirmed immutable plan streams a custom-format dump to protected staging with cancellation, locking, space checks, and cleanup.
- The two corresponding 4.4 task lines are visibly marked `[x]` in `tasks.md`; 4.5 and 4.6 remain unchecked.

### Files changed

- `internal/migration/backup_action.go`
- `internal/migration/destination_store.go`
- `internal/migration/backup_action_test.go`
- `openspec/changes/contextual-installation-menu/tasks.md`
- `openspec/changes/contextual-installation-menu/apply-progress.md`

### TDD Cycle Evidence

| Cycle | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- |
| Protected plan, staging, and cleanup | `go test ./internal/migration` failed to compile because `BackupAction`, destination plans/store, staging, locking, and typed outcomes did not exist. A final lock-release test then failed because success retained the operation lock. | Added a read-only preflight that resolves config/container evidence and destination safety, then a post-confirmation `Run` that creates the directory/lock/staging and streams the proven helper stdout directly to the staged file. | Added unsafe source/symlink/insecure-permission destination cases, free-space and non-blocking-lock cases, exclusive staging preservation, zero-output, dump-failure, timeout, cancellation cleanup, and successful-stream lock release cases. All use `t.TempDir()`-derived fixtures and fake executors; no Docker or database command runs. | Kept plan evidence private/immutable, make redacted copies for inspection, centralize path/component checks, enforce `0700` directory and `0600` exclusive staging, release the operation lock after a synced/closed staged dump, and retain only `BackupStaged` (never `validated`). |

### Safety and design conformance

- Preflight is read-only: it creates no destination, lock, staging file, credential, or process. Calling `Run` is the explicit-confirmation boundary.
- The plan privately binds resolved config evidence, exact immutable container ID/image identity, protected destination plan, platform, and timeout. The helper invocation retains exact PostgreSQL 11 custom-format direct argv from Slice 4.3.
- Destination checks reject source-tree destinations, symlinks, non-directories, unsafe ownership/permissions, inadequate conservative free space, and active locks. New approved directories are created `0700` only in `Run`; staging names are collision-resistant and opened `O_CREATE|O_EXCL` at `0600`.
- Cancellation is checked before preflight work, destination mutation, helper launch, and post-stream handling. Process timeout/cancellation/failure, empty output, sync/close failure, and destination failures return typed redacted outcomes and remove operation-created staging/lock/credentials without touching existing files.
- A successful Slice 4.4 outcome is `BackupStaged`, not a completed or validated backup. Slice 4.5 remains responsible for validation, checksum, manifest, atomic rename, and any final publication.

### Verification

- `go test ./internal/migration` — RED compile failure observed, then passed after GREEN/TRIANGULATE/REFACTOR.
- `go test ./...` — passed.
- `go vet ./...` — passed.
- `go build ./...` — passed.
- `git diff --check` — passed.
- No real Docker or database command was executed.

### Deviations from design

- None. The Slice 4.3 ephemeral helper-container seam is reused exactly as the approved process boundary; Slice 4.4 deliberately stops at synced, closed, unvalidated staging.

### Remaining tasks

- [ ] **Start:** 4.4 produces only staged, unvalidated output and 4.3 transport remains proven.
- [ ] **Finish:** only a non-empty structurally valid PostgreSQL 11 custom dump yields a paired protected dump/manifest publication.
- [ ] **Start:** slices 4.1–4.5 are reviewed, all blockers pass, and the action remains absent from production wiring.
- [ ] **Finish:** Linux amd64/arm64 interactive Migration Step 1 is explicitly confirmed, cancellable, redacted, validated-only, and followed by a blocked later-step screen; explicit CLI/non-interactive routes remain unchanged.

### Workload / PR boundary

- Delivery strategy: `auto-chain`; assigned/applied boundary: Migration Slice 4.4 only.
- No commit or PR was created. Slice 4.5 is the next chained work unit and must retain the unvalidated staging boundary until successful atomic pair publication.

### Structured status consumed

- `changeName`: `contextual-installation-menu`; `artifactStore`: `openspec` (authoritative).
- Native status before work: `applyState: ready`, `nextRecommended: apply`, `blockedReasons: []`.
- `actionContext`: `repo-local`; workspace root and only allowed edit root: `/home/dev/Documents/works/ebenezer/alice-installer`.
- Strict TDD was active from `openspec/config.yaml`; the parent assigned the resolved `auto-chain` Slice 4.4 work-unit boundary. No action-context warning was raised.

---

## Migration Slice 4.5 — PostgreSQL 11 validation and atomic publication (2026-07-11)

**Status:** complete for the assigned Slice 4.5 boundary only. No TUI wiring, restore, source mutation, or real Docker/database command was added.

### Completed task and persisted checkbox update

- [x] **Start:** 4.4 produces only staged, unvalidated output and 4.3 transport remains proven.
- [x] **Finish:** only a non-empty structurally valid PostgreSQL 11 custom dump yields a paired protected dump/manifest publication.
- The two Slice 4.5 task lines are visibly marked `[x]`; Slice 4.6 remains unchecked.

### Files changed

- `internal/migration/validator.go`
- `internal/migration/validator_test.go`
- `internal/migration/manifest.go`
- `internal/migration/publication_test.go`
- `internal/migration/backup_action.go`
- `internal/migration/backup_action_test.go`
- `openspec/changes/contextual-installation-menu/tasks.md`
- `openspec/changes/contextual-installation-menu/apply-progress.md`

### TDD Cycle Evidence

| Cycle | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- |
| Pinned archive validation | `go test ./internal/migration -run TestPG11ArchiveValidator` failed because the validator contract did not exist. | Added a pinned ephemeral PostgreSQL 11 `pg_restore --list` helper with a read-only staged-dump mount, typed result handling, direct named cleanup, and non-empty structural-listing validation. | Covered empty, malformed, and failed listing outcomes; asserted no database connection arguments or credentials are constructed. | Bounded retained listing data and kept only a typed validation error outside the boundary. |
| Manifest and pair publication | `go test ./internal/migration -run TestPublishBackupPair` failed because the publication API did not exist. | Added SHA-256/byte-size computation, deterministic versioned allowlisted manifest encoding, restrictive staged manifest writing, no-replace atomic renames, and directory synchronization. | Covered cancellation, manifest-rename and directory-sync failure cleanup, plus pre-existing final-artifact preservation. | Isolated schema encoding, hashing, filesystem synchronization, and operation-created-pair cleanup. |
| Validated backup outcome | `go test ./internal/migration -run TestBackupActionPublishesOnlyValidatedOutcome` failed because `BackupAction` had no validation/publication boundary or validated-only result fields. | Routed closed staging through validation then publication; only `BackupValidated` returns final dump/manifest paths, checksum, and size. | Added validation-failure cleanup assertion proving no artifact fields or files remain. | Retained `BackupStaged` only as an internal historical boundary; public successful results expose final data solely after pair publication. |

### Verification

- `go test ./internal/migration` — passed.
- `go test ./...` — passed.
- `go vet ./...` — passed.
- `go build ./...` — passed.
- `git diff --check` — passed.
- Secret-sentinel scans passed: production migration code contains neither synthetic secret sentinel; manifest/action production files contain no `PGPASSWORD`, `config.js`, or `pgpass` text.
- No real Docker or database command was executed.

### Safety and design conformance

- Validation invokes direct pinned PostgreSQL 11 `pg_restore --list` in the proven ephemeral helper boundary with a read-only staged dump and no host/database/credential arguments. Empty, malformed, failed, cancelled, or cleanup-failed validation fails closed.
- Publication computes SHA-256 and byte size from closed staging bytes. The deterministic manifest allows only schema/version, UTC time, safe container/image identity, selected environment, endpoint/database/user, custom format, size/checksum, fixed PostgreSQL 11 client labels, and validation status.
- Dump and manifest are `0600`; no-replace `renameat2` publication plus directory sync makes the pair visible only on success. Rename, sync, manifest, or cancellation failures remove only operation-created paths and preserve pre-existing artifacts.
- Only `BackupValidated` exposes final paths, checksum, and size. Other outcomes return fixed redacted messages without final artifact details.

### Deviations from design

- None. The validator reuses the approved pinned helper-container boundary instead of a host client or database connection.

### Remaining tasks

- [ ] **Start:** slices 4.1–4.5 are reviewed, all blockers pass, and the action remains absent from production wiring.
- [ ] **Finish:** Linux amd64/arm64 interactive Migration Step 1 is explicitly confirmed, cancellable, redacted, validated-only, and followed by a blocked later-step screen; explicit CLI/non-interactive routes remain unchanged.

### Workload / PR boundary

- Delivery strategy: `auto-chain`; assigned/applied boundary: Migration Slice 4.5 only.
- No commit or PR was created. Slice 4.6 remains the next chained work unit.

### Structured status consumed

- `changeName`: `contextual-installation-menu`; `artifactStore`: `openspec` (authoritative).
- Native status before work: `applyState: ready`, `nextRecommended: apply`, `blockedReasons: []`.
- `actionContext`: `repo-local`; workspace root and only allowed edit root: `/home/dev/Documents/works/ebenezer/alice-installer`.
- Strict TDD was active from `openspec/config.yaml`; parent resolved the `auto-chain` Migration Slice 4.5 work-unit boundary. No action-context warning was raised.

---

## Migration Slice 4.5 — Ownership cleanup correction (2026-07-11)

**Status:** complete correction within the already assigned Slice 4.5 boundary. This fixes only critical cleanup ownership flaws; it does not add Slice 4.6 TUI/wiring scope.

### Completed task and persisted checkbox update

- [x] Slice 4.5 remains completed in `tasks.md`; no new task was created and Slice 4.6 remains unchecked.
- Corrected publication cleanup so it removes only paths backed by an internal operation-created capability or paths whose successful creation/rename was recorded by this invocation.

### Files changed

- `internal/migration/manifest.go`
- `internal/migration/backup_action.go`
- `internal/migration/publication_test.go`
- `openspec/changes/contextual-installation-menu/apply-progress.md`

### TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 4.5 ownership cleanup correction | `internal/migration/publication_test.go` | Unit | `go test ./internal/migration` passed before edits | Added an unowned-request preservation test; it failed to compile because `publishBackupPair` did not exist. | Added private operation-created dump capability validation and ownership-gated cleanup; focused publication tests passed. | Added pre-existing exact manifest staging, invalid caller dump, dump-rename, manifest-rename, directory-sync, cancellation, and successful-pair preservation coverage; focused tests passed. | Centralized cleanup around explicit staged/final creation flags; reran focused and package tests. |

### Safety and design conformance

- A caller-supplied `DumpPath` is not ownership proof. `PublishBackupPair` now rejects a request without the package-private operation-created capability and does not remove its dump path.
- `BackupAction` obtains that capability only from its operation-created `StagedArtifact` path.
- Deferred cleanup tracks the staged dump, staged manifest, final dump, and final manifest independently, and removes each only after this operation created or renamed it successfully.
- A pre-existing manifest staging file surviving `O_EXCL` failure, pre-existing caller dump on invalid request, and existing final/operator artifacts are preserved. Failed owned operations still clean their own staged/final half-pairs.
- Transactional pair semantics, `RENAME_NOREPLACE`, mode `0600`, and validated-only final result behavior remain unchanged.

### Verification

- `go test ./internal/migration` — passed (baseline, RED/Green focused cycles, and final package run).
- `go test ./...` — passed.
- `go vet ./...` — passed.
- `go build ./...` — passed.
- `git diff --check` — passed.
- `git status --short` — completed; it reports the pre-existing Slice 1–5 workspace changes and no commit was created.
- No real Docker or database command was executed.

### Deviations from design

- None. The correction strengthens the design-mandated operation-created-only cleanup rule without exposing a public ownership capability.

### Remaining tasks

- [ ] **Start:** slices 4.1–4.5 are reviewed, all blockers pass, and the action remains absent from production wiring.
- [ ] **Finish:** Linux amd64/arm64 interactive Migration Step 1 is explicitly confirmed, cancellable, redacted, validated-only, and followed by a blocked later-step screen; explicit CLI/non-interactive routes remain unchanged.

### Workload / PR boundary

- Delivery strategy: `auto-chain`; assigned/applied boundary: Migration Slice 4.5 ownership cleanup correction only.
- No commit or PR was created. Slice 4.6 remains out of scope and is the next chained work unit.

### Structured status consumed

- `changeName`: `contextual-installation-menu`; `artifactStore`: `openspec` (authoritative).
- Native status before work: `applyState: ready`, `nextRecommended: apply`, `blockedReasons: []`.
- `actionContext`: `repo-local`; workspace root and only allowed edit root: `/home/dev/Documents/works/ebenezer/alice-installer`.
- Strict TDD was active from `openspec/config.yaml`; parent supplied the resolved `auto-chain` Slice 4.5 correction boundary. No action-context warning was raised.

---

## Migration Slice 4.6 — final interactive activation (2026-07-11)

**Status:** complete for the assigned final activation boundary. Migration Step 1 is now available only through the interactive Linux amd64/arm64 legacy route. No real Docker, database, or backup command was executed during this work.

### Completed task and persisted checkbox update

- [x] **Start:** slices 4.1–4.5 are reviewed, all blockers pass, and the action remains absent from production wiring.
- [x] **Finish:** Linux amd64/arm64 interactive Migration Step 1 is explicitly confirmed, cancellable, redacted, validated-only, and followed by a blocked later-step screen; explicit CLI/non-interactive routes remain unchanged.
- The final two Migration Step 1 checklist items are marked `[x]` in `tasks.md`.

### Files changed

- `internal/tui/model.go`
- `internal/tui/context_menu.go`
- `internal/tui/migration_flow_test.go`
- `internal/tui/golden_test.go`
- `internal/tui/testdata/golden/migration_{confirm,running,failed,blocked}.golden`
- `cmd/installer/main.go`
- `cmd/installer/main_test.go`
- `internal/migration/backup_integration_test.go`
- `README.md`
- `RUNBOOK.md`
- `openspec/changes/contextual-installation-menu/tasks.md`
- `openspec/changes/contextual-installation-menu/apply-progress.md`

### TDD Cycle Evidence

| Cycle | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- |
| TUI activation | `go test ./internal/tui ./cmd/installer ./internal/migration` failed because backup dependencies, state/messages, confirmation flow, and completion handling did not exist. | Added only `LegacyBackupAction`, explicit read-only preflight/review, Enter confirmation, cancellable running state, typed completion, and validated-only blocked continuation. | Added direct state tests for no pre-confirmation run, duplicate Enter suppression, Escape/q cancellation handshake, validated success, and failed/cancelled outcomes; added deterministic migration goldens and command composition assertions. | Kept result rendering to final paths/checksum/size only and fixed text/category-only views; explicit routes still use operational dependencies without backup wiring. |

### Verification

- `go test ./internal/tui ./cmd/installer ./internal/migration` — passed after RED/GREEN.
- `go test ./internal/tui -run TestGoldenMigrationViews -update` — passed; generated reviewed migration goldens.
- `go test ./internal/tui -run TestGoldenMigrationViews` — passed without `-update`.
- `go test ./...` — passed.
- `go test -cover ./...` — passed; coverage remains below the configured 80% in `internal/migration` (79.9%) and `internal/tui` (79.2%), while several existing packages are also below 80%. This is recorded, not hidden.
- `go vet ./...` — passed.
- `go build ./...` — passed.
- `git diff --check` — passed.
- `git status --short` — completed; pre-existing Slice 1–5 changes remain in the workspace and no commit was created.
- Integration seam: `internal/migration/backup_integration_test.go` is build-tagged `integration`, skips with `testing.Short()`, and requires `ALICE_MIGRATION_INTEGRATION_FIXTURE`; it ran no real command in this environment.

### Safety and design conformance

- The menu injects only the narrow `LegacyBackupAction`; no generic migration executor exists.
- Production wiring uses `migration.LegacyConfigPath` through `migration.Resolver`, explicitly selects `production`, uses the migration-only Docker inspector/helper/validator boundary, and binds the existing protected destination, validation, and publication action.
- Linux amd64/arm64 interactive composition alone receives the backup action. Update, restart, unattended, and dry-run use `newOperationalDependencies` or explicit branches and never receive it.
- Preflight is the review boundary; confirmation is required before `Run`, which is the first point at which the existing destination store can create an artifact. Escape, q, and Ctrl-C request cancellation while remaining on the running screen until the action returns.
- Running UI uses only fixed dump/validation/publication categories. Failures/cancellation remain blocked; only `BackupValidated` renders final dump/manifest paths, SHA-256, and byte size before the explicit later-steps-blocked screen.
- No restore, schema transform, PM2 stop, deletion, cutover, volume mutation, or source/container lifecycle operation was added.

### Deviations from design

- The existing migration package exposes no incremental `ProgressSink`; the running view therefore uses fixed redacted operation categories rather than live command-derived detail. No raw output crosses the TUI boundary.
- The documented default is `${TMPDIR:-/tmp}/alice-installer-backups`, created `0700` only after confirmation. Operators must move validated artifacts to durable storage.

### Remaining tasks

- [ ] Completed (Slice 2.6 quality gate) remains unchecked in the persisted task artifact. It is outside the assigned 4.6 boundary, so it was not changed. Native status therefore remains `apply: ready`; do not claim final verify readiness until it is reconciled.

### Workload / PR boundary

- Delivery strategy: `auto-chain`; assigned/applied boundary: Migration Slice 4.6 final activation only.
- No commit or PR was created.

### Structured status consumed

- `changeName`: `contextual-installation-menu`; `artifactStore`: `openspec` (authoritative).
- Native status before work: `applyState: ready`, `nextRecommended: apply`, `blockedReasons: []`.
- `actionContext`: `repo-local`; workspace root and only allowed edit root: `/home/dev/Documents/works/ebenezer/alice-installer`.
- Strict TDD was active from `openspec/config.yaml`; parent provided `auto-chain` and the Slice 4.6 work-unit boundary. No action-context warning was raised.

---

## Migration Slice 4.6 — gate blocker correction (2026-07-11)

**Status:** complete for the assigned Slice 4.6 gate blockers only. This correction adds no restore, cutover, lifecycle, deletion, volume, schema, or source-system mutation behavior. The opt-in integration fixture was **not run**.

### Completed task and persisted checkbox update

- [x] **Start:** slices 4.1–4.5 are reviewed, all blockers pass, and the action remains absent from production wiring.
- [x] **Finish:** Linux amd64/arm64 interactive Migration Step 1 is explicitly confirmed, cancellable, redacted, validated-only, and followed by a blocked later-step screen; explicit CLI/non-interactive routes remain unchanged.
- The assigned 4.6 persisted task lines were already visibly `[x]`; they remain checked after all required gates passed. No unrelated Slice 2.6 checkbox was changed.

### Files changed

- `internal/migration/backup_action.go`
- `internal/migration/backup_action_test.go`
- `internal/migration/validator_test.go`
- `internal/migration/backup_integration_test.go`
- `internal/tui/model.go`
- `internal/tui/migration_flow_test.go`
- `internal/tui/testdata/golden/migration_confirm.golden`
- `openspec/changes/contextual-installation-menu/apply-progress.md`

### TDD Cycle Evidence

| Cycle | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- |
| Immutable confirmation review | Added review-rendering and secret-sentinel tests; they failed because the confirmation view omitted all required review fields and `BackupPlan.Review` did not exist. | Added an allowlisted immutable `BackupReview` projection and rendered its selected environment, host/port endpoint, database, user, full container ID, exact image, and destination. | Added pre-confirm Escape/quit no-run, small-terminal, nil-action, stale-message, duplicate-submit, cancellation, and validated-only result tests. | Kept config/password, pgpass, and raw configuration outside the projection and regenerated/rechecked the deterministic confirmation golden. |
| Validator and fixture safety | Added cancellation, oversized-listing, and cleanup-failure validator scenarios. | Validator remains fail-closed on each outcome; fake cleanup behavior now exposes cleanup failure. | Added an executable `integration`-tagged fixture which requires `ALICE_MIGRATION_INTEGRATION_DOCKER`, creates a labeled random private Docker network/container, has no host ports, probes only its own PostgreSQL 11 container, and cleans both resources. | Bounded fixture command diagnostics and retained default plus `testing.Short()` skips. The fixture was not run. |

### Verification

- `go test ./internal/tui ./cmd/installer ./internal/migration` — passed.
- `go test ./...` — passed.
- `go test -cover ./...` — passed; `internal/migration` **80.0%**, `internal/tui` **80.0%**.
- `go vet ./...` — passed.
- `go build ./...` — passed.
- `git diff --check` — passed.
- `git status --short` — completed; existing uncommitted Slice 1–5 and OpenSpec workspace changes remain. No commit was created.
- `go test ./internal/tui -run TestGoldenMigrationViews -update` then `go test ./internal/tui -run TestGoldenMigrationViews` — passed.
- No real Docker/PostgreSQL integration command was run.

### Deviations from design

- None. The confirmation screen now makes the design-required immutable redacted review explicit. The integration fixture remains opt-in and build-tagged.

### Remaining tasks

- [ ] Completed (Slice 2.6 quality gate) remains unchecked and is outside this assigned Migration Slice 4.6 correction boundary.

### Workload / PR boundary

- Delivery strategy: `auto-chain`; assigned/applied boundary: Migration Slice 4.6 gate blockers only.
- No commit or PR was created.

### Structured status consumed

- `changeName`: `contextual-installation-menu`; `artifactStore`: `openspec` (authoritative); `applyState`: `ready`; `nextRecommended`: `apply`; `blockedReasons`: `[]`.
- `actionContext`: `repo-local`; workspace root and allowed edit root: `/home/dev/Documents/works/ebenezer/alice-installer`.
- Strict TDD was active from `openspec/config.yaml`. The task's `auto-chain` delivery boundary was applied. No action-context warning was raised.

---

## Slice 2.6 — final quality-gate bookkeeping reconciliation (2026-07-11)

**Status:** complete. This reconciliation changes no production code, tests, or documentation. It closes the only remaining persisted task because its stated quality gates now pass and the coverage threshold is met in the relevant packages.

### Completed task and persisted checkbox update

- [x] **Completed** (Slice 2.6 quality gates and rollback confirmation) is visibly marked `[x]` in `tasks.md`.
- The Final Acceptance Checklist was already fully `[x]`; it remains accurate. No checklist claim was broadened.

### Verification

- `go test -count=1 ./internal/tui ./cmd/installer ./internal/migration` — passed.
- `go test -count=1 ./...` — passed.
- `go test -cover ./...` — passed; `internal/tui` **80.0%**, `internal/migration` **80.0%**, and `cmd/installer` **80.6%**.
- `go vet ./...` — passed.
- `go build ./...` — passed.
- `git diff --check` — passed.

### TDD Cycle Evidence

No RED/GREEN/TRIANGULATE/REFACTOR cycle was required: this was a bookkeeping-only reconciliation after independently verified implementation and test evidence. No production or test code changed.

### Deviations from design

- None.

### Integration-runtime scope

- The opt-in Docker/PostgreSQL integration runtime was intentionally **not executed**. It remains a separate explicit environment validation and is not claimed by the commands above.

### Remaining tasks

- None. All persisted implementation task checkboxes are `[x]`.

### Workload / PR boundary

- Delivery strategy: `auto-chain`; assigned boundary: final Slice 2.6 quality-gate bookkeeping only.
- No commit or PR was created.

### Structured status consumed

- `changeName`: `contextual-installation-menu`; `artifactStore`: `openspec` (authoritative); native pre-reconciliation `applyState`: `ready`; `nextRecommended`: `apply`; `blockedReasons`: `[]`.
- `actionContext`: `repo-local`; workspace root and only allowed edit root: `/home/dev/Documents/works/ebenezer/alice-installer`.
- Strict TDD is active from `openspec/config.yaml`; no code was written, so no RED/GREEN cycle applied. No action-context warning was raised.
