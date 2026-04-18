package tui

import (
	"testing"

	"github.com/jcaltamar/alice-installer/internal/preflight"
)

// TestPreflightRearmResetsReport verifies that Rearm() sets the report to nil.
func TestPreflightRearmResetsReport(t *testing.T) {
	m := newTestPreflight(buildTestCoordinator(true, true, true))
	existingReport := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckOS, Status: preflight.StatusPass, Title: "OS"},
		},
	}
	m.report = &existingReport

	m.Rearm()

	if m.report != nil {
		t.Error("Rearm() should reset report to nil")
	}
}

// TestPreflightRearmReturnsNonNilCmd verifies that Rearm() returns a non-nil cmd.
func TestPreflightRearmReturnsNonNilCmd(t *testing.T) {
	m := newTestPreflight(buildTestCoordinator(true, true, true))
	cmd := m.Rearm()
	if cmd == nil {
		t.Fatal("Rearm() should return a non-nil cmd")
	}
}

// TestPreflightRearmCmdProducesPreflightResultMsg verifies the cmd result type.
func TestPreflightRearmCmdProducesPreflightResultMsg(t *testing.T) {
	m := newTestPreflight(buildTestCoordinator(true, true, true))
	cmd := m.Rearm()
	msg := cmd()
	if _, ok := msg.(PreflightResultMsg); !ok {
		t.Errorf("Rearm() cmd should produce PreflightResultMsg, got %T", msg)
	}
}
