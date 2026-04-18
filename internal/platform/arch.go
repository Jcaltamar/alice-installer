package platform

import "runtime"

// Arch represents the CPU architecture.
type Arch string

const (
	ArchAMD64   Arch = "amd64"
	ArchARM64   Arch = "arm64"
	ArchUnknown Arch = "unknown"
)

// ArchDetector detects the CPU architecture.
type ArchDetector interface {
	Detect() Arch
}

// goArchFunc is a function that returns the current GOARCH string.
// It is injectable so tests can swap it without globals.
type goArchFunc func() string

// RuntimeArchDetector reads the GOARCH at call time via an injectable func.
type RuntimeArchDetector struct {
	goarch goArchFunc
}

// NewRuntimeArchDetector creates a detector with the supplied goarch func.
// Pass nil to use runtime.GOARCH (production default).
func NewRuntimeArchDetector(fn goArchFunc) *RuntimeArchDetector {
	if fn == nil {
		fn = func() string { return runtime.GOARCH }
	}
	return &RuntimeArchDetector{goarch: fn}
}

// Detect returns the Arch for the current runtime.
func (d *RuntimeArchDetector) Detect() Arch {
	switch d.goarch() {
	case "amd64":
		return ArchAMD64
	case "arm64":
		return ArchARM64
	default:
		return ArchUnknown
	}
}
