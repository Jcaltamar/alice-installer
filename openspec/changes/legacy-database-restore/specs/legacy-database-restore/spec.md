# Legacy Database Restore Specification

## Purpose

Provide a fail-closed, destructive legacy PostgreSQL restore for the confirmed interactive Migration route on Linux amd64 and arm64, while preserving recovery artifacts and leaving existing installation and operational routes unchanged.

## Requirements

### Requirement: Interactive migration is the only restore route

The system MUST expose legacy database restore only after the operator confirms the interactive Migration option and the existing validated legacy-backup gate succeeds. The system MUST NOT offer or execute restore through unattended, headless, update, restart, dry-run, or ordinary install routes.

#### Scenario: Confirmed interactive migration reaches restore

- GIVEN the operator selected and confirmed Migration in the interactive installer
- AND the existing legacy backup and manifest are validated
- WHEN the existing installation flow completes successfully
- THEN the system MUST enter the restore flow
- AND the system MUST preserve the existing installation behavior before restore begins

#### Scenario: Restore is invoked through an unsupported route

- GIVEN restore is requested by `--unattended`, a headless route, or an indirect unsupported invocation
- WHEN the request is evaluated
- THEN the system MUST fail closed before restore side effects
- AND the system MUST NOT drop, recreate, restore, stop, or restart any database or service

### Requirement: The post-start wait is exactly 60 seconds

The system MUST perform one context-cancellable wait of exactly 60 seconds after the existing Compose stack startup succeeds and before stopping `backend`. The system MUST NOT use the existing health timeout, a polling loop, or an additional wait as a substitute for this interval.

#### Scenario: Stack startup succeeds

- GIVEN the confirmed interactive Migration flow has completed Compose startup successfully
- WHEN the post-start migration interval begins
- THEN the system MUST wait exactly 60 seconds before requesting backend stop
- AND cancellation MUST be observable during the wait

#### Scenario: Cancellation occurs during the wait

- GIVEN the system is in the 60-second wait
- WHEN the operation context is cancelled
- THEN the system MUST stop the restore flow without database mutation
- AND the result MUST identify cancellation at the wait stage

### Requirement: Compose identities remain immutable

The system MUST use the existing Compose service and container identities without renaming, replacing, or guessing them: application service `backend` and container `alice_backend`; database service `postgresql-master` and container `alice_postgresql-master`. The system MUST NOT alter the associated Compose files, volumes, or identity variables as part of restore.

#### Scenario: Restore controls the known deployment

- GIVEN the restore flow is operating on the supported workspace
- WHEN it resolves services and containers for cutover
- THEN it MUST target the fixed identities exactly
- AND it MUST fail closed if the expected identities are absent, ambiguous, or changed

### Requirement: Cutover stops and starts only the backend service

The system MUST stop only Compose service `backend` before database mutation and MUST start only that service after a validated restore. The system MUST NOT call whole-stack shutdown, stop PostgreSQL, restart PostgreSQL, stop the database container, or use a container-wide operation that can affect `postgresql-master`.

#### Scenario: Backend stop succeeds

- GIVEN the 60-second wait has completed and all pre-mutation gates have succeeded
- WHEN cutover begins
- THEN the system MUST stop Compose service `backend` only
- AND `postgresql-master` MUST remain running

#### Scenario: Backend stop fails

- GIVEN the system has not mutated the target database
- WHEN the backend-only stop operation fails
- THEN the system MUST abort without dropping, recreating, or restoring the target database
- AND the result MUST NOT report migration success

#### Scenario: Validated restore completes

- GIVEN the database restore and integrity checks have succeeded
- WHEN service recovery begins
- THEN the system MUST start Compose service `backend` only
- AND the system MUST require the established backend healthy state before success

### Requirement: PostgreSQL remains available during cutover

The system MUST verify that PostgreSQL service `postgresql-master` and container `alice_postgresql-master` remain running and reachable while `backend` is stopped and throughout database replacement. The system MUST use a bounded, secret-free availability check and MUST fail closed when PostgreSQL is unavailable or the target identity cannot be verified.

#### Scenario: PostgreSQL remains reachable

