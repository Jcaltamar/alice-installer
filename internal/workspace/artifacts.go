package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jcaltamar/alice-installer/internal/platform"
)

type ResolvedArtifacts struct {
	EnvFile  string
	BaseFile string
	GPUFile  string
}

func ResolveArtifacts(workspaceDir string) (ResolvedArtifacts, error) {
	if workspaceDir == "" {
		return ResolvedArtifacts{}, fmt.Errorf("existing installation artifacts not found; run install first or pass --workspace-dir (workspace is empty)")
	}

	envPath := filepath.Join(workspaceDir, ".env")
	basePath := filepath.Join(workspaceDir, "docker-compose.yml")
	if err := requireFile(envPath); err != nil {
		return ResolvedArtifacts{}, err
	}
	if err := requireFile(basePath); err != nil {
		return ResolvedArtifacts{}, err
	}

	return ResolvedArtifacts{
		EnvFile:  envPath,
		BaseFile: basePath,
		GPUFile:  filepath.Join(workspaceDir, "docker-compose.gpu.yml"),
	}, nil
}

func ComposeFiles(ctx context.Context, gpuDetector platform.GPUDetector, resolved ResolvedArtifacts) []string {
	files := []string{resolved.BaseFile}
	if gpuDetector == nil {
		return files
	}
	if !gpuDetector.Detect(ctx).ToolkitInstalled {
		return files
	}
	if info, err := os.Stat(resolved.GPUFile); err == nil && !info.IsDir() {
		files = append(files, resolved.GPUFile)
	}
	return files
}

func requireFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("existing installation artifacts not found; run install first or pass --workspace-dir (missing %s)", path)
		}
		return fmt.Errorf("failed to read artifact %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("existing installation artifacts not found; run install first or pass --workspace-dir (expected file, got directory: %s)", path)
	}
	return nil
}
