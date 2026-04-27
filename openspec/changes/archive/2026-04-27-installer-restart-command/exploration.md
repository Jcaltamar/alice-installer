## Exploration: installer-restart-command

### Current State
`alice-installer` already supports a positional `update` command via `parseCLI` in `cmd/installer/main.go`, and routes that mode before TUI/headless install paths. The update flow is implemented in `internal/update.Run`, which resolves persisted artifacts from `--workspace-dir` (`.env` + `docker-compose.yml`, optional `docker-compose.gpu.yml`) and executes `docker compose pull` then `docker compose up -d` through `compose.ComposeRunner`.

There is currently no `restart` command path. `parseCLI` only recognizes `update`, and `runWithStaleCheck` only dispatches `modeUpdate` vs install modes. The compose abstraction (`internal/compose`) also has no `Restart` method; it supports `Pull`, `Up`, `Down`, and `HealthStatus`.

Given the confirmed semantics (`restart = docker compose restart`) plus the requirement to reuse the same artifact resolution pattern as `update`, the closest baseline is to reuse update-style workspace artifact resolution and add a restart-specific execution step.

### Affected Areas
- `cmd/installer/main.go` — extend CLI mode parsing/routing to recognize `restart` and dispatch to a non-install path before unattended/TUI branches.
- `cmd/installer/main_test.go` — table-driven parser tests and routing tests for `restart` (including duplicate/unknown positional handling and non-zero failure behavior).
- `internal/update/run.go` — source of truth for artifact resolution contract to be reused by restart; likely requires extracting/exporting resolver logic to avoid duplication.
- `internal/update/run_test.go` — reference test patterns proving deterministic artifact targeting and optional GPU overlay behavior.
- `internal/compose/runner.go` — if implementing true `docker compose restart`, `ComposeRunner` needs a `Restart` contract and `CLICompose` implementation.
- `internal/compose/fake.go` (+ related tests) — extend fake runner call recording to support restart assertions and maintain interface conformance.
- `README.md` and `RUNBOOK.md` — document `alice-installer restart` usage and clarify it reuses persisted workspace artifacts.

### Approaches
1. **Add a dedicated restart command path + compose Restart API (recommended)** — add `modeRestart`, route `alice-installer restart`, reuse/extract update artifact resolver, and invoke `ComposeRunner.Restart` implemented as `docker compose restart`.
   - Pros: Exact semantic match to user-confirmed behavior; keeps compose calls behind one abstraction; highly testable with existing fake pattern.
   - Cons: Requires interface expansion (`ComposeRunner`) and touching fake/tests across packages.
   - Effort: Medium

2. **Implement restart by composing existing Down/Up or Pull/Up operations** — avoid interface expansion by reusing current methods.
   - Pros: Smaller immediate code surface.
   - Cons: Violates confirmed semantics (`restart` is not `down+up` and not `pull+up`), changing container lifecycle and potentially behavior/state; higher product risk.
   - Effort: Low/Medium

3. **Inline raw `docker compose restart` in CLI/update package** — use command runner directly without extending compose abstraction.
   - Pros: Fastest path to command execution.
   - Cons: Duplicates compose execution patterns, bypasses existing fake seams/error conventions, increases drift risk.
   - Effort: Low

### Recommendation
Choose **Approach 1**: implement `alice-installer restart` as a first-class CLI mode and add a `ComposeRunner.Restart` method that executes `docker compose restart`. Reuse the same artifact resolution contract as update by extracting update’s resolver into a shared helper (or shared internal package) consumed by both update and restart flows, so workspace targeting rules remain identical and single-sourced.

### Risks
- **Parser regressions**: extending positional parsing from one command (`update`) to two (`update`, `restart`) can break flag normalization if token handling is not table-tested.
- **Contract drift**: duplicating artifact resolution logic can cause update/restart to target different files over time; extraction is safer than copy-paste.
- **Interface blast radius**: expanding `ComposeRunner` requires fake and implementation updates; missing one will break build/tests widely.
- **Semantic mismatch risk**: implementing restart via other compose operations would violate explicit user semantics and could alter runtime behavior.

### Ready for Proposal
Yes — scope, affected modules, and execution semantics are clear. Proposal should lock two decisions: (1) shared artifact resolver strategy (extract vs duplicate), and (2) compose API shape for `Restart` with test-first coverage (`go test ./...`) under strict TDD.
