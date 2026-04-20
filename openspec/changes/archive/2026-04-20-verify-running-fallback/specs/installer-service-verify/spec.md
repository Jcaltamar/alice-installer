# installer-service-verify Specification

## Purpose

Define the acceptance rule the installer uses during the verify stage to decide whether the compose-managed services are "ready", and the diagnostic output the E2E harness MUST emit when they are not. Applies equally to the interactive (TUI) and unattended (`--unattended`) paths.

## Requirements

### Requirement: Service Health and State Capture

The installer MUST collect both the `Health` and `State` fields of every service when querying `docker compose ps --format json`, and MUST expose both to the verify loop.

#### Scenario: Both fields present in compose output

- GIVEN `docker compose ps --format json` returns a row with `Service=backend`, `State=running`, `Health=healthy`
- WHEN the installer parses the row
- THEN the `ServiceHealth` record for `backend` MUST contain `State="running"` AND `Status="healthy"`

#### Scenario: Health field absent for a service without healthcheck

- GIVEN the JSON row has `State=running` and no `Health` key (or `Health=""`)
- WHEN the installer parses the row
- THEN `ServiceHealth.Status` MUST be `""` (or exactly `"none"` if compose returned that)
- AND `ServiceHealth.State` MUST be `"running"`

#### Scenario: State field absent on older compose versions

- GIVEN the JSON row omits the `State` key
- WHEN the installer parses the row
- THEN `ServiceHealth.State` MUST default to `""`
- AND the acceptance rule MUST treat an empty State as "not OK" (conservative bias)

### Requirement: Service Acceptance Rule

The verify loop in BOTH the TUI and the `--unattended` path MUST accept a service as ready when either (a) its `Health` is `"healthy"`, or (b) its `Health` is empty or `"none"` AND its `State` is `"running"`. All other combinations MUST be treated as not yet ready.

#### Scenario: Healthy and running

- GIVEN a service reports `Health="healthy"` and `State="running"`
- WHEN the verify loop evaluates it
- THEN the service MUST be counted as ready

#### Scenario: No healthcheck but running

- GIVEN a service reports `Health=""` and `State="running"`
- WHEN the verify loop evaluates it
- THEN the service MUST be counted as ready

#### Scenario: No healthcheck and restarting (crash-loop)

- GIVEN a service reports `Health=""` and `State="restarting"`
- WHEN the verify loop evaluates it
- THEN the service MUST NOT be counted as ready
- AND the service MUST be reported in the unhealthy list with its State visible to the user

#### Scenario: Unhealthy but running

- GIVEN a service reports `Health="unhealthy"` and `State="running"`
- WHEN the verify loop evaluates it
- THEN the service MUST NOT be counted as ready

#### Scenario: Healthy but exited

- GIVEN a service reports `Health="healthy"` and `State="exited"`
- WHEN the verify loop evaluates it
- THEN the service MUST NOT be counted as ready
- AND the unhealthy message MUST include the exited State

#### Scenario: No healthcheck and state unknown

- GIVEN a service reports `Health=""` and `State=""`
- WHEN the verify loop evaluates it
- THEN the service MUST NOT be counted as ready

### Requirement: Backward Compatibility of Reported Identifiers

The `ServiceHealth` struct MUST remain backward-compatible with existing callers: the `Service` and `Status` fields MUST retain their prior names and semantics. `State` MUST be a newly added field, never renamed from an existing one.

#### Scenario: Pre-existing caller reads Service and Status only

- GIVEN a caller outside the verify loop that only reads `ServiceHealth.Service` and `ServiceHealth.Status`
- WHEN the installer upgrades to the new verify rule
- THEN that caller MUST continue to compile and MUST receive unchanged values

### Requirement: E2E Per-Service Log Dump on Timeout

The E2E harness MUST, when the FULL_DEPLOY pass exits non-zero, iterate over services that never reached the acceptance rule and print each one's `docker compose logs <service>` output, capped at 50 lines per service.

#### Scenario: FULL_DEPLOY pass times out with at least one unready service

- GIVEN FULL_DEPLOY=1 runs the installer
- AND the installer exits non-zero because one or more services did not reach readiness within the verify timeout
- WHEN the harness's diagnostic step runs
- THEN for EACH service that did NOT satisfy the acceptance rule, the harness MUST print a labeled section containing up to 50 lines of that service's container logs
- AND the harness MUST use the same compose files and env file that the installer wrote, so the services are addressable

#### Scenario: FULL_DEPLOY pass succeeds

- GIVEN FULL_DEPLOY=1 runs the installer
- AND every service reaches the acceptance rule
- WHEN the harness finishes
- THEN no per-service log dump MUST be printed
- AND the success assertions MUST run as today

### Requirement: Verify View Surfaces State When It Disagrees With Health

The interactive (TUI) verify screen MUST render each service's `State` in addition to its `Health` whenever the two would lead to different acceptance decisions, so users can distinguish "running without healthcheck" from "crash-looping".

#### Scenario: Mixed statuses shown in the live view

- GIVEN services with combinations of `{healthy+running, ""+running, ""+restarting}`
- WHEN the TUI renders the verify screen
- THEN the first service MUST show as healthy
- AND the second MUST show as ready with a visible `running` indicator
- AND the third MUST show as not-ready with a visible `restarting` indicator
