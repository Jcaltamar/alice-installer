package restart

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/platform"
)

func TestRun_FailsFastWhenPersistedArtifactsMissing(t *testing.T) {
	tests := []struct {
		name       string
		createEnv  bool
		createBase bool
		wantErr    string
	}{
		{name: "missing both", wantErr: "existing installation artifacts not found"},
		{name: "missing env", createBase: true, wantErr: ".env"},
		{name: "missing compose", createEnv: true, wantErr: "docker-compose.yml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := t.TempDir()
			if tt.createEnv {
				mustWrite(t, workspace+"/.env", "WORKSPACE=test\n")
			}
			if tt.createBase {
				mustWrite(t, workspace+"/docker-compose.yml", "services: {}\n")
			}

			composeFake := &compose.FakeComposeRunner{}
			err := Run(context.Background(), Config{WorkspaceDir: workspace}, Dependencies{
				Compose: composeFake,
				GPU:     &platform.FakeGPUDetector{},
			}, &bytes.Buffer{})
			if err == nil {
				t.Fatalf("Run() expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.wantErr)) {
				t.Fatalf("Run() error = %q, want contains %q", err.Error(), tt.wantErr)
			}
			if len(composeFake.RestartCalls) != 0 {
				t.Fatalf("RestartCalls len = %d, want 0", len(composeFake.RestartCalls))
			}
		})
	}
}

func TestRun_RestartsExactlyOnceWithResolvedArtifacts(t *testing.T) {
	workspace := t.TempDir()
	mustWrite(t, workspace+"/.env", "WORKSPACE=test\n")
	mustWrite(t, workspace+"/docker-compose.yml", "services: {}\n")
	mustWrite(t, workspace+"/docker-compose.gpu.yml", "services: {}\n")

	composeFake := &compose.FakeComposeRunner{}
	err := Run(context.Background(), Config{WorkspaceDir: workspace}, Dependencies{
		Compose: composeFake,
		GPU: &platform.FakeGPUDetector{Info: platform.GPUInfo{
			ToolkitInstalled: true,
		}},
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}

	if len(composeFake.RestartCalls) != 1 {
		t.Fatalf("RestartCalls len = %d, want 1", len(composeFake.RestartCalls))
	}
	if composeFake.RestartCalls[0].EnvFile != workspace+"/.env" {
		t.Fatalf("RestartCalls[0].EnvFile = %q, want %q", composeFake.RestartCalls[0].EnvFile, workspace+"/.env")
	}
	files := composeFake.RestartCalls[0].Files
	if len(files) != 2 {
		t.Fatalf("Restart files len = %d, want 2", len(files))
	}
	if files[0] != workspace+"/docker-compose.yml" || files[1] != workspace+"/docker-compose.gpu.yml" {
		t.Fatalf("Restart files = %v, want [%s %s]", files, workspace+"/docker-compose.yml", workspace+"/docker-compose.gpu.yml")
	}
	if len(composeFake.PullCalls) != 0 || len(composeFake.UpCalls) != 0 {
		t.Fatalf("unexpected pull/up calls: pull=%d up=%d", len(composeFake.PullCalls), len(composeFake.UpCalls))
	}
	if len(composeFake.CallOrder) != 1 || composeFake.CallOrder[0] != "restart" {
		t.Fatalf("CallOrder = %v, want [restart]", composeFake.CallOrder)
	}
}

func TestRun_WrapsComposeRestartErrors(t *testing.T) {
	workspace := t.TempDir()
	mustWrite(t, workspace+"/.env", "WORKSPACE=test\n")
	mustWrite(t, workspace+"/docker-compose.yml", "services: {}\n")

	errRestart := errors.New("daemon unavailable")
	err := Run(context.Background(), Config{WorkspaceDir: workspace}, Dependencies{
		Compose: &compose.FakeComposeRunner{RestartErr: errRestart},
		GPU:     &platform.FakeGPUDetector{},
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "restart:") {
		t.Fatalf("Run() error = %q, want restart context", err.Error())
	}
	if !strings.Contains(err.Error(), "daemon unavailable") {
		t.Fatalf("Run() error = %q, want compose cause", err.Error())
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%s): %v", path, err)
	}
}
