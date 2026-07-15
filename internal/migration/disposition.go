package migration

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"time"
)

type ContainerDisposition uint8

const (
	DispositionStop ContainerDisposition = iota
	DispositionRemove
)

const (
	DispositionStoppedCode          = "legacy-container-stopped"
	DispositionRemovedCode          = "legacy-container-removed-volumes-preserved"
	DispositionRestartedCode        = "legacy-container-restart-verified"
	DispositionManualRecoveryCode   = "legacy-container-removed-manual-recovery"
	DispositionRecoveryUnprovenCode = "legacy-container-recovery-unproven"
)

type ContainerDispositionResult struct {
	Code     string
	Verified bool
}

type LegacyContainerController interface {
	Apply(context.Context, string, ContainerDisposition) (ContainerDispositionResult, error)
	Recover(context.Context, string, ContainerDisposition) (ContainerDispositionResult, error)
}

type DockerLegacyContainerController struct{ Executor BinaryExecutor }

func (c DockerLegacyContainerController) Apply(ctx context.Context, id string, disposition ContainerDisposition) (ContainerDispositionResult, error) {
	if !fullContainerID.MatchString(id) || c.Executor == nil || disposition > DispositionRemove {
		return ContainerDispositionResult{}, ErrProcessPrecondition
	}
	if result := c.run(ctx, "stop", id); result.Outcome != ProcessSucceeded {
		if result.StderrCode == SudoDockerPermissionCode {
			return ContainerDispositionResult{Code: result.StderrCode}, ErrSudoDockerPermission
		}
		return ContainerDispositionResult{Code: result.StderrCode}, errors.New("legacy container disposition failed")
	}
	if disposition == DispositionRemove {
		if result := c.run(ctx, "rm", id); result.Outcome != ProcessSucceeded {
			if result.StderrCode == SudoDockerPermissionCode {
				return ContainerDispositionResult{Code: DispositionStoppedCode, Verified: true}, ErrSudoDockerPermission
			}
			return ContainerDispositionResult{Code: DispositionStoppedCode, Verified: true}, errors.New("legacy container disposition failed")
		}
		return ContainerDispositionResult{Code: DispositionRemovedCode, Verified: true}, nil
	}
	return ContainerDispositionResult{Code: DispositionStoppedCode, Verified: true}, nil
}

func (c DockerLegacyContainerController) Recover(ctx context.Context, id string, disposition ContainerDisposition) (ContainerDispositionResult, error) {
	if !fullContainerID.MatchString(id) || c.Executor == nil {
		return ContainerDispositionResult{Code: DispositionRecoveryUnprovenCode}, ErrProcessPrecondition
	}
	if disposition == DispositionRemove {
		return ContainerDispositionResult{Code: DispositionManualRecoveryCode}, nil
	}
	if result := c.run(ctx, "start", id); result.Outcome != ProcessSucceeded {
		return ContainerDispositionResult{Code: DispositionRecoveryUnprovenCode}, errors.New("legacy container recovery failed")
	}
	var running bytes.Buffer
	result := c.Executor.Run(ctx, ProcessSpec{Name: "docker", Args: []string{"inspect", "--format", "{{.State.Running}}", id}, Timeout: 30 * time.Second}, &running)
	if result.Outcome != ProcessSucceeded || strings.TrimSpace(running.String()) != "true" {
		return ContainerDispositionResult{Code: DispositionRecoveryUnprovenCode}, errors.New("legacy container recovery failed")
	}
	return ContainerDispositionResult{Code: DispositionRestartedCode, Verified: true}, nil
}

func (c DockerLegacyContainerController) run(ctx context.Context, args ...string) ProcessResult {
	return c.Executor.Run(ctx, ProcessSpec{Name: "docker", Args: append([]string(nil), args...), Timeout: 30 * time.Second}, io.Discard)
}
