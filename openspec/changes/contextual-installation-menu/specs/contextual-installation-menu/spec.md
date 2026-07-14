# Contextual Installation Menu Specification

## Purpose

Provide a safe contextual installation-state probe and interactive menu after the splash screen, while preserving explicit CLI and non-interactive routes. The first slice MUST select only actions supported by reliable evidence. Migration Step 1 MAY execute only a confirmation-driven, validated PostgreSQL backup; it MUST NOT perform destructive migration or Uninstall operations.

## Requirements

### Requirement: Typed Installation-State Detection

The installer MUST classify the host into exactly one typed state: `not-installed`, `current`, `legacy-pm2`, `conflict`, or `unknown`. Detection MUST be side-effect-free and MUST be separate from installation preflight.

#### Scenario: No installation evidence is present

- GIVEN current-installation artifacts are absent and the legacy probe returns no Alice-specific evidence
- WHEN detection runs
- THEN the result MUST be `not-installed`
- AND the result MUST include safe evidence categories sufficient for the menu and display

#### Scenario: A valid current installation is present

- GIVEN the complete validated current artifact set is present
- AND no Alice-specific legacy PM2 evidence is present
- WHEN detection runs
- THEN the result MUST be `current`
- AND stopped service health MUST NOT change this classification

#### Scenario: A legacy Alice PM2 installation is confidently identified

- GIVEN current artifacts are not detected
- AND the Linux PM2 probe returns configured Alice-specific evidence
- WHEN detection runs
- THEN the result MUST be `legacy-pm2`

#### Scenario: Current and legacy evidence coexist

- GIVEN valid current artifacts are detected
- AND Alice-specific legacy PM2 evidence is detected
- WHEN detection runs
- THEN the result MUST be `conflict`
- AND the result MUST retain safe evidence categories for both installations

#### Scenario: Detection cannot classify reliably

- GIVEN artifacts are partial, unreadable, malformed, wrongly typed, ambiguous, or otherwise inconclusive
- OR the legacy probe fails in a way that prevents reliable classification
- WHEN detection runs
- THEN the result MUST be `unknown`
- AND the installer MUST NOT downgrade the result to `not-installed`

### Requirement: Current Artifact Detection Contract

Current-installation detection MUST use one workspace source of truth and MUST validate the expected regular-file artifact set, including the configured `--workspace-dir` override. Detection MUST NOT execute commands, mutate files, regenerate artifacts, or infer installation state from runtime health.

#### Scenario: Explicit workspace directory contains the validated artifact set

- GIVEN `--workspace-dir` points to a directory containing every required current artifact
- AND each required artifact is readable, well-formed, and a regular file
- WHEN the current-artifact probe runs
- THEN it MUST report current-installation evidence
- AND it MUST use that directory rather than an unrelated default directory

#### Scenario: Default workspace contains the validated artifact set

- GIVEN no workspace override is supplied
- AND the repository's established workspace resolution selects a directory containing the complete valid artifact set
- WHEN the current-artifact probe runs
- THEN it MUST report current-installation evidence from that selected workspace

#### Scenario: Artifact set is partial or invalid

- GIVEN one or more required artifacts are missing, unreadable, malformed, or not regular files
- WHEN the current-artifact probe runs
- THEN it MUST report inconclusive current-installation evidence
- AND classification MUST be `unknown` when the evidence prevents a reliable decision
- AND Install MUST NOT be enabled

#### Scenario: Artifact contents include secrets

- GIVEN detection evidence is rendered or logged
- WHEN environment or process-related artifacts are summarized
- THEN the output MUST expose only safe categories and sanitized paths
- AND it MUST NOT expose environment values, credentials, tokens, or secret process metadata

### Requirement: Confirmed Legacy Directory Contract

On Linux amd64 and arm64, the detector MUST treat the exact directory `/opt/backend_alice_guardian/node` as confirmed Alice-specific legacy evidence without requiring PM2 access. The detector MUST use only this policy-owned exact path: it MUST NOT match a parent directory, path prefix, or user-configured arbitrary path. A missing path MUST provide no legacy directory evidence; a non-directory path or any permission/stat error MUST be uncertain. Unsupported platforms MUST NOT inspect this Unix path.

#### Scenario: Confirmed legacy directory exists

- GIVEN `/opt/backend_alice_guardian/node` exists and is a directory on a supported Linux platform
- WHEN legacy detection runs
- THEN it MUST return confirmed Alice legacy evidence
- AND it MUST NOT require or invoke PM2

