# Exploration Addendum: PostgreSQL backup as the first legacy migration step

**Change:** `contextual-installation-menu`  
**Scope:** exploration only; no implementation  
**Date:** 2026-07-11

## Executive decision

The first executable migration step should be a standalone, confirmation-driven PostgreSQL backup operation. It must prove the exact legacy database container, statically resolve a supported database configuration, create and validate a PostgreSQL 11-compatible custom-format dump, and only then report migration readiness. It must never start, stop, restart, mutate, or delete the legacy application, database container, volumes, or source files.

The existing `Migration` menu destination is informational/blocked. Extend it to a backup preflight and backup operation only; keep all later migration and destructive behavior out of this slice.

## Repository findings

- `internal/tui` already has `ContextActionMigration`, `StateBlockedOperation`, injected dependencies, and a command boundary for future operations. Migration currently performs no command or filesystem mutation.
- `internal/docker` exposes only daemon probe/info/version/runtime operations. It has no container listing, inspect, exec, or stream-to-file abstraction.
- `internal/compose` exposes lifecycle operations and health status for the current Compose workspace, but it cannot identify an unrelated legacy container or execute a binary dump.
- `internal/platform.CommandRunner` buffers stdout/stderr and has no environment override or binary-stream-to-file operation. It is unsuitable for an unbounded custom-format dump.
- `internal/platform.OSCommandRunner` uses `exec.CommandContext`; this is the right low-level process boundary to extend with a narrowly scoped streaming executor rather than embedding `os/exec` in the migration/TUI packages.
- The existing legacy evidence is the exact Linux path `/opt/backend_alice_guardian/node`; it is sufficient to open legacy migration discovery, but it does not identify a PostgreSQL container by itself.
- Current menu policy intentionally blocks Migration. The new operation must preserve that safety boundary for all steps after backup.

## Legacy inputs and safe static configuration resolution

Known inputs:

- application root: `/opt/backend_alice_guardian/node`;
- configuration file: `/opt/backend_alice_guardian/node/config/config.js`;
- CommonJS exports contain production, test, and development Sequelize-like objects;
- development defaults are username `postgres`, database `alice_guardian`, host `HOST_POSTGRES` or `127.0.0.1`, port `PORT_POSTGRES` or `5435`, dialect PostgreSQL;
- production may omit port and therefore uses PostgreSQL's default port;
- legacy database image is `bitnami/postgresql:11-debian-10`.

Do not import, execute, `require`, transpile, or evaluate `config.js`. It is untrusted application code and may contain side effects, arbitrary imports, getters, shell calls, or dynamic credential construction.

Use a dedicated static resolver with a deliberately small accepted grammar:

1. Read the exact regular file without logging its contents.
2. Parse only object/property syntax and approved literal values plus narrowly recognized environment expressions such as `process.env.NAME || literal` / `process.env.NAME ?? literal`.
3. Select the environment explicitly. Prefer a migration configuration field/confirmation that names `production`, otherwise use the legacy application's documented production selection; never silently use test. Development is supported only when the selected object is explicitly development.
4. Apply precedence deterministically: selected config value, then approved referenced host environment value, then the documented development fallback; production's absent port means 5432. Do not merge arbitrary fields across environments.
5. Require PostgreSQL dialect and reject dynamic expressions, unsupported syntax, missing required fields, malformed types, unresolved required environment values, and ambiguous environment selection.
6. Return a redacted typed configuration. Password values must be retained only in memory long enough to establish the dump process environment, never in evidence, errors, UI, manifests, logs, or test fixtures.

The resolver should record only safe facts: selected environment label, host/port/database/user presence, dialect, and the source category of each non-secret value. It must not expose the password or the contents of the config file.

## Exact legacy container discovery

Do not select a container solely because its image contains `postgres`, or because it is the only container returned by a broad image search.

Recommended discovery contract:

