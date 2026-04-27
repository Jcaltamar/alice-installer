package update

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

type countingGPUDetector struct {
	info  platform.GPUInfo
	calls int
}

func (d *countingGPUDetector) Detect(_ context.Context) platform.GPUInfo {
	d.calls++
	return d.info
}

func TestRun_RequiresPersistedArtifacts(t *testing.T) {
	tests := []struct {
		name       string
		createEnv  bool
		createBase bool
		wantErr    string
	}{
		{
			name:       "missing both env and compose",
			createEnv:  false,
			createBase: false,
			wantErr:    "existing installation artifacts not found",
		},
		{
			name:       "missing env",
			createEnv:  false,
			createBase: true,
			wantErr:    ".env",
		},
		{
			name:       "missing compose",
			createEnv:  true,
			createBase: false,
			wantErr:    "docker-compose.yml",
		},
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

			var out bytes.Buffer
			err := Run(context.Background(), Config{WorkspaceDir: workspace}, Dependencies{
				Compose: &compose.FakeComposeRunner{},
				GPU:     &platform.FakeGPUDetector{},
			}, &out)
			if err == nil {
				t.Fatalf("Run() expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.wantErr)) {
				t.Fatalf("Run() error = %q, want contains %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRun_UsesPersistedArtifactsNonInteractive(t *testing.T) {
	workspace := t.TempDir()
	mustWrite(t, workspace+"/.env", "WORKSPACE=test\n")
	mustWrite(t, workspace+"/docker-compose.yml", "services: {}\n")

	composeFake := &compose.FakeComposeRunner{
		PullProgressMsgs: []compose.PullProgressMsg{{Raw: "pull line"}},
		UpProgressMsgs:   []compose.UpProgressMsg{{Raw: "up line"}},
	}

	var out bytes.Buffer
	err := Run(context.Background(), Config{WorkspaceDir: workspace}, Dependencies{
		Compose: composeFake,
		GPU:     &platform.FakeGPUDetector{},
	}, &out)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}

	output := out.String()
	if !strings.Contains(output, "[pull]") {
		t.Fatalf("output missing [pull] prefix: %q", output)
	}
	if !strings.Contains(output, "[deploy]") {
		t.Fatalf("output missing [deploy] prefix: %q", output)
	}
}

func TestRun_PullBeforeUpAndRecordsComposeArguments(t *testing.T) {
	workspace := t.TempDir()
	mustWrite(t, workspace+"/.env", "WORKSPACE=test\n")
	mustWrite(t, workspace+"/docker-compose.yml", "services: {}\n")

	composeFake := &compose.FakeComposeRunner{}
	err := Run(context.Background(), Config{WorkspaceDir: workspace}, Dependencies{
		Compose: composeFake,
		GPU:     &platform.FakeGPUDetector{},
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}

	if len(composeFake.CallOrder) != 2 {
		t.Fatalf("CallOrder len = %d, want 2", len(composeFake.CallOrder))
	}
	if composeFake.CallOrder[0] != "pull" || composeFake.CallOrder[1] != "up" {
		t.Fatalf("CallOrder = %v, want [pull up]", composeFake.CallOrder)
	}
	if len(composeFake.PullCalls) != 1 {
		t.Fatalf("PullCalls len = %d, want 1", len(composeFake.PullCalls))
	}
	if len(composeFake.UpCalls) != 1 {
		t.Fatalf("UpCalls len = %d, want 1", len(composeFake.UpCalls))
	}
	if composeFake.PullCalls[0].EnvFile != workspace+"/.env" {
		t.Fatalf("PullCalls[0].EnvFile = %q, want %q", composeFake.PullCalls[0].EnvFile, workspace+"/.env")
	}
	if len(composeFake.PullCalls[0].Files) != 1 {
		t.Fatalf("PullCalls[0].Files len = %d, want 1", len(composeFake.PullCalls[0].Files))
	}
	if composeFake.PullCalls[0].Files[0] != workspace+"/docker-compose.yml" {
		t.Fatalf("PullCalls[0].Files[0] = %q, want %q", composeFake.PullCalls[0].Files[0], workspace+"/docker-compose.yml")
	}
}

func TestRun_DoesNotRunUpWhenPullFails(t *testing.T) {
	workspace := t.TempDir()
	mustWrite(t, workspace+"/.env", "WORKSPACE=test\n")
	mustWrite(t, workspace+"/docker-compose.yml", "services: {}\n")

	composeFake := &compose.FakeComposeRunner{PullErr: errPullFailed}
	err := Run(context.Background(), Config{WorkspaceDir: workspace}, Dependencies{
		Compose: composeFake,
		GPU:     &platform.FakeGPUDetector{},
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("Run() expected pull error, got nil")
	}
	if !strings.Contains(err.Error(), "pull") {
		t.Fatalf("Run() error = %q, want pull context", err.Error())
	}
	if len(composeFake.UpCalls) != 0 {
		t.Fatalf("UpCalls len = %d, want 0", len(composeFake.UpCalls))
	}
}

func TestRun_PropagatesUpFailure(t *testing.T) {
	workspace := t.TempDir()
	mustWrite(t, workspace+"/.env", "WORKSPACE=test\n")
	mustWrite(t, workspace+"/docker-compose.yml", "services: {}\n")

	composeFake := &compose.FakeComposeRunner{UpErr: errUpFailed}
	err := Run(context.Background(), Config{WorkspaceDir: workspace}, Dependencies{
		Compose: composeFake,
		GPU:     &platform.FakeGPUDetector{},
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("Run() expected deploy error, got nil")
	}
	if !strings.Contains(err.Error(), "deploy") {
		t.Fatalf("Run() error = %q, want deploy context", err.Error())
	}
}

func TestRun_GPUOverlaySelection(t *testing.T) {
	tests := []struct {
		name            string
		gpuDetected     bool
		createGPUFile   bool
		wantFiles       int
		wantLastOverlay bool
	}{
		{
			name:            "include overlay only when gpu detected and file exists",
			gpuDetected:     true,
			createGPUFile:   true,
			wantFiles:       2,
			wantLastOverlay: true,
		},
		{
			name:            "gpu detected but overlay missing",
			gpuDetected:     true,
			createGPUFile:   false,
			wantFiles:       1,
			wantLastOverlay: false,
		},
		{
			name:            "overlay exists but gpu not detected",
			gpuDetected:     false,
			createGPUFile:   true,
			wantFiles:       1,
			wantLastOverlay: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := t.TempDir()
			mustWrite(t, workspace+"/.env", "WORKSPACE=test\n")
			mustWrite(t, workspace+"/docker-compose.yml", "services: {}\n")
			if tt.createGPUFile {
				mustWrite(t, workspace+"/docker-compose.gpu.yml", "services: {}\n")
			}

			composeFake := &compose.FakeComposeRunner{}
			err := Run(context.Background(), Config{WorkspaceDir: workspace}, Dependencies{
				Compose: composeFake,
				GPU: &platform.FakeGPUDetector{Info: platform.GPUInfo{
					ToolkitInstalled: tt.gpuDetected,
				}},
			}, &bytes.Buffer{})
			if err != nil {
				t.Fatalf("Run() error = %v, want nil", err)
			}

			if len(composeFake.PullCalls) != 1 {
				t.Fatalf("PullCalls len = %d, want 1", len(composeFake.PullCalls))
			}
			files := composeFake.PullCalls[0].Files
			if len(files) != tt.wantFiles {
				t.Fatalf("compose file count = %d, want %d, files=%v", len(files), tt.wantFiles, files)
			}
			hasOverlay := len(files) > 1 && strings.HasSuffix(files[len(files)-1], "docker-compose.gpu.yml")
			if hasOverlay != tt.wantLastOverlay {
				t.Fatalf("overlay inclusion = %v, want %v, files=%v", hasOverlay, tt.wantLastOverlay, files)
			}
		})
	}
}

func TestRun_ResolvesComposeFilesOnceAndReusesForPullAndUp(t *testing.T) {
	workspace := t.TempDir()
	mustWrite(t, workspace+"/.env", "WORKSPACE=test\n")
	mustWrite(t, workspace+"/docker-compose.yml", "services: {}\n")
	mustWrite(t, workspace+"/docker-compose.gpu.yml", "services: {}\n")

	composeFake := &compose.FakeComposeRunner{}
	gpu := &countingGPUDetector{info: platform.GPUInfo{ToolkitInstalled: true}}
	err := Run(context.Background(), Config{WorkspaceDir: workspace}, Dependencies{
		Compose: composeFake,
		GPU:     gpu,
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if gpu.calls != 1 {
		t.Fatalf("GPU detect calls = %d, want 1", gpu.calls)
	}
}

var (
	errPullFailed = errors.New("pull failed")
	errUpFailed   = errors.New("up failed")
)

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%s): %v", path, err)
	}
}