- GIVEN `backend` is stopped for cutover
- WHEN the availability check runs
- THEN the system MUST establish a successful connection to the expected PostgreSQL instance
- AND it MUST continue only while PostgreSQL remains available

#### Scenario: PostgreSQL becomes unavailable

- GIVEN the target database has not yet been safely validated after replacement, or a later cutover stage is active
- WHEN PostgreSQL becomes unavailable
- THEN the system MUST report the actual stage and failure
- AND it MUST keep `backend` stopped when starting it could use an unvalidated database

### Requirement: Two validated backups are mandatory recovery gates

The system MUST require and revalidate the readable legacy custom-format dump and matching manifest under `/opt/alice/backups/` before destructive work. After the backend is stopped and before target replacement, the system MUST create, validate, and atomically publish a second custom-format backup of the newly installed post-migration/post-seed target database. The system MUST retain both validated backups and MUST NOT drop or recreate the target unless both are validated.

#### Scenario: Both backup gates succeed

- GIVEN the legacy dump and manifest pass path, checksum, format, and readability validation
- WHEN the newly installed target database backup is created
- THEN the system MUST validate and publish that backup with protected permissions
- AND only then MAY the system begin target replacement

#### Scenario: Either backup gate fails

- GIVEN either backup is missing, unreadable, malformed, mismatched, incomplete, or invalid
- WHEN the gate is evaluated
- THEN the system MUST stop before target drop/recreate
- AND the system MUST preserve any complete validated backup
- AND the result MUST NOT report success

### Requirement: Generated `.env` credentials are narrowly and safely resolved

The system MUST read only the required target database values from the installer-generated `.env` through an allowlisted parser. The parser MUST reject missing, malformed, empty, duplicate, conflicting, or ambiguous values, including an invalid database host, port, name, or user. The system MUST NOT obtain target credentials through broad environment inspection, arbitrary files, container inspection, or guessed defaults.

#### Scenario: Valid generated credentials are available

- GIVEN the generated `.env` is the workspace-owned artifact for the installation
- WHEN the restore flow parses it
- THEN the system MUST accept only the required allowlisted database values
- AND it MUST use the resolved target identity for subsequent database operations

#### Scenario: Credential input is ambiguous or invalid

- GIVEN a required value is missing, duplicated, malformed, empty, conflicting, or sourced outside the generated `.env`
- WHEN credential resolution runs
- THEN the system MUST fail closed before target mutation
- AND it MUST identify a bounded credential-validation failure without revealing the password

### Requirement: Temporary credential material is protected and cleaned

The system MUST keep database passwords out of command arguments, logs, UI messages, errors, manifests, Docker metadata, and persisted evidence. Any temporary credential boundary MUST be operation-owned, protected with mode `0600` or stronger, and removed on every success, failure, cancellation, and timeout path.

#### Scenario: Restore executes with a password

- GIVEN a valid password is required by a database client
- WHEN the client process is launched
- THEN the password MUST be supplied only through the approved protected credential boundary
- AND neither the password nor an equivalent secret-bearing diagnostic MUST be observable in recorded evidence

#### Scenario: Restore exits on any path

- GIVEN temporary credential material exists
- WHEN restore completes, fails, is cancelled, or times out
- THEN the system MUST remove only operation-owned temporary credential material
- AND the result MUST remain secret-free

### Requirement: Target replacement is explicit and never merges data

The system MUST safely terminate active sessions to the target database through a maintenance connection, then explicitly drop and recreate the target database before restore. The system MUST NOT merge rows or objects, perform best-effort continuation, rely on `--clean` as the sole destructive operation, or restore into the existing database without recreation. Database identifiers MUST be validated and safely quoted.

#### Scenario: Target replacement begins

- GIVEN both backups are validated, `backend` is stopped, and PostgreSQL is reachable
- WHEN target replacement executes
- THEN the system MUST terminate target sessions safely
- AND it MUST drop the target database and create a new target database before invoking restore
- AND no legacy data may be merged with the newly installed data

#### Scenario: Session termination or replacement fails

- GIVEN target replacement has not completed
- WHEN sessions cannot be terminated safely or drop/create fails
- THEN the system MUST fail closed
- AND it MUST NOT continue with a partial best-effort import

