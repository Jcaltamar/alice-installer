# Apply Progress: installer-update-command

## Implementation Progress

**Change**: installer-update-command  
**Mode**: Strict TDD

### Completed Tasks
- [x] 1.1 RED parse/update positional parsing and legacy-mode CLI tests
- [x] 1.2 GREEN `parseCLI(args)` + mode routing pre-flag parsing
- [x] 1.3 REFACTOR split helpers (`rootFlagNeedsValue`, mode constants/hooks)
- [x] 2.1 RED update orchestration artifact precondition tests
- [x] 2.2 GREEN `internal/update.Run` + deterministic artifact resolution
- [x] 2.3 RED pull-before-up + failure propagation tests
- [x] 2.4 GREEN compose execution (`Pull` then `Up`) + prefixed streaming
- [x] 2.5 RED GPU overlay inclusion tests
- [x] 2.6 GREEN optional overlay inclusion only when file exists + GPU detected
- [x] 3.1 RED assertions requiring compose fake call recording in update/CLI tests
- [x] 3.2 GREEN compose fake call recording (`PullCalls`, `UpCalls`, `CallOrder`)
- [x] 3.3 RED CLI wiring tests for update path and install-path bypass
- [x] 3.4 GREEN route glue in `cmd/installer/main.go` invoking `update.Run`
- [x] 4.1 README update command docs
- [x] 4.2 RUNBOOK upgrade flow docs moved to in-place `update`
- [x] 4.3 Full verification: `go test ./...`

### Files Changed
| File | Action | What Was Done |
|------|--------|---------------|
| `cmd/installer/main.go` | Modified | Added CLI mode parser and update/headless run hooks; routed `modeUpdate` after stale gate |
| `cmd/installer/main_test.go` | Modified | Added table-driven parse tests and update wiring/order assertions |
| `internal/update/run.go` | Created | Added update orchestration with required artifacts, pull→up ordering, prefixed progress output |
| `internal/update/run_test.go` | Created | Added table-driven tests for artifacts, non-interactive behavior, ordering/failures, GPU overlay selection |
| `internal/compose/fake.go` | Modified | Added compose invocation recording for pull/up order and args assertions |
| `README.md` | Modified | Documented `alice-installer update` semantics and workspace artifacts |
| `RUNBOOK.md` | Modified | Switched upgrade guidance to in-place `update` flow |
| `openspec/changes/installer-update-command/tasks.md` | Modified | Marked all tasks complete |

### TDD Cycle Evidence
| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 1.1 | `cmd/installer/main_test.go` | Unit | ✅ `go test ./cmd/installer/...` | ✅ Written first | ✅ Pass | ✅ table cases (update first/last, duplicate, unknown, legacy) | ✅ helper boundaries cleaned |
| 1.2 | `cmd/installer/main_test.go` | Unit | ✅ baseline above | ✅ failing compile/runtime first | ✅ pass after `parseCLI` + mode wiring | ✅ multi positional/flag value cases | ✅ extracted `rootFlagNeedsValue` |
| 1.3 | `cmd/installer/main_test.go` | Unit | ✅ | ✅ existing behavior locked by tests | ✅ pass | ✅ no behavior drift | ✅ hooks/constants cleanup |
| 2.1 | `internal/update/run_test.go` | Unit | N/A (new package) | ✅ written before `run.go` | ✅ pass | ✅ missing both/env/compose variants | ➖ minimal code already clean |
| 2.2 | `internal/update/run_test.go` | Unit | N/A | ✅ | ✅ | ✅ success + artifact fail-fast branches | ✅ split resolver helpers |
| 2.3 | `internal/update/run_test.go` | Unit | ✅ `go test ./internal/update/...` | ✅ failing due missing call recording | ✅ pass after fake recording | ✅ pull fail/no-up + up fail branches | ➖ none needed |
| 2.4 | `internal/update/run_test.go` | Unit | ✅ | ✅ | ✅ | ✅ order and error-context cases | ✅ small helper extraction |
| 2.5 | `internal/update/run_test.go` | Unit | ✅ | ✅ table assertions added | ✅ pass | ✅ GPU true+file / true+missing / false+file | ➖ none needed |
| 2.6 | `internal/update/run_test.go` | Unit | ✅ | ✅ | ✅ | ✅ deterministic file list verified | ➖ none needed |
| 3.1 | `internal/update/run_test.go`, `cmd/installer/main_test.go` | Unit | ✅ cmd+update package baselines | ✅ assertions required call-order/args | ✅ pass after fake seam | ✅ both orchestration and CLI surfaces | ➖ none needed |
| 3.2 | `internal/update/run_test.go`, `cmd/installer/main_test.go` | Unit | ✅ `go test ./internal/compose/...` | ✅ tests required new fields | ✅ pass after `fake.go` change | ✅ pull/up count + args + order | ➖ none needed |
| 3.3 | `cmd/installer/main_test.go` | Unit | ✅ cmd baseline | ✅ update routing tests fail before hooks/wiring | ✅ pass | ✅ success+error routing scenarios | ✅ run hook seams |
| 3.4 | `cmd/installer/main_test.go` | Unit | ✅ | ✅ | ✅ | ✅ ensures update bypasses headless/TUI | ➖ none needed |
| 4.1 | docs | N/A | N/A | ➖ structural docs task | ✅ updated | Triangulation skipped: docs-only | ➖ none |
| 4.2 | docs | N/A | N/A | ➖ structural docs task | ✅ updated | Triangulation skipped: docs-only | ➖ none |
| 4.3 | full suite | Unit/Integration | N/A | ✅ verify command required by task | ✅ `go test ./...` passing | ➖ single command | ➖ none |

### Test Summary
- **Total tests written**: 8 new test functions (multi-case table-driven)
- **Total tests passing**: full suite green via `go test ./...`
- **Layers used**: Unit
- **Approval tests (refactoring)**: None — behavior-lock tests covered helper refactors
- **Pure functions created**: `parseCLI`, `rootFlagNeedsValue`, `resolveArtifacts`, `requireFile`, `composeFiles`

### Deviations from Design
None — implementation matches design intent (CLI pre-parse + dedicated `internal/update` orchestration + compose abstraction reuse).

### Issues Found
None.

### Remaining Tasks
- [x] All tasks complete.

### Status
16/16 tasks complete. Ready for `sdd-verify`.
