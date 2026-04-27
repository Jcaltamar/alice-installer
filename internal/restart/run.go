package restart

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

func Run(ctx context.Context, cfg Config, deps Dependencies, _ io.Writer) error {
	if deps.Compose == nil {
		return fmt.Errorf("restart: compose runner is required")
	}
	if deps.GPU == nil {
		return fmt.Errorf("restart: gpu detector is required")
	}

	resolved, err := workspace.ResolveArtifacts(cfg.WorkspaceDir)
	if err != nil {
		return fmt.Errorf("restart: %w", err)
	}

	files := workspace.ComposeFiles(ctx, deps.GPU, resolved)
	if err := deps.Compose.Restart(ctx, files, resolved.EnvFile); err != nil {
		return fmt.Errorf("restart: %w", err)
	}
	return nil
}
