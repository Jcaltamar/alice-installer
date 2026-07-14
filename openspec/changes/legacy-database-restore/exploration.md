# Legacy database restore — exploration

## Executive summary

The repository already has a fail-closed legacy PostgreSQL backup pipeline and a TUI migration entry point, but it intentionally stops after publishing a validated dump. The requested flow should extend that capability with a **separate restore coordinator** inserted after the existing install `compose up` completes. The existing deployment command and service/container names can remain unchanged. The coordinator must own the 60-second migration drain, backend-only stop, credential resolution, destructive restore, integrity evidence, and backend restart; it must never reuse `down`, restart PostgreSQL, rename artifacts, or claim success before the final backend start succeeds.

This is a high-risk, Linux-only operational slice. It is not a small change: the existing `ComposeRunner` has no service-scoped stop method, there is no existing `.env` value parser, and the current TUI/headless install flows have different seams. The safest first implementation is a new domain-level restore interface with narrow injected process/Compose seams, then wiring only the confirmed interactive migration flow. Do not silently broaden the existing non-interactive install/update routes without an explicit product decision.

## Existing implementation map

### Legacy backup and validation

- `internal/migration/backup_action.go`
  - `BackupAction.Preflight` resolves the static production config, discovers one exact PostgreSQL 11 container, and preflights a protected destination without side effects.
  - `BackupAction.Run` streams a custom-format dump, validates it, computes checksum/size, and atomically publishes the dump plus manifest.
  - `BackupResult.Outcome == BackupValidated` is the only successful backup result; `DumpPath`, `ManifestPath`, `SHA256`, and `Size` are safe evidence.
  - `LegacyApplicationRoot` is already `/opt/backend_alice_guardian/node`; the requested source config is therefore governed by `internal/migration/config_open_linux.go`, `config_parser.go`, `config_resolver.go`, and the production wiring in `cmd/installer/main.go`.
- `internal/migration/process.go`
  - `BuildHelperDump` uses a pinned PostgreSQL 11 helper, custom format, and a temporary protected pgpass boundary. This is the best starting point for restore process construction, but restore must have a distinct operation and must not reuse dump semantics.
- `internal/migration/validator.go`
  - `PG11ArchiveValidator.Validate` uses `pg_restore --list` in a pinned helper with the dump mounted read-only and no database connection. This is suitable for the initial archive gate, not for proving destination restore success.
- `internal/migration/manifest.go`, `destination_store.go`
  - Protected mode-0600 artifacts, atomic publication, no replacement of existing names, ownership-aware cleanup, and secret-free manifest conventions already exist and should be preserved.

### Compose and install orchestration

- `internal/compose/runner.go`
  - `ComposeRunner` currently exposes `Pull`, `Up`, `Restart`, `Down`, and `HealthStatus`, but **no `Stop` or service-scoped operation**.
  - `CLICompose.Up` executes `docker compose ... up --detach`.
  - `CLICompose.Down` is whole-stack and is unsafe for this flow because PostgreSQL must remain running.
  - `baseArgs` is the established argument builder; a new service-scoped method must use it and append exactly `stop backend` (or the reviewed equivalent), never container names.
- `internal/compose/fake.go` and `runner_test.go`
  - Existing recording fake and command-argument tests are the test seam for proving only service `backend` is stopped and that Compose files/.env are passed through.
- `internal/assets/docker-compose.yml` and `docker-compose.gpu.yml`
  - Required identities are already present: service `backend` / `container_name: alice_backend`; service `postgresql-master` / `container_name: alice_postgresql-master`.
  - PostgreSQL uses host networking and volume `postgresql_master_data1`; backend depends on PostgreSQL health. The restore must not alter these names, volumes, Compose variables, or YAML identities.
- `internal/headless/run.go`
  - Sequential stages are env-write, pull, deploy, then health verify. `Compose.Up` completes at lines around 296–313 and verify starts immediately afterward.
  - Defaults are 3-second polling and 60-second health timeout; this is **not** the requested exact post-start 60-second migration wait. The restore wait needs a separate injectable clock/sleeper and must not overload `VerifyTimeout`.