#### Scenario: Confirmed legacy path is absent, invalid, or unreadable

- GIVEN the exact path is missing, is not a directory, or cannot be inspected
- WHEN legacy detection runs
- THEN missing MUST allow the optional PM2 fallback
- AND wrong type or stat failure MUST return uncertain without PM2 overriding that uncertainty

### Requirement: Linux PM2 Probe Contract

The legacy PM2 probe MUST remain injectable as optional corroboration or fallback and MUST be supported first on Linux amd64 and arm64. It MUST require Alice-specific evidence from the configured detection policy, such as an exact configured process identifier or validated script, working-directory, ecosystem, or deployment-path evidence. Generic PM2 presence MUST NOT qualify as legacy evidence.

#### Scenario: Linux reports a configured Alice identifier

- GIVEN the target platform is Linux amd64 or arm64
- AND the injected PM2 command result contains an exact identifier configured by policy
- WHEN the PM2 probe runs
- THEN it MUST return confirmed Alice legacy evidence

#### Scenario: Linux reports an unrelated PM2 application

- GIVEN the target platform is Linux amd64 or arm64
- AND PM2 output contains processes but none match configured Alice-specific evidence
- WHEN the PM2 probe runs
- THEN it MUST return no legacy evidence
- AND classification MUST NOT become `legacy-pm2`

#### Scenario: PM2 is absent

- GIVEN the target platform is Linux amd64 or arm64
- AND the PM2 command is unavailable
- WHEN the PM2 probe runs
- THEN it MUST return no legacy evidence
- AND current-artifact evidence alone MUST determine whether the state is `not-installed` or `current`

#### Scenario: PM2 output or execution is unreliable

- GIVEN PM2 execution returns a permission error, non-zero failure, malformed output, or inconclusive Alice ownership
- WHEN the PM2 probe runs
- THEN it MUST return an explicit probe-error or inconclusive result
- AND classification MUST be `unknown` when no reliable classification remains

#### Scenario: Legacy probing is unsupported

- GIVEN the target platform is Windows amd64 or another platform outside Linux amd64 and arm64 support
- WHEN the legacy probe runs
- THEN it MUST return an explicit unsupported result
- AND it MUST NOT infer legacy evidence from generic processes, files, or PM2-like output
- AND the menu MUST not claim that legacy support was positively detected

#### Scenario: Detection policy is configured narrowly

- GIVEN legacy identifiers, the exact confirmed legacy directory, and deployment-path hints are supplied through the centralized detection policy
- WHEN the probe evaluates evidence
- THEN it MUST use those exact configured values
- AND default policy values MUST NOT guess broad identifiers that could match unrelated workloads

### Requirement: Contextual Action Matrix

The interactive menu MUST derive available actions exclusively from the typed detection state and operation availability. The menu MUST show concise evidence and MUST never offer a destructive action for `unknown` or `conflict`.

#### Scenario: Action matrix for no installation

- GIVEN detection returns `not-installed`
- WHEN the contextual menu is rendered
- THEN it MUST offer Install
- AND it MUST NOT offer Update, Uninstall, or Migration

#### Scenario: Action matrix for current installation

- GIVEN detection returns `current`
- WHEN the contextual menu is rendered
- THEN it MUST offer Update
- AND it MUST offer Uninstall only when an explicitly injected Uninstall operation is available and safe to enter
- AND it MUST NOT offer Migration

#### Scenario: Action matrix for legacy installation

- GIVEN detection returns `legacy-pm2`
- WHEN the contextual menu is rendered
- THEN it MUST offer Migration only when an explicitly injected Migration operation is available and safe to enter
- AND it MUST NOT offer Install, Update, or Uninstall

#### Scenario: Action matrix for conflict or unknown

- GIVEN detection returns `conflict` or `unknown`
- WHEN the contextual menu is rendered
- THEN it MUST offer no Install, Update, Uninstall, or Migration action
- AND it MUST show the evidence category, safety explanation, and remediation guidance
- AND it MUST allow safe exit without changes

### Requirement: Post-Splash Transition

Interactive mode MUST perform detection after the splash screen and before entering installation preflight. Detection failures MUST be represented by typed results rather than being routed as a clean install.

#### Scenario: Interactive session leaves the splash

