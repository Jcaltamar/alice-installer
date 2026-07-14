package compose

import "context"

// Call records a compose invocation for assertion in tests.
type Call struct {
	Files   []string
	EnvFile string
}

// FakeComposeRunner is a test double for ComposeRunner.
// Set the exported fields to control what each method returns.
type FakeComposeRunner struct {
	VersionVal       Version
	VersionErr       error
	PullProgressMsgs []PullProgressMsg
	PullErr          error
	UpProgressMsgs   []UpProgressMsg
	UpErr            error
	RestartErr       error
	DownErr          error
	StopServiceErr   error
	StartServiceErr  error
	// Healths is the slice of ServiceHealth returned by HealthStatus.
	// Both Status (Health column) and State (lifecycle column) are honoured
	// by compose.IsReady — set both fields in tests that exercise the
	// State-aware acceptance rule.
	Healths   []ServiceHealth
	HealthErr error

	PullCalls         []Call
	UpCalls           []Call
	RestartCalls      []Call
	StopServiceCalls  []Call
	StartServiceCalls []Call

	CallOrder []string
}

// Version returns VersionVal, VersionErr.
func (f *FakeComposeRunner) Version(_ context.Context) (Version, error) {
	return f.VersionVal, f.VersionErr
}

// Pull sends PullProgressMsgs to the progress channel then returns PullErr.
func (f *FakeComposeRunner) Pull(_ context.Context, files []string, envFile string, progress chan<- PullProgressMsg) error {
	f.PullCalls = append(f.PullCalls, Call{})
	if len(f.PullCalls) > 0 {
		last := len(f.PullCalls) - 1
		f.PullCalls[last] = Call{Files: append([]string(nil), files...), EnvFile: envFile}
	}
	f.CallOrder = append(f.CallOrder, "pull")
	for _, m := range f.PullProgressMsgs {
		progress <- m
	}
	return f.PullErr
}

// Up sends UpProgressMsgs to the progress channel then returns UpErr.
func (f *FakeComposeRunner) Up(_ context.Context, files []string, envFile string, progress chan<- UpProgressMsg) error {
	f.UpCalls = append(f.UpCalls, Call{})
	if len(f.UpCalls) > 0 {
		last := len(f.UpCalls) - 1
		f.UpCalls[last] = Call{Files: append([]string(nil), files...), EnvFile: envFile}
	}
	f.CallOrder = append(f.CallOrder, "up")
	for _, m := range f.UpProgressMsgs {
		progress <- m
	}
	return f.UpErr
}

// Restart records call args and returns RestartErr.
func (f *FakeComposeRunner) Restart(_ context.Context, files []string, envFile string) error {
	f.RestartCalls = append(f.RestartCalls, Call{})
	if len(f.RestartCalls) > 0 {
		last := len(f.RestartCalls) - 1
		f.RestartCalls[last] = Call{Files: append([]string(nil), files...), EnvFile: envFile}
	}
	f.CallOrder = append(f.CallOrder, "restart")
	return f.RestartErr
}

// Down returns DownErr.
func (f *FakeComposeRunner) Down(_ context.Context, _ []string, _ string) error {
	return f.DownErr
}

// StopService records the allowlisted service call and returns StopServiceErr.
func (f *FakeComposeRunner) StopService(_ context.Context, files []string, envFile, service string) error {
	f.StopServiceCalls = append(f.StopServiceCalls, Call{Files: append([]string(nil), files...), EnvFile: envFile})
	f.CallOrder = append(f.CallOrder, "stop:"+service)
	return f.StopServiceErr
}

// StartService records the allowlisted service call and returns StartServiceErr.
func (f *FakeComposeRunner) StartService(_ context.Context, files []string, envFile, service string) error {
	f.StartServiceCalls = append(f.StartServiceCalls, Call{Files: append([]string(nil), files...), EnvFile: envFile})
	f.CallOrder = append(f.CallOrder, "start:"+service)
	return f.StartServiceErr
}

// HealthStatus returns Healths, HealthErr.
func (f *FakeComposeRunner) HealthStatus(_ context.Context, _ []string, _ string) ([]ServiceHealth, error) {
	return f.Healths, f.HealthErr
}