### Requirement: Legacy archive restore uses fail-fast ownership and privilege semantics

The system MUST restore the validated custom-format legacy dump with `pg_restore` semantics equivalent to `--no-owner --no-privileges --exit-on-error`. The restore MUST stop on the first restore error and MUST NOT silently continue, rewrite incompatible data, or suppress an unsuccessful restore result.

#### Scenario: Archive restore succeeds

- GIVEN a newly recreated target database and a validated legacy custom-format dump
- WHEN the restore command executes
- THEN it MUST apply no owner changes and no privilege/ACL restoration
- AND it MUST exit on the first error
- AND the system MUST continue only when the restore exits successfully

#### Scenario: Archive restore fails

- GIVEN target replacement has begun
- WHEN `pg_restore` returns a failure or violates the required semantics
- THEN the system MUST classify the operation as a partial cutover
- AND it MUST NOT start `backend` against the unvalidated target

### Requirement: Restore completion requires bounded integrity evidence

The system MUST collect secret-free evidence that the restore succeeded before backend start: successful `pg_restore` exit, a new connection to the restored target database, application-table presence in a non-system schema, and continued PostgreSQL availability. System catalogs alone MUST NOT satisfy application-table evidence. The system MUST require backend start and the established healthy state before reporting migration success.

#### Scenario: Restored database passes integrity checks

- GIVEN `pg_restore` completed successfully
- WHEN the system validates the target
- THEN a new target connection MUST succeed
- AND at least one application table MUST exist in a non-system schema
- AND PostgreSQL MUST remain reachable
- AND only then MAY the system start `backend` and perform the final health check

#### Scenario: Integrity or final health evidence fails

- GIVEN any required database, application-table, PostgreSQL, backend-start, or backend-health check fails
- WHEN completion is evaluated
- THEN the system MUST NOT report migration success
- AND it MUST report the failed stage using bounded, redacted evidence

### Requirement: Automatic rollback uses the validated new-database backup

After target replacement begins, the system MUST use the validated backup of the newly installed post-migration/post-seed database as the sole automatic data-rollback source. Automatic rollback MUST use the same fail-closed drop/recreate, restore, integrity-validation, and backend-health sequence. The system MUST never merge, use an unvalidated artifact, overwrite either validated backup, or delete either backup.

#### Scenario: Restore fails after target replacement

- GIVEN the validated new-database backup exists and target replacement has begun
- WHEN the legacy restore or subsequent integrity validation fails
- THEN the system MUST keep `backend` stopped
- AND it MUST revalidate and use the validated new-database backup for automatic replacement rollback
- AND it MUST start `backend` only after rollback integrity and health checks succeed

#### Scenario: Automatic rollback cannot complete

- GIVEN automatic rollback has been attempted
- WHEN rollback validation, restore, backend start, or health verification fails
- THEN the system MUST leave PostgreSQL running and `backend` stopped
- AND it MUST report an explicit partial cutover with bounded recovery guidance
- AND it MUST preserve both validated backups

### Requirement: Partial cutover is explicit and stage-aware

Any failure after target drop/recreate begins MUST be classified as a partial cutover rather than an ordinary installation failure. The system MUST report the actual cutover stage, MUST not expose raw command output or secrets, and MUST provide bounded recovery guidance based on whether the target database was mutated and whether automatic rollback succeeded.

#### Scenario: Failure after destructive mutation

- GIVEN target drop/recreate has begun
- WHEN restore, integrity, PostgreSQL availability, backend start, or backend health fails
- THEN the result MUST explicitly identify partial cutover
- AND it MUST state whether rollback was attempted and succeeded
- AND it MUST never claim normal migration success

### Requirement: Cancellation preserves recovery and cleanup guarantees

The system MUST honor cancellation at every cancellable restore stage. Cancellation MUST clean only operation-owned temporary secrets and incomplete staging artifacts, MUST preserve both validated backups, MUST report the actual stage, and MUST keep `backend` stopped whenever the target may be mutated or unvalidated.

#### Scenario: Cancellation before target replacement

