# Tasks: Installer Update Command

## Phase 1: CLI Mode Parsing & Routing

- [x] 1.1 **RED** Add table-driven failing cases in `cmd/installer/main_test.go` for `update` positional parsing (`update --workspace-dir`, `--workspace-dir ... update`, duplicate/unknown positional) and unchanged legacy install/TUI/`--unattended` behavior.
- [x] 1.2 **GREEN** Modify `cmd/installer/main.go` to add `parseCLI(args)` that strips one bare `update` token, preserves root flag parsing, and routes update mode before install flow selection.
- [x] 1.3 **REFACTOR** Clean CLI mode helpers in `cmd/installer/main.go`/`cmd/installer/main_test.go` (names, small functions, no behavior change) while keeping tests green.

## Phase 2: Update Orchestration Core

- [x] 2.1 **RED** Create `internal/update/run_test.go` with table-driven failing tests for required artifact checks (`.env`, `docker-compose.yml`), non-interactive execution, and fail-fast errors when artifacts are missing/ambiguous.
- [x] 2.2 **GREEN** Create `internal/update/run.go` with `Run(ctx, cfg, deps, out)` and artifact resolver based on `flags.WorkspaceDir`, returning actionable errors without install fallback/regeneration.
- [x] 2.3 **RED** Extend `internal/update/run_test.go` with failing cases for pull-before-up ordering, no `Up` after `Pull` failure, and direct propagation of `Up` failures.
- [x] 2.4 **GREEN** Implement compose execution in `internal/update/run.go` using `ComposeRunner.Pull` then `ComposeRunner.Up` only; stream progress with `[pull]`/`[deploy]` prefixes and propagate non-zero failures.
- [x] 2.5 **RED** Add failing tests in `internal/update/run_test.go` for optional `docker-compose.gpu.yml` inclusion only when file exists and GPU detection is currently true.
- [x] 2.6 **GREEN** Implement optional GPU overlay selection in `internal/update/run.go` and keep deterministic base file ordering.

## Phase 3: Compose Test Seam & Wiring

- [x] 3.1 **RED** Add failing assertions in `internal/update/run_test.go` and `cmd/installer/main_test.go` that require recording of Pull/Up call order and arguments from the compose fake.
- [x] 3.2 **GREEN** Modify `internal/compose/fake.go` to record Pull/Up invocations (files/env/order/errors) for orchestration assertions.
- [x] 3.3 **RED** Add failing CLI wiring tests in `cmd/installer/main_test.go` asserting update mode invokes `internal/update` path and bypasses TUI/headless install stages.
- [x] 3.4 **GREEN** Finalize routing glue in `cmd/installer/main.go` (after stale-group gate) to invoke `update.Run` with injected dependencies.

## Phase 4: Docs & Verification

- [x] 4.1 Update `README.md` with `alice-installer update` usage, workspace artifact expectations, and non-interactive semantics.
- [x] 4.2 Update `RUNBOOK.md` upgrade guidance to in-place `update` flow (no env rewrite/reinstall path).
- [x] 4.3 Run `go test ./...` to verify all RED→GREEN task outcomes and legacy compatibility scenarios pass.
