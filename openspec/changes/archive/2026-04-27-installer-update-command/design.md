# Design: Installer Update Command

## Technical Approach

Add a small command parser in `cmd/installer` that detects a single `update` subcommand while preserving all existing root flags. Route that mode into a new non-TUI `internal/update` orchestration package that resolves persisted workspace artifacts, then executes `ComposeRunner.Pull` followed by `ComposeRunner.Up`. Install, dry-run, version, stale-group recovery, TUI, and headless flows remain unchanged when `update` is not selected.

## Architecture Decisions

| Decision | Choice | Alternatives | Rationale |
|----------|--------|--------------|-----------|
| CLI parsing | Add `parseCLI(args)` that removes one bare `update` token, then reuses current flag parsing | Pure `flag.FlagSet`; separate binary | Go flags stop at first positional, so shared root flags would break for `update --workspace-dir ...` without a thin pre-parse step |
| Update orchestration | Create `internal/update/run.go` with `Run(ctx, cfg, deps, out)` | Put logic in `main.go`; reuse `headless.Run` | Keeps CLI thin, avoids env rewrite/preflight/verify side effects, and matches current package-level orchestration style |
| Artifact resolution | Resolve from `WorkspaceDir`: `.env` + `docker-compose.yml` required; `docker-compose.gpu.yml` optional and included only when present and GPU is currently detected | Reuse `EnvOutput`; always include overlay | `WorkspaceDir` is already the persisted artifact root; `EnvOutput` is install-only and would make update ambiguous |
| Streaming/error reporting | Stream raw compose lines to `out` with `[pull]` / `[deploy]` prefixes; stop on pull failure; return `up` failure directly | Buffered summary only; direct `exec.Command` | Reuses existing compose stderr enrichment and preserves operator-visible progress |
| Test seam | Extend `compose.FakeComposeRunner` with call recording; test file resolution with `t.TempDir()` fixtures | Global monkeypatching; real docker | Matches repo conventions: interface injection + temp dirs + table-driven tests |

## Data Flow

```text
argv
  │
  ▼
cmd/installer parseCLI ──► mode=update? ──no──► existing install paths
  │ yes
  ▼
internal/update.Run
  │ resolve workspace artifacts
  ▼
ComposeRunner.Pull(files, env)
  │ success
  ▼
ComposeRunner.Up(files, env)
  ▼
exit 0 / propagated error
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `cmd/installer/main.go` | Modify | Add CLI mode parsing and update routing after stale-group gate |
| `cmd/installer/main_test.go` | Modify | Table-driven tests for legacy flags, `update` routing, ordering, and exit codes |
| `internal/update/run.go` | Create | Non-interactive update orchestration + artifact resolution helpers |
| `internal/update/run_test.go` | Create | TDD coverage for missing files, overlay selection, pull→up ordering, and error propagation |
| `internal/compose/fake.go` | Modify | Record Pull/Up calls and arguments for orchestration assertions |
| `README.md` | Modify | Document `alice-installer update` usage and workspace expectations |
| `RUNBOOK.md` | Modify | Replace upgrade guidance that rewrites env with the new in-place update flow |

## Interfaces / Contracts

```go
type cliMode string

const (
    modeInstall cliMode = "install"
    modeUpdate  cliMode = "update"
)

type UpdateConfig struct { WorkspaceDir string }

type UpdateDependencies struct {
    Compose compose.ComposeRunner
    GPU     platform.GPUDetector
}
```

Resolution contract:
- workspace root = `flags.WorkspaceDir`
- required: `<workspace>/.env`, `<workspace>/docker-compose.yml`
- optional: `<workspace>/docker-compose.gpu.yml`
- if required files are missing: fail with “existing installation artifacts not found; run install first or pass --workspace-dir”

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit — CLI | `update` before/after flags, legacy flag-only flows, unknown positional handling | Table-driven tests in `cmd/installer/main_test.go` |
| Unit — update | Required file resolution, optional overlay selection, pull before up, no up after pull error | `t.TempDir()` fixtures + recording `FakeComposeRunner` |
| Unit — update | Error text preserves compose failure context | Fake `PullErr` / `UpErr` assertions |
| Docs | Upgrade examples point to `update`, not reinstall | README/RUNBOOK review in change tasks |

## Migration / Rollout

No migration required. Existing installations are updated in place from their persisted workspace artifacts.

## Open Questions

- [ ] None blocking; future work may add explicit subcommand-only flags if operators need force/verify variants.
