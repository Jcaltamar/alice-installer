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

// ArtifactPaths identifies the required files for a persisted workspace.
type ArtifactPaths struct {
	EnvFile  string
	BaseFile string
}

// RequiredArtifactPaths returns the required workspace artifact paths without reading them.
func RequiredArtifactPaths(workspaceDir string) ArtifactPaths {
	return ArtifactPaths{
		EnvFile:  filepath.Join(workspaceDir, ".env"),
		BaseFile: filepath.Join(workspaceDir, "docker-compose.yml"),
	}
}

func ResolveArtifacts(workspaceDir string) (ResolvedArtifacts, error) {
	if workspaceDir == "" {
		return ResolvedArtifacts{}, fmt.Errorf("existing installation artifacts not found; run install first or pass --workspace-dir (workspace is empty)")
	}

	paths := RequiredArtifactPaths(workspaceDir)
	if err := requireFile(paths.EnvFile); err != nil {
		return ResolvedArtifacts{}, err
	}
	if err := requireFile(paths.BaseFile); err != nil {
		return ResolvedArtifacts{}, err
	}

	return ResolvedArtifacts{
		EnvFile:  paths.EnvFile,
		BaseFile: paths.BaseFile,
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
