# Proposal: installer-bootstrap

## Problem

The alice-installer preflight phase reports `FAIL` for `CheckMediaWritable` and `CheckConfigWritable` when the
installer runs as a non-root user on a fresh Linux host where `/opt/alice-media` and `/opt/alice-config` do not
yet exist and the current user cannot create them directly. Today the TUI displays a blocking error and the user
must exit, run a manual `sudo mkdir / chown` sequence, and re-launch — a friction point that accounts for the
majority of first-run support requests.

## Scope

A new `StateBootstrap` TUI state sits between `StatePreflight` and `StateWorkspaceInput`. When preflight
detects that ALL blocking failures are **filesystem directory failures for MediaDir or ConfigDir only**, the root
model routes to bootstrap instead of stopping. The bootstrap screen presents an explicit action list and asks the
user to confirm before running `sudo sh -c "mkdir -p … && chown -R USER:USER …"` via `tea.ExecProcess` — which
releases the alt-screen so the system sudo password prompt reaches the real TTY. On success the preflight is
automatically re-run; if it now passes the flow continues normally. The user may also decline with `N/Esc`, in
which case the original failing report is preserved and displayed on the preflight screen.

## Approach

`ClassifyBlockers` is a pure function in `internal/tui/bootstrap.go` that inspects a `preflight.Report` and
splits failures into `fixable []Action` (only `CheckMediaWritable` / `CheckConfigWritable`) and
`nonFixable []preflight.CheckResult`. Root `Model.Update` calls this on every `PreflightResultMsg`; if all
blockers are fixable the model transitions to `StateBootstrap`; if any non-fixable blocker exists it stays on
`StatePreflight` as today. A new `BootstrapModel` sub-model manages the confirm/execute/progress UI backed by an
injectable `Executor` interface (`ExecCmd(Action) tea.Cmd`). The production `teaExecutor` wraps `tea.ExecProcess`.
After all actions succeed a `BootstrapCompleteMsg` causes the root to return to `StatePreflight` and call
`preflight.Rearm()` + `Init()` so checks are re-evaluated without restarting the program.

## Out of scope (document in RUNBOOK instead)

- Docker group membership fix (`usermod -aG docker $USER`) — requires logout/re-login; not automatable inline.
- Docker daemon start (`sudo systemctl start docker`) — service management is out of installer scope.
- ARM/GPU-related permission fixes.
- Any check other than `CheckMediaWritable` and `CheckConfigWritable`.