- GIVEN the interactive installer has completed the splash screen
- WHEN the splash transition occurs
- THEN the installer MUST invoke the injected detector
- AND it MUST render the contextual menu or safe blocked state from the detector result
- AND it MUST NOT enter install preflight before the user selects Install

#### Scenario: Detection is cancelled or the user quits

- GIVEN detection or the contextual menu is active
- WHEN the user presses Escape, cancels, or quits
- THEN the installer MUST exit without issuing commands
- AND it MUST not mutate files, processes, or persistent installation state

### Requirement: Install Routing Preserves Existing Flow

When the contextual menu offers Install and the user selects it, the installer MUST route to the existing installation preflight and installation flow without changing its established behavior.

#### Scenario: Install is selected from `not-installed`

- GIVEN detection returns `not-installed`
- AND the menu offers Install
- WHEN the user selects Install
- THEN the installer MUST transition to the existing installation preflight
- AND it MUST preserve existing workspace, compose, environment, cancellation, and error contracts
- AND it MUST not run Uninstall, Migration, or update operations

#### Scenario: Install is selected from uncertain evidence

- GIVEN detection returns `unknown`, `conflict`, or any state with stale or ambiguous evidence
- WHEN the user attempts to continue
- THEN Install MUST not be available
- AND no installation command or filesystem mutation MUST occur

### Requirement: Update Routing Preserves Explicit and Existing Behavior

The contextual menu MUST route Update through the established update behavior or a thin adapter that preserves its contract. Explicit CLI routes MUST remain unchanged, including `alice-installer update`, restart, unattended, and dry-run behavior.

#### Scenario: Update is selected for a current installation

- GIVEN detection returns `current`
- AND Update is available
- WHEN the user selects Update
- THEN the installer MUST invoke the established update behavior
- AND it MUST preserve deterministic artifact targeting and the existing `docker compose pull` before `docker compose up -d` order
- AND it MUST report failures without falling back to Install

#### Scenario: Explicit update CLI route is invoked

- GIVEN the operator invokes `alice-installer update`
- WHEN CLI arguments are parsed
- THEN the installer MUST enter the existing non-interactive update flow
- AND it MUST NOT start the splash, contextual menu, install bootstrap, or interactive preflight

#### Scenario: Existing non-interactive routes are invoked

- GIVEN the operator invokes restart, unattended, or dry-run routes
- WHEN the command is executed
- THEN behavior MUST remain unchanged by contextual menu detection
- AND the contextual menu MUST not be entered

### Requirement: Informational Blocked Destinations

The first slice MAY display Uninstall for a current installation and Migration for a legacy installation only when each destination is explicitly available and safe to enter. If its execution contract is not implemented, selecting the destination MUST show an informational blocked screen and MUST NOT execute changes.

#### Scenario: Uninstall is unavailable

- GIVEN detection returns `current`
- AND no safe Uninstall operation is injected
- WHEN the contextual menu is rendered
- THEN Uninstall MUST be hidden or visibly marked unavailable
- AND selecting another available action MUST remain safe

#### Scenario: Uninstall is informationally blocked

- GIVEN detection returns `current`
- AND Uninstall is exposed as an informational destination without an approved execution contract
- WHEN the user selects Uninstall
- THEN the installer MUST show why the operation is blocked
- AND it MUST provide safe exit guidance
- AND it MUST issue no process, shell, or filesystem mutation

#### Scenario: Migration has no safe backup operation

- GIVEN detection returns `legacy-pm2`
- AND Migration Step 1 is unavailable or its preconditions are not satisfied
- WHEN the user selects Migration
- THEN the installer MUST show an informational blocked result and safe exit guidance
- AND it MUST issue no process, shell, filesystem, database, or lifecycle mutation

#### Scenario: Migration Step 1 is selected

- GIVEN detection returns `legacy-pm2`
- AND the injected backup operation is available and safe to enter
- WHEN the user selects Migration
- THEN the installer MUST enter the backup review and confirmation flow
- AND it MUST not proceed to any later migration step automatically

#### Scenario: Destructive operation contracts are absent

- GIVEN ownership, path safety, affected resources, confirmation, backup/recovery, idempotency, partial-failure, compatibility, or rollback semantics for later operations are not separately specified and implemented
- WHEN the contextual menu or a completed backup is used
- THEN the TUI and migration operation MUST NOT delete files or volumes, stop or restart PM2 or containers, alter source files, mutate a target database, or execute later migration steps

### Requirement: Migration Step 1 Static Legacy Configuration Resolution

