# installer-restart-command Specification

## Purpose

Define a dedicated `alice-installer restart` command that restarts an existing deployment in place using persisted compose artifacts, with exact `docker compose restart` semantics and no regressions in install, TUI, unattended, or update behavior.

## Requirements

### Requirement: Restart Subcommand Recognition

The installer CLI MUST recognize `alice-installer restart` as a dedicated command path and MUST route it before install-flow selection.

#### Scenario: Restart positional command is provided

- GIVEN the operator invokes `alice-installer restart`
- WHEN CLI arguments are parsed
- THEN the installer MUST enter the restart flow
- AND it MUST NOT start install bootstrap, wizard/TUI, unattended setup, or update flow logic

#### Scenario: Restart command is not provided

- GIVEN the operator invokes existing entrypoints (interactive install, `--unattended`, or `update`)
- WHEN CLI arguments are parsed
- THEN behavior MUST remain identical to current behavior for those paths

### Requirement: Exact Compose Restart Semantics

The restart flow MUST execute the equivalent of `docker compose restart` through the compose abstraction and MUST NOT substitute `down/up`, `pull/up`, or install fallback sequences.

#### Scenario: Restart execution succeeds

- GIVEN required artifacts resolve successfully
- WHEN restart executes compose actions
- THEN exactly one restart operation MUST be requested from the compose abstraction
- AND the command MUST report success only if restart completes successfully

#### Scenario: No semantic drift to other workflows

- GIVEN restart is invoked
- WHEN compose operations are selected
- THEN the flow MUST NOT call pull, up, down, install bootstrap, or artifact generation operations

### Requirement: Update-Consistent Artifact Resolution

The restart flow MUST resolve `docker-compose.yml`, `.env`, and optional GPU overlay artifacts using the same resolution contract as update, and MUST reuse persisted artifacts from prior installs.

#### Scenario: Persisted artifacts are present

- GIVEN a workspace with artifacts from a prior successful install
- WHEN restart performs preflight resolution
- THEN restart MUST target the same artifact set update would target in that workspace
- AND restart execution MUST remain non-interactive

#### Scenario: Required artifacts are missing or unresolvable

- GIVEN required compose/env artifacts are missing or cannot be resolved under workspace rules
- WHEN restart preflight validation runs
- THEN restart MUST fail fast with an actionable error identifying missing/invalid artifacts
- AND it MUST NOT attempt install fallback or artifact regeneration

### Requirement: Clean Failure Propagation

The restart flow MUST return non-zero exit status and human-readable errors when artifact preconditions fail or compose restart fails.

#### Scenario: Compose restart returns an error

- GIVEN artifacts resolve successfully
- WHEN compose restart returns an error
- THEN restart MUST exit non-zero and surface the compose failure
- AND it MUST NOT report partial or successful completion

### Requirement: Cross-Platform Behavioral Parity

Restart command behavior MUST be functionally equivalent on Linux (amd64/arm64) and Windows (amd64) for command recognition, artifact targeting, success signaling, and failure signaling.

#### Scenario: Supported platform execution

- GIVEN the command runs on a supported target platform
- WHEN restart succeeds or fails
- THEN observable CLI behavior MUST match this spec without platform-specific semantic divergence

### Requirement: Strict TDD Verification Contract

Implementation of this capability MUST be test-first and MUST include automated coverage for routing precedence, exact restart semantics, update-consistent artifact resolution, clean failure paths, cross-platform expectations, and non-regression of existing install/update flows.

#### Scenario: Mandatory automated coverage exists

- GIVEN tests for restart-related behavior
- WHEN the test suite runs
- THEN tests MUST include multi-case table-driven scenarios for routing and failure states
- AND tests MUST verify unchanged behavior for TUI, `--unattended`, and `update`

#### Scenario: Project TDD gate is satisfied

- GIVEN implementation is complete
- WHEN project verification runs
- THEN `go test ./...` MUST pass
- AND the change MUST satisfy strict TDD policy before merge
