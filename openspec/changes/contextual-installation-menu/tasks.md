# Contextual Installation Menu Tasks

## Review Workload Forecast

| Field | Value |
| ------- | ------- |
| Estimated changed lines | 2,400–3,300 total; six migration slices targeted at 280–390 each |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | Existing slices → Migration 1 → 2 → 3 → 4 → 5 → 6 |
| Delivery strategy | auto-chain |
| Chain strategy | pending |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High

The completed contextual-menu tasks below remain preserved. Migration Step 1 is a separate fail-closed chain. Each migration slice must be independently reviewable, keep tests with the behavior they verify, and remain buildable without exposing an executable Migration entry point until the final activation slice. Slice 1 is safe to implement first: it is static, read-only, testable with synthetic fixtures, and cannot execute a dump or mutate the source deployment.

## Delivery Boundaries

### Slice 1 — detector domain, probes, and shared workspace contract (apply first)

- **Start:** existing repository behavior; no contextual menu dependency is required.
- **Finish:** `internal/installation` provides typed detection, conservative classification, portable workspace probing, Linux-first injectable PM2 probing, and deterministic tests; workspace artifact paths have one shared source of truth.
- **Dependency boundary:** no imports from `internal/tui`, `internal/update`, or `cmd/installer`; no lifecycle commands, deletion, process control, or persistent state mutation.
- **Verification:** focused installation/workspace tests, then `go test ./...`, `go vet ./...`, and `go build ./...`.
- **Rollback:** revert the new installation package and shared path extraction; existing installer flows remain unchanged.
- **Follow-up:** Slice 2 consumes only the stable public types/interfaces and production constructors established here.

### Slice 2 — TUI/menu, update adapter, wiring, and documentation

- **Start:** Slice 1 is merged and its public contracts are available.
- **Finish:** post-splash detection, contextual menu, safe blocked screens, Install/Update routing, dependency wiring, route regression tests, rendering coverage, and documentation are complete.
- **Dependency boundary:** no changes to destructive Uninstall/Migration behavior; blocked destinations issue no command or filesystem/process mutation.
- **Verification:** focused TUI and command tests, golden checks, `go test ./...`, `go vet ./...`, `go build ./...`, and coverage review against the configured 80% threshold.
- **Rollback:** restore splash → preflight and remove detector/menu wiring; explicit update, restart, unattended, and dry-run routes remain available.
- **Out of scope:** full Uninstall or Migration execution, deletion, volume removal, PM2 shutdown, migration compatibility/rollback, or guessed historical identifiers.

## 1. Slice 1 — Detector Domain, Probes, and Shared Workspace Contract

### 1.1 Establish the shared current-artifact contract

- [x] Completed
- **Files:** `internal/workspace/artifacts.go`, related existing workspace tests.
- Extract an exported `workspace.RequiredArtifactPaths(workspaceDir)` helper for `.env` and `docker-compose.yml`; update `ResolveArtifacts` to consume it without changing existing resolution, GPU-overlay, or update/restart behavior.
- **RED:** add tests proving the helper returns deterministic paths and that existing artifact resolution still honors the configured workspace directory and existing contracts; confirm the new test fails before the helper exists.
- **GREEN:** implement the smallest helper extraction and route existing resolution through it.
- **REFACTOR:** remove duplicated path construction, preserve naming/conventions, run `gofmt`, and verify no unrelated workspace behavior changed.
- **Acceptance:** `go test ./internal/workspace`; paths are the single source of truth and no file contents are read by the helper.

### 1.2 Define typed installation state, evidence, probe results, and classification