- GIVEN cancellation occurs before target drop/recreate
- WHEN the operation exits
- THEN the system MUST avoid database mutation
- AND it MUST restore the backend only if it was stopped and the target remains unchanged
- AND it MUST verify backend health before treating recovery as complete

#### Scenario: Cancellation after target replacement begins

- GIVEN cancellation occurs after target mutation may have begun
- WHEN the operation exits
- THEN the system MUST classify the result as partial cutover
- AND it MUST keep `backend` stopped until a validated database and healthy backend are established
- AND it MUST preserve both validated backups

### Requirement: Restore is Linux amd64/arm64 only and fails closed elsewhere

The system MUST support this restore flow only on Linux amd64 and Linux arm64. On Windows and any other unsupported platform, the installer MUST not offer Migration restore and MUST return an explicit unsupported result without restore side effects. Unattended execution MUST fail closed in the same manner.

#### Scenario: Supported Linux platform

- GIVEN the host is Linux amd64 or Linux arm64
- AND the operator is using the confirmed interactive Migration route
- WHEN restore prerequisites pass
- THEN the system MAY execute the restore flow

#### Scenario: Windows or unsupported platform

- GIVEN the host is Windows or an unsupported operating system/architecture
- WHEN restore is evaluated or invoked indirectly
- THEN the system MUST return an explicit unsupported result
- AND it MUST not stop services, mutate databases, create a cutover backup, or invoke restore commands

### Requirement: Existing non-restore paths remain unchanged

The system MUST preserve the behavior and sequencing of existing Install, Update, Restart, `--dry-run`, and `--unattended` paths, including their existing Compose startup, migration, seed, health, and routing behavior. Enabling interactive restore MUST NOT broaden destructive behavior into those paths.

#### Scenario: Ordinary installation or operation runs

- GIVEN the operator uses Install, Update, Restart, `--dry-run`, or `--unattended` without the supported confirmed interactive Migration route
- WHEN the installer executes
- THEN it MUST follow the pre-existing route and service behavior
- AND it MUST not invoke legacy restore orchestration or destructive database replacement

#### Scenario: Interactive migration reaches restore boundary

- GIVEN the existing interactive Migration flow has completed its unchanged installation steps
- WHEN the restore boundary is reached
- THEN only the newly specified restore stages MAY execute
- AND all pre-boundary behavior MUST remain unchanged

### Requirement: Legacy PM2 processes are selectively quiesced before installation

On Linux amd64 and Linux arm64 only, after the legacy backup has been revalidated and before the first side effect of the new installation, the system MUST acquire a complete PM2/process/socket snapshot and stop only qualifying PM2 processes through individual PM2 commands. A process qualifies only when its canonical working directory is `/opt/alice-guardian` or a descendant of that directory and it listens on TCP port `8080`, or its canonical working directory is `/opt/backend_alice_guardian` or a descendant and it listens on TCP port `9090` or `4550`. Directory-prefix collisions, such as `/opt/alice-guardian-old`, MUST NOT qualify. The system MUST NOT use `pm2 stop all`, stop unrelated PM2 processes, stop non-PM2 processes, or invoke broad service/process shutdown.

#### Scenario: Linux PM2-only quiescence matches exact path and port conjunctions

- GIVEN the validated legacy backup gate has succeeded on Linux amd64 or arm64
- AND PM2, socket, and process-incarnation evidence identifies a running PM2 process under `/opt/alice-guardian` listening on `8080`
- AND another qualifying process is under `/opt/backend_alice_guardian` listening on `9090` or `4550`
- WHEN quiescence is requested
- THEN the system MUST stop each qualifying PM2 identity individually before new installation begins
- AND it MUST not stop a process matching only a directory, only a port, a wrong port, an unrelated PM2 record, or a non-PM2 listener

#### Scenario: Quiescence is gated by the validated backup and first installation side effect

- GIVEN the legacy backup is missing, invalid, ambiguous, or not yet revalidated
- OR the PM2 quiescence operation has not completed successfully
- WHEN the migration handoff is evaluated
- THEN the system MUST abort before stopping any process or starting the new installation
- AND no new-installation side effect, including Compose startup, MAY occur

#### Scenario: Process and socket identity is correlated fail-closed