Migration Step 1 MUST resolve effective PostgreSQL settings from the exact regular file `/opt/backend_alice_guardian/node/config/config.js` without importing, requiring, transpiling, executing, or otherwise evaluating JavaScript. The resolver MUST use a closed, documented grammar limited to object/property syntax, approved literals, and recognized expressions of the form `process.env.NAME || literal` or `process.env.NAME ?? literal`. It MUST return redacted typed settings only.

#### Scenario: Supported production configuration is resolved safely

- GIVEN the exact configuration file is readable and contains a supported production object with PostgreSQL settings
- WHEN Step 1 resolves configuration
- THEN it MUST select `production` unless an explicit migration configuration selects another supported environment
- AND it MUST require PostgreSQL dialect, database, user, and a resolvable host
- AND an absent production port MUST resolve to PostgreSQL port `5432`
- AND it MUST not execute any JavaScript or expose configuration contents

#### Scenario: Environment override precedence is applied deterministically

- GIVEN a selected configuration property contains a literal and/or an approved `process.env.NAME` fallback
- WHEN Step 1 resolves that property
- THEN the selected environment's explicit value MUST take precedence over a referenced host environment value
- AND a referenced host environment value MUST take precedence over the documented development fallback
- AND arbitrary values from other environments MUST NOT be merged into the selected environment
- AND the selected environment MUST be explicit when `test` or `development` is used; Step 1 MUST never silently select `test`

#### Scenario: Unsupported or ambiguous configuration fails closed

- GIVEN the file is missing, not regular, unreadable, malformed, dynamically constructed, uses unsupported syntax, has an ambiguous environment selection, has unresolved required environment values, has invalid types, or has a non-PostgreSQL dialect
- WHEN Step 1 resolves configuration
- THEN it MUST return a redacted configuration error before container execution
- AND it MUST not guess credentials, host, port, database, or user
- AND the result MUST not expose passwords, tokens, configuration contents, or secret-bearing values

#### Scenario: Resolved secrets remain contained

- GIVEN a password is required to connect
- WHEN Step 1 prepares the dump process
- THEN the password MUST remain only in memory long enough to establish the child process environment
- AND it MUST not appear in argv, evidence, errors, UI, manifests, logs, test fixtures, or persisted artifacts
- AND a missing required password MUST fail without revealing the attempted value

### Requirement: Migration Step 1 Exact Legacy PostgreSQL Container Identity

Step 1 MUST identify exactly one usable legacy PostgreSQL container by inspecting candidates, including stopped containers, filtered initially by the exact normalized image reference `bitnami/postgresql:11-debian-10`. Image matching alone MUST NOT establish identity. Candidate correlation MUST use non-secret metadata and the resolved configuration, including available immutable container ID, exact image identity or digest, database/user, endpoint or mapped-port evidence, and relevant mounts, labels, network, or health metadata.

#### Scenario: Exactly one corroborated running container is selected

- GIVEN one candidate has the exact image identity and sufficient non-secret corroborating evidence
- AND its immutable container ID is available
- AND it is running and usable
- WHEN container discovery completes
- THEN Step 1 MUST select that exact container ID
- AND it MUST not select by list order, name order, creation time, or broad PostgreSQL image matching

#### Scenario: Container identity is absent or ambiguous

- GIVEN there are zero candidates, multiple plausible candidates, conflicting endpoint/configuration evidence, an image alias without sufficient corroboration, malformed inspect output, an inspect failure, or incomplete identity evidence
- WHEN container discovery completes
- THEN Step 1 MUST return a precondition or ambiguous-container result
- AND it MUST not execute a dump or choose a candidate automatically

#### Scenario: Stopped or unhealthy container is found

- GIVEN the selected candidate is stopped, or has a declared healthcheck that is not healthy
- WHEN Step 1 evaluates container readiness
- THEN it MUST stop with an actionable precondition result
- AND it MUST not start, stop, restart, or otherwise mutate the container
- AND a running candidate without a healthcheck MAY proceed only when the dump connection itself succeeds

#### Scenario: Docker access is unreliable

- GIVEN the Docker daemon is unavailable or permission is denied
- WHEN Step 1 performs discovery
- THEN it MUST report an infrastructure/precondition failure rather than `no legacy database`
- AND it MUST not execute lifecycle or database operations

### Requirement: Migration Step 1 Streaming PostgreSQL 11 Custom Backup

