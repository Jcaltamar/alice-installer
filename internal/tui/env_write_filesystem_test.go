package tui

// env_write_filesystem_test.go: Filesystem-assertion tests for EnvWriteModel.
//
// These tests use the real embedded assets and AtomicWriter to verify that
// Init() actually writes all three files to a temp WorkspaceDir.

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/jcaltamar/alice-installer/internal/assets"
	"github.com/jcaltamar/alice-installer/internal/envgen"
	"github.com/jcaltamar/alice-installer/internal/platform"
	"github.com/jcaltamar/alice-installer/internal/secrets"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

// buildEnvWriteModelWithRealAssets constructs an EnvWriteModel backed by the
// real embedded assets and real AtomicWriter, targeting workspaceDir.
func buildEnvWriteModelWithRealAssets(workspaceDir string) EnvWriteModel {
	templater := &envgen.Templater{
		PasswordGen: &secrets.FakeGenerator{Val: "test-password"},
	}
	assetBundle := TemplateAssets{
		BaselineYAML: assets.DockerComposeYAML,
		OverlayYAML:  assets.DockerComposeGPU,
		EnvExample:   assets.EnvExample,
	}
	input := envgen.Input{
		Workspace:        "fs-test-site",
		Arch:             platform.ArchAMD64,
		GeneratePassword: true,
	}
	targetPath := filepath.Join(workspaceDir, ".env")
	return NewEnvWriteModel(
		theme.Default(),
		templater,
		envgen.AtomicWriter{},
		assetBundle,
		targetPath,
		input,
	)
}

// TestEnvWriteFilesystemAllThreeFilesExist verifies that Init() writes .env,
// docker-compose.yml, and docker-compose.gpu.yml into the workspace directory.
func TestEnvWriteFilesystemAllThreeFilesExist(t *testing.T) {
	workspaceDir := t.TempDir()

	m := buildEnvWriteModelWithRealAssets(workspaceDir)

	// Execute Init synchronously.
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() returned nil cmd")
	}
	msg := cmd()

	// Should be EnvWrittenMsg, not InstallFailureMsg.
	switch v := msg.(type) {
	case EnvWrittenMsg:
		// expected
		_ = v
	case InstallFailureMsg:
		t.Fatalf("Init() returned InstallFailureMsg: stage=%s err=%v", v.Stage, v.Err)
	default:
		t.Fatalf("Init() returned unexpected message type %T", msg)
	}

	// All three files must exist.
	for _, name := range []string{".env", "docker-compose.yml", "docker-compose.gpu.yml"} {
		path := filepath.Join(workspaceDir, name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected file %s to exist after Init(), got: %v", name, err)
		}
	}
}

// TestEnvWriteFilesystemComposeFilesMatchEmbeddedBytes verifies that the written
// compose files contain exactly the embedded bytes (not truncated or corrupted).
func TestEnvWriteFilesystemComposeFilesMatchEmbeddedBytes(t *testing.T) {
	workspaceDir := t.TempDir()

	m := buildEnvWriteModelWithRealAssets(workspaceDir)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() returned nil cmd")
	}
	if _, ok := cmd().(EnvWrittenMsg); !ok {
		t.Skip("Init() did not return EnvWrittenMsg; skipping content check")
	}

	tests := []struct {
		filename string
		want     []byte
	}{
		{"docker-compose.yml", assets.DockerComposeYAML},
		{"docker-compose.gpu.yml", assets.DockerComposeGPU},
	}

	for _, tt := range tests {
		path := filepath.Join(workspaceDir, tt.filename)
		got, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("could not read %s: %v", tt.filename, err)
			continue
		}
		if !bytes.Equal(got, tt.want) {
			t.Errorf("%s content differs from embedded bytes (got %d bytes, want %d bytes)",
				tt.filename, len(got), len(tt.want))
		}
	}
}

// TestEnvWriteFilesystemEnvFileContainsWorkspace verifies that the written .env
// contains the expected WORKSPACE value.
func TestEnvWriteFilesystemEnvFileContainsWorkspace(t *testing.T) {
	workspaceDir := t.TempDir()

	m := buildEnvWriteModelWithRealAssets(workspaceDir)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() returned nil cmd")
	}
	msg := cmd()
	written, ok := msg.(EnvWrittenMsg)
	if !ok {
		t.Fatalf("Init() returned %T, want EnvWrittenMsg", msg)
	}

	data, err := os.ReadFile(written.Path)
	if err != nil {
		t.Fatalf("could not read .env at %s: %v", written.Path, err)
	}

	content := string(data)
	if !containsSubstring(content, "WORKSPACE=fs-test-site") {
		t.Errorf("written .env should contain WORKSPACE=fs-test-site, got partial content (len=%d)", len(content))
	}
}

// TestEnvWriteFilesystemWorksInNestedDirectory verifies that Init() creates the
// workspace directory if it doesn't exist (nested path scenario).
func TestEnvWriteFilesystemWorksInNestedDirectory(t *testing.T) {
	base := t.TempDir()
	workspaceDir := filepath.Join(base, "nested", "alice-guardian")

	// Directory does NOT exist yet.
	if _, err := os.Stat(workspaceDir); !os.IsNotExist(err) {
		t.Fatal("expected workspaceDir to not exist before Init()")
	}

	m := buildEnvWriteModelWithRealAssets(workspaceDir)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() returned nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(EnvWrittenMsg); !ok {
		t.Fatalf("Init() returned %T, want EnvWrittenMsg", msg)
	}

	// All three files should exist in the nested directory.
	for _, name := range []string{".env", "docker-compose.yml", "docker-compose.gpu.yml"} {
		path := filepath.Join(workspaceDir, name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected file %s to exist, got: %v", name, err)
		}
	}
}

// containsSubstring is a helper to check substring without importing strings in
// the test file (keeps the import list minimal).
func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && stringContains(s, sub))
}

func stringContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
