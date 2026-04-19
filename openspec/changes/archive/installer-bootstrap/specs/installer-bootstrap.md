# Spec: installer-bootstrap (delta)

## REQ-BS-1 — Detection of fixable vs non-fixable blockers

`ClassifyBlockers(report preflight.Report) (fixable []Action, nonFixable []preflight.CheckResult)` must
distinguish fixable from non-fixable blocking failures.

**Scenario 1 — all blockers fixable**
GIVEN a report with only `CheckMediaWritable=FAIL` and `CheckConfigWritable=FAIL`
WHEN `ClassifyBlockers` is called
THEN `fixable` has 2 entries (one per directory) and `nonFixable` is empty.

**Scenario 2 — mixed blockers**
GIVEN a report with `CheckDockerDaemon=FAIL` and `CheckMediaWritable=FAIL`
WHEN `ClassifyBlockers` is called
THEN `nonFixable` has 1 entry (docker) and `fixable` has 1 entry (media).

**Scenario 3 — no blockers**
GIVEN a report where every item is PASS or WARN
WHEN `ClassifyBlockers` is called
THEN both slices are empty.

---

## REQ-BS-2 — Action list rendering and user confirmation

The bootstrap screen must render the action list and await explicit user confirmation before executing.

**Scenario 1 — confirming screen shows actions**
GIVEN a BootstrapModel with 2 actions and `confirming=true`
WHEN `View()` is called
THEN the output contains each action's Description and a prompt containing "Y" and "N".

**Scenario 2 — pressing Y transitions to executing**
GIVEN a BootstrapModel in confirming state
WHEN the user presses `y`
THEN `confirming` becomes false and the executor's `ExecCmd` is called for the first action.

**Scenario 3 — pressing N emits BootstrapSkippedMsg**
GIVEN a BootstrapModel in confirming state
WHEN the user presses `n` or `Esc`
THEN the model emits `BootstrapSkippedMsg` and does not call the executor.

---

## REQ-BS-3 — Auto-elevation via tea.ExecProcess

The production executor must release the alt-screen so the sudo password prompt reaches the real TTY.

**Scenario 1 — ExecCmd wraps tea.ExecProcess**
GIVEN the real `teaExecutor`
WHEN `ExecCmd(action)` is called
THEN it returns a `tea.Cmd` produced by `tea.ExecProcess` with the action's Command+Args.

**Scenario 2 — successful execution emits BootstrapCompleteMsg**
GIVEN a BootstrapModel with 1 action and a FakeExecutor returning `Err=nil`
WHEN the model processes the `BootstrapActionResultMsg`
THEN `BootstrapCompleteMsg` is emitted.

**Scenario 3 — failed execution emits BootstrapFailedMsg**
GIVEN a BootstrapModel with 1 action and a FakeExecutor returning `Err=some_error`
WHEN the model processes the `BootstrapActionResultMsg`
THEN `BootstrapFailedMsg{ActionID, Err}` is emitted.

---

## REQ-BS-4 — Automatic re-preflight; abort preserves original report

**Scenario 1 — BootstrapCompleteMsg triggers re-preflight**
GIVEN the root Model is in `StateBootstrap`
WHEN `BootstrapCompleteMsg` is received
THEN state transitions to `StatePreflight`, `PreflightModel` is rearmed, and `Init()` is called.

**Scenario 2 — BootstrapSkippedMsg preserves report on preflight screen**
GIVEN the root Model is in `StateBootstrap` with the original failing report stored
WHEN `BootstrapSkippedMsg` is received
THEN state transitions to `StatePreflight` with the original report frozen and a declined banner visible.

**Scenario 3 — re-preflight that now passes continues to workspace-input**
GIVEN the installer went through bootstrap → re-preflight
WHEN the re-preflight report has no blocking failures
THEN pressing Enter advances to `StateWorkspaceInput` as in the normal path.