- `internal/tui/model.go`, `internal/tui/deploy.go`, `internal/tui/verify.go`
  - TUI transitions `DeployCompleteMsg` to `HealthTickMsg`, then creates `VerifyModel`.
  - The natural insertion point for an enabled legacy restore is the `DeployCompleteMsg` branch, before the existing health verification branch. The existing `Up` operation remains unchanged; a new restore state/model should run after it.
  - On final restore success, the coordinator should emit/re-enter the existing verification path only after backend has been started again. On restore failure, transition to a terminal failure/result state with explicit recovery guidance and retained backup evidence.
- `internal/tui/context_menu.go`
  - `LegacyBackupAction` currently exposes only `Preflight` and `Run`; the migration menu is explicitly labelled unavailable in the current UI and `StateMigrationBlocked` is reached after validated backup.
  - The requested flow needs a new, separately named restore/migration action interface rather than widening `LegacyBackupAction` in a way that makes a validated backup automatically authorize destructive work.
- `cmd/installer/main.go`
  - Interactive production dependencies already wire Linux amd64/arm64 backup to `/opt/alice/backups/`, production environment, `migration.OSBinaryExecutor`, and the Docker inspector.
  - New restore dependencies should be wired only on supported platforms, with the same runtime Compose runner and workspace path. Existing explicit `update`, `restart`, `--dry-run`, and `--unattended` routing must remain unchanged unless the proposal explicitly includes them.

### Installer-generated credentials

- `internal/envgen/env.go` renders the generated `.env`; `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DATABASE`, host, and port are written there.
- `internal/workspace/artifacts.go` centralizes `.env` and Compose artifact paths, but only checks existence; it does not parse `.env`.
- No reusable safe dotenv parser was found. Add a narrow parser (or extend the workspace package) that reads only allowlisted keys, handles the repository’s generated `KEY=value` format, rejects malformed/ambiguous values, and returns a secret-bearing value only inside the restore process boundary. Never log its result, include it in errors, or pass it through broad environment inspection.
- `internal/migration/config_resolver.go` is for legacy JavaScript config resolution and must not be used to read newly generated `.env` credentials; it has different semantics and a secret-bearing private type.

## Proposed restore state machine

```text
legacy migration selected
  -> existing validated backup action (mandatory gate)
  -> existing install/env/pull/up path unchanged
  -> deployment completes
  -> exact 60s wait (context-aware, test clock)
  -> resolve and validate installer .env credentials
  -> Compose stop service backend only
  -> verify postgres service/container remains running and reachable
  -> validate published dump + manifest/checksum again
  -> destination preflight (database identity, connection, no active backend)
  -> terminate target DB sessions
  -> DROP DATABASE target; CREATE DATABASE target
  -> pg_restore custom dump with exit-on-error and reviewed ownership/ACL policy
  -> safe integrity/usability evidence (connection, expected catalog/table evidence, no secret output)
  -> Compose start service backend only
  -> verify backend is running/healthy and PostgreSQL remains healthy
  -> success
```

The 60-second wait must occur after `Up` returns successfully and before stopping `backend`. “Exactly 60 seconds” should mean one bounded wait of `60 * time.Second`, cancellable by context; do not substitute the existing health timeout or a polling loop that may exceed the required interval. The coordinator should not report restore success at the database step: backend start and final usability verification are part of success.

For the TUI, the likely new states are `StateDatabaseRestoreWait`, `StateDatabaseRestore`, and/or a single restore model with explicit substage rendering. Preserve the existing global cancellation behavior. For headless support, introduce a separate opt-in dependency/configuration rather than automatically treating every `Deploy=true` install as a destructive migration.

## Safe restore mechanics

