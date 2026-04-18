package platform

import "runtime"

// OSGuard detects the host operating system.
type OSGuard interface {
	IsLinux() bool
	OSName() string
}

// goosFunc is a function that returns the current GOOS string.
type goosFunc func() string

// RuntimeOSGuard reads GOOS at call time via an injectable func.
type RuntimeOSGuard struct {
	goos goosFunc
}

// NewRuntimeOSGuard creates an OSGuard with the supplied goos func.
// Pass nil to use runtime.GOOS (production default).
func NewRuntimeOSGuard(fn goosFunc) *RuntimeOSGuard {
	if fn == nil {
		fn = func() string { return runtime.GOOS }
	}
	return &RuntimeOSGuard{goos: fn}
}

// IsLinux returns true iff the current OS is Linux.
func (g *RuntimeOSGuard) IsLinux() bool {
	return g.goos() == "linux"
}

// OSName returns the raw GOOS string.
func (g *RuntimeOSGuard) OSName() string {
	return g.goos()
}
