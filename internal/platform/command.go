package platform

import (
	"bufio"
	"context"
	"os/exec"
)

// OSCommandRunner is the exported production CommandRunner backed by os/exec.
// It is the default used by CLIDocker and other packages that need shelling out.
type OSCommandRunner struct{}

// Run executes name+args and returns buffered stdout, stderr, and any error.
func (o *OSCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	var stderr []byte
	if exitErr, ok := err.(*exec.ExitError); ok {
		stderr = exitErr.Stderr
	}
	return out, stderr, err
}

// StreamingCommandRunner abstracts streaming command execution line by line.
// It is the seam used when compose pull/up must deliver real-time progress.
type StreamingCommandRunner interface {
	// Stream runs name+args and calls onStdout for each stdout line and
	// onStderr for each stderr line. It blocks until the command finishes.
	// Returns the first execution error (non-zero exit or OS error).
	Stream(ctx context.Context, onStdout, onStderr func(string), name string, args ...string) error
}

// OSStreamingCommandRunner is the production StreamingCommandRunner.
type OSStreamingCommandRunner struct{}

// Stream executes name+args and fans out stdout/stderr line by line via
// onStdout and onStderr callbacks respectively.
func (o *OSStreamingCommandRunner) Stream(
	ctx context.Context,
	onStdout, onStderr func(string),
	name string,
	args ...string,
) error {
	cmd := exec.CommandContext(ctx, name, args...)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	// Drain stdout in a goroutine.
	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			onStdout(scanner.Text())
		}
	}()

	// Drain stderr synchronously in the calling goroutine.
	scanner := bufio.NewScanner(stderrPipe)
	for scanner.Scan() {
		onStderr(scanner.Text())
	}

	<-done
	return cmd.Wait()
}

// ---------------------------------------------------------------------------
// Fakes for unit testing
// ---------------------------------------------------------------------------

// FakeCmdOutput holds the pre-configured response for FakeCommandRunner.
type FakeCmdOutput struct {
	Stdout []byte
	Stderr []byte
	Err    error
}

// FakeCommandRunner is a test double for the CommandRunner interface.
// Keyed by command name (first arg to Run).
type FakeCommandRunner struct {
	Outputs map[string]FakeCmdOutput
}

// Run returns the pre-configured output for the given command name.
// If no entry exists, it returns nil/nil/nil.
func (f *FakeCommandRunner) Run(_ context.Context, name string, _ ...string) ([]byte, []byte, error) {
	if f.Outputs == nil {
		return nil, nil, nil
	}
	if out, ok := f.Outputs[name]; ok {
		return out.Stdout, out.Stderr, out.Err
	}
	return nil, nil, nil
}

// FakeStreamingCommandRunner is a test double for StreamingCommandRunner.
// It sends Lines to onStdout then returns Err.
type FakeStreamingCommandRunner struct {
	Lines []string
	Err   error
}

// Stream delivers Lines one by one to onStdout, then returns Err.
func (f *FakeStreamingCommandRunner) Stream(
	_ context.Context,
	onStdout, _ func(string),
	_ string,
	_ ...string,
) error {
	for _, line := range f.Lines {
		onStdout(line)
	}
	return f.Err
}
