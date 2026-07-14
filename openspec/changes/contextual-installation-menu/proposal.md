# Add a safe contextual installation menu with a non-destructive migration backup

## Intent

After the splash screen, the interactive installer will detect the local Alice installation state and present only actions justified by reliable evidence. For a confidently identified legacy installation, Migration Step 1 will be executable as an explicitly confirmed, validated PostgreSQL 11 backup. Every later migration action remains blocked.

The business outcome is to let operators begin migration with a trustworthy recovery artifact while preventing the installer from changing or removing the legacy system. Ambiguous evidence, failed validation, cancellation, or unsupported conditions must leave the source deployment untouched and must not authorize further migration.

## Product outcome

| Installation state | Available action(s) |
| --- | --- |
| No installation detected | Install |
| Current Compose installation detected | Update; Uninstall only when separately available |
| Legacy Alice installation confidently detected | Migration Step 1: review, confirm, create, and validate a PostgreSQL backup |
| Unknown, conflicting, or incomplete evidence | No mutating action; show safe evidence and remediation guidance |

The Migration experience is:

1. Review a redacted legacy configuration, container, and destination summary.
2. Explicitly confirm backup creation.
3. Stream and validate a protected PostgreSQL 11 custom-format dump.
4. Receive either a validated backup summary or a safe, redacted failure/cancellation result.
5. Stop at a blocked next-step screen. No restore, transformation, shutdown, deletion, or cutover follows.

## Current-state gap

The existing interactive flow assumes a new installation after the splash. The contextual menu work establishes typed, side-effect-free detection, but Migration was originally informational only. Operators need a safe first migration action before any source or destination mutation is considered.

A process exit alone is not sufficient evidence of a usable backup. The first executable migration step must prove the legacy configuration and database container, create the dump without buffering it or exposing secrets, validate the archive with a PostgreSQL 11-compatible client, and publish a protected dump and manifest as one complete result.

## Scope

### Contextual menu and detection

- Insert installation detection between the splash and existing install preflight.
- Classify the host as `not-installed`, `current`, `legacy-pm2`, `conflict`, or `unknown`.
- Detect current installations from validated workspace artifacts, honoring `--workspace-dir`.
- Detect the confirmed legacy installation on Linux amd64/arm64 from Alice-specific evidence, including the exact known legacy directory and narrowly configured PM2 evidence.
- Preserve the existing Install, Update, restart, unattended, and dry-run contracts.
- Block actions when evidence is partial, conflicting, unreadable, malformed, unsupported, or otherwise inconclusive.

### Migration Step 1: executable validated backup

Migration Step 1 may:

- read the exact legacy configuration file as data using a closed static grammar;
- inspect Docker metadata to identify exactly one corroborated legacy PostgreSQL container using the exact PostgreSQL 11 image identity as an initial filter;
- review a safe destination and require explicit operator confirmation before creating files;
- create a uniquely named, mode-`0600` PostgreSQL custom-format dump by streaming from a pinned PostgreSQL 11 client;
- validate the staged dump with a pinned PostgreSQL 11-compatible `pg_restore --list` operation that does not connect to a database;
- compute SHA-256 and byte size;
- atomically publish the protected dump and a paired secret-free manifest;
- cancel safely, clean partial artifacts, and report typed, redacted outcomes.

Only a validated backup is successful. Even then, it authorizes no later migration action in this change.

## Approved Migration Slice 4.3 helper-container decision (2026-07-11)

The previously rejected `docker exec`/`docker cp` credential transport is superseded for Slice 4.3 only. A PostgreSQL 11-compatible helper is started with direct `docker run --rm` argv using the reviewed exact `bitnami/postgresql:11-debian-10` image identity and `--pull=never`; no image aliases or unreviewed tags are accepted. On Linux it uses host networking to the already resolved legacy endpoint, never guesses or translates an endpoint, and mounts a host-created credential file read-only at the fixed container path `/run/alice-installer/pgpass`.