- [x] Completed
- **Files:** `internal/installation/detection.go`, `internal/installation/detection_test.go`.
- Add `State`, `EvidenceKind`, sanitized `Evidence`, `Detection`, `Presence`, `ProbeResult`, `Probe`, `Detector`, and `CompositeDetector` contracts from `design.md`.
- Implement pure `Classify(current, legacy)` with deterministic evidence ordering and conservative precedence: current uncertainty or legacy uncertainty yields `unknown`; positive current + positive legacy yields `conflict`; absent/unsupported legacy does not become positive evidence.
- **RED:** table-driven tests cover every classification matrix row, conflict precedence, uncertainty precedence, unsupported behavior, and stable evidence ordering.
- **GREEN:** implement value types, classifier, and sequential composite detector without mapping errors to `not-installed`.
- **REFACTOR:** keep classification pure, sort evidence by stable kind/path, and ensure `Detect` cannot mutate state.
- **Acceptance:** all five states are reachable exactly as specified; detector returns typed evidence rather than an error that callers could misclassify.

### 1.3 Implement the portable workspace probe

- [x] Completed
- **Files:** `internal/installation/workspace_probe.go`, `internal/installation/workspace_probe_test.go`.
- Implement `WorkspaceProbe` with an injectable `FileSystem` seam and production `osFS`, using `workspace.RequiredArtifactPaths` and validating regular files only.
- Classify complete artifacts as present, both missing as absent, partial/invalid/wrong-typed artifacts as uncertain with safe evidence, and stat failures as unreadable uncertainty. Never execute commands or read artifact contents.
- **RED:** table-driven `t.TempDir()` tests cover complete, empty, partial, directory/wrong type, unreadable fake filesystem, explicit `--workspace-dir` equivalent, sanitized evidence paths, and no file mutation.
- **GREEN:** implement probe behavior and deterministic evidence categories/details.
- **REFACTOR:** centralize safe path cleaning, avoid developer-home dependencies, and keep filesystem operations limited to `Stat`.
- **Acceptance:** workspace override is honored; optional GPU overlay does not affect presence; partial or invalid artifacts cannot enable Install through classification.

### 1.4 Implement the Linux amd64/arm64 injectable PM2 probe

- [x] Completed
- **Files:** `internal/installation/pm2_probe.go`, `internal/installation/pm2_probe_test.go`.
- Add `CommandRunner`, `LegacyPolicy`, and injected `Platform`; gate execution to Linux amd64/arm64 and invoke exactly `pm2 jlist` under a bounded context timeout.
- Parse only `name`, `pm_exec_path`, and `pm2_env.cwd`/`pm_exec_path`; require exact configured identifiers plus corroborating deployment-root evidence. Empty policy matches nothing. Treat not-found as safe absence; malformed JSON, permission/non-zero errors, timeout, and weak/ambiguous ownership as uncertainty; unsupported platforms must not invoke the runner.
- **RED:** table-driven parser/matcher and fake-runner tests cover empty/unrelated output, every exact match form, weak evidence, path traversal/prefix collision, malformed JSON, not-found, non-zero, timeout, permission error, exact command arguments, unsupported platforms, and no secret retention.
- **GREEN:** implement policy matching, path containment via `filepath.Rel`, platform gating, bounded execution, and fixed safe evidence.
- **REFACTOR:** keep PM2 JSON schema private, avoid retaining stderr/environment maps, isolate command and matching concerns, and make results deterministic.
- **Acceptance:** generic PM2 presence never produces legacy evidence; Linux supported targets exercise the seam; Windows/other targets return explicit unsupported without command execution.

### 1.5 Integrate Slice 1 and verify its rollback boundary

- [x] Completed
- **Files:** `internal/installation/*_test.go`, existing package tests only as needed.
- Wire `CompositeDetector` from the two probes and verify complete end-to-end state results for no evidence, current, legacy, conflict, partial current, and probe failure.
- **RED:** add integration-at-package-boundary tests using fake probes and temporary workspace fixtures that fail before composition is complete.
- **GREEN:** compose probes and classifier with no TUI or command-wiring dependency.
- **REFACTOR:** inspect package exports for the smallest stable API and document any compatibility assumptions in code-level package documentation if the repository convention requires it.
- **Acceptance:** `go test ./internal/installation ./internal/workspace`, `go test ./...`, `go vet ./...`, and `go build ./...` pass; Slice 1 can be reverted without changing interactive behavior.

## 2. Slice 2 — TUI/Menu, Update Adapter, Wiring, and Documentation