1. Require `BackupValidated` plus a readable dump and manifest. Recompute SHA-256 and compare to the manifest before any destination mutation; reject path changes, symlinks, missing files, or partial artifacts.
2. Resolve `.env` through the workspace path and parse only the required allowlisted keys. Validate host/port/database/user and reject empty or conflicting values. Keep the password in a private secret wrapper or temporary pgpass file with mode 0600; remove it on every exit path.
3. Stop only Compose service `backend` using a new service-scoped Compose abstraction. Do not call `Down`, `Restart`, `docker stop alice_backend`, or any operation that can stop `postgresql-master`.
4. Prove PostgreSQL remains running using a narrow, non-secret health/connection check against the known Compose service/container identity. Do not infer identity from arbitrary container listings.
5. Use a pinned PostgreSQL-compatible helper/client boundary, analogous to `BuildHelperDump`, with direct argv and protected pgpass transport. Avoid `PGPASSWORD` in argv, Docker environment metadata, logs, or error strings. A helper can use host networking because the generated Compose PostgreSQL service already uses host networking, but this needs an explicit Linux-only contract and a Windows alternative or a fail-closed unsupported result.
6. For destructive replacement, connect to a maintenance database (normally `postgres`), terminate connections to the target database, then execute `DROP DATABASE` and `CREATE DATABASE` with strict identifier validation/quoting. Do not interpolate untrusted identifiers into shell strings. Restore into the newly created target using `pg_restore --exit-on-error`; decide explicitly whether `--no-owner` and ACL handling are required for the legacy dump. Do not use `--clean` alone as the only destructive guarantee: explicit drop/recreate makes the no-merge contract clear and removes objects absent from the dump.
7. Evidence should be bounded and secret-free: command exit status, target connection success, database identity, catalog/schema/table count or a known safe query, and final PostgreSQL health. Do not expose raw `pg_restore` output or credentials.
8. If restore fails after the drop, do not attempt an unvalidated merge or silently retry. Leave PostgreSQL running but backend stopped, retain the validated legacy backup, mark the installation/migration incomplete, and provide operator recovery guidance. If backend stop fails, do not mutate the database. If backend start fails after a successful restore, report a partial cutover with the backup retained and backend restart command guidance; never claim success.
9. A rollback cannot reconstruct the pre-restore new database from the current requirements. The only safe automatic rollback is process/lifecycle rollback (ensure backend is stopped or, after a successful restore, attempt the single defined backend start) and preservation of the legacy dump. If data rollback is required, create a separate pre-restore destination backup/change proposal before destructive mutation.

## Test seams and strict-TDD coverage

Tests should be written RED first under `go test ./...` using fakes; no unit test should require a live Docker daemon.

- `internal/compose`: add `Stop(ctx, files, envFile, service)` or an equivalent narrowly typed `StopService` interface; test exact argv/order and reject empty/multiple/unexpected service names. Extend `FakeComposeRunner` with stop calls/errors and call order.
- New restore package (recommended `internal/migration/restore.go` plus tests): inject Compose service control, bounded command executor, clock/sleeper, filesystem/hash/manifest reader, and a safe credential/parser boundary. Table-test every state transition and failure point.
- Restore tests must prove: backup gate; 60-second wait; `.env` parsing without secret leakage; backend-only stop; PostgreSQL remains untouched; checksum mismatch blocks destructive work; drop/recreate before restore; no merge flags; restore validation; backend restart; restart/restore failures never report success; cancellation preserves backup; context timeout cleanup; no secret sentinel in argv/errors/log/evidence.
- `internal/tui`: direct `Model.Update` tests for deploy-complete → wait/restore, successful final start → verify/result, each failure stage, cancellation, duplicate completion, and terminal partial-state messaging. Existing migration-flow tests in `internal/tui/migration_flow_test.go` are the model for this seam.
- `internal/headless`: only if the confirmed product scope includes unattended migration. Add a separate flow test proving ordinary install remains unchanged and migration call order is backup → existing deploy → wait → restore → backend start.
- Integration tests should be opt-in/build-tagged and run in clean Linux PostgreSQL containers/VMs. Linux amd64/arm64 are the supported restore targets. Windows amd64 should fail closed with an explicit unsupported result unless a reviewed Windows process/network/client contract is added.

## Security and operational risks

