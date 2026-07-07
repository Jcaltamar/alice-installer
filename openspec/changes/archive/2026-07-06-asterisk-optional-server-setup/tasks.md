# Tasks: Optional Asterisk SIP Audio Server Setup

TDD-paired list. Every implementation task is preceded by a RED test task.

---

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 650-900 |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 → Asterisk installer core and rollback; PR 2 → TUI/env/assets wiring; PR 3 → CLI wiring and regression coverage |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Stand up the Asterisk installer core with explicit AMI contract and rollback snapshotting | PR 1 | Isolated `internal/asterisk` package; tests and fakes included |
| 2 | Wire the optional package UX and generated env/assets contract | PR 2 | TUI selection flow, env rendering, and shared handoff files |
| 3 | Connect CLI runtime wiring and prove end-to-end regression behavior | PR 3 | `cmd/installer`, full-flow tests, skipped-path and failure recovery coverage |

## Phase 1: Asterisk Installer Foundation

- [x] 1.1 **RED** Add table-driven tests in `internal/asterisk/*_test.go` asserting that supported hosts are accepted only when the concrete Linux/package-manager/systemd detector says the host is supported, and that unsupported OS/package-manager/systemd combinations fail fast with actionable errors.
- [x] 1.2 **GREEN** Create `internal/asterisk/{interfaces.go,state.go,templates.go,installer.go,fake.go}` with a single `Options`/contract object, host-state snapshotting, concrete support detection, and deterministic renderers for the AMI/env handoff values.
- [x] 1.3 **RED** Add tests that prove the AMI credential source-of-truth is shared across the generated `.env`, compose env, and `/opt/alice-config/asterisk/integration.env`, and that the same input object renders the same credentials everywhere.
- [x] 1.4 **GREEN** Implement the shared AMI credential contract and rendering path so `.env`, compose env, and `/opt/alice-config/asterisk/integration.env` cannot diverge.

## Phase 2: Host Setup, Verification, and Rollback

- [x] 2.1 **RED** Add tests in `internal/asterisk/installer_test.go` for the selected-install path: package install is attempted, managed config sections are written without clobbering operator content, service is enabled/restarted, localhost AMI verification runs against `127.0.0.1:5038`, and a failed verification returns an optional-setup failure.
- [x] 2.2 **GREEN** Implement the install orchestration, managed-section editing, service control, localhost AMI verification, and failure reporting for the optional Asterisk setup.
- [x] 2.3 **RED** Add rollback-focused tests that start from an already-installed/operator-managed Asterisk instance and prove rollback does not disable, uninstall, or rewrite pre-existing service/config state outside alice-managed markers.
- [x] 2.4 **GREEN** Implement rollback bookkeeping and restoration logic that restores only installer-owned changes and preserves pre-existing package/service/config state.
- [x] 2.5 **RED** Add tests that validate secure filesystem handling for `/opt/alice-config/asterisk` resources, including secure permissions and the expected bundle layout for `integration.env`, templates, sounds, and recordings.
- [x] 2.6 **GREEN** Implement the shared resource creation path and permissions handling for `/opt/alice-config/asterisk` so the backend can read the bundle safely from the host mount.

## Phase 3: TUI, Env Generation, and Assets Wiring

- [x] 3.1 **RED** Add Bubble Tea tests in `internal/tui/messages_test.go`, `internal/tui/model_test.go`, and new Asterisk-specific TUI tests asserting that `Optional Packages -> Asterisk SIP Audio Server` is visible, unchecked by default, skipped cleanly when unselected, and transitions from env-write into `StateAsteriskSetup` when selected.
- [x] 3.2 **GREEN** Update `internal/tui/messages.go`, `internal/tui/model.go`, and new `internal/tui/optional_packages.go` / `internal/tui/asterisk_setup.go` to carry the optional-package state, emit the new messages, and run Asterisk setup only when selected.
- [x] 3.3 **RED** Add tests in `internal/envgen/env_test.go` and `internal/assets/*_test.go` proving the optional Asterisk contract is rendered from one shared input, the backend mount path remains `/opt/alice-config:/opt/alice-config`, and the embedded compose/env assets include the Asterisk references without breaking the base install.
- [x] 3.4 **GREEN** Update `internal/envgen/env.go`, `internal/assets/.env.example`, `internal/assets/docker-compose.yml`, `internal/assets/assets.go`, and any new Asterisk resource templates so the generated `.env`, compose env, and `/opt/alice-config/asterisk/integration.env` all point to the same credentials and shared resource layout.

## Phase 4: CLI Wiring and Regression Coverage

- [x] 4.1 **RED** Add integration-style tests in `internal/tui/fullflow_*_test.go` and `cmd/installer/main_test.go` covering the selected path, the skipped path, non-Linux/unsupported-package-manager gating, and the regression that the installer still behaves as a normal base install when Asterisk is not selected.
- [x] 4.2 **GREEN** Wire `cmd/installer/main.go` and any remaining dependency injection so the optional package flow is reachable from the CLI, unsupported hosts hide or disable the option, and the happy path reaches pull/deploy only after Asterisk setup succeeds.
- [x] 4.3 **RED** Add a final failure-recovery regression test that simulates partial Asterisk installation and asserts the installer reports the optional package as failed instead of silently continuing.
- [x] 4.4 **GREEN** Polish any final guards or fixtures needed to keep the base install path unchanged when Asterisk is skipped or unsupported.

## Phase 5: Final Verification

- [x] 5.1 **RED** Add or update any narrow regression tests needed to prove that AMI credentials, rollback state, and Linux gating stay stable across reruns.
- [x] 5.2 **GREEN** Run the final targeted test suite for the touched packages and verify the change stays strict-TDD clean.
