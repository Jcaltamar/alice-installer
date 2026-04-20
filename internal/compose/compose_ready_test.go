package compose_test

import (
	"testing"

	"github.com/jcaltamar/alice-installer/internal/compose"
)

// TestIsReady covers all 7 scenarios from the installer-service-verify spec.
func TestIsReady(t *testing.T) {
	tests := []struct {
		name   string
		status string
		state  string
		want   bool
	}{
		{
			name:   "healthy+running → ready",
			status: "healthy",
			state:  "running",
			want:   true,
		},
		{
			name:   "empty health+running (no healthcheck) → ready",
			status: "",
			state:  "running",
			want:   true,
		},
		{
			name:   "none health+running (no healthcheck) → ready",
			status: "none",
			state:  "running",
			want:   true,
		},
		{
			name:   "empty health+restarting (crash-loop) → NOT ready",
			status: "",
			state:  "restarting",
			want:   false,
		},
		{
			name:   "unhealthy+running → NOT ready",
			status: "unhealthy",
			state:  "running",
			want:   false,
		},
		{
			name:   "healthy+exited → NOT ready",
			status: "healthy",
			state:  "exited",
			want:   false,
		},
		{
			name:   "empty health+empty state (unknown) → NOT ready",
			status: "",
			state:  "",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := compose.ServiceHealth{Status: tt.status, State: tt.state}
			got := compose.IsReady(s)
			if got != tt.want {
				t.Errorf("IsReady(%+v) = %v, want %v", s, got, tt.want)
			}
		})
	}
}
