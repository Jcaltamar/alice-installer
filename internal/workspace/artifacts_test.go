package workspace

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jcaltamar/alice-installer/internal/platform"
)

func TestResolveArtifacts_RequiresPersistedWorkspaceFiles(t *testing.T) {
	tests := []struct {
		name       string
		workspace  string
		createEnv  bool
		createBase bool
		wantErr    string
	}{
		{
			name:      "empty workspace dir",
			workspace: "",
			wantErr:   "workspace is empty",
		},
		{
			name:       "missing both env and compose",
			workspace:  t.TempDir(),
			createEnv:  false,
			createBase: false,
			wantErr:    "existing installation artifacts not found",
		},
		{
			name:       "missing env file",
			workspace:  t.TempDir(),
			createEnv:  false,
			createBase: true,
			wantErr:    ".env",
		},
		{
			name:       "missing compose file",
			workspace:  t.TempDir(),
			createEnv:  true,
			createBase: false,
			wantErr:    "docker-compose.yml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.workspace != "" {
				if tt.createEnv {
					mustWrite(t, tt.workspace+"/.env", "WORKSPACE=test\n")
				}
				if tt.createBase {
					mustWrite(t, tt.workspace+"/docker-compose.yml", "services: {}\n")
				}
			}

			_, err := ResolveArtifacts(tt.workspace)
			if err == nil {
				t.Fatalf("ResolveArtifacts() expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.wantErr)) {
				t.Fatalf("ResolveArtifacts() error = %q, want contains %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%s): %v", path, err)
	}
}

func TestResolveArtifacts_ReturnsRequiredAndOptionalPaths(t *testing.T) {
	workspace := t.TempDir()
	mustWrite(t, workspace+"/.env", "WORKSPACE=test\n")
	mustWrite(t, workspace+"/docker-compose.yml", "services: {}\n")

	resolved, err := ResolveArtifacts(workspace)
	if err != nil {
		t.Fatalf("ResolveArtifacts() error = %v, want nil", err)
	}
	if resolved.EnvFile != workspace+"/.env" {
		t.Fatalf("EnvFile = %q, want %q", resolved.EnvFile, workspace+"/.env")
	}
	if resolved.BaseFile != workspace+"/docker-compose.yml" {
		t.Fatalf("BaseFile = %q, want %q", resolved.BaseFile, workspace+"/docker-compose.yml")
	}
	if resolved.GPUFile != workspace+"/docker-compose.gpu.yml" {
		t.Fatalf("GPUFile = %q, want %q", resolved.GPUFile, workspace+"/docker-compose.gpu.yml")
	}
}

func TestComposeFiles_DeterministicOrderingAndOverlaySelection(t *testing.T) {
	tests := []struct {
		name          string
		gpuDetected   bool
		createOverlay bool
		wantFiles     []string
	}{
		{
			name:          "base only when gpu false",
			gpuDetected:   false,
			createOverlay: true,
		},
		{
			name:          "base only when overlay missing",
			gpuDetected:   true,
			createOverlay: false,
		},
		{
			name:          "base then overlay when gpu true and overlay exists",
			gpuDetected:   true,
			createOverlay: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := t.TempDir()
			mustWrite(t, workspace+"/.env", "WORKSPACE=test\n")
			mustWrite(t, workspace+"/docker-compose.yml", "services: {}\n")
			if tt.createOverlay {
				mustWrite(t, workspace+"/docker-compose.gpu.yml", "services: {}\n")
			}

			resolved, err := ResolveArtifacts(workspace)
			if err != nil {
				t.Fatalf("ResolveArtifacts() error = %v, want nil", err)
			}

			files := ComposeFiles(context.Background(), &platform.FakeGPUDetector{Info: platform.GPUInfo{
				ToolkitInstalled: tt.gpuDetected,
			}}, resolved)

			want := []string{workspace + "/docker-compose.yml"}
			if tt.gpuDetected && tt.createOverlay {
				want = append(want, workspace+"/docker-compose.gpu.yml")
			}

			if len(files) != len(want) {
				t.Fatalf("ComposeFiles() len = %d, want %d; files=%v want=%v", len(files), len(want), files, want)
			}
			for i := range want {
				if files[i] != want[i] {
					t.Fatalf("ComposeFiles() files[%d] = %q, want %q", i, files[i], want[i])
				}
			}
		})
	}
}