### 2.1 Add contextual menu policy and safe informational blocked destinations

- [x] Completed
- **Files:** `internal/tui/context_menu.go`, `internal/tui/context_menu_test.go`, relevant TUI test fixtures.
- Derive immutable actions solely from `installation.Detection` and injected operation availability: Install for `not-installed`; Update and optionally informational Uninstall for `current`; optionally informational Migration for `legacy-pm2`; no lifecycle actions for `conflict`/`unknown`.
- Render safe evidence labels, explanations, and remediation guidance without arbitrary PM2 JSON, stderr, environment values, or secrets. Escape/cancel/quit must have no command or side effect. Uninstall and Migration screens must be blocked and informational only.
- **RED:** direct `Model.Update` tests fail for action matrices, cursor/selection, blocked choices, evidence rendering, Escape, quit, and small-terminal behavior.
- **GREEN:** implement menu model, action messages, blocked state, and deterministic rendering.
- **REFACTOR:** separate policy from view formatting, keep action availability explicit, and preserve global window-size/quit handling.
- **Acceptance:** conflict/unknown expose no Install, Update, Uninstall, or Migration; blocked destinations execute nothing.

### 2.2 Insert detection into the post-splash root state machine

- [x] Completed
- **Files:** `internal/tui/model.go`, `internal/tui/splash.go`, `internal/tui/model_test.go` and existing splash/full-flow tests.
- Add `StateDetecting`, `StateContextMenu`, `StateBlockedOperation`, and `StateActionResult` (plus `StateUpdating` where the existing state model requires it), detection/action messages, injected detector dependency, and detector command handling.
- Change only the interactive splash Enter transition to `DetectionStartedMsg`; invoke detection through an injected `tea.Cmd`; construct the menu from `DetectionCompletedMsg`; keep detection failures typed and out of install preflight.
- **RED:** direct update tests prove splash → detecting → menu, no preflight before Install, conflict/unknown safe exits, cancellation/quit side-effect absence, resize handling, and deterministic completion behavior.
- **GREEN:** implement state transitions and command boundaries while retaining the existing `PreflightStartedMsg` path for Install selection.
- **REFACTOR:** keep existing install/bootstrap transitions untouched, avoid duplicated evidence policy, and preserve the 80×24 guard.
- **Acceptance:** interactive mode detects before preflight; selecting Install from `not-installed` enters the established preflight exactly once; uncertain states cannot route to installation.

### 2.3 Add the UpdateAction adapter and preserve update semantics

- [x] Completed
- **Files:** `internal/tui/model.go` or `internal/tui/context_menu.go`, `internal/update` adapter location established by repository conventions, `internal/tui/update_test.go`, relevant update tests.
- Define the narrow `UpdateAction` interface and production adapter over `update.Run`, preserving workspace targeting and existing compose `pull` before `up -d` behavior. Route Update through one injected `tea.Cmd`, then show success/failure without falling back to Install.
- **RED:** fake update/compose tests prove one invocation, workspace propagation, pull-before-up ordering, pull failure preventing up, and failure result rendering.
- **GREEN:** implement the adapter and `StateUpdating` → `StateActionResult` flow.
- **REFACTOR:** avoid duplicating update artifact resolution or textual progress plumbing; keep the adapter independently replaceable.
- **Acceptance:** contextual Update preserves established behavior and explicit update CLI remains unchanged.

### 2.4 Wire interactive dependencies without changing explicit routes

- [x] Completed
- **Files:** `cmd/installer/main.go`, `cmd/installer/main_test.go` or existing command tests.
- Extend only interactive dependency construction with `WorkspaceProbe` using the resolved workspace directory, narrow centralized empty-by-default `LegacyPolicy`, Linux-gated PM2 probe using the existing command runner, `CompositeDetector`, and the update adapter.
- Confirm explicit `update`, restart, unattended, and dry-run branches return before Bubbletea and never construct or invoke contextual detection.
- **RED:** command tests fail if detector construction leaks into explicit/non-interactive routes or if route behavior changes; test interactive composition receives the workspace override.
- **GREEN:** add dependency wiring and route guards without moving CLI parsing or existing operational branches.
- **REFACTOR:** keep production defaults conservative and centralize policy construction; do not guess historical PM2 identifiers or paths.
- **Acceptance:** `go test ./cmd/installer`; explicit routes remain behaviorally unchanged and no PM2 probe runs outside interactive mode.

