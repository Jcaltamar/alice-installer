package tui

import (
	"testing"

	"github.com/jcaltamar/alice-installer/internal/preflight"
)

// buildTestDepsWithExecutor builds test deps that include a FakeExecutor.
func buildTestDepsWithExecutor(exec Executor) Dependencies {
	deps := buildTestDeps()
	deps.Executor = exec
	// Healthy env: docker present, user in group, systemd present.
	// Tests that need specific Docker scenarios override this.
	deps.Env = healthyEnv()
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
// Uses non-systemd env so CheckDockerDaemon FAIL is non-fixable.
func TestPreflightResultNonFixableBlockerStaysPreflight(t *testing.T) {
	fe := &FakeExecutor{}
	deps := buildTestDepsWithExecutor(fe)
	// Non-systemd stuck: docker binary present, user in group, but no systemd → non-fixable
	deps.Env = BootstrapEnv{
		UserName:            "testuser",
		DockerBinaryPresent: true,
		UserInDockerGroup:   true,
		SystemdPresent:      false,
	}
	m := NewModel(deps)
	m.state = StatePreflight

	report := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckDockerDaemon, Status: preflight.StatusFail, Title: "Docker"},
		},
	}
	updated, _ := m.Update(PreflightResultMsg{Report: report})
	m = updated.(Model)
	if m.state != StatePreflight {
		t.Errorf("non-fixable blocker (non-systemd) → state = %v, want StatePreflight", m.state)
	}
}

// TestPreflightResultDockerDaemonFixableGoesToBootstrap verifies Docker FAIL
// with a fixable env (binary missing) goes to StateBootstrap.
func TestPreflightResultDockerDaemonFixableGoesToBootstrap(t *testing.T) {
	fe := &FakeExecutor{Results: []BootstrapActionResultMsg{{ActionID: ActionIDDockerInstall, Err: nil}}}
	deps := buildTestDepsWithExecutor(fe)
	deps.Env = noDockerEnv() // binary missing → fixable
	m := NewModel(deps)
	m.state = StatePreflight

	report := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckDockerDaemon, Status: preflight.StatusFail, Title: "Docker"},
		},
	}
	updated, _ := m.Update(PreflightResultMsg{Report: report})
	m = updated.(Model)
	if m.state != StateBootstrap {
		t.Errorf("docker_install fixable → state = %v, want StateBootstrap", m.state)
	}
}

// TestPreflightResultMixedBlockersStaysPreflight uses compose fail (always non-fixable)
// + media fail (fixable) → mixed → StatePreflight.
func TestPreflightResultMixedBlockersStaysPreflight(t *testing.T) {
	fe := &FakeExecutor{}
	m := NewModel(buildTestDepsWithExecutor(fe))
	m.state = StatePreflight

	report := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckComposeVersion, Status: preflight.StatusFail, Title: "Compose"},
			{ID: preflight.CheckMediaWritable, Status: preflight.StatusFail, Title: "Media"},
		},
	}
	updated, _ := m.Update(PreflightResultMsg{Report: report})
	m = updated.(Model)
	if m.state != StatePreflight {
		t.Errorf("mixed blockers (compose+media) → state = %v, want StatePreflight", m.state)
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
