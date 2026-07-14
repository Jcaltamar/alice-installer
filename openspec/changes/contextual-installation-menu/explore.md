# Exploration: contextual-installation-menu

**Change**: `contextual-installation-menu`
**Project**: `alice-installer`
**Date**: 2026-07-11
**Artifact Store**: openspec

## Executive finding

The current interactive flow is a linear Bubbletea wizard: `StateSplash` → `StatePreflight` → workspace input → optional packages → port scan → env write → pull → deploy → verify → result. There is no installation-management menu, no uninstall implementation, and no PM2/legacy-backend detection. The requested menu should be inserted after the splash and should perform a separate, injectable installation-state probe before presenting actions; it should not reuse the install preflight as the state detector because preflight assumes a new/current Docker Compose installation and can produce remediation flows or failures unrelated to action selection.

## Current TUI flow

- `internal/tui/splash.go`: `SplashModel` waits for Enter and emits `PreflightStartedMsg`.
- `internal/tui/model.go`: `NewModel` initializes `StateSplash`; root `Update` handles `PreflightStartedMsg` by entering `StatePreflight` and running preflight.
- The state enum has no management/menu state. Root `View` and test resize tables enumerate all current states.
- `cmd/installer/main.go` constructs production dependencies and launches Bubbletea with alt-screen. Explicit CLI modes currently include `update` and `restart`; normal interactive mode always constructs the install TUI.
- Existing splash and full-flow tests assert that Enter emits `PreflightStartedMsg` and that the next state is `StatePreflight`; these are the primary regression points when inserting a menu.

## Existing installation-state mechanisms

### Current Compose installation

`--workspace-dir` defaults to `$XDG_CONFIG_HOME/alice-guardian` or `$HOME/.config/alice-guardian`. The current install writes `.env` and `docker-compose.yml` there, while the runbook also documents older/manual artifacts under `/opt/alice-media`.

`internal/workspace/artifacts.go` provides the strongest existing current-install signal:

- `.env` must exist and be a regular file.
- `docker-compose.yml` must exist and be a regular file.
- `docker-compose.gpu.yml` is optional and included only when GPU detection is positive.
- `ResolveArtifacts` returns an error for missing/invalid artifacts; it is used by both `internal/update` and `internal/restart`.

This is an artifact-presence signal, not proof that services are running or that the files belong to Alice. A future detector should distinguish `not installed` from `unknown/error` and should avoid treating a partial or unreadable artifact set as a safe install target.

Compose lifecycle behavior already exists:

- Update resolves artifacts, then executes `docker compose pull` followed by `docker compose up` through `compose.ComposeRunner`.
- Restart resolves the same artifacts and executes exact `docker compose restart`; it does not pull or recreate.
- `compose.FakeComposeRunner` and injected GPU detectors provide useful test seams.

### Legacy PM2 installation

No PM2, `ecosystem.config.*`, `pm2 list/jlist`, PM2 process-name, or legacy deployment path is present in the repository. No existing package or command abstraction detects it. Therefore the requirement “backend deployed via PM2” cannot currently be implemented by reusing a known signal; the change needs an explicit legacy detector and a documented detection contract.

Likely signals to investigate and make injectable, preferably in a Linux-specific detector:

- `pm2 jlist` or `pm2 list` availability and successful output.
- A PM2 process whose name or script/cwd identifies the Alice backend. A generic “any PM2 process exists” check is unsafe and must not classify a host as Alice legacy.
- Known legacy deployment directories, ecosystem files, or package metadata, if confirmed from deployment history or operator documentation.
- The process command/script and cwd should be recorded as evidence for the migration screen, not silently inferred from a weak name match.

On Windows, PM2 detection should be explicitly unsupported or return “not detected” unless a cross-platform legacy contract is established. The detector must not shell out directly from the TUI; use a small command/process interface so tests never depend on a real PM2 installation.

## Requested action matrix and unresolved precedence

| Detected state | Actions to show |
| --- | --- |
| No current installation and no legacy installation | Install |
| Current installation only | Update, Uninstall |
| Legacy PM2 installation only | Migration |
| Current and legacy both detected | **Open product decision**; do not silently choose or delete either |

The combined state is the principal safety edge case. Recommended exploration outcome for proposal/spec: show both findings and require an explicit migration/cleanup decision, or show a conservative “conflicting installations detected” screen with no destructive action. Do not expose Uninstall as an automatic answer to a legacy/current conflict. Migration should not start until its source, destination, ownership, backup, and rollback semantics are defined.

Other edge cases to specify:

- Current artifacts are partial, unreadable, directories instead of files, or belong to an unknown Compose project.
- PM2 exists but no Alice-specific process can be proven.
- PM2 command is missing, exits non-zero, returns malformed JSON, or is inaccessible due to permissions.
- Docker/Compose is unavailable while current artifacts exist: Update/Uninstall may still be listed, but execution must fail safely and explain what was not changed.
- Current deployment is stopped: it is still a current installation; action selection must not depend on health status.
- Multiple workspaces or an explicitly supplied `--workspace-dir`: detection scope and precedence need a single source of truth.
- A user selects Install while stale artifacts exist: this must not overwrite files or data without an explicit replacement/repair policy.
- User cancels or presses `q`/Esc: no command, filesystem, or process side effect.