### 2.5 Update deterministic rendering and end-user operational documentation

- [x] Completed
- **Files:** `internal/tui/testdata/golden/*` or repository-equivalent golden paths, `internal/tui/*_test.go`, `README.md`, `RUNBOOK.md`.
- Add deterministic golden coverage for each typed state, blocked destination, and small-terminal rendering; use the repository’s `-update` path only and rerun without `-update`.
- Document the post-splash detection flow, supported Linux PM2 architectures, unsupported-platform behavior, safe evidence limitations, ambiguous/conflict remediation, explicit CLI fallback, and that Uninstall/Migration are informational/blocked in this slice.
- **RED:** golden and documentation acceptance tests/checklists fail before the new views and operational guidance exist.
- **GREEN:** add fixtures, update approved golden outputs, and write concise progressive-disclosure documentation.
- **REFACTOR:** remove secret-bearing examples, keep review path scannable, and ensure docs do not imply destructive operation support.
- **Acceptance:** golden tests are deterministic; README/RUNBOOK explicitly state platform limits and blocked operation status.

### 2.6 Run Slice 2 quality gates and confirm complete rollback

- [x] Completed
- **Files:** all Slice 2 files; no new implementation scope.
- Execute focused TUI/update/command tests, then `go test ./...`, `go test -cover ./...`, `go vet ./...`, and `go build ./...`; inspect changed-line count and both slice diffs independently.
- **RED:** record any failing acceptance criterion or coverage regression before correction.
- **GREEN:** resolve failures without weakening safety assertions or adding destructive operation behavior.
- **REFACTOR:** run `gofmt -w` on changed Go files, review the final diff for scope creep, and verify rollback restores splash → preflight while explicit routes remain operational.
- **Acceptance:** all specified tests and build gates pass, coverage is reviewed against 80%, no detector/menu path mutates files or processes, and Slice 2 is independently reviewable atop Slice 1.

## 3. Confirmed Legacy Directory Detection

### 3.1 Add exact legacy directory evidence with optional PM2 fallback

- [x] Completed
- **Files:** `internal/installation/detection.go`, `internal/installation/legacy_directory_probe.go`, related tests, and `cmd/installer` wiring.
- **RED:** regression tests require `/opt/backend_alice_guardian/node` directory evidence to classify as present without invoking PM2 and require production wiring to use the fallback composition.
- **GREEN:** add a platform-gated exact-directory stat probe and compose PM2 only when the exact path is absent or unsupported.
- **TRIANGULATE:** cover directory, missing, regular-file, stat-error, unsupported-platform, PM2 fallback, and PM2 short-circuit cases.
- **REFACTOR:** centralize the exact path as a production constant; expose no arbitrary path input and add no lifecycle or migration behavior.

## 4. Migration Step 1 — Validated PostgreSQL Backup

Migration tasks below are strictly hierarchical and dependency ordered. No task authorizes restore, transformation, shutdown, deletion, cutover, schema migration, volume mutation, or any later migration behavior. Slices 4.1–4.5 expose no TUI migration entry point; only 4.6 may wire the validated backup action.

### 4.1 Migration Slice 1 — Static configuration parser and environment resolver (safe first slice)

