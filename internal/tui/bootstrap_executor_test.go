package tui

import (
	"errors"
	"testing"
)

func TestNewExecutorReturnsNonNil(t *testing.T) {
	exec := NewExecutor()
	if exec == nil {
		t.Fatal("NewExecutor() should return a non-nil Executor")
	}
}

func TestFakeExecutorExecCmdReturnsQueuedResult(t *testing.T) {
	sentinel := errors.New("fake error")
	fe := &FakeExecutor{
		Results: []BootstrapActionResultMsg{
			{ActionID: "check_media_writable", Err: nil},
			{ActionID: "check_config_writable", Err: sentinel},
		},
	}

	a1 := Action{ID: "check_media_writable", Command: "sudo", Args: []string{"sh", "-c", "mkdir /m"}}
	a2 := Action{ID: "check_config_writable", Command: "sudo", Args: []string{"sh", "-c", "mkdir /c"}}

	cmd1 := fe.ExecCmd(a1)
	if cmd1 == nil {
		t.Fatal("ExecCmd should return a non-nil cmd")
	}
	msg1 := cmd1()
	r1, ok := msg1.(BootstrapActionResultMsg)
	if !ok {
		t.Fatalf("first cmd should produce BootstrapActionResultMsg, got %T", msg1)
	}
	if r1.ActionID != "check_media_writable" {
		t.Errorf("first result ActionID = %q", r1.ActionID)
	}
	if r1.Err != nil {
		t.Errorf("first result Err should be nil, got %v", r1.Err)
	}

	cmd2 := fe.ExecCmd(a2)
	msg2 := cmd2()
	r2, ok := msg2.(BootstrapActionResultMsg)
	if !ok {
		t.Fatalf("second cmd should produce BootstrapActionResultMsg, got %T", msg2)
	}
	if r2.Err != sentinel {
		t.Errorf("second result Err = %v, want sentinel", r2.Err)
	}
}
