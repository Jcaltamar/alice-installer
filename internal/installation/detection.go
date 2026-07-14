package installation

import (
	"context"
	"sort"
)

// State is the conservative result of installation detection.
type State uint8

const (
	StateNotInstalled State = iota
	StateCurrent
	StateLegacyPM2
	StateConflict
	StateUnknown
)

type EvidenceKind uint8

const (
	EvidenceWorkspaceComplete EvidenceKind = iota
	EvidenceWorkspaceAbsent
	EvidenceWorkspacePartial
	EvidenceWorkspaceInvalid
	EvidenceWorkspaceUnreadable
	EvidencePM2AliceProcess
	EvidencePM2Absent
	EvidencePM2Unavailable
	EvidencePM2Unsupported
	EvidencePM2Ambiguous
	EvidencePM2Failed
	EvidenceLegacyDirectory
	EvidenceLegacyDirectoryAbsent
	EvidenceLegacyDirectoryInvalid
	EvidenceLegacyDirectoryUnreadable
	EvidenceLegacyDirectoryUnsupported
)

// Evidence is safe to display: it never contains artifact contents or process metadata.
type Evidence struct {
	Kind   EvidenceKind
	Source string
	Detail string
	Path   string
}

type Detection struct {
	State    State
	Evidence []Evidence
}

type Presence uint8

const (
	PresenceAbsent Presence = iota
	PresencePresent
	PresenceUncertain
	PresenceUnsupported
)

type ProbeResult struct {
	Presence Presence
	Evidence []Evidence
}

type Probe interface {
	Probe(context.Context) ProbeResult
}

type Detector interface {
	Detect(context.Context) Detection
}

// CompositeDetector runs portable evidence before legacy evidence.
type CompositeDetector struct {
	Current Probe
	Legacy  Probe
}

func (d CompositeDetector) Detect(ctx context.Context) Detection {
	current := d.Current.Probe(ctx)
	legacy := d.Legacy.Probe(ctx)
	return Classify(current, legacy)
}

// Classify maps probe results to exactly one safe installation state.
func Classify(current, legacy ProbeResult) Detection {
	evidence := append(append([]Evidence{}, current.Evidence...), legacy.Evidence...)
	sort.SliceStable(evidence, func(i, j int) bool {
		if evidence[i].Source != evidence[j].Source {
			return evidence[i].Source == "workspace"
		}
		if evidence[i].Kind != evidence[j].Kind {
			return evidence[i].Kind < evidence[j].Kind
		}
		return evidence[i].Path < evidence[j].Path
	})
	state := StateUnknown
	switch {
	case current.Presence == PresenceUncertain || current.Presence == PresenceUnsupported || legacy.Presence == PresenceUncertain:
		state = StateUnknown
	case current.Presence == PresencePresent && legacy.Presence == PresencePresent:
		state = StateConflict
	case current.Presence == PresencePresent:
		state = StateCurrent
	case legacy.Presence == PresencePresent:
		state = StateLegacyPM2
	case current.Presence == PresenceAbsent && (legacy.Presence == PresenceAbsent || legacy.Presence == PresenceUnsupported):
		state = StateNotInstalled
	}
	return Detection{State: state, Evidence: evidence}
}