Slice 4.3 MUST use a separate PostgreSQL 11-compatible helper container, not `docker exec` or `docker cp` against the selected legacy container. It MUST invoke direct `docker run --rm --pull=never` argv with only the exact reviewed `bitnami/postgresql:11-debian-10` image identity, a random collision-resistant name, and operation labels. On Linux it MUST use `--network host` and the already resolved legacy host/port without translation or guessing. A host-created unique `0700` temporary directory containing a `0600` `.pgpass` file MUST be bind-mounted read-only at the fixed `/run/alice-installer/pgpass` path. Docker argv/environment MUST contain only the non-secret `PGPASSFILE` path, never a password. The boundary MUST use no `sh -c`, shell interpolation, unpinned host client, or password in argv; it MUST stream binary stdout, bound and classify stderr without returning it raw, and use cancellation/timeout process-group termination. `--rm` plus direct named cleanup (`docker rm --force`) MUST be available for client-recovery reconciliation. The helper MUST NOT mutate, start, stop, restart, exec into, or write to the legacy container. Slice 4.3 alone MUST NOT orchestrate a dump, write backup artifacts, validate an archive, or wire TUI behavior.

#### Scenario: Helper process boundary is proven without exposing secrets

- GIVEN the selected immutable legacy identity and resolved endpoint are available
- WHEN Slice 4.3 constructs the PostgreSQL 11 helper process
- THEN it MUST use exact pinned helper-image direct Docker argv, Linux host networking, a read-only protected `.pgpass` bind mount, and only `PGPASSFILE=/run/alice-installer/pgpass`
- AND synthetic-secret scans MUST prove that argv, observable environment metadata, errors, stderr classification, cleanup metadata, and fake-harness records contain no password
- AND cancellation/timeout MUST terminate the helper process group, while `--rm` and name-based cleanup provide reconciliation evidence
- AND no legacy-container lifecycle or filesystem command MAY be constructed

#### Scenario: Dump streams without exposing secrets

- GIVEN configuration and container identity have passed validation
- AND the operator has confirmed the backup destination
- WHEN Step 1 runs `pg_dump`
- THEN it MUST request `--format=custom` and stream binary stdout without buffering the complete dump in memory
- AND it MUST use the immutable selected container ID
- AND it MUST pass the password only through a restrictive temporary `PGPASSFILE` or an equally protected child environment mechanism
- AND stderr MUST be captured only for redacted classification, never shown raw

#### Scenario: Dump fails, times out, or is cancelled

- GIVEN `pg_dump` exits unsuccessfully, its stream is interrupted, the timeout expires, or the operator cancels
- WHEN Step 1 handles the result
- THEN it MUST terminate the child process, remove the partial file, and report no validated backup
- AND it MUST not continue to validation, publication, or later migration

### Requirement: Migration Step 1 Protected Atomic Backup Publication

Step 1 MUST require an explicit operator-selected destination or a documented protected default outside `/opt/backend_alice_guardian/node` and other migration source paths. It MUST reject unsafe or unverifiable symlinked destination components, create new directories restrictively, and use a per-installation/destination operation lock. The temporary dump and manifest MUST be uniquely named, created with exclusive creation and mode `0600`, and published only through validated atomic rename with file and directory synchronization where supported. Existing final backups MUST never be replaced.

#### Scenario: Protected destination and partial cleanup are enforced

- GIVEN the destination is safe and has sufficient conservative free space
- WHEN Step 1 starts a backup
- THEN it MUST create the destination only after confirmation
- AND it MUST stream into a unique mode-`0600` temporary file
- AND any write-full error, checksum failure, validation failure, cancellation, timeout, or publication failure MUST close and remove temporary artifacts
- AND it MUST retain no misleading completed dump or unpaired manifest

#### Scenario: Destination is unsafe or concurrently active

- GIVEN the destination is unsafe, inside a source tree, insufficiently writable, lacks required space, or already has an active matching operation lock
- WHEN Step 1 preflight runs
- THEN it MUST return a destination/storage precondition failure before `pg_dump`
- AND it MUST not delete existing files or start a second matching backup

### Requirement: Migration Step 1 Validation, Manifest, and Fail-Closed Sequencing

A successful process exit MUST NOT be sufficient for success. Before making a backup visible, Step 1 MUST validate the temporary custom-format dump with a PostgreSQL 11-compatible `pg_restore --list` client, require non-empty structurally valid listing output, compute SHA-256 and byte size, and publish a protected atomic manifest. Validation MUST not connect to or mutate any database. Only a result classified as `validated backup` MAY unlock a later separately specified migration step.

