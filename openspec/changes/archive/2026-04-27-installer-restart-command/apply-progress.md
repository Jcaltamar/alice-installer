# Apply Progress: installer-restart-command

## Implementation Progress

**Change**: installer-restart-command  
**Mode**: Strict TDD

### Completed Tasks
- [x] 1.1–1.5 CLI parsing + restart routing (`modeRestart`, mixed-command validation, `runRestartFn` dispatch)
- [x] 2.1–2.5 Shared workspace artifact resolver + update flow migration to shared resolver
- [x] 3.1–3.6 Compose restart API/fake support + dedicated restart orchestration package
- [x] 4.1–4.3 Documentation updates and full-suite verification (`go test ./...`)

### Files Changed
| File | Action | What Was Done |
|---|---|---|
| `cmd/installer/main.go` | Modified | Added `modeRestart`, generalized command parsing for `{update,restart}`, and restart flow dispatch via `runRestartFn`. |
| `cmd/installer/main_test.go` | Modified | Added table-driven parser cases for restart/mixed commands and restart routing/non-zero failure tests. |
| `internal/workspace/artifacts.go` | Created | Added shared persisted-artifact resolution + compose file selection (`ResolveArtifacts`, `ComposeFiles`). |
| `internal/workspace/artifacts_test.go` | Created | Added table-driven tests for required artifacts, optional GPU overlay, deterministic ordering, actionable errors. |
| `internal/update/run.go` | Modified | Replaced private resolver with shared workspace helpers; resolved compose files once and reused for pull/up. |
| `internal/update/run_test.go` | Modified | Added regression test to ensure compose file resolution reused (single GPU detect call) while behavior remains unchanged. |
| `internal/compose/runner.go` | Modified | Extended `ComposeRunner` + `CLICompose` with exact `Restart(ctx, files, envFile)` semantics (`docker compose restart`). |
| `internal/compose/fake.go` | Modified | Added restart observability (`RestartCalls`, `RestartErr`, `CallOrder` tracking). |
| `internal/compose/runner_test.go` | Modified | Added restart success/error tests for CLI and fake runner restart call tracking tests. |
| `internal/compose/invocation_test.go` | Modified | Added invocation regression ensuring `docker compose ... restart` command shape. |
| `internal/restart/run.go` | Created | Implemented non-interactive restart orchestration with artifact preflight + single compose restart call. |
| `internal/restart/run_test.go` | Created | Added table-driven restart tests for preflight failures, exact restart call semantics, and wrapped errors. |
| `README.md` | Modified | Documented `alice-installer restart` usage and exact restart semantics. |
| `RUNBOOK.md` | Modified | Added operator restart guidance, artifact prerequisites, and cross-platform behavior notes. |
| `openspec/changes/installer-restart-command/tasks.md` | Modified | Marked all tasks complete `[x]`. |

### TDD Cycle Evidence
| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| 1.1–1.2 | `cmd/installer/main_test.go` | Unit | ✅ `go test ./cmd/installer` | ✅ Added restart parser tests, failed (`modeRestart` undefined) | ✅ parseCLI generalized for `{update,restart}` | ✅ Multiple cases (first positional, after flags, mixed/duplicate, root-flag values) | ✅ Parser command counting/helper cleanup |
| 1.4–1.5 | `cmd/installer/main_test.go` | Unit | ✅ `go test ./cmd/installer` | ✅ Added restart routing tests, failed (missing restart package/wiring) | ✅ Added `runRestartFn` dispatch and mode routing branch | ✅ Success + failure branches + unattended bypass assertions | ✅ Kept CLI branching consistent with existing update style |
| 2.1–2.2 | `internal/workspace/artifacts_test.go` | Unit | N/A (new files) | ✅ Added resolver tests, failed (missing helper/functions) | ✅ Implemented shared resolver API | ✅ Missing env/base, empty workspace, overlay include/exclude ordering | ✅ Centralized path rules + helper decomposition |
| 2.4–2.5 | `internal/update/run_test.go` | Unit | ✅ `go test ./internal/update` | ✅ Added regression test, failed (GPU detect called twice) | ✅ Update now resolves files once via shared helpers | ✅ Existing update behavior tests still pass (pull→up order, error paths, overlay behavior) | ✅ Removed duplicate artifact logic from update package |
| 3.1–3.4 | `internal/compose/runner_test.go`, `internal/compose/invocation_test.go` | Unit | ✅ `go test ./internal/compose` | ✅ Added restart API tests, failed (method/fields missing) | ✅ Added `ComposeRunner.Restart`, `CLICompose.Restart`, fake restart tracking | ✅ Success + error propagation + invocation command-shape assertions | ✅ Preserved existing Pull/Up/Down behavior |
| 3.5–3.6 | `internal/restart/run_test.go` | Unit | N/A (new package) | ✅ Added restart flow tests, failed (stub returned nil, no preflight/calls) | ✅ Implemented restart flow with shared artifacts + single restart call | ✅ Table-driven missing-artifact scenarios + compose failure wrapping | ✅ Kept orchestration minimal and non-interactive |
| 4.1–4.3 | `README.md`, `RUNBOOK.md`, full suite | Docs + Verification | ✅ package-level safety nets previously green | ✅ Docs gaps identified for restart command | ✅ Updated docs + verified with `go test ./...` | ➖ None needed |

### Test Summary
- **Total tests written/extended**: 17 (new + expanded table-driven cases)
- **Total tests passing**: all in targeted packages + full suite (`go test ./...`)
- **Layers used**: Unit
- **Approval tests**: None — feature addition, not behavior-preserving refactor only
- **Pure functions created**: N/A (flow wiring + I/O orchestration)

### Deviations from Design
None — implementation matches design intent (shared artifact resolver, exact compose restart API, dedicated restart flow package, CLI routing precedence).

### Issues Found
None.

### Remaining Tasks
- [x] None.

### Status
16/16 tasks complete. Ready for `sdd-verify`.
