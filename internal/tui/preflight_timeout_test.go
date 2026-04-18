package tui

import (
	"testing"
	"time"
)

// TestPreflightTimeout_IsReasonable guards against the classic Go bug of passing
// a bare integer to context.WithTimeout (nanoseconds) instead of multiplying by
// time.Second. If a future refactor drops the multiplier, this test fails fast.
func TestPreflightTimeout_IsReasonable(t *testing.T) {
	if preflightTimeout < 10*time.Second {
		t.Fatalf("preflightTimeout too small: %v — did you forget to multiply by time.Second?", preflightTimeout)
	}
	if preflightTimeout > 5*time.Minute {
		t.Fatalf("preflightTimeout too large: %v — preflight should not hang the TUI for minutes", preflightTimeout)
	}
}