#### Scenario: Validated backup is published

- GIVEN the streamed dump completed successfully
- AND pinned PostgreSQL 11-compatible `pg_restore --list` succeeds with non-empty valid output
- WHEN checksum, size, rename, synchronization, and manifest publication complete
- THEN Step 1 MUST publish the protected dump and paired mode-`0600` manifest
- AND the manifest MUST contain only schema/version, UTC timestamp, safe source/container identity, exact image reference or digest, selected environment label, non-secret database/user/host/port, custom format, byte size, SHA-256, client/validation version, and redacted tool outcome
- AND it MUST omit passwords, configuration contents, environment maps, raw stderr, and secret-bearing paths

#### Scenario: Validation or manifest publication fails

- GIVEN `pg_restore --list` fails or is unpinned, listing output is empty/invalid, checksum computation fails, atomic rename/synchronization fails, or the manifest cannot be paired safely
- WHEN Step 1 completes
- THEN it MUST remove or unpublish the dump and manifest so no completed artifact is implied
- AND it MUST return a validation or destination/storage failure
- AND it MUST not unlock later migration

#### Scenario: Fail-closed sequencing is preserved

- GIVEN any configuration, identity, readiness, destination, dump, cancellation, timeout, validation, checksum, or manifest precondition fails
- WHEN Step 1 reports its result
- THEN it MUST distinguish the applicable cancelled, precondition, ambiguous-container, config, dump, validation, or destination/storage outcome
- AND it MUST not stop PM2, stop or mutate any container, alter volumes or source files, restore data, mutate a target database, or execute later migration steps

### Requirement: Migration Step 1 Confirmation, Progress, and Result UI

The interactive Migration flow MUST show a redacted review before backup, require explicit confirmation, expose cancellable progress, and render a terminal result. UI text MUST contain only safe configuration/container summaries and operation status; it MUST never show credentials, raw command output, config contents, or secret-bearing paths.

#### Scenario: Operator confirms and observes progress

- GIVEN a uniquely identified ready container, supported configuration, and safe destination have passed preflight
- WHEN the operator reviews and confirms the backup
- THEN the UI MUST show the selected environment, non-secret database endpoint summary, exact image identity, protected destination summary, and progress through dump and validation
- AND it MUST allow cancellation without entering a later migration phase

#### Scenario: Operator cancels or receives a result

- GIVEN the review, running backup, or result screen is active
- WHEN the operator cancels, quits, or the operation succeeds or fails
- THEN the UI MUST show a safe cancelled, validated-backup, or redacted failure result
- AND cancellation or failure MUST issue no PM2 stop and no migration mutation
- AND success MUST show the manifest summary and a blocked/next-step screen rather than automatically continuing

### Requirement: Strict TDD and Cross-Platform Verification

Implementation MUST follow strict RED-GREEN-REFACTOR TDD and MUST include automated tests for every detection state, probe outcome, action matrix, post-splash transition, routing contract, blocked destination, cancellation path, and target-platform distinction. Tests MUST use injected command/process boundaries and temporary filesystem fixtures rather than real user homes or uncontrolled processes.

#### Scenario: Unit behavior is tested deterministically

- GIVEN detector, classifier, menu model, and routing tests
- WHEN `go test ./...` runs
- THEN tests MUST cover table-driven success, error, partial, conflict, unsupported-platform, and cancellation cases
- AND tests MUST assert state transitions, action availability, command ordering, and absence of side effects

#### Scenario: PM2 integration behavior is isolated

- GIVEN tests exercise an external PM2 command boundary
- WHEN tests run in short mode
- THEN external-command integration tests MUST be skippable
- AND unit tests MUST use small fakes or interfaces for PM2 execution

#### Scenario: Target-platform behavior is verified

- GIVEN the supported test matrix includes Linux amd64, Linux arm64, and Windows amd64
- WHEN platform-specific detection tests run
- THEN Linux supported targets MUST exercise the injectable PM2 contract
- AND Windows and other unsupported targets MUST assert explicit unsupported behavior and no legacy false positive

#### Scenario: Required quality gates run

- GIVEN the change is implemented
- WHEN verification is performed
- THEN `go test ./...` MUST pass
- AND `go build ./...` MUST pass
- AND coverage SHOULD meet the configured 80 percent threshold
