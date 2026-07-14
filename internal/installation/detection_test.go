package installation

import (
	"context"
	"testing"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name            string
		current, legacy ProbeResult
		want            State
	}{
		{"absent probes", ProbeResult{Presence: PresenceAbsent}, ProbeResult{Presence: PresenceAbsent}, StateNotInstalled},
		{"unsupported legacy", ProbeResult{Presence: PresenceAbsent}, ProbeResult{Presence: PresenceUnsupported}, StateNotInstalled},
		{"unsupported current", ProbeResult{Presence: PresenceUnsupported}, ProbeResult{Presence: PresencePresent}, StateUnknown},
		{"current", ProbeResult{Presence: PresencePresent}, ProbeResult{Presence: PresenceUnsupported}, StateCurrent},
		{"legacy", ProbeResult{Presence: PresenceAbsent}, ProbeResult{Presence: PresencePresent}, StateLegacyPM2},
		{"conflict", ProbeResult{Presence: PresencePresent}, ProbeResult{Presence: PresencePresent}, StateConflict},
		{"current uncertainty wins", ProbeResult{Presence: PresenceUncertain}, ProbeResult{Presence: PresencePresent}, StateUnknown},
		{"legacy uncertainty wins", ProbeResult{Presence: PresenceAbsent}, ProbeResult{Presence: PresenceUncertain}, StateUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.current, tt.legacy).State; got != tt.want {
				t.Fatalf("Classify() = %v, want %v", got, tt.want)
			}
		})
	}
}

type fakeProbe struct{ result ProbeResult }

func (f fakeProbe) Probe(context.Context) ProbeResult { return f.result }

func TestCompositeDetector_CombinesAndSortsEvidence(t *testing.T) {
	detector := CompositeDetector{
		Current: fakeProbe{ProbeResult{Presence: PresencePresent, Evidence: []Evidence{{Kind: EvidenceWorkspaceComplete, Source: "workspace", Path: "/z"}}}},
		Legacy:  fakeProbe{ProbeResult{Presence: PresencePresent, Evidence: []Evidence{{Kind: EvidencePM2AliceProcess, Source: "pm2", Path: "/a"}}}},
	}
	got := detector.Detect(context.Background())
	if got.State != StateConflict {
		t.Fatalf("state = %v, want conflict", got.State)
	}
	if len(got.Evidence) != 2 || got.Evidence[0].Kind != EvidenceWorkspaceComplete || got.Evidence[1].Kind != EvidencePM2AliceProcess {
		t.Fatalf("evidence = %#v, want workspace then pm2", got.Evidence)
	}
}
