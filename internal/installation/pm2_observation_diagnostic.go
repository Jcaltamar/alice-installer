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
	Stage              string
	Operation          string
	Command            string
	Cause              string
	Stderr             string
	StopProofTimedOut  bool
	StopProofCancelled bool
	PMID               int64
	Port               uint16
}

func (d PM2ObservationDiagnostic) String() string {
	if d.StopProofTimedOut {
		return fmt.Sprintf("stop command succeeded for PM2 ID %d; proof timed out waiting for PM2 status stopped and port release on %d", d.PMID, d.Port)
	}
	if d.StopProofCancelled {
		return fmt.Sprintf("stop command succeeded for PM2 ID %d; proof was cancelled before PM2 status stopped and port release on %d were proven", d.PMID, d.Port)
	}
	parts := []string{
		"stage=" + d.Stage,
		"operation=" + d.Operation,
		"command=" + d.Command,
		"cause=" + d.Cause,
	}
	if stderr := safeObservationStderr([]byte(d.Stderr)); stderr != "" {
		parts = append(parts, "stderr="+stderr)
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
	case "sudo: a password is required", "sudo: no tty present and no askpass program specified", "sudo authentication required":
		return "sudo authentication required"
	case "sudo: command not found", "sudo command unavailable":
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