## Update/restart and uninstall implications

Update and restart already have non-interactive CLI routes but no interactive action adapters. The menu needs a clear boundary between selecting an action and running it. Options include routing selected Update to the existing update runner and selected Restart only if separately offered; the user requirement specifically says Update, so restart should remain an explicit CLI feature unless product scope adds it to the menu.

Uninstall is currently only documented in `RUNBOOK.md` as manual `docker compose down -v`, removal of `/opt/alice-media` and `/opt/alice-config`, and binary removal. It is explicitly destructive because `down -v` removes database volumes. Any TUI Uninstall path requires:

- A dedicated operation package/runner, not ad-hoc shell strings in a view.
- A confirmation screen that names exact workspace/config/data paths and whether volumes will be deleted.
- A backup or at least an explicit data-loss acknowledgement policy.
- Ownership checks and path safety guards; never recursively remove arbitrary user-provided paths without validation.
- Idempotent, observable phases and failure reporting so partial cleanup is not reported as success.
- Tests proving cancellation causes no calls and that path/volume safeguards reject unsafe inputs.

Migration is also potentially destructive and should be treated at least as risky as uninstall. It needs a source backup, dry-run/preview, explicit destination, and rollback strategy before implementation planning can be considered complete.

## Likely affected packages/files

- `internal/tui/model.go`, `messages.go`, `splash.go`: add the post-splash menu state, transitions, rendering, and action messages; preserve the existing install flow after Install.
- New `internal/tui/menu.go` (or equivalent): pure menu selection model with direct `Model.Update` tests.
- New detection package, likely `internal/installation` or `internal/management`: current-artifact and PM2 probes, typed state/evidence, error classification, and a fake detector.
- `internal/workspace/artifacts.go`: likely share or extend artifact validation without making update/restart semantics less strict.
- `internal/update` and possibly `internal/restart`: adapters for menu actions, preserving existing pull/up and exact restart contracts.
- New uninstall and migration packages: operation interfaces, production command/filesystem implementations, and fakes. These should not be embedded in `internal/tui`.
- `cmd/installer/main.go`: wire production detection and operation dependencies while leaving explicit `update`, `restart`, unattended, and dry-run routing unchanged.
- Tests: TUI model/full-flow/golden/resize tests; detector table tests; operation safety tests; CLI regression tests.
- `README.md` and `RUNBOOK.md`: action semantics, detected-state rules, legacy support limits, backup/data-loss warnings, and conflict handling.

Cross-platform impact is material: current workspace detection is portable, but PM2 process probing, Docker/Compose execution, filesystem permissions, and recursive cleanup differ on Linux versus Windows. Linux amd64/arm64 should be the first supported legacy-management target; Windows should have an explicit conservative behavior rather than a false positive.

## Testing seams and recommended coverage

Follow the repository's strict TDD and Go-testing conventions:

- Pure `ClassifyInstallationState` / action-matrix logic: table-driven tests for none/current/legacy/both, partial artifacts, detector errors, and ambiguous legacy evidence.
- Filesystem detection: `t.TempDir()` with regular files, directories, missing files, permission/error cases, and workspace override cases.
- PM2 detection: inject a small command runner returning fixture JSON, command-not-found, non-zero exit, malformed output, unrelated processes, and one valid Alice process. Do not invoke real PM2 in unit tests.
- TUI state transitions: call `Model.Update()` directly with messages and key events; assert state, selected action, and emitted commands. Preserve tests that currently assert splash Enter → `PreflightStartedMsg` by updating them to the new expected intermediate menu behavior.
- Menu rendering: golden snapshot for the three normal action sets and the conflict/error view; update only through the repository's golden update path.
- Update/restart adapters: use existing compose fakes and assert exact call order and no unintended fallback.
- Uninstall/migration: fake command and filesystem executors; assert cancellation and failed preconditions produce zero destructive calls, confirmations are mandatory, paths are validated, and partial failures are surfaced.
- End-to-end TUI flow: add a focused teatest scenario for splash → detection → Install and at least one existing-install action. External PM2/Docker integration should be skippable in short tests.

## Recommendation and readiness

The change is structurally feasible, but proposal work must first settle two product contracts: (1) what exact filesystem/process evidence identifies a current or legacy Alice installation, and (2) what the menu does when current and legacy installations coexist. The safest implementation direction is a typed, injectable detector with an explicit `unknown/conflict` state, a post-splash menu, reuse of existing update/restart artifact resolution, and separate confirmation-heavy operation packages for uninstall and migration. There is no evidence in this repository that PM2 legacy signals or migration semantics are already defined.
