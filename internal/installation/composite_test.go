package installation

import (
	"context"
	"testing"
)

func TestCompositeDetector_EndToEndStates(t *testing.T) {
	tests := []struct {
		name            string
		current, legacy Presence
		want            State
	}{
		{"no evidence", PresenceAbsent, PresenceAbsent, StateNotInstalled},
		{"current", PresencePresent, PresenceAbsent, StateCurrent},
		{"legacy", PresenceAbsent, PresencePresent, StateLegacyPM2},
		{"conflict", PresencePresent, PresencePresent, StateConflict},
		{"partial workspace", PresenceUncertain, PresenceAbsent, StateUnknown},
		{"probe failure", PresenceAbsent, PresenceUncertain, StateUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := CompositeDetector{Current: fakeProbe{ProbeResult{Presence: tt.current}}, Legacy: fakeProbe{ProbeResult{Presence: tt.legacy}}}
			if got := detector.Detect(context.Background()).State; got != tt.want {
				t.Fatalf("Detect() = %v, want %v", got, tt.want)
			}
		})
	}
}
