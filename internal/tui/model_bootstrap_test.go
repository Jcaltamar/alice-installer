package tui

import (
	"testing"

	"github.com/jcaltamar/alice-installer/internal/preflight"
)

// buildTestDepsWithExecutor builds test deps that include a FakeExecutor.
func buildTestDepsWithExecutor(exec Executor) Dependencies {
	deps := buildTestDeps()
	deps.Executor = exec
	return deps
}

// TestPreflightResultOnlyFixableBlockers → StateBootstrap.
func TestPreflightResultOnlyFixableBlockersToBootstrap(t *testing.T) {
	fe := &FakeExecutor{Results: []BootstrapActionResultMsg{{ActionID: "a", Err: nil}}}
	m := NewModel(buildTestDepsWithExecutor(fe))
	m.state = StatePreflight

	report := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckMediaWritable, Status: preflight.StatusFail, Title: "Media"},
		},
	}
	updated, _ := m.Update(PreflightResultMsg{Report: report})
	m = updated.(Model)
	if m.state != StateBootstrap {
		t.Errorf("only fixable blockers → state = %v, want StateBootstrap", m.state)
	}
}

// TestPreflightResultNonFixableBlocker → stays StatePreflight.
func TestPreflightResultNonFixableBlockerStaysPreflight(t *testing.T) {
	fe := &FakeExecutor{}
	m := NewModel(buildTestDepsWithExecutor(fe))
	m.state = StatePreflight

	report := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckDockerDaemon, Status: preflight.StatusFail, Title: "Docker"},
		},
	}
	updated, _ := m.Update(PreflightResultMsg{Report: report})
	m = updated.(Model)
	if m.state != StatePreflight {
		t.Errorf("non-fixable blocker → state = %v, want StatePreflight", m.state)
	}
}

// TestPreflightResultMixedBlockers → stays StatePreflight.
func TestPreflightResultMixedBlockersStaysPreflight(t *testing.T) {
	fe := &FakeExecutor{}
	m := NewModel(buildTestDepsWithExecutor(fe))
	m.state = StatePreflight

	report := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckDockerDaemon, Status: preflight.StatusFail, Title: "Docker"},
			{ID: preflight.CheckMediaWritable, Status: preflight.StatusFail, Title: "Media"},
		},
	}
	updated, _ := m.Update(PreflightResultMsg{Report: report})
	m = updated.(Model)
	if m.state != StatePreflight {
		t.Errorf("mixed blockers → state = %v, want StatePreflight", m.state)
	}
}

// TestPreflightResultNoBlockers → stays StatePreflight (delegates; preflight emits pass on Enter).
func TestPreflightResultNoBlockersStaysPreflight(t *testing.T) {
	fe := &FakeExecutor{}
	m := NewModel(buildTestDepsWithExecutor(fe))
	m.state = StatePreflight

	report := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckOS, Status: preflight.StatusPass, Title: "OS"},
		},
	}
	updated, _ := m.Update(PreflightResultMsg{Report: report})
	m = updated.(Model)
	if m.state != StatePreflight {
		t.Errorf("no blockers → state = %v, want StatePreflight", m.state)
	}
}

// TestBootstrapCompleteMsgTransitionsToPreflightRearmed.
func TestBootstrapCompleteMsgToPreflightRearmed(t *testing.T) {
	fe := &FakeExecutor{}
	m := NewModel(buildTestDepsWithExecutor(fe))
	m.state = StateBootstrap

	updated, cmd := m.Update(BootstrapCompleteMsg{})
	m = updated.(Model)
	if m.state != StatePreflight {
		t.Errorf("BootstrapCompleteMsg → state = %v, want StatePreflight", m.state)
	}
	if cmd == nil {
		t.Fatal("BootstrapCompleteMsg should return a cmd (preflight Init)")
	}
	// The cmd should produce PreflightResultMsg when run.
	msg := cmd()
	if _, ok := msg.(PreflightResultMsg); !ok {
		t.Errorf("rearmed preflight cmd should produce PreflightResultMsg, got %T", msg)
	}
}

// TestBootstrapSkippedMsgTransitionsToPreflightWithFrozenReport.
func TestBootstrapSkippedMsgToPreflightFrozen(t *testing.T) {
	fe := &FakeExecutor{}
	m := NewModel(buildTestDepsWithExecutor(fe))
	m.state = StateBootstrap

	// Pre-load the preflight with a failing report.
	failReport := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckMediaWritable, Status: preflight.StatusFail, Title: "Media"},
		},
	}
	m.preflight.report = &failReport

	updated, _ := m.Update(BootstrapSkippedMsg{})
	m = updated.(Model)
	if m.state != StatePreflight {
		t.Errorf("BootstrapSkippedMsg → state = %v, want StatePreflight", m.state)
	}
	if m.preflight.report == nil {
		t.Error("preflight report should be preserved after BootstrapSkippedMsg")
	}
}