1. Query Docker for all containers, including stopped containers, using the exact normalized image reference `bitnami/postgresql:11-debian-10` as an initial candidate filter. Image matching is only a filter, not identity proof.
2. Inspect every candidate by immutable container ID. Collect only non-secret metadata: ID, exact image/repository identity and digest where available, name, state, health, labels, mounts, network mode, and published ports. Never include the full environment in output; if environment inspection is needed, compare selected non-secret keys in memory and redact values.
3. Correlate candidates with the statically resolved config: database/user, configured host/port or mapped port, and Bitnami/PostgreSQL-specific metadata such as expected data mount or Compose/service labels when present. Do not require one fragile signal when deployment history may vary.
4. Require exactly one candidate with sufficient corroborating evidence. If there are zero, multiple, image-tag aliases without enough evidence, or conflicting endpoint/config evidence, stop with an actionable ambiguity result and require operator selection or a later explicit container-ID input. Never choose by name order, creation time, or first result.
5. A stopped candidate is not usable for backup. Do not start it automatically: starting a database is a mutating/lifecycle action and can change recovery semantics. Report that the identified container must be started by the operator and re-run.
6. A candidate with a declared healthcheck must be healthy. An unhealthy candidate must block the dump. A running candidate with no healthcheck may proceed only after the dump connection itself succeeds; absence of a healthcheck is not health evidence.
7. Docker daemon unavailable, permission denied, inspect failure, malformed inspect output, or incomplete identity evidence is an unknown/precondition failure—not “no legacy database.”

This requires a migration-specific Docker inspector interface. Do not overload `DockerClient` with migration policy or expose generic unbounded Docker methods to the TUI.

## Dump execution choice

Use the PostgreSQL 11 client from the exact Bitnami 11 container through `docker exec`, streaming its binary stdout directly into a host temporary file. This avoids accidentally using a newer host `pg_dump`, which can produce a dump with compatibility behavior not guaranteed for PostgreSQL 11. It also avoids writing a dump into the database container and then using `docker cp`, which mutates container filesystem state and adds cleanup/space ambiguity.

The preferred command shape is an argument-vector invocation equivalent to:

- `docker exec <immutable-container-id> pg_dump --format=custom --file=- --host=<resolved-host-for-container> --port=<resolved-port> --username=<resolved-user> --dbname=<resolved-database>`

The implementation must not use `sh -c`, interpolate a shell command, or put a password in argv. The actual connection host used inside the container must be resolved explicitly: typically the container-local PostgreSQL endpoint rather than a host-mapped address; if that cannot be proven from the deployment, fail closed rather than guessing.

Use a migration-specific binary process interface supporting context cancellation, bounded timeout, explicit environment, and streaming stdout to an `io.Writer`. The existing buffered `CommandRunner` and text `StreamingCommandRunner` are insufficient. Password handling should prefer a mode-0600 temporary `PGPASSFILE` supplied through the child environment and removed immediately after the process; a process environment is not perfect secrecy, but it avoids argv and ordinary command-log leakage. Do not print the command with its environment. If a platform cannot provide this safely, refuse the operation rather than falling back to argv or interactive echo.

`docker exec` stderr must be captured only for fixed/redacted error classification. Do not surface raw stderr because it can contain connection details or credentials from client diagnostics. On timeout or cancellation, terminate the process, remove the partial file, and report that no validated backup exists.

## Dump format, destination, and atomicity

Use PostgreSQL custom format (`pg_dump --format=custom`, commonly `.dump`). It is appropriate for a single-database migration because it is portable, supports `pg_restore --list`, and avoids plain SQL credential/comment leakage patterns. Do not use directory format in this first slice because it complicates atomicity and manifest handling.

Destination contract:

