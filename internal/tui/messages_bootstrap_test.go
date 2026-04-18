package tui

import (
	"errors"
	"testing"
)

// TestActionPostActionBannerField verifies the PostActionBanner field exists and defaults to "".
func TestActionPostActionBannerField(t *testing.T) {
	a := Action{
		ID:      "test",
		Command: "sudo",
		Args:    []string{"echo"},
	}
	if a.PostActionBanner != "" {
		t.Errorf("PostActionBanner zero value should be empty, got %q", a.PostActionBanner)
	}
	a.PostActionBanner = "Log out and back in."
	if a.PostActionBanner != "Log out and back in." {
		t.Errorf("PostActionBanner should be settable, got %q", a.PostActionBanner)
	}
}

// TestBootstrapMessageTypes verifies that all bootstrap message types and Action
// compile correctly and are distinct types.
func TestBootstrapMessageTypes(t *testing.T) {
	t.Run("Action has required fields", func(t *testing.T) {
		a := Action{
			ID:          "check_media_writable",
			Description: "Create /opt/alice-media and grant ownership",
			Command:     "sudo",
			Args:        []string{"sh", "-c", "mkdir -p /opt/alice-media"},
		}
		if a.ID == "" {
			t.Error("Action.ID should not be empty")
		}
		if a.Command != "sudo" {
			t.Errorf("Action.Command = %q, want sudo", a.Command)
		}
		if len(a.Args) == 0 {
			t.Error("Action.Args should not be empty")
		}
	})

	t.Run("BootstrapNeededMsg contains actions slice", func(t *testing.T) {
		msg := BootstrapNeededMsg{Actions: []Action{{ID: "x"}}}
		if len(msg.Actions) != 1 {
			t.Errorf("BootstrapNeededMsg.Actions len = %d, want 1", len(msg.Actions))
		}
	})

	t.Run("BootstrapConfirmedMsg is a distinct zero-value type", func(t *testing.T) {
		var msg BootstrapConfirmedMsg
		_ = msg // just needs to compile
	})

	t.Run("BootstrapSkippedMsg is a distinct zero-value type", func(t *testing.T) {
		var msg BootstrapSkippedMsg
		_ = msg
	})

	t.Run("BootstrapActionResultMsg carries ActionID and Err", func(t *testing.T) {
		errSentinel := errors.New("sudo: failed")
		msg := BootstrapActionResultMsg{ActionID: "check_media_writable", Err: errSentinel}
		if msg.ActionID != "check_media_writable" {
			t.Errorf("ActionID = %q", msg.ActionID)
		}
		if msg.Err == nil {
			t.Error("Err should be non-nil")
		}
	})

	t.Run("BootstrapCompleteMsg is a distinct zero-value type", func(t *testing.T) {
		var msg BootstrapCompleteMsg
		_ = msg
	})

	t.Run("BootstrapFailedMsg carries ActionID and Err", func(t *testing.T) {
		msg := BootstrapFailedMsg{ActionID: "id", Err: errors.New("fail")}
		if msg.ActionID == "" {
			t.Error("BootstrapFailedMsg.ActionID empty")
		}
		if msg.Err == nil {
			t.Error("BootstrapFailedMsg.Err nil")
		}
	})

	t.Run("PreflightReRunMsg is a distinct zero-value type", func(t *testing.T) {
		var msg PreflightReRunMsg
		_ = msg
	})
}