The host credential artifact is created in a unique `0700` temporary directory as a `0600` `.pgpass` file. Only the non-secret `PGPASSFILE` container path is passed to Docker. The password is not in argv, Docker environment values, logs, errors, UI, manifests, or persisted artifacts. `docker run --rm`, a random collision-resistant helper name, operation labels, and direct named `docker rm --force` reconciliation provide cleanup evidence; unit fakes scan every observable boundary with a synthetic secret sentinel. The helper has no legacy-container lifecycle, filesystem, volume, or database mutation capability. This slice supplies only the credential/process boundary; it does not orchestrate a dump, create a backup destination, validate an archive, or wire the TUI.

## Security and data-loss boundary

This change has a strict non-destructive boundary:

- It must not execute, import, require, transpile, or evaluate the legacy JavaScript configuration.
- It must not guess a database container, endpoint, environment selection, or unsupported configuration.
- It must not start, stop, restart, or otherwise mutate PM2, the legacy application, Docker containers, databases, or volumes.
- It must not restore data, run schema changes, transform data, modify source files, delete resources, or perform cutover.
- It must not overwrite an existing backup.
- It must not continue after ambiguity, unsafe destination checks, insufficient space, cancellation, timeout, dump failure, validation failure, checksum failure, or incomplete publication.
- Credentials and secret-bearing configuration data must never appear in arguments, evidence, errors, UI, manifests, logs, test fixtures, or persisted artifacts. Secret material may exist only in the narrow in-memory/process boundary required for the confirmed backup and must be cleaned immediately afterward.
- Raw command output and unrestricted Docker environment metadata must not reach the UI or logs.

Confirmation is the first point at which destination directories, staging files, locks, or other backup artifacts may be created. Before confirmation, the migration flow is read-only.

## Business rules

- Weak evidence never grants permission to mutate or overwrite anything.
- Exactly one sufficiently corroborated, immutable container identity is required; image matching alone is insufficient.
- A stopped container must be started by the operator outside the installer. An unhealthy container blocks backup.
- Docker unavailability, permission errors, malformed metadata, or conflicting endpoint evidence are precondition failures, not proof that no legacy database exists.
- Backup files must be written outside the legacy source tree to an operator-selected or documented protected destination.
- Temporary and final backup artifacts must use restrictive permissions, exclusive creation, safe path checks, operation locking, cleanup, synchronization, and atomic publication.
- A successful dump process is not success until archive validation, checksum, size calculation, and paired manifest publication complete.
- A validated backup is a prerequisite for a future separately specified migration step; it is not proof of restorability and does not authorize that step.

## Platform scope

- Contextual menu and current Compose artifact detection remain portable across installer-supported platforms.
- Legacy detection and Migration Step 1 are supported first on Linux amd64 and arm64.
- Unsupported platforms must report an explicit unsupported or blocked result and must not infer legacy ownership from weak evidence.
- The backup relies on the exact supported legacy PostgreSQL 11 deployment contract and pinned PostgreSQL 11-compatible dump and validation tooling.
- Broader platform or legacy database support requires a separate approved contract.

## Affected areas

| Area | Expected impact |
| --- | --- |
| `internal/tui` | Detection/menu states plus backup review, confirmation, cancellable progress, result, and blocked-continuation states |
| `internal/installation` | Typed state, safe evidence, current workspace probe, confirmed legacy-directory probe, PM2 fallback, and conservative classification |
| `internal/migration` | Static configuration resolution, container discovery, backup orchestration, destination safety, validation, checksum, manifest, and typed outcomes |
| `internal/platform` | Narrow cancellable binary-streaming process boundary where reusable; no secret-bearing rendered commands |
| `internal/workspace` | Shared current-artifact contract without changing existing update/restart behavior |
| `internal/update` | Thin menu adapter preserving established update behavior |
| `cmd/installer` | Dependency wiring while preserving explicit and non-interactive route behavior |
| `README.md` and `RUNBOOK.md` | Platform limits, backup operation, storage and Docker prerequisites, cancellation, recovery, and blocked later steps |

## Non-goals

- Executing Uninstall.
- Starting, stopping, or repairing a legacy deployment.
- Restoring the backup or proving end-to-end restorability.
- Creating or mutating a destination database.
- Running schema migrations, data transformations, compatibility changes, or cutover.
- Removing PM2 processes, containers, images, files, or volumes.
- Automatically resolving current/legacy conflicts or ambiguous container identity.
- Supporting dynamic JavaScript configuration by executing it.
- Supporting arbitrary PostgreSQL versions, images, legacy layouts, or platforms beyond the approved first scope.
- Adding Restart to the contextual menu or changing explicit CLI/non-interactive routes.

