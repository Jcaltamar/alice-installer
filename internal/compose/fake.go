package compose

import "context"

// FakeComposeRunner is a test double for ComposeRunner.
// Set the exported fields to control what each method returns.
type FakeComposeRunner struct {
	VersionVal       Version
	VersionErr       error
	PullProgressMsgs []PullProgressMsg
	PullErr          error
	UpProgressMsgs   []UpProgressMsg
	UpErr            error
	DownErr          error
	// Healths is the slice of ServiceHealth returned by HealthStatus.
	// Both Status (Health column) and State (lifecycle column) are honoured
	// by compose.IsReady — set both fields in tests that exercise the
	// State-aware acceptance rule.
	Healths   []ServiceHealth
	HealthErr error
}

// Version returns VersionVal, VersionErr.
func (f *FakeComposeRunner) Version(_ context.Context) (Version, error) {
	return f.VersionVal, f.VersionErr
}

// Pull sends PullProgressMsgs to the progress channel then returns PullErr.
func (f *FakeComposeRunner) Pull(_ context.Context, _ []string, _ string, progress chan<- PullProgressMsg) error {
	for _, m := range f.PullProgressMsgs {
		progress <- m
	}
	return f.PullErr
}

// Up sends UpProgressMsgs to the progress channel then returns UpErr.
func (f *FakeComposeRunner) Up(_ context.Context, _ []string, _ string, progress chan<- UpProgressMsg) error {
	for _, m := range f.UpProgressMsgs {
		progress <- m
	}
	return f.UpErr
}

// Down returns DownErr.
func (f *FakeComposeRunner) Down(_ context.Context, _ []string, _ string) error {
	return f.DownErr
}

// HealthStatus returns Healths, HealthErr.
func (f *FakeComposeRunner) HealthStatus(_ context.Context, _ []string, _ string) ([]ServiceHealth, error) {
	return f.Healths, f.HealthErr
}
