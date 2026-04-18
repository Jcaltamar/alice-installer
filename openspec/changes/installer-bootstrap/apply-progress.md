# Apply Progress: installer-bootstrap (Batch 1 — single-batch implementation)

## Status: COMPLETE

All T-BS-001 through T-BS-018 implemented and verified GREEN.

## Completed Tasks

### Messages (T-BS-001, T-BS-002)
- Added `Action`, `BootstrapNeededMsg`, `BootstrapConfirmedMsg`, `BootstrapSkippedMsg`,
  `BootstrapActionResultMsg`, `BootstrapCompleteMsg`, `BootstrapFailedMsg`, `PreflightReRunMsg`
  to `internal/tui/messages.go`.

### Classification (T-BS-003, T-BS-004)
- Implemented `ClassifyBlockers(report, mediaDir, configDir)` in `internal/tui/bootstrap.go`.
- Table-driven tests in `bootstrap_classify_test.go` cover 6 scenarios.
- `buildDirAction` resolves username via `os/user.Current()` with `$USER` fallback.

### Executor (T-BS-005, T-BS-006)
- `Executor` interface, `teaExecutor` (wraps `tea.ExecProcess`), `NewExecutor()`,
  and `FakeExecutor` all in `internal/tui/bootstrap.go`.
- `execProcessCmd` helper in `internal/tui/bootstrap_exec.go` (isolates `os/exec` import).

### BootstrapModel (T-BS-007, T-BS-008, T-BS-009)
- `BootstrapModel` struct with `confirming`, `currentIdx`, `done`, `failed`, `declined` fields.
- `NewBootstrapModel`, `Init`, `Update`, `View` fully implemented.
- Tests in `bootstrap_model_test.go` cover all 9 scenarios.

### Root Model (T-BS-010, T-BS-011)
- Added `StateBootstrap` to the State enum (after `StatePreflight`).
- Added `bootstrap BootstrapModel` and `Executor Executor` fields to `Model` and `Dependencies`.
- `PreflightResultMsg` handler updated: calls `ClassifyBlockers`; routes to `StateBootstrap`
  when all blockers are fixable; also stores original report in preflight sub-model for skip path.
- `BootstrapCompleteMsg` handler: transitions to `StatePreflight`, calls `m.preflight.Rearm()`.
- `BootstrapSkippedMsg` handler: transitions to `StatePreflight` (report preserved).
- `StateBootstrap` added to delegate switch and View switch.

### cmd/installer/main.go (T-BS-012)
- `tui.NewExecutor()` wired into `Dependencies.Executor`.

### PreflightModel.Rearm() (T-BS-013, T-BS-014)
- `Rearm()` method added to `PreflightModel` in `internal/tui/preflight.go`.
- Resets `report` and `err` to nil, returns fresh `Init()` cmd.
- Tests in `preflight_rearm_test.go`.

### Integration tests (T-BS-015, T-BS-016, T-BS-017)
- `fullflow_bootstrap_test.go` with `countingDirChecker` and `buildBootstrapFlowDeps`.
- `TestFullFlowBootstrapHappyPath`: full flow through bootstrap → re-preflight → workspace → result.
- `TestFullFlowBootstrapSkippedPreservesReport`: n press → frozen report on preflight screen.

### Final validation (T-BS-018)
- `go vet ./...` — clean.
- `go test -short ./...` — 11 packages GREEN.
- `go test ./...` — 11 packages GREEN.

## Files changed

- `internal/tui/messages.go` — added bootstrap messages and Action type
- `internal/tui/bootstrap.go` — new file: ClassifyBlockers, Executor, FakeExecutor, BootstrapModel
- `internal/tui/bootstrap_exec.go` — new file: execProcessCmd (production tea.ExecProcess wrapper)
- `internal/tui/preflight.go` — added Rearm() method
- `internal/tui/model.go` — StateBootstrap, bootstrap field, Executor in Dependencies, routing handlers
- `cmd/installer/main.go` — wire NewExecutor()
- `internal/tui/messages_bootstrap_test.go` — new file
- `internal/tui/bootstrap_classify_test.go` — new file
- `internal/tui/bootstrap_executor_test.go` — new file
- `internal/tui/bootstrap_model_test.go` — new file
- `internal/tui/model_bootstrap_test.go` — new file
- `internal/tui/preflight_rearm_test.go` — new file
- `internal/tui/fullflow_bootstrap_test.go` — new file
- `openspec/changes/installer-bootstrap/proposal.md` — new file
- `openspec/changes/installer-bootstrap/specs/installer-bootstrap.md` — new file
- `openspec/changes/installer-bootstrap/design.md` — new file
- `openspec/changes/installer-bootstrap/tasks.md` — new file
- `openspec/changes/installer-bootstrap/apply-progress.md` — this file
