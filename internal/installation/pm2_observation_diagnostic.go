package installation

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// PM2ObservationDiagnostic contains only reviewed, bounded operator context.
// Command stdout is deliberately absent because pm2 jlist contains environment data.
type PM2ObservationDiagnostic struct {
	Stage     string
	Operation string
	Command   string
	Cause     string
	Stderr    string
}

func (d PM2ObservationDiagnostic) String() string {
	parts := []string{
		"stage=" + d.Stage,
		"operation=" + d.Operation,
		"command=" + d.Command,
		"cause=" + d.Cause,
	}
	if d.Stderr != "" {
		parts = append(parts, "stderr="+d.Stderr)
	}
	return strings.Join(parts, " ")
}

type pm2ObservationError struct{ Diagnostic PM2ObservationDiagnostic }

func (e pm2ObservationError) Error() string { return e.Diagnostic.String() }

type observationUnavailableError struct {
	message string
	cause   error
}

func (e observationUnavailableError) Error() string { return e.message }
func (e observationUnavailableError) Unwrap() error { return e.cause }

func wrapObservationUnavailable(message string, err error) error {
	return observationUnavailableError{message: message, cause: err}
}

func observationCommandError(ctx context.Context, operation, command string, stderr []byte, err error) error {
	return pm2ObservationError{Diagnostic: PM2ObservationDiagnostic{
		Operation: operation,
		Command:   command,
		Cause:     observationCause(ctx, err),
		Stderr:    safeObservationStderr(stderr),
	}}
}

func observationCause(ctx context.Context, err error) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return "cancelled"
	}
	var exitCoder interface{ ExitCode() int }
	if errors.As(err, &exitCoder) && exitCoder.ExitCode() >= 0 {
		return "exit-" + strconv.Itoa(exitCoder.ExitCode())
	}
	const prefix = "exit status "
	if text := err.Error(); strings.HasPrefix(text, prefix) {
		if code, parseErr := strconv.Atoi(strings.TrimPrefix(text, prefix)); parseErr == nil && code >= 0 && code <= 255 {
			return "exit-" + strconv.Itoa(code)
		}
	}
	return "execution-failed"
}

func safeObservationStderr(stderr []byte) string {
	if len(stderr) > 256 {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(string(stderr))) {
	case "sudo: a password is required", "sudo: no tty present and no askpass program specified":
		return "sudo authentication required"
	case "sudo: command not found":
		return "sudo command unavailable"
	}
	return ""
}

func withObservationStage(err error, stage string) *PM2ObservationDiagnostic {
	var observation pm2ObservationError
	if !errors.As(err, &observation) {
		return nil
	}
	diagnostic := observation.Diagnostic
	diagnostic.Stage = stage
	return &diagnostic
}

func procObservationCommand(operation string, pid int) string {
	var executable, file string
	switch operation {
	case "proc-cwd":
		executable, file = "readlink", "cwd"
	case "proc-exe":
		executable, file = "readlink", "exe"
	case "proc-stat":
		executable, file = "cat", "stat"
	default:
		return ""
	}
	return fmt.Sprintf("sudo -n %s /proc/%d/%s", executable, pid, file)
}