## Phased delivery

The work should remain fail-closed and reviewable throughout:

1. **Context detection and menu foundation:** typed installation state, workspace and legacy probes, action policy, existing Install/Update routing, and blocked operations.
2. **Static migration inputs:** redacted domain types and closed, non-executing legacy configuration resolution.
3. **Legacy database discovery:** migration-specific Docker inspection and deterministic exact-container selection.
4. **Backup execution and storage:** cancellable binary streaming, protected destination, locking, restrictive staging, space checks, and cleanup.
5. **Validation and publication:** pinned PostgreSQL 11 archive validation, SHA-256, size, and atomic dump/manifest publication.
6. **TUI activation and documentation:** confirmation, progress, cancellation handshake, result screens, wiring, and operator guidance.

Migration remains informationally blocked until every Step 1 dependency and fail-closed acceptance criterion is complete. Slices before final activation expose no executable TUI migration entry point. Later migration steps require new proposals/specifications and remain blocked after Step 1 succeeds.

## Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Wrong legacy workload is selected | Require exact image filtering plus corroborating non-secret evidence and one immutable container ID; fail on ambiguity |
| Static configuration rejects dynamic deployments | Fail closed; unsafe code execution is not an acceptable fallback |
| Credentials leak through process or diagnostics | Keep secrets out of arguments and artifacts; use a narrow protected process boundary and fixed redacted errors |
| Backup is corrupt or incomplete | Require pinned archive validation, non-empty structural evidence, checksum, size, and paired atomic publication |
| Cancellation or storage failure leaves misleading artifacts | Terminate execution, clean all operation-created partial files, and publish only a complete validated pair |
| Database lifecycle changes source state | Never start, stop, restart, restore, or otherwise mutate the container or database |
| Docker permissions expand operational risk | Document the requirement, use narrow inspection/execution interfaces, and fail without fallback when access is unsafe |
| A validated backup creates false confidence | State clearly that validation is only the gate for a future separately approved step, not proof of restoration or migration completion |
| Unsupported platforms produce false confidence | Limit Step 1 to Linux amd64/arm64 and render explicit blocked outcomes elsewhere |

## Rollback

Rollback removes or disables the injected `LegacyBackupAction` and returns Migration to its informational blocked screen. If necessary, the contextual menu itself can be rolled back by restoring splash-to-preflight routing. Existing explicit update, restart, unattended, and dry-run routes remain unchanged.

Rollback must never delete operator-owned backup artifacts. Because Step 1 does not mutate the legacy application, database, containers, volumes, or source files, no source-system reversal is required. Any validated dump and manifest already created remain protected files under operator control.

## Success criteria

- Interactive mode detects installation state after the splash and before install preflight.
- Install and Update preserve their established behavior and are offered only for the correct reliable state.
- Unknown, conflict, partial, failed, or unsupported evidence enables no unsafe action.
- A confidently detected supported legacy installation may enter Migration Step 1 only when its backup dependency is available.
- The operator sees a redacted review and explicitly confirms before any backup artifact is created.
- Static configuration resolution executes no JavaScript and fails closed on unsupported or ambiguous input.
- Exactly one corroborated running legacy PostgreSQL 11 container is required; stopped, unhealthy, absent, conflicting, or ambiguous candidates block execution.
- The dump streams to a protected temporary file, is cancellable and bounded, and never exposes credentials or raw diagnostics.
- PostgreSQL 11-compatible archive validation, SHA-256, size, restrictive permissions, and atomic dump/manifest publication are required for success.
- Every cancellation and failure path leaves no misleading completed or partial backup created by the operation.
- A validated result shows a safe manifest summary and then blocks all later migration actions.
- No Step 1 path starts, stops, restarts, restores, transforms, modifies, or deletes legacy or destination resources.
- Linux amd64/arm64 support and all platform, Docker, storage, cancellation, recovery, and migration-limit expectations are documented.
