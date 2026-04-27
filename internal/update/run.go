package update

import (
	"context"
	"fmt"
	"io"

	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/platform"
	"github.com/jcaltamar/alice-installer/internal/workspace"
)

type Config struct {
	WorkspaceDir string
}

type Dependencies struct {
	Compose compose.ComposeRunner
	GPU     platform.GPUDetector
}

func Run(ctx context.Context, cfg Config, deps Dependencies, out io.Writer) error {
	if out == nil {
		out = io.Discard
	}
	if deps.Compose == nil {
		return fmt.Errorf("update: compose runner is required")
	}
	if deps.GPU == nil {
		return fmt.Errorf("update: gpu detector is required")
	}

	resolved, err := workspace.ResolveArtifacts(cfg.WorkspaceDir)
	if err != nil {
		return err
	}
	files := workspace.ComposeFiles(ctx, deps.GPU, resolved)

	if err := runPull(ctx, deps, files, resolved.EnvFile, out); err != nil {
		return err
	}
	if err := runDeploy(ctx, deps, files, resolved.EnvFile, out); err != nil {
		return err
	}
	return nil
}

func runPull(ctx context.Context, deps Dependencies, files []string, envFile string, out io.Writer) error {
	progress := make(chan compose.PullProgressMsg, 64)
	done := make(chan error, 1)

	go func() {
		done <- deps.Compose.Pull(ctx, files, envFile, progress)
		close(progress)
	}()

	for msg := range progress {
		if msg.Raw != "" {
			fmt.Fprintf(out, "[pull] %s\n", msg.Raw)
		}
	}

	if err := <-done; err != nil {
		return fmt.Errorf("pull: %w", err)
	}
	return nil
}

func runDeploy(ctx context.Context, deps Dependencies, files []string, envFile string, out io.Writer) error {
	progress := make(chan compose.UpProgressMsg, 64)
	done := make(chan error, 1)

	go func() {
		done <- deps.Compose.Up(ctx, files, envFile, progress)
		close(progress)
	}()

	for msg := range progress {
		if msg.Raw != "" {
			fmt.Fprintf(out, "[deploy] %s\n", msg.Raw)
		}
	}

	if err := <-done; err != nil {
		return fmt.Errorf("deploy: %w", err)
	}
	return nil
}
