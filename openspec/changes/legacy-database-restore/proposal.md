# Restore a validated legacy database during interactive migration

## Intent

Extend the existing interactive legacy migration path from validated backup creation through a controlled replacement of the newly installed database. The installer will keep the current installation sequence unchanged, wait exactly 60 seconds after the Compose stack starts, stop only the `backend` service, create and validate a rollback backup of the newly installed database, replace that database with the validated legacy dump, and restart the backend only after restore checks pass.

This change gives Linux operators an end-to-end migration path while preserving two independent recovery artifacts: the original validated legacy backup and a validated snapshot of the newly installed post-migration/post-seed database. The operation is destructive and therefore fails closed at every uncertain boundary.

## Product outcome

For a supported legacy installation on Linux amd64 or arm64, the interactive Migration option will:

1. Require the existing validated legacy backup under `/opt/alice/backups/`.
2. Run the existing installation flow without changing its behavior.
3. Wait one context-aware interval of exactly 60 seconds after stack startup completes.
4. Stop only Compose service `backend`; keep PostgreSQL running.
5. Create and validate a second backup of the newly installed target database.
6. Drop and recreate the target database, then restore the legacy custom dump without merging data.
7. Validate database usability and application-table presence.
8. Restart `backend` and require it to become healthy before reporting success.

The fixed identities remain unchanged:

| Role | Compose service | Container |
| --- | --- | --- |
| Application | `backend` | `alice_backend` |
| Database | `postgresql-master` | `alice_postgresql-master` |

Windows and unattended execution do not offer migration and fail closed if the restore path is invoked indirectly.

## Current-state gap

The existing migration flow intentionally ends after creating a validated legacy PostgreSQL backup. It does not restore that backup, coordinate service-level downtime, read target credentials, preserve the newly installed database for rollback, or distinguish a partial cutover from an ordinary installation failure.

The current Compose abstraction also lacks a service-scoped stop operation, and the installer has no narrow parser for its generated `.env`. Reusing whole-stack shutdown, broad environment inspection, or ordinary health timeouts would violate the required PostgreSQL availability, secret-handling, and exact-wait contracts.

## Scope

### Interactive migration orchestration

- Enable restore only from the confirmed interactive Migration option after the existing validated legacy-backup gate.
- Keep the existing environment generation, image pull, Compose startup, migrations, and seeds unchanged.
- Insert one cancellable 60-second wait immediately after successful stack startup and before stopping the backend.
- Stop only Compose service `backend`; never stop or restart `postgresql-master` as part of cutover.
- Preserve all existing service names, container names, volumes, and Compose identities.

### Backup prerequisites and rollback artifact

- Require a readable, validated legacy custom-format dump and matching manifest in `/opt/alice/backups/`.
- Revalidate the legacy artifact, including checksum and safe path constraints, before destructive work.
- After the backend is stopped and before target replacement, create a second custom-format backup of the newly installed post-migration/post-seed target database.
- Validate and atomically publish that second backup with protected permissions and secret-free evidence.
- Do not drop or recreate the target database unless both backups are validated and retained.

### Credentials and restore

- Read only the required target database values from the installer-generated `.env` through a narrow, allowlisted parser.
- Reject missing, malformed, empty, duplicate, or ambiguous credential values.
- Keep passwords out of argv, logs, UI messages, errors, manifests, Docker metadata, and persisted evidence.
- Connect through a protected temporary credential boundary such as a mode-`0600` pgpass file, cleaned on every exit path.
- Terminate target sessions safely, then explicitly drop and recreate the target database; no object or row merge is permitted.
- Restore the legacy custom dump with `pg_restore --no-owner --no-privileges --exit-on-error` or an equivalent direct-argv operation with the same ownership, ACL, and fail-fast semantics.

### Validation and completion

Restore completion requires all of the following:

- `pg_restore` exits successfully.
- A new connection to the restored target database succeeds.
- At least one application table exists in a non-system schema; system catalogs alone are insufficient.
- PostgreSQL remains running and reachable.
- Compose service `backend` is started again and reaches the established healthy state.

Success must not be emitted at the database-restore step. Backend restart and health are part of the cutover transaction.

