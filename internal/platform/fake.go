package platform

import "context"

// FakeArchDetector is a test double for ArchDetector.
// Set Arch to control what Detect() returns.
type FakeArchDetector struct {
	Arch Arch
}

// Detect returns the configured Arch value.
func (f *FakeArchDetector) Detect() Arch {
	return f.Arch
}

// FakeOSGuard is a test double for OSGuard.
// Set Linux and Name to control the returned values.
type FakeOSGuard struct {
	Linux bool
	Name  string
}

// IsLinux returns the configured Linux field.
func (f *FakeOSGuard) IsLinux() bool {
	return f.Linux
}

// OSName returns the configured Name field.
func (f *FakeOSGuard) OSName() string {
	return f.Name
}

// FakeGPUDetector is a test double for GPUDetector.
// Set Info to control what Detect() returns.
type FakeGPUDetector struct {
	Info GPUInfo
}

// Detect returns the configured GPUInfo.
func (f *FakeGPUDetector) Detect(_ context.Context) GPUInfo {
	return f.Info
}
