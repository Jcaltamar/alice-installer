# Tasks: Installer Restart Command

## Phase 1: CLI Parsing & Restart Routing

- [x] 1.1 **RED** Add table-driven failing cases in `cmd/installer/main_test.go` for `restart` parsing (`restart --workspace-dir`, `--workspace-dir ... restart`, duplicate/mixed `update`+`restart`, root-flag values like `--workspace-name restart`).
- [x] 1.2 **GREEN** Modify `cmd/installer/main.go` `parseCLI(args)` to normalize one positional command from `{update,restart}` while preserving existing root-flag parsing and unknown positional errors.
- [x] 1.3 **REFACTOR** Clean command-mode helpers in `cmd/installer/main.go`/`cmd/installer/main_test.go` without behavior changes.
- [x] 1.4 **RED** Add failing routing tests in `cmd/installer/main_test.go` asserting restart mode runs `runRestartFn`, bypasses install/TUI/`--unattended`/update branches, and returns non-zero on restart errors.
- [x] 1.5 **GREEN** Add `modeRestart` + `runRestartFn` wiring in `cmd/installer/main.go`, dispatched after stale-group gating and before install-flow selection.

## Phase 2: Shared Artifact Resolution Contract

- [x] 2.1 **RED** Create table-driven failing tests in `internal/workspace/artifacts_test.go` for required `.env` + `docker-compose.yml`, optional `docker-compose.gpu.yml`, deterministic file ordering, and actionable errors for missing/invalid workspace paths.
- [x] 2.2 **GREEN** Create `internal/workspace/artifacts.go` with shared `ResolveArtifacts(workspaceDir)` and `ComposeFiles(ctx, gpuDetector, resolved)` used by update/restart.
- [x] 2.3 **REFACTOR** Keep helpers small and cross-platform-safe (`filepath.Join` only), centralizing artifact path rules in `internal/workspace/artifacts.go`.
- [x] 2.4 **RED** Extend `internal/update/run_test.go` with failing regression cases proving update behavior stays unchanged while sourcing artifacts through shared workspace helpers.
- [x] 2.5 **GREEN** Modify `internal/update/run.go` to replace private artifact resolution with `internal/workspace` helpers, preserving pull→up behavior and current progress/error semantics.

## Phase 3: Compose Restart API & Restart Orchestration

- [x] 3.1 **RED** Add failing tests in `internal/compose/runner_test.go` and `internal/compose/invocation_test.go` that require exact `docker compose ... restart` invocation and error propagation.
- [x] 3.2 **GREEN** Extend `internal/compose/runner.go` `ComposeRunner` and `CLICompose` with `Restart(ctx, files, envFile) error` using exact compose restart semantics.
- [x] 3.3 **RED** Add failing assertions in `internal/compose/fake.go`-backed tests (update/restart suites) for `RestartCalls`, `RestartErr`, and `CallOrder` observability.
- [x] 3.4 **GREEN** Modify `internal/compose/fake.go` to record restart invocations and order without changing existing Pull/Up/Down behavior.
- [x] 3.5 **RED** Create table-driven failing tests in `internal/restart/run_test.go` for fast-fail artifact preflight, single `Restart` call with resolved files/env, no pull/up/down fallback, and wrapped `restart:` errors.
- [x] 3.6 **GREEN** Create `internal/restart/run.go` implementing non-interactive `Run(ctx, cfg, deps, out)` that resolves artifacts, computes compose files, and calls `ComposeRunner.Restart` exactly once.

## Phase 4: Documentation & Verification

- [x] 4.1 Update `README.md` with `alice-installer restart` usage, persisted-artifact prerequisites, and exact restart (no reinstall/down-up) semantics.
- [x] 4.2 Update `RUNBOOK.md` with operator restart guidance, failure expectations, and cross-platform notes (Linux amd64/arm64, Windows amd64).
- [x] 4.3 Run `go test ./...` to validate all RED→GREEN outcomes plus non-regression for interactive install, `--unattended`, and `update` flows.