- [x] **Start:** existing blocked Migration behavior; no migration package or executable backup path.
- [x] **Finish:** a closed, non-executing parser and redacted resolver support only the approved legacy CommonJS object grammar and explicit environment selection.
- **Files/discovery:** `internal/migration/config.go`, `internal/migration/config_parser.go`, `internal/migration/config_resolver.go`, focused tests; confirm exact legacy input `/opt/backend_alice_guardian/node/config/config.js` and approved fields from proposal/design/exploration.
- **RED:** table-driven tests prove production/development selection, `process.env.NAME || literal` and `??` precedence, production port `5432`, documented development fallbacks only when explicitly selected, typed validation, duplicate/missing fields, dialect mismatch, malformed/dynamic syntax rejection, symlink/non-regular/size-limit rejection, and absence of a synthetic secret sentinel from errors, redacted values, serialized plans, and fixtures.
- **GREEN:** implement a bounded static lexer/parser and resolver; never import, require, transpile, evaluate, execute, merge environments, or log source contents. Keep password material in an unexported narrow holder with explicit release/zero semantics and expose only redacted facts/source categories.
- **TRIANGULATE:** add adversarial cases for calls, imports, computed properties, spreads, templates, getters, concatenation, arbitrary member access, ambiguous environment selection, empty overrides, invalid ports, and unsupported platforms; scan all returned diagnostics for secret/config content.
- **REFACTOR:** keep parser, resolver, and secret holder separate; use fixed field/category errors and deterministic outputs. Run `gofmt` and `go test ./internal/migration`.
- **Rollback/verification:** remove only the new static-input package; existing TUI and explicit routes remain unchanged. This is the first slice safe to implement.

### 4.2 Migration Slice 2 — Exact Docker container correlation

- [x] **Start:** 4.1 provides a validated redacted config and secret holder; no process execution exists.
- [x] **Finish:** migration-specific Docker inspection selects exactly one corroborated immutable container or returns a typed fail-closed outcome.
- **Files/discovery:** `internal/migration/container_inspector.go`, `internal/migration/container_selector.go`, Docker fixtures/tests; do not expand `internal/docker` or expose environment maps to TUI.
- **RED:** table-driven tests cover exact normalized `bitnami/postgresql:11-debian-10` filtering, full immutable container IDs, every candidate inspection, corroborating endpoint/database/user/mount/label evidence, zero/multiple candidates, image-only and alias ambiguity, stopped, unhealthy, no-healthcheck, daemon/permission/inspect/malformed metadata failures, and container-local endpoint proof.
- **GREEN:** implement safe metadata DTOs, deterministic correlation, fixed redacted evidence, and typed `ambiguous-container`/precondition outcomes. Never select first/newest/by-name and never invoke Docker start/stop/restart.
- **TRIANGULATE:** fuzz or table-test prefix-collision IDs, conflicting endpoints, sensitive labels/mounts/environment maps, stopped candidates, and exact candidate ordering; assert secrets and raw Docker output are absent from all results.
- **REFACTOR:** isolate normalization and correlation predicates, retain only allowlisted metadata, and keep Docker policy out of TUI. Run focused migration tests plus `go vet ./internal/migration`.
- **Rollback/verification:** revert inspector/selector and fixtures without changing source lifecycle behavior; no dump command can be constructed by this slice.

### 4.3 Migration Slice 3 — Secure credential transport proof and binary streaming boundary (hard blocker)

**Status: COMPLETE (2026-07-11, approved helper-container decision).** The rejected `docker exec`, `docker cp`, secret-bearing stdin, `PGPASSWORD`, shell, and password-argv alternatives remain prohibited. Slice 4.3 instead constructs only a separate helper-container process boundary: direct `docker run --rm --pull=never` with the exact reviewed `bitnami/postgresql:11-debian-10` image, Linux `--network host`, a read-only mount of a host `0700` temporary directory's `0600` pgpass file at `/run/alice-installer/pgpass`, and non-secret `PGPASSFILE` path. Random name/operation labels, direct named `docker rm --force` reconciliation, bounded redacted stderr, binary stdout streaming, and process-group cancellation/timeout are proven by focused fake/OS process tests. No real Docker/database command, dump orchestration, backup file, validation, TUI wiring, or legacy-container mutation is included.

