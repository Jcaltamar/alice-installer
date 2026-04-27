# installer-update-command Specification

## Purpose

Define the CLI update behavior that refreshes an already-installed deployment using existing workspace compose artifacts, while preserving all current install/TUI/unattended behavior when `update` is not requested.

## Requirements

### Requirement: Update Subcommand Recognition

The installer CLI MUST recognize `alice-installer update` as a dedicated command path and MUST route it before normal install-flow selection.

#### Scenario: Update positional command is provided

- GIVEN the operator invokes `alice-installer update`
- WHEN CLI arguments are parsed
- THEN the installer MUST enter the update flow
- AND it MUST NOT start install bootstrap, wizard/TUI, or unattended install stages

#### Scenario: Update command is not provided

- GIVEN the operator invokes existing install entrypoints (interactive or `--unattended`)
- WHEN CLI arguments are parsed
- THEN behavior MUST remain identical to current install behavior

### Requirement: Deterministic Artifact Targeting

The update flow MUST resolve `docker-compose.yml` and env artifacts deterministically using the project's current workspace conventions, and MUST reuse persisted artifacts without regenerating them.

#### Scenario: Required persisted artifacts exist

- GIVEN the current workspace contains compose/env artifacts produced by a prior successful install
- WHEN update starts
- THEN the installer MUST target those exact artifacts according to existing workspace resolution rules
- AND update execution MUST be non-interactive

#### Scenario: Artifact resolution is ambiguous or missing

- GIVEN required compose/env artifacts cannot be resolved uniquely under workspace conventions
- WHEN update starts
- THEN the installer MUST fail fast with a clear actionable error
- AND it MUST NOT attempt install fallback or artifact regeneration

### Requirement: Pull-Before-Up Execution Order

The update flow MUST execute `docker compose pull` before `docker compose up -d`, through the existing compose abstraction contract.

#### Scenario: Compose update succeeds

- GIVEN artifacts resolve successfully
- WHEN update executes compose actions
- THEN pull MUST run first and up `-d` MUST run second
- AND the command MUST report success only if both complete successfully

### Requirement: Clean Failure Propagation

The update flow MUST return non-zero exit status and human-readable errors when preconditions or compose operations fail.

#### Scenario: Compose file or env artifact is missing

- GIVEN update is invoked but required compose/env artifacts are absent
- WHEN preflight validation runs
- THEN update MUST exit non-zero
- AND the error MUST identify which artifact is missing

#### Scenario: Docker compose pull or up fails

- GIVEN artifacts resolve successfully
- WHEN either pull or up returns an error
- THEN update MUST exit non-zero and surface the compose failure
- AND it MUST NOT report partial success

### Requirement: Strict TDD Verification Contract

Implementation of this capability MUST be test-first and MUST include automated coverage for routing, deterministic targeting, pull-before-up ordering, clean failure paths, and legacy install compatibility.

#### Scenario: New behavior coverage exists

- GIVEN tests for the update command suite
- WHEN test cases are executed
- THEN they MUST include multi-case table-driven coverage for success/error routing and ordering assertions
- AND they MUST verify unchanged behavior for TUI and unattended install invocations

#### Scenario: Red-green-refactor workflow enforcement

- GIVEN the change is implemented
- WHEN CI/local verification runs
- THEN `go test ./...` MUST pass
- AND the change MUST follow strict TDD expectations defined by project policy