- **Destructive data loss:** drop/recreate is irreversible without a pre-restore backup. Require validated legacy backup and strongly consider a destination pre-restore backup as a separate prerequisite.
- **Credential exposure:** `.env`, Docker inspect output, `PGPASSWORD`, argv, process listings, panic/error wrapping, and progress logs can leak secrets. Keep a narrow pgpass boundary and redact all evidence.
- **Wrong target:** fixed Compose service/container identities and workspace-scoped Compose files must be used; no fuzzy container discovery or volume manipulation.
- **Concurrent access:** backend must be stopped before database mutation; other clients can still connect. Terminate target DB sessions and fail on unexpected active writers where possible.
- **Migration compatibility:** legacy PostgreSQL 11 custom archive into the current Timescale/PostgreSQL 15 image may contain extensions, ownership, or version-specific objects that fail. Validate archive format first, then capture bounded restore errors; do not silently transform data.
- **Compose portability:** service-scoped stop syntax is available in Compose v2, but host networking and helper-container behavior are platform-specific. Current legacy detection/backup policy is Linux amd64/arm64; Windows must not infer support.
- **Timing/races:** the fixed 60-second drain is a business requirement, not proof migrations completed. Final DB and backend evidence remains mandatory.
- **Failure semantics:** after database destruction, an error is a partial migration, not a normal install failure. The UI/result model and documentation need distinct wording and operator recovery steps.

## Files/symbols likely affected

| Area | Symbols/files | Expected role |
| --- | --- | --- |
| Restore domain | new `internal/migration/restore.go`, tests | State machine, dump/manifest gate, credentials, destructive restore, evidence, cleanup |
| Process boundary | `internal/migration/process.go` or new restore process helper, tests | Pinned pg client/helper, pgpass, bounded redacted results |
| Compose | `internal/compose/runner.go`, `fake.go`, tests | Service-only backend stop/start abstraction; preserve names |
| TUI | `internal/tui/model.go`, new restore model/messages, tests/goldens | Insert after deploy completion and before final verification |
| Wiring | `cmd/installer/main.go`, `main_test.go` | Supported-platform dependency injection; preserve explicit routes |
| Env/workspace | `internal/workspace` or new parser, tests | Safe allowlisted `.env` parsing; no secret logging |
| Docs | `README.md`, `RUNBOOK.md` | Destructive semantics, partial failure recovery, platform limits |
| Integration | `internal/migration/*_integration_test.go` / scripts | Opt-in Linux database restore evidence |

## Changed-line forecast and slicing

Forecast: approximately **500–800 changed lines** for the complete interactive flow, including tests and documentation; review-budget risk is high and chained PRs are appropriate. A safer forecasted slice is:

1. **Restore foundation (about 180–260 lines):** typed restore states/results, allowlisted `.env` parser, manifest/checksum gate, process/credential abstractions, unit tests.
2. **Compose control and restore executor (about 180–280 lines):** service-scoped stop/start, safe pg restore command construction, destructive sequencing, failure cleanup, unit tests.
3. **TUI/wiring (about 120–180 lines):** post-deploy insertion, wait screen, result semantics, dependency injection, transition tests/goldens.
4. **Documentation/integration (about 80–160 lines):** RUNBOOK/README, opt-in Linux integration coverage, platform notes.

Do not mix unrelated detection/menu refactors into this change. The existing `contextual-installation-menu` artifact explicitly says validated backup authorizes no later migration action; this change must be the new proposal/specification that changes that boundary deliberately.

## Open decisions for proposal

- Is restore enabled only from the interactive confirmed legacy migration route, or also from `--unattended`? The current code has no legacy backup wiring in `headless.Dependencies`, so interactive-only is the least surprising first slice.
- Is a pre-restore backup of the newly generated database mandatory? Without it, rollback is not data-safe after `DROP DATABASE`.
- What exact PostgreSQL 15/Timescale restore compatibility policy applies to legacy PostgreSQL 11 archives (`--no-owner`, ACLs, extensions, `--no-tablespaces`)?
- What is the accepted final integrity evidence (known schema/table set, row counts, application probe, or only connection/catalog checks)?
- What is the reviewed Windows behavior? Current host-network/helper assumptions and supported legacy flow are Linux-only; Windows should remain blocked rather than receive a best-effort implementation.

## Ready for proposal

Yes, with the above decisions recorded explicitly. The implementation insertion point and existing seams are concrete, but the destructive rollback boundary and restore compatibility policy must be resolved before specification/design.