- [x] **Start:** 4.1 and 4.2 are reviewed; implementation is limited to the approved helper-container process boundary.
- [x] **Prerequisite/blocker A:** document and test the host `0700`/file `0600` read-only bind-mount transport; password is absent from argv, rendered commands, process logs, Docker diagnostics metadata, UI, errors, fixtures, and persisted artifacts.
- [x] **Prerequisite/blocker B:** fake process/Docker harness captures argv, non-secret environment/mount metadata, stderr classification, and named cleanup metadata; synthetic-secret scans cover every observable fake-harness boundary.
- [x] **Prerequisite/blocker C:** prove exact PostgreSQL 11 helper-image policy, direct argv/no shell, resolved endpoint args, cancellation/timeout process-group termination, bounded fixed-code stderr classification, binary stdout streaming, `--rm`, and explicit name-based cleanup.
- **Files/discovery:** `internal/migration/process.go`, `internal/migration/credential_transport.go`, narrowly reusable `internal/platform` process adapter only if required, focused tests and a security decision record in this task section.
- **RED:** tests fail for password-in-argv/logs, unallowlisted environment, missing protected mount/stdin proof, shell invocation, buffered stdout, leaked stderr, cancellation, timeout, and process-tree cleanup.
- **GREEN:** implement only the proven `ProcessSpec`/`BinaryExecutor` seam, secure temporary-secret lifecycle, direct argv, bounded stderr, streaming writer, and typed cancellation results. Do not add `pg_dump` orchestration until blockers A–C pass.
- **TRIANGULATE:** run secret-sentinel scans over arguments, captured logs, errors, progress, and serialized metadata; verify cleanup on success, failure, timeout, and cancellation without printing credentials.
- **REFACTOR:** narrow interfaces, remove raw diagnostics, and make transport failure fail closed. Run focused tests and `go test ./...` before unlocking 4.4.
- **Rollback/verification:** if any prerequisite fails, retain Migration as blocked and revert only the process seam. This is a release blocker, not a reason to weaken credential handling.

### 4.4 Migration Slice 4 — Protected destination and streaming backup engine

- [x] **Start:** 4.3 secure transport proof is accepted; no TUI action is wired.
- [x] **Finish:** a confirmed immutable backup plan can stream a custom-format dump to protected staging with cancellation, locking, space checks, and complete cleanup.
- **Files/discovery:** `internal/migration/backup_action.go`, `internal/migration/destination_store.go`, `internal/migration/lock.go`, `internal/migration/backup_*_test.go`; use `t.TempDir()` and fakes only.
- **RED:** tests cover confirmation boundary, safe destination outside source tree, symlink/path/ownership/permission rejection, `0700` directory creation after confirmation, conservative free-space check, non-blocking lock, unique `O_CREATE|O_EXCL` mode-`0600` staging, exact container ID/config endpoint, custom format, empty/short output, write-full, dump failure, cancellation at every gate, timeout, and idempotent partial/secret-file/lock cleanup without overwriting existing files.
- **GREEN:** implement preflight and run orchestration using the proven binary seam; stream stdout directly into staging, sync/close before validation, and return typed redacted outcomes. No source lifecycle operation is permitted.
- **TRIANGULATE:** inject clock/space/lock/filesystem/executor failures and assert no misleading completed artifact or raw diagnostic remains. Verify repeat operations and pre-existing files are preserved.
- **REFACTOR:** separate plan validation from execution, keep `BackupPlan` immutable, and make cancellation checks explicit before discovery, process start, post-stream, validation, and publication. Run focused tests, `go vet ./...`, and `go build ./...`.
- **Rollback/verification:** remove backup orchestration and storage adapters; no published artifact is deleted by code rollback and no TUI route exists.

### 4.5 Migration Slice 5 — PostgreSQL 11 validation, checksum, manifest, and atomic publication