- GIVEN a PM2 record appears to match a legacy root and port
- WHEN the system correlates PM2 id, PID, canonical cwd, executable identity, listening socket owner, and Linux process start-time/incarnation evidence
- THEN all required evidence MUST agree for the same process incarnation before it is selected
- AND PID reuse, changed cwd or executable, changed start time, changed socket owner or port, missing or contradictory evidence, duplicate ownership, or any ambiguity MUST fail closed without stopping that identity

#### Scenario: Race is detected at the stop boundary

- GIVEN a complete candidate snapshot has been acquired
- WHEN any selected or competing process changes, disappears, respawns, changes port ownership, or becomes ambiguous before or between individual stops
- THEN the system MUST stop no further process
- AND it MUST recover only identities already proven stopped
- AND it MUST not chase a replacement process or affect an unrelated process

### Requirement: PM2 stop identities and compensating recovery are exact and verifiable

The system MUST record the exact successfully stopped PM2 identities, including PM2 id, PID, canonical cwd, executable evidence, matched port, and process-incarnation evidence, in deterministic, bounded, secret-free evidence. On any quiescence, installation, or restore failure after one or more identities have been stopped, it MUST restart only those recorded identities individually and MUST verify their identity, incarnation, expected cwd/executable relationship, and expected listening port after restart. It MUST leave the recorded processes stopped on successful migration and MUST never use name-only or broad restart operations.

#### Scenario: Partial stop is recovered narrowly

- GIVEN two qualifying PM2 identities were selected and only the first stop was proven successful
- WHEN the second stop fails or evidence becomes ambiguous
- THEN the system MUST restart only the first recorded identity
- AND it MUST verify recovery before reporting the failure
- AND it MUST leave the second identity and every unrelated process untouched

#### Scenario: Installation or restore failure recovers the stopped set

- GIVEN one or more exact PM2 identities were proven stopped
- WHEN new installation or database restore fails, is cancelled, or times out
- THEN the system MUST attempt bounded recovery only for that stopped set
- AND it MUST verify each recovered identity before reporting recovery
- AND it MUST fail closed with the stopped-set evidence if recovery cannot be proven

#### Scenario: Successful migration retains PM2 quiescence

- GIVEN installation, database restore, validation, backend restart, and health checks all succeed
- WHEN migration completes
- THEN the system MUST leave exactly the selected legacy PM2 identities stopped
- AND it MUST not restart any selected or unrelated PM2 process

#### Scenario: Cancellation, permissions, and missing tools fail closed

- GIVEN cancellation or timeout occurs, or PM2, socket inspection, or required `/proc` evidence is unavailable or permission-denied
- WHEN quiescence or recovery is attempted
- THEN the system MUST stop before any unsafe next action
- AND it MUST recover only identities already proven stopped under a bounded recovery context
- AND it MUST report a stage-aware, secret-free failure without treating missing evidence as proof that no process exists

### Requirement: PM2 evidence and non-migration routes remain bounded and unchanged

PM2 quiescence evidence MUST contain only bounded identities, ports, canonical paths, process-incarnation data, stop/recovery status, and stable diagnostic codes; it MUST NOT contain command stderr, environment dumps, passwords, or arbitrary command output. The capability MUST be constructed only for confirmed interactive Migration on supported Linux platforms. Install, Update, Restart, `--dry-run`, `--unattended`, headless, Windows, unsupported-platform, and ordinary detection routes MUST remain unchanged and MUST receive no PM2 quiescer.

#### Scenario: Secret-free evidence is persisted

- GIVEN PM2 snapshot, stop, recovery, or failure evidence is recorded
- WHEN the result is formatted, displayed, or persisted
- THEN it MUST contain no secret-bearing or raw command content
- AND it MUST retain enough exact identity and stable-code information to explain the outcome safely

#### Scenario: Existing routes do not gain quiescence

- GIVEN the operator uses Install, Update, Restart, `--dry-run`, `--unattended`, a headless route, Windows, an unsupported platform, or contextual PM2 detection
- WHEN that route executes
- THEN it MUST follow its existing behavior
- AND it MUST not invoke PM2 socket/process correlation, PM2 stop/start, or legacy restore quiescence
