# Design: installer-bootstrap

## Classification function

```go
// ClassifyBlockers splits failing items in report into fixable and non-fixable sets.
// Fixable = only CheckMediaWritable or CheckConfigWritable with StatusFail.
// All other StatusFail items are non-fixable.
func ClassifyBlockers(report preflight.Report, mediaDir, configDir string) (fixable []Action, nonFixable []preflight.CheckResult)
```

The function iterates `report.Items`, ignores PASS/WARN, and for each FAIL checks the CheckID:
- `CheckMediaWritable` → add Action for mediaDir
- `CheckConfigWritable` → add Action for configDir
- anything else → add to nonFixable

---

## Action type

```go
type Action struct {
    ID          string   // matches CheckID being remediated
    Description string   // human-readable
    Command     string   // "sudo"
    Args        []string // ["sh", "-c", "mkdir -p DIR && chown -R USER:USER DIR"]
}
```

Command templates (USER resolved via `os/user.Current().Username` at classification time):
- MediaDir: `sudo sh -c "mkdir -p /opt/alice-media && chown -R USER:USER /opt/alice-media"`
- ConfigDir: `sudo sh -c "mkdir -p /opt/alice-config && chown -R USER:USER /opt/alice-config"`

---

## Executor interface

```go
type Executor interface {
    ExecCmd(action Action) tea.Cmd
}

// Production impl — releases alt-screen for sudo TTY.
type teaExecutor struct{}
func NewExecutor() Executor { return teaExecutor{} }
func (teaExecutor) ExecCmd(a Action) tea.Cmd {
    c := exec.Command(a.Command, a.Args...)
    return tea.ExecProcess(c, func(err error) tea.Msg {
        return BootstrapActionResultMsg{ActionID: a.ID, Err: err}
    })
}

// Test double.
type FakeExecutor struct {
    Results []BootstrapActionResultMsg
    calls   int
}
func (f *FakeExecutor) ExecCmd(a Action) tea.Cmd {
    idx := f.calls; f.calls++
    return func() tea.Msg { return f.Results[idx] }
}
```

---

## BootstrapModel

```go
type BootstrapModel struct {
    theme      theme.Theme
    executor   Executor
    actions    []Action
    currentIdx int
    results    []BootstrapActionResultMsg
    confirming bool   // true = awaiting Y/N; false = executing
    done       bool
    skipped    bool
    failed     *BootstrapFailedMsg
}
```

Update flow:
1. `confirming=true` + `KeyMsg{y/Enter}` → `confirming=false`, return `executor.ExecCmd(actions[0])`
2. `confirming=true` + `KeyMsg{n/Esc}` → return `func() tea.Msg { return BootstrapSkippedMsg{} }`
3. `BootstrapActionResultMsg{Err!=nil}` → return `func() tea.Msg { return BootstrapFailedMsg{...} }`
4. `BootstrapActionResultMsg{Err==nil}` + more actions → `currentIdx++`, return next `executor.ExecCmd`
5. `BootstrapActionResultMsg{Err==nil}` + no more actions → `done=true`, return `func() tea.Msg { return BootstrapCompleteMsg{} }`

---

## State machine addendum

```
StatePreflight
    |
    | PreflightResultMsg → all blockers fixable
    ↓
StateBootstrap  ←── new state (iota position: after StatePreflight)
    |                  |
    | BootstrapCompleteMsg   | BootstrapSkippedMsg
    ↓                        ↓
StatePreflight (rearm)   StatePreflight (frozen report + banner)
    |
    | PreflightPassedMsg (on re-preflight pass + Enter)
    ↓
StateWorkspaceInput
```

---

## Root model routing (PreflightResultMsg handler)

```go
case PreflightResultMsg:
    fixable, nonFixable := ClassifyBlockers(msg.Report, m.deps.MediaDir, m.deps.ConfigDir)
    if len(nonFixable) > 0 || (msg.Report.HasBlockingFailure() && len(fixable) == 0) {
        // Non-fixable failure — delegate to preflight sub-model as today.
        updated, cmd := m.preflight.Update(msg)
        m.preflight = updated
        return m, cmd
    }
    if len(fixable) > 0 {
        // All blockers are fixable → bootstrap.
        m.state = StateBootstrap
        m.bootstrap = NewBootstrapModel(m.deps.Theme, m.deps.Executor, fixable)
        return m, m.bootstrap.Init()
    }
    // No blockers → delegate to preflight (it will emit PreflightPassedMsg on Enter).
    updated, cmd := m.preflight.Update(msg)
    m.preflight = updated
    return m, cmd
```

## PreflightModel re-arm

Add `Rearm()` method that resets `report` and `err` to nil, returning a fresh `Init()` cmd:

```go
func (p *PreflightModel) Rearm() tea.Cmd {
    p.report = nil
    p.err = nil
    return p.Init()
}
```

Root calls this on `BootstrapCompleteMsg`.
