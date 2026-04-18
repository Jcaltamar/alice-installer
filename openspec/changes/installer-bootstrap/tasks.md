# Tasks: installer-bootstrap

TDD-paired list. Every impl task is preceded by a RED test task.
Run `go test -short ./...` after each RED→GREEN cycle.

---

## Messages

- [x] T-BS-001 (RED) — Write tests in `messages_bootstrap_test.go` asserting that all new message types compile
  and are distinct types: `BootstrapNeededMsg`, `BootstrapConfirmedMsg`, `BootstrapSkippedMsg`,
  `BootstrapActionResultMsg`, `BootstrapCompleteMsg`, `BootstrapFailedMsg`, `PreflightReRunMsg`, `Action`.
- [x] T-BS-002 (GREEN) — Add new messages and `Action` type to `internal/tui/messages.go`. Tests go GREEN.

## Classification function

- [x] T-BS-003 (RED) — Write table-driven tests in `bootstrap_classify_test.go` for `ClassifyBlockers`:
  - both dirs fail → 2 fixable, 0 non-fixable
  - docker+media fail → 1 fixable, 1 non-fixable
  - only warnings → 0 fixable, 0 non-fixable
  - all pass → 0, 0
  - only config fail → 1 fixable, 0 non-fixable
- [x] T-BS-004 (GREEN) — Implement `ClassifyBlockers` in `internal/tui/bootstrap.go`. Tests go GREEN.

## Executor interface + real impl + fake

- [x] T-BS-005 (RED) — Write test in `bootstrap_executor_test.go` asserting:
  - `NewExecutor()` returns non-nil
  - `FakeExecutor.ExecCmd` returns a callable cmd that posts the queued result
- [x] T-BS-006 (GREEN) — Define `Executor` interface, `teaExecutor`, `NewExecutor()`,
  and `FakeExecutor` in `internal/tui/bootstrap.go`. Tests go GREEN.

## BootstrapModel

- [x] T-BS-007 (RED) — Write tests in `bootstrap_model_test.go`:
  - `NewBootstrapModel` sets `confirming=true`
  - `Init()` returns nil cmd (nothing to start automatically)
  - pressing `y` → `confirming=false`, cmd is non-nil (executor called)
  - pressing `n` → cmd produces `BootstrapSkippedMsg`
  - pressing `Esc` → cmd produces `BootstrapSkippedMsg`
  - `BootstrapActionResultMsg{Err=nil}` on last action → cmd produces `BootstrapCompleteMsg`
  - `BootstrapActionResultMsg{Err=someErr}` → cmd produces `BootstrapFailedMsg`
  - `BootstrapActionResultMsg{Err=nil}` when more actions remain → currentIdx advances, next ExecCmd called
- [x] T-BS-008 (GREEN) — Implement `BootstrapModel`, `NewBootstrapModel`, `Init`, `Update`, `View`
  in `internal/tui/bootstrap.go`. Tests go GREEN.
- [x] T-BS-009 (REFACTOR) — Polish View: confirming screen shows action descriptions + Y/N prompt;
  executing screen shows "N/M: description…"; done shows checkmarks; error shows danger text.

## Root Model routing

- [x] T-BS-010 (RED) — Write tests in `model_bootstrap_test.go`:
  - `PreflightResultMsg` with only fixable blockers → `StateBootstrap`
  - `PreflightResultMsg` with non-fixable blocker → stays `StatePreflight`
  - `PreflightResultMsg` with mixed (fixable + non-fixable) → stays `StatePreflight`
  - `PreflightResultMsg` with no blockers → stays `StatePreflight` (delegated, preflight emits passed on Enter)
  - `BootstrapCompleteMsg` → `StatePreflight` (re-armed)
  - `BootstrapSkippedMsg` → `StatePreflight` (frozen report)
- [x] T-BS-011 (GREEN) — Update `internal/tui/model.go`:
  - Add `StateBootstrap` to enum (after `StatePreflight`)
  - Add `bootstrap BootstrapModel` and `Executor` fields
  - Rewrite `PreflightResultMsg` handler using `ClassifyBlockers`
  - Add `BootstrapCompleteMsg` / `BootstrapSkippedMsg` handlers
  - Add `StateBootstrap` to delegate switch + View switch
- [x] T-BS-012 (GREEN) — Update `cmd/installer/main.go` to wire `NewExecutor()` into `Dependencies.Executor`.

## PreflightModel re-arm

- [x] T-BS-013 (RED) — Write test in `preflight_rearm_test.go`:
  - `Rearm()` resets report to nil
  - `Rearm()` returns a non-nil cmd
  - Calling the returned cmd produces `PreflightResultMsg`
- [x] T-BS-014 (GREEN) — Add `Rearm()` method to `PreflightModel` in `internal/tui/preflight.go`.
  Tests go GREEN.

## Integration test

- [x] T-BS-015 (RED) — Write `fullflow_bootstrap_test.go` with skeleton test that fails
  (references types not yet fully wired).
- [x] T-BS-016 (GREEN) — Wire everything; integration test passes:
  1. Fake dir checker fails for ConfigDir on first call, passes on second.
  2. Splash Enter → preflight starts → ConfigDir FAIL → `StateBootstrap`.
  3. Send 'y' → executor runs fake action (Err=nil) → `BootstrapCompleteMsg`.
  4. Re-preflight runs → all pass → `StateWorkspaceInput` after Enter.
  5. Continue happy path → `StateResult` success.
- [x] T-BS-017 (GREEN) — Verify `BootstrapSkippedMsg` path: preflight frozen with original report.
- [x] T-BS-018 (FINAL) — Run `go vet ./...`, `go test -short ./...`, `go test ./...`. All GREEN.
  Tick off completed items in this file.