- Require an explicit operator-selected backup directory or use a documented protected default outside the legacy application tree; never write beneath `/opt/backend_alice_guardian/node` or a migration source path.
- Create the directory only after confirmation, with restrictive permissions, and reject symlinked or unsafe destination components where the safety contract cannot be proven.
- Create a uniquely named temporary file in the destination directory, mode `0600`, using `O_EXCL`; do not use a predictable filename.
- Stream the dump into that file. On any error, cancellation, timeout, short/empty output, validation failure, or checksum failure, close and remove the temporary file. Do not replace an existing final backup.
- `fsync` the file, close it, atomically rename it to the final name, then `fsync` the directory where supported. The final file remains mode `0600` and must be owned by the invoking user (or the explicitly documented service account).
- Check available space before starting using a conservative minimum threshold. A preflight estimate is not a guarantee; treat write-full errors as failure and clean up. Do not claim exact dump size from database metadata unless obtained safely.
- Prevent concurrent backup operations for the same selected legacy installation/destination through an operation lock. Cancellation must be honored before discovery, during dump streaming, and before rename.

## Validation and manifest

A successful process exit is not sufficient. Validate the custom dump before making it visible as the completed backup:

1. Run PostgreSQL 11-compatible `pg_restore --list` against the temporary dump, preferably through the same Bitnami 11 container or another explicitly version-pinned 11 client. It must read the dump without connecting to the database. Do not use an unpinned host `pg_restore`.
2. Require non-empty, structurally valid listing output and a successful exit. Do not restore or mutate the target database in this step.
3. Compute SHA-256 while writing or by a second read before rename; record byte size and checksum only after validation.
4. Write a sidecar manifest atomically after the dump is validated and renamed. The manifest contains schema/version, UTC timestamp, safe source identity (container ID may be truncated only if collision-safe), exact image reference/digest, selected environment label, database/user/host/port with password omitted, dump format, byte size, SHA-256, client/validation version, and tool outcome. It must not contain config contents, environment maps, passwords, raw stderr, or secret-bearing paths.
5. Use restrictive permissions for the manifest and apply the same temp-file/rename/fsync rules. If manifest publication fails, report the backup as incomplete and retain neither an apparently complete dump nor an unpaired manifest.

The operation result should distinguish `validated backup`, `cancelled`, `precondition failure`, `ambiguous container`, `config unsupported/malformed`, `dump failed`, `validation failed`, and `destination/storage failure`. Only `validated backup` may unlock a later migration phase.

## Failure and safety matrix

| Condition | Required behavior |
| --- | --- |
| No exact-image candidates | Stop; explain that no legacy PostgreSQL candidate was found; no lifecycle action |
| Multiple plausible candidates | Stop; require explicit operator disambiguation; never pick one |
| Candidate stopped | Stop; tell operator to start it; installer does not start it |
| Candidate unhealthy | Stop; no dump and no lifecycle action |
| Docker unavailable/permission denied | Stop as infrastructure failure; do not classify as absent |
| Config missing, malformed, dynamic, ambiguous, or unsupported | Stop before Docker exec; no guessed defaults except documented development/default-port rules |
| Required password unavailable | Stop without revealing which secret value was attempted |
| Host/port/database/user mismatch | Stop; require operator review; do not connect to a guessed endpoint |
| Insufficient space or unsafe destination | Stop before dump; do not delete existing files |
| Dump timeout/cancellation/interrupted stream | Remove partial file; report no validated backup |
| pg_dump exits non-zero | Remove partial file; redact diagnostics |
| `pg_restore --list` fails | Remove/unpublish dump and manifest; report no validated backup |
| Checksum/manifest publication fails | Do not unlock migration; leave no misleading completed artifact |
| Backup succeeds | Publish protected dump + manifest; unlock only the next separately specified migration step |

## Recommended OpenSpec changes

### Proposal/spec