## Business rules

- A validated legacy backup remains mandatory and never authorizes destructive work by itself.
- A validated backup of the newly installed database is a second mandatory gate before target replacement.
- The installer must use the fixed workspace, Compose service identities, and generated target credentials; it must not guess a database, container, endpoint, or credential source.
- PostgreSQL must remain running while the backend alone is stopped.
- Database replacement is explicit drop/recreate. Merge, best-effort import, and continuation after restore errors are prohibited.
- Any ambiguity, unsupported platform, cancellation, validation failure, credential failure, service-control failure, or health failure blocks success.
- Both validated backups are operator recovery artifacts and must be preserved on every success, cancellation, and failure path.
- Existing Install, Update, Restart, `--dry-run`, and `--unattended` behavior remains unchanged.

## Failure semantics

| Failure point | Required result |
| --- | --- |
| Before backend stop | Abort migration without database mutation; preserve both backups that already exist |
| Backend stop fails | Do not create a destructive cutover state and do not mutate the target database |
| New-database backup creation or validation fails | Do not drop the target database; preserve the legacy backup and any complete validated rollback artifact |
| Failure after target drop/recreate begins | Report an explicit partial cutover; keep PostgreSQL running, keep backend stopped, preserve both backups, and provide recovery guidance |
| Restore validation fails | Treat as partial cutover; do not start the backend against an unvalidated database |
| Backend start or health fails after a valid restore | Report an explicit partial cutover, preserve both backups, and provide a bounded backend recovery action; never report migration success |
| Cancellation | Honor context cancellation, clean only operation-owned temporary secret/partial files, preserve validated backups, and report the actual cutover stage |
| Unsupported platform or unattended invocation | Fail closed before migration side effects; do not offer the option |

Raw command output and secrets must not be surfaced as recovery guidance. Failure evidence must be bounded, redacted, and sufficient to identify whether the target database was mutated.

## Affected areas and platforms

| Area | Expected impact |
| --- | --- |
| `internal/migration` | Restore coordinator, two-backup gates, destructive sequencing, validation evidence, redacted outcomes, and cleanup |
| `internal/compose` | Narrow service-scoped backend stop/start operations and recording fakes; no whole-stack shutdown |
| `internal/workspace` / environment boundary | Allowlisted parser for installer-generated target database values |
| `internal/tui` | Post-deploy wait/restore states, progress, cancellation, success, and explicit partial-cutover messaging |
| `cmd/installer` | Interactive Linux dependency wiring only; existing explicit and unattended routes remain unchanged |
| `README.md` / `RUNBOOK.md` | Destructive behavior, backup locations, platform limits, recovery, and partial-cutover operations |
| Tests | Strict-TDD unit and transition coverage; optional isolated Linux integration coverage |

Supported restore platforms are Linux amd64 and Linux arm64. Windows remains an installer target for existing features, but legacy restore is not offered there and must return an explicit unsupported result without side effects.

## Non-goals

- Supporting legacy migration through `--unattended` or any headless route.
- Renaming Compose services, containers, volumes, environment variables, or workspace artifacts.
- Changing the existing installation, migration, or seed commands before the post-start wait.
- Stopping PostgreSQL or using whole-stack `compose down` during migration.
- Merging the legacy dump into the newly installed database.
- Transforming incompatible schemas, extensions, ownership, ACLs, or legacy data automatically.
- Supporting Windows restore, arbitrary PostgreSQL versions, arbitrary dump formats, or unknown legacy layouts.
- Deleting either validated backup automatically.
- Claiming application correctness beyond bounded database evidence and backend health.
- Refactoring unrelated detection, update, restart, uninstall, or menu behavior.

## Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Irreversible target data loss | Require and validate both legacy and newly installed database backups before drop/recreate |
| Wrong database or service mutation | Use fixed Compose identities, workspace-scoped artifacts, allowlisted credentials, and explicit database identity checks |
| Credential disclosure | Use protected temporary credential transport and redacted, bounded diagnostics; scan tests for secret sentinels |
| Partial cutover is mistaken for normal failure | Use typed stage-aware outcomes; keep backend stopped after unvalidated mutation and show explicit recovery guidance |
| Legacy PostgreSQL 11 archive is incompatible with the current target | Validate archive first, restore fail-fast with ownership/ACL suppression, and preserve both recovery artifacts |
| Concurrent target access | Stop backend first, connect through a maintenance database, terminate target sessions, and fail on unsafe conditions |
| Fixed wait is mistaken for readiness proof | Keep the exact 60-second requirement separate from final connection, table, PostgreSQL, and backend health checks |
| Platform-specific helper/network behavior | Limit support to Linux amd64/arm64 and fail closed everywhere else |
| Review fatigue hides destructive-path defects | Deliver as chained slices under the 400 changed-line review budget with strict TDD and independent risk/resilience review |

## Rollback plan

### Before target replacement

Abort safely. If the backend was stopped, restart it against the unchanged newly installed database and verify health. Preserve the validated legacy backup and the validated new-database backup if already created. Remove only operation-owned temporary credential files and incomplete staging artifacts.

### After target replacement begins

The automatic data rollback source is the validated backup of the newly installed post-migration/post-seed database. Recovery must use the same fail-closed replacement mechanics: keep the backend stopped, revalidate that backup, drop/recreate the target, restore it, validate connection and application-table evidence, then start and health-check the backend. Never merge or restore from an unvalidated artifact.

If automatic rollback cannot complete, leave PostgreSQL running and backend stopped, retain both backups, report a partial cutover, and provide operator recovery instructions. No failure path may delete or overwrite either backup.

### Feature rollback

Disable the interactive restore coordinator and return Migration to the existing validated-backup completion/blocked state. Existing installation, update, restart, dry-run, and unattended paths remain unaffected. Feature rollback must not remove backups already created by operators.

## Success criteria

- Interactive Migration is the only route that can invoke legacy restore; `--unattended` and Windows do not offer it and fail closed if reached.
- The existing validated legacy dump and manifest under `/opt/alice/backups/` are required and revalidated before cutover.
- The existing installation runs unchanged and is followed by one cancellable wait of exactly 60 seconds after stack startup.
- Only Compose service `backend` is stopped; `postgresql-master` remains running and reachable throughout cutover.
- Service/container identities remain exactly `backend`/`alice_backend` and `postgresql-master`/`alice_postgresql-master`.
- Target credentials are read only from the generated `.env`, allowlisted and validated, and no secret appears in observable output or persisted evidence.
- A second validated backup of the newly installed post-migration/post-seed database exists before destructive replacement.
- Target replacement explicitly terminates sessions, drops and recreates the database, and performs no merge.
- Restore uses `--no-owner`, `--no-privileges`, and `--exit-on-error` semantics.
- Success requires restore exit success, target connection success, application tables in non-system schemas, PostgreSQL availability, and a restarted healthy backend.
- Every failure and cancellation reports the true stage, preserves both validated backups, cleans temporary secret material, and never reports false success.
- Any failure after destructive replacement begins is clearly identified as a partial cutover with bounded recovery guidance.
- Strict TDD covers operation ordering, exact timing, service isolation, two-backup gates, secret non-disclosure, destructive sequencing, cancellation, rollback, and all partial-cutover outcomes.

## Chained delivery recommendation

The exploration forecasts approximately 500–800 changed lines, exceeding the 400 changed-line review budget. Deliver this change as an automatically sliced chain, with each slice independently testable and fail-closed:

1. **Restore safety foundation:** typed restore states/results, generated `.env` parser, legacy backup revalidation, new-database backup contract, redaction tests.
2. **Compose and database executor:** backend-only stop/start, protected credential/process boundary, target backup execution, drop/recreate, `pg_restore`, and failure cleanup tests.
3. **Interactive orchestration:** exact 60-second wait, TUI state transitions, Linux-only wiring, cancellation, partial-cutover and rollback behavior.
4. **Operational validation:** documentation and opt-in isolated Linux integration evidence for amd64/arm64.

Keep every review slice below 400 changed lines where practical. A slice must not activate destructive TUI behavior until all prerequisite safety gates it depends on are complete. The chain should receive reliability and resilience review, plus risk review for credential handling and destructive database operations.