- [x] **Start:** 4.4 produces only staged, unvalidated output and 4.3 transport remains proven.
- [x] **Finish:** only a non-empty structurally valid PostgreSQL 11 custom dump yields a paired protected dump/manifest publication.
- **Files/discovery:** `internal/migration/validator.go`, `internal/migration/manifest.go`, publication/storage tests; use a pinned fake PostgreSQL 11 validator and never a host-unpinned client.
- **RED:** tests cover `pg_restore --list` without database connection, pinned client identity, empty/malformed listing, validation failure cleanup, SHA-256/byte size, deterministic versioned secret-free manifest, `0600` permissions, cancellation before publication, no-overwrite renames, directory sync, each rename/sync/manifest failure, and removal of operation-created half-pairs while preserving pre-existing artifacts.
- **GREEN:** implement validation, checksum/size, manifest encoding with only approved safe fields, and transactional pair publication. Publish neither file as complete unless both renames and directory synchronization succeed.
- **TRIANGULATE:** scan manifest, errors, logs, progress, and artifact text for synthetic credentials, config contents, pgpass paths/content, raw stderr, and unallowlisted metadata; verify only `validated` carries final paths/checksum/size.
- **REFACTOR:** isolate manifest schema/versioning and atomic filesystem operations; run focused tests, `go test ./...`, `go vet ./...`, and `go build ./...`.
- **Rollback/verification:** revert validator/publication while retaining no misleading completion state; later migration remains impossible.

### 4.6 Migration Slice 6 — TUI confirmation/progress/results, activation, documentation, and integration verification

- [x] **Start:** slices 4.1–4.5 are reviewed, all blockers pass, and the action remains absent from production wiring.
- [x] **Finish:** Linux amd64/arm64 interactive Migration Step 1 is explicitly confirmed, cancellable, redacted, validated-only, and followed by a blocked later-step screen; explicit CLI/non-interactive routes remain unchanged.
- **Files/discovery:** `internal/tui` backup states/messages/views/tests/goldens, `cmd/installer/main.go` and route tests, `README.md`, `RUNBOOK.md`, opt-in integration tests; preserve existing blocked behavior until final wiring.
- **RED:** direct `Model.Update`/teatest tests cover preflight→confirm→running→result, no artifact before confirmation, duplicate-submit prevention, cancellation handshake, progress categories only, every failure staying blocked, validated result→`MigrationBlocked`, resize/small terminal, and absence of secrets/raw output. Route tests prove explicit update/restart/unattended/dry-run bypass detection and backup wiring.
- **GREEN:** inject only `LegacyBackupAction`, add redacted review/confirmation/progress/result screens, wire supported platform composition, and document Docker permissions, destination/storage, cancellation/recovery, platform limits, validation limits, and no later migration behavior.
- **TRIANGULATE:** run integration tests only with `testing.Short()` skip behavior and clean VM/container fixtures; run focused TUI/command tests, golden update path then non-update run, `go test ./...`, `go test -cover ./...`, `go vet ./...`, and `go build ./...`.
- **REFACTOR:** inspect the complete chained diff and secret scans; confirm no restore, schema change, transformation, shutdown, deletion, volume mutation, cutover, or generic migration executor was introduced.
- **Rollback/verification:** remove `LegacyBackupAction` wiring to return Migration to informational blocked state; never delete operator-owned backup artifacts. This is the only activation slice.

## Final Acceptance Checklist

- [x] Exactly one typed detection state is returned for every probe combination.
- [x] Partial, malformed, unreadable, ambiguous, failed, or conflicting evidence never enables Install or destructive actions.
- [x] The exact confirmed legacy directory is policy-owned, directory-validated, Linux-gated, and sufficient without PM2.
- [x] PM2 matching is exact, policy-driven, corroborated, Linux amd64/arm64 only, and fully injectable as fallback.
- [x] Workspace detection uses the configured override and one shared artifact-path contract.
- [x] Install reuses the existing preflight; Update preserves pull-before-up and explicit CLI behavior.
- [x] Uninstall remains blocked; Migration remains blocked until final activation and exposes only the validated Step 1 backup, never later migration behavior.
- [x] Escape, cancellation, quit, and blocked screens have no command or filesystem/process side effects.
- [x] Existing explicit update, restart, unattended, and dry-run routes bypass contextual detection.
- [x] Documentation and golden tests reflect the delivered behavior and platform limits.
- [x] Migration static parsing, exact container correlation, secure credential transport proof, streaming, protected publication, validation, TUI, and integration gates are complete.
- [x] Slice 1 is applied and reviewed before Slice 2; each migration slice has its own test evidence and rollback boundary.
