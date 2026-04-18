package tui

import (
	"errors"
	"testing"
)

// TestMessagesCompile is a compile-time check that verifies every message type
// defined in messages.go can be instantiated and has the expected fields.
// It is NOT a behavioural test — the only assertion is that the code compiles.
func TestMessagesCompile(t *testing.T) {
	// Global messages
	_ = ErrorMsg{Err: errors.New("boom"), Fatal: true}
	_ = AbortMsg{}
	_ = QuitMsg{}

	// Preflight
	_ = PreflightStartedMsg{}

	// Workspace
	_ = WorkspaceEnteredMsg{Value: "my-site"}

	// Port scan
	_ = PortScanResultMsg{
		Conflicts: []PortConflict{{Key: "POSTGRES_PORT", Requested: 5432, Reason: "occupied"}},
		FreePlan:  map[string]int{"POSTGRES_PORT": 5432},
	}
	_ = PortConflict{Key: "POSTGRES_PORT", Requested: 5432, Reason: "occupied"}
	_ = PortResolvedMsg{Key: "POSTGRES_PORT", Chosen: 5433}
	_ = PortsConfirmedMsg{FinalPorts: map[string]int{"POSTGRES_PORT": 5432}}

	// Env write
	_ = EnvWrittenMsg{Path: "/tmp/test/.env"}

	// Compose
	_ = PullStartedMsg{}
	_ = PullCompleteMsg{}
	_ = DeployStartedMsg{}
	_ = DeployCompleteMsg{}

	// Health
	_ = HealthTickMsg{}

	// Result
	_ = InstallSuccessMsg{WorkspaceDir: "/tmp/ws", EnvPath: "/tmp/.env"}
	_ = InstallFailureMsg{Err: errors.New("fail"), Stage: "deploy"}

	t.Log("all message types compile correctly")
}

// TestPreflightResultMsg checks that PreflightResultMsg has the expected Report field.
func TestPreflightResultMsg(t *testing.T) {
	msg := PreflightResultMsg{}
	// Report is zero-value by default; just ensure the struct has the field.
	if msg.Report.HasBlockingFailure() {
		t.Error("empty report should not have blocking failures")
	}
}

// TestHealthReportMsg checks that HealthReportMsg has the expected fields.
func TestHealthReportMsg(t *testing.T) {
	msg := HealthReportMsg{Done: true}
	if !msg.Done {
		t.Error("Done field should be true")
	}
	if len(msg.Services) != 0 {
		t.Error("Services should be empty by default")
	}
}
