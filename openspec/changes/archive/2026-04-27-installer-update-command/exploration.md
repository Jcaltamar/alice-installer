# Exploration: installer-update-command

## Current State

`alice-installer` currently has a **flag-only** CLI (`parseFlags` in `cmd/installer/main.go`) and does not route positional commands. `flag.FlagSet.Parse(args)` is called, but the remaining positional args (`fs.Args()`) are never inspected, so `alice-installer update` is treated like a normal invocation and falls through to the regular TUI/headless installer flow.

The existing install flows (`internal/tui/model.go` and `internal/headless/run.go`) always execute full install stages (preflight/bootstrap/port-scan/env-write/pull/up/verify). Both flows also rewrite `.env` and compose files from embedded assets before pull/up.

That means there is currently **no command** that updates an existing deployment in place by only using the already-present `docker-compose.yml` and running:

1. `docker compose pull`
2. `docker compose up -d`

Relevant runtime primitives already exist:

- `compose.ComposeRunner.Pull(...)`
- `compose.ComposeRunner.Up(...)`
- `compose.CLICompose` (streams progress, wraps stderr on pull failures)

## Affected Areas

- `cmd/installer/main.go` — CLI command parsing and top-level routing (`update` branch before TTY/TUI path).
- `cmd/installer/main_test.go` — parser/routing tests for positional command behavior and exit codes.
- `internal/compose/runner.go` — likely reused as-is for pull/up execution (no required API change), but command construction assumptions must be validated for update mode inputs.
- `internal/compose/fake.go` — reused in tests to assert Pull/Up invocation order and error propagation.
- `README.md` and/or `RUNBOOK.md` — user-facing command docs should include `alice-installer update` semantics and working-directory/workspace expectations.

## Approaches

### 1. Add a real `update` subcommand path in `cmd/installer` (RECOMMENDED)

Route `alice-installer update` to a dedicated non-interactive flow that:

- Resolves compose files from existing installation artifacts (workspace-based and/or explicit paths).
- Uses existing `ComposeRunner` to run Pull then Up.
- Skips TUI, preflight, env regeneration, and compose file rewrites.

Pros:
- Matches requested UX exactly (`alice-installer update`).
- Reuses existing compose abstraction and fakes (keeps testability + no globals).
- Keeps installer and updater responsibilities explicit.

Cons:
- Requires explicit command parser design (root flags vs subcommand flags).
- Must define file resolution contract clearly (workspace vs cwd, env file behavior).

Effort: Medium

### 2. Reuse existing `--unattended` flow for update

Attempt to map update to `headless.Run` with specific flags.

Pros:
- Minimal new top-level routing.

Cons:
- `headless.Run` currently rewrites `.env` and compose files from embedded assets, which conflicts with “take existing docker-compose.yml into account”.
- Includes preflight/bootstrap/port-scan concerns that are outside a simple update command.

Effort: Medium (and behavior mismatch risk is high)

### 3. Shell out directly in `main.go` with raw `docker compose` commands

Implement update by calling `exec.Command("docker", "compose", ... )` directly in CLI entrypoint.

Pros:
- Fastest implementation.

Cons:
- Duplicates compose execution logic already centralized in `internal/compose`.
- Harder to test consistently with existing fake runner patterns.
- Increases risk of drift in error formatting/progress behavior.

Effort: Low

## Recommendation

Adopt **Approach 1**: add a proper `update` subcommand route in `cmd/installer` and execute through `ComposeRunner`.

Key design points to settle in proposal/spec:

1. **Command parsing contract**: detect positional subcommand before/while parsing flags so `alice-installer update` is unambiguous and future-safe.
2. **Artifact resolution**: define where update reads from by default (likely workspace dir artifacts written by installer), and how users can override.
3. **Execution contract**: strict order `pull` then `up -d`; stop on pull error; propagate compose stderr details.
4. **Non-interactive behavior**: update path must not require TTY and must not enter Bubbletea states.

## Risks

- **Positional parsing ambiguity**: current parser ignores trailing args; naive subcommand support can break existing flag UX (especially if flags appear after subcommand).
- **Wrong compose target**: if update resolves files from the wrong directory, it can update a different stack than intended.
- **Env-file mismatch**: running pull/up without the expected `.env` may change image tags or service config resolution.
- **Behavior drift**: bypassing `internal/compose` would lose existing stderr-tail enrichment and test seams.

## Ready for Proposal

**Yes** — scope is clear, affected modules are identified, and there is a straightforward recommended approach with defined open decisions for proposal/spec.
