# Design: Installer Restart Command

## Technical Approach

Add a second non-install CLI mode, `restart`, that is parsed with the same positional-command normalization as `update`, then dispatched from `cmd/installer/main.go` before dry-run, unattended, or TUI execution. Both `update` and `restart` will consume one shared persisted-artifact resolver so `.env`, `docker-compose.yml`, and optional GPU overlay selection stay single-sourced. `restart` will run exact `docker compose restart` through an expanded `compose.ComposeRunner` API and return non-interactive, wrapped errors to the CLI.

## Architecture Decisions

| Decision | Options | Choice | Rationale |
|---|---|---|---|
| CLI command parsing | Keep update-only parsing; bolt restart on later; generalize positional command parsing | Generalize `parseCLI` to track one command from `{update,restart}` while preserving root-flag/value skipping | Prevents flag regressions and keeps `--workspace-name restart` treated as a value, not a command. |
| Shared artifact resolver | Duplicate update logic; export from `internal/update`; extract neutral shared package | Create `internal/workspace/artifacts.go` used by update and restart | Avoids drift without making restart semantically depend on update. Keeps `filepath.Join` cross-platform behavior centralized. |
| Restart orchestration | Inline in `cmd/installer`; add to update package; create dedicated package | Create `internal/restart.Run` with `Config` + `Dependencies` mirroring update | Matches existing package-per-flow structure and keeps CLI thin. |
| Compose restart API | Emulate with `down/up`; raw exec in restart; add `ComposeRunner.Restart` | Add `Restart(ctx, files, envFile) error` to `ComposeRunner` and `CLICompose` | Guarantees exact `docker compose restart` semantics and reuses existing command seams/fakes. |
| Error ownership | Format detailed CLI text in lower layers; wrap at each layer | Restart package wraps step context (`restart: ...`), CLI prints `error:` and exits `1` | Consistent with current update/headless error behavior and easy to test. |

## Data Flow

```text
argv
  -> parseCLI (command + normalized root flags)
  -> parseFlags
  -> runWithStaleCheck
  -> modeRestart branch
  -> restart.Run(cfg.WorkspaceDir, deps.Compose, deps.GPU)
  -> workspace.ResolveArtifacts + workspace.ComposeFiles
  -> compose.Restart(files, envFile)
  -> stderr/outcome returned to CLI
```

Sequence:

```text
main -> parseCLI -> parseFlags -> restart.Run
restart.Run -> workspace.ResolveArtifacts
restart.Run -> workspace.ComposeFiles
restart.Run -> ComposeRunner.Restart
ComposeRunner.Restart -> docker compose restart
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `cmd/installer/main.go` | Modify | Add `modeRestart`, `runRestartFn`, and a second non-install dispatch branch without changing root flags or stale-group gating. |
| `cmd/installer/main_test.go` | Modify | Extend table-driven parser/routing tests for restart, mixed commands, and failure exit codes. |
| `internal/workspace/artifacts.go` | Create | Shared persisted-artifact resolver and compose-file selection helper for update/restart. |
| `internal/workspace/artifacts_test.go` | Create | Table-driven tests for required files, GPU overlay inclusion, and directory/error cases. |
| `internal/update/run.go` | Modify | Replace private resolver usage with shared workspace helper. |
| `internal/update/run_test.go` | Modify | Keep update behavior locked while relying on shared artifact contract. |
| `internal/restart/run.go` | Create | Non-interactive restart orchestration calling shared resolver + compose restart. |
| `internal/restart/run_test.go` | Create | Verify artifact requirements, exact restart call, and wrapped error propagation. |
| `internal/compose/runner.go` | Modify | Extend interface and `CLICompose` with exact `docker compose restart`. |
| `internal/compose/fake.go` | Modify | Record `RestartCalls`, `RestartErr`, and `CallOrder` for observability. |
| `internal/compose/runner_test.go` / `internal/compose/invocation_test.go` | Modify | Lock success/error behavior and exact `docker compose ... restart` invocation. |
| `README.md`, `RUNBOOK.md` | Modify | Document restart semantics, persisted-artifact expectations, and examples. |

## Interfaces / Contracts

```go
type ComposeRunner interface {
    Version(ctx context.Context) (Version, error)
    Pull(ctx context.Context, files []string, envFile string, progress chan<- PullProgressMsg) error
    Up(ctx context.Context, files []string, envFile string, progress chan<- UpProgressMsg) error
    Restart(ctx context.Context, files []string, envFile string) error
    Down(ctx context.Context, files []string, envFile string) error
    HealthStatus(ctx context.Context, files []string, envFile string) ([]ServiceHealth, error)
}
```

`workspace.ResolveArtifacts(workspaceDir)` returns required `.env` + base compose path and optional GPU overlay path; `workspace.ComposeFiles(ctx, gpuDetector, resolved)` returns the exact file list passed to both update and restart.

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | `parseCLI` update+restart normalization | Extend existing table-driven tests first; include duplicate, mixed-command, and root-flag-value cases. |
| Unit | Shared artifact resolution | New table-driven `internal/workspace` tests using `t.TempDir()` only. |
| Unit | Restart orchestration | Fake compose + fake GPU: assert one `Restart` call, resolved env/file arguments, and `restart:` error wrapping. |
| Unit | Compose adapter | Add `CLICompose.Restart` success/error/invocation tests via fake command runner. |
| Regression | Update unchanged | Keep current update tests passing against shared resolver. |

Strict-TDD order: (1) parser tests, (2) routing tests with `runRestartFn`, (3) workspace resolver tests, (4) compose restart tests, (5) restart flow tests, (6) minimal update regression adjustments, (7) docs.

## Migration / Rollout

No migration required. Existing installations only need persisted workspace artifacts already required by `update`.

## Open Questions

- [ ] None.