- Replace the current statement that Migration is wholly informational with: Migration is still non-destructive in this slice, but its first executable sub-step is an explicit PostgreSQL backup; no migration mutation is allowed until the backup is validated.
- Add the operational inputs and supported legacy scope above, including the exact application/config paths, image identity, static-only config parsing, and PostgreSQL 11 client requirement.
- Define the user-visible flow: Migration → review source/container/config summary → choose protected destination → confirm backup → running/cancel state → validation result. Keep secrets and raw command output out of the UI.
- Add the container ambiguity, stopped/unhealthy, malformed-config, cancellation, timeout, storage, and Docker-unavailable rules as acceptance criteria.
- State that the backup operation is Linux amd64/arm64 first, matching current legacy detection support, unless a separate platform contract is approved.

### Design

- Add a dedicated `internal/migration` package with separate interfaces for static config resolution, Docker candidate inspection, binary command execution, filesystem/destination handling, clock, and checksum. Keep policy out of Bubbletea.
- Extend the TUI with backup-specific states/messages and an injected `LegacyBackupAction`; Migration must not directly execute generic commands. After success, show the manifest summary and a blocked/next-step screen rather than continuing into destructive migration.
- Add a Docker inspection abstraction for exact container IDs, image identity, state/health, mounts, labels, and ports. Add a stream-capable process abstraction with cancellation, timeout, environment injection, and binary stdout; do not reuse buffered command output for the dump.
- Specify static CommonJS parsing and environment precedence as a closed grammar with fail-closed behavior. Never import or execute `config.js`.
- Specify custom-format dump, pinned PostgreSQL 11 `pg_dump`/`pg_restore`, protected destination, atomic temp-file publication, SHA-256, and atomic manifest publication.
- Preserve existing `internal/docker`, `internal/compose`, and explicit CLI route behavior. Compose health status for the current installation must not be treated as legacy-container identity.

### Tasks

Create a new first migration slice before any destructive-operation tasks:

1. Define redacted legacy config value types and table-driven static parser tests; cover all supported environments, environment fallbacks, default port, malformed/dynamic syntax, missing fields, dialect mismatch, and secret non-retention.
2. Add Docker candidate inspection and deterministic selection tests for exact image, corroborating metadata, multiple candidates, stopped/unhealthy candidates, daemon/inspect failures, and no broad-image-only selection.
3. Add a cancellable binary executor seam and tests proving argv contains no password, environment handling is redacted, timeout/cancel terminates, stderr is not returned raw, and stdout streams without buffering the complete dump.
4. Add protected destination, free-space, lock, mode, symlink/path, atomic rename, fsync, partial cleanup, and concurrent-operation tests using `t.TempDir()`.
5. Add the PostgreSQL 11 custom-format backup action with fake Docker/executor/filesystem seams; test command arguments, exact container ID, config-derived endpoint, pg_dump failure, cancellation, and cleanup.
6. Add pinned `pg_restore --list` validation tests, SHA-256/size computation, and atomic manifest tests; verify no credential or config content reaches artifact text, errors, UI, or logs.
7. Add TUI tests for confirmation/cancel/no-side-effect, progress/result states, blocked continuation after success, and failures that never unlock migration.
8. Add documentation and golden coverage for the backup flow and operational recovery. Keep later restore, schema migration, data transformation, application shutdown, volume changes, and deletion explicitly out of scope.
9. Run focused tests, then the repository’s full Go test/vet/build gates. Do not run real Docker or database commands as part of this exploration or unit-test slice; any external integration test must be opt-in and skippable.

## Risks

- Static parsing can reject configurations that use dynamic JavaScript. This is intentional: accepting unsafe execution is worse than requiring an operator-supplied reviewed connection descriptor in a later contract.
- Docker metadata and health are imperfect identity signals. Requiring a unique candidate and failing on ambiguity is safer than automatic selection.
- `docker exec` still exposes operational metadata to the Docker daemon and may require elevated Docker access. The design avoids argv leakage and container filesystem mutation, but must document Docker permission risk.
- Custom dumps may be large; the current buffered command abstraction cannot be used. Streaming, cancellation, disk checks, and cleanup are mandatory.
- A validated backup is not proof that a future migration is reversible. It is only the gate for the next separately specified step.
