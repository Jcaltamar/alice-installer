package migration

import (
	"bytes"
	"context"
	"errors"
	"io"
	"time"
)

const SudoDockerPermissionCode = "sudo-docker-permission"

var ErrSudoDockerPermission = errors.New(SudoDockerPermissionCode)

// SudoDockerExecutor is the migration-only privilege boundary. It accepts only
// Docker process specifications and never delegates through a shell.
type SudoDockerExecutor struct{ Executor BinaryExecutor }

func (e SudoDockerExecutor) Run(ctx context.Context, spec ProcessSpec, stdout io.Writer) ProcessResult {
	if spec.Name != "docker" || len(spec.Args) == 0 || stdout == nil {
		return ProcessResult{Outcome: ProcessFailed, StderrCode: "process-precondition"}
	}
	executor := e.Executor
	if executor == nil {
		executor = OSBinaryExecutor{}
	}
	args := append([]string{"-n", "docker"}, spec.Args...)
	result := executor.Run(ctx, ProcessSpec{Name: "sudo", Args: args, Timeout: spec.Timeout}, stdout)
	if result.Outcome == ProcessFailed && result.StderrCode == SudoDockerPermissionCode {
		return ProcessResult{Outcome: ProcessFailed, StderrCode: SudoDockerPermissionCode}
	}
	return result
}

// Run implements the bounded, buffered inspector boundary.
func (e SudoDockerExecutor) RunDocker(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	if name != "docker" || len(args) == 0 {
		return nil, nil, ErrProcessPrecondition
	}
	var stdout inspectorBoundedOutput
	result := e.Run(ctx, ProcessSpec{Name: name, Args: args, Timeout: 30 * time.Second}, &stdout)
	if result.Outcome == ProcessSucceeded {
		return stdout.Bytes(), nil, nil
	}
	if result.StderrCode == SudoDockerPermissionCode {
		return nil, nil, ErrSudoDockerPermission
	}
	return nil, nil, ErrProcessPrecondition
}

type inspectorBoundedOutput struct{ bytes.Buffer }

func (b *inspectorBoundedOutput) Write(p []byte) (int, error) {
	const limit = 1 << 20
	if b.Len()+len(p) > limit {
		return 0, io.ErrShortWrite
	}
	return b.Buffer.Write(p)
}

type sudoDockerRunner struct{ SudoDockerExecutor }

func (r sudoDockerRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	return r.RunDocker(ctx, name, args...)
}

func NewSudoDockerRunner(executor BinaryExecutor) DockerRunner {
	return sudoDockerRunner{SudoDockerExecutor{Executor: executor}}
}
