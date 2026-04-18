package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/jcaltamar/alice-installer/internal/envgen"
	"github.com/jcaltamar/alice-installer/internal/platform"
	"github.com/jcaltamar/alice-installer/internal/secrets"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

// buildEnvWriteModel returns an EnvWriteModel wired with fakes for testing.
func buildEnvWriteModel(writer envgen.FileWriter, genErr error) EnvWriteModel {
	var gen secrets.PasswordGenerator
	if genErr != nil {
		gen = &secrets.FakeGenerator{Err: genErr}
	} else {
		gen = &secrets.FakeGenerator{Val: "test-password"}
	}
	templater := &envgen.Templater{PasswordGen: gen}
	assets := TemplateAssets{
		EnvExample: []byte("WORKSPACE=\nPOSTGRES_PASSWORD=\n"),
	}
	input := envgen.Input{
		Workspace:        "my-site",
		Arch:             platform.ArchAMD64,
		GeneratePassword: true,
	}
	return NewEnvWriteModel(theme.Default(), templater, writer, assets, "/tmp/.env", input)
}

// TestEnvWriteModelInitEmitsEnvWrittenMsg verifies that with a FakeWriter and valid
// input, Init returns a Cmd that produces an EnvWrittenMsg with correct path.
func TestEnvWriteModelInitEmitsEnvWrittenMsg(t *testing.T) {
	fw := &envgen.FakeWriter{Written: make(map[string][]byte)}
	m := buildEnvWriteModel(fw, nil)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() should return a non-nil Cmd")
	}
	msg := cmd()
	written, ok := msg.(EnvWrittenMsg)
	if !ok {
		t.Fatalf("Init() cmd produced %T, want EnvWrittenMsg", msg)
	}
	if written.Path != "/tmp/.env" {
		t.Errorf("EnvWrittenMsg.Path = %q, want /tmp/.env", written.Path)
	}
	// FakeWriter should have received the content.
	data, found := fw.Written["/tmp/.env"]
	if !found {
		t.Fatal("FakeWriter.Written should contain /tmp/.env")
	}
	if !strings.Contains(string(data), "WORKSPACE=my-site") {
		t.Errorf("written content should contain WORKSPACE=my-site, got:\n%s", string(data))
	}
}

// TestEnvWriteModelInitEmitsInstallFailureMsgOnError verifies that a render error
// (via FakeGenerator returning an error) causes an InstallFailureMsg.
func TestEnvWriteModelInitEmitsInstallFailureMsgOnError(t *testing.T) {
	fw := &envgen.FakeWriter{Written: make(map[string][]byte)}
	genErr := errors.New("crypto/rand failure")
	m := buildEnvWriteModel(fw, genErr)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() should return a non-nil Cmd even on error path")
	}
	msg := cmd()
	fail, ok := msg.(InstallFailureMsg)
	if !ok {
		t.Fatalf("Init() cmd produced %T, want InstallFailureMsg", msg)
	}
	if fail.Stage != "env-write" {
		t.Errorf("InstallFailureMsg.Stage = %q, want env-write", fail.Stage)
	}
	if fail.Err == nil {
		t.Error("InstallFailureMsg.Err should be non-nil")
	}
}

// TestEnvWriteModelWriteErrorEmitsInstallFailureMsg verifies that a FileWriter error
// causes an InstallFailureMsg.
func TestEnvWriteModelWriteErrorEmitsInstallFailureMsg(t *testing.T) {
	fw := &envgen.FakeWriter{
		Written: make(map[string][]byte),
		Err:     errors.New("disk full"),
	}
	m := buildEnvWriteModel(fw, nil)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() should return a non-nil Cmd")
	}
	msg := cmd()
	fail, ok := msg.(InstallFailureMsg)
	if !ok {
		t.Fatalf("Init() cmd produced %T, want InstallFailureMsg", msg)
	}
	if fail.Stage != "env-write" {
		t.Errorf("InstallFailureMsg.Stage = %q, want env-write", fail.Stage)
	}
}

// TestEnvWriteModelViewBeforeDoneContainsWriting verifies the in-flight view.
func TestEnvWriteModelViewBeforeDoneContainsWriting(t *testing.T) {
	fw := &envgen.FakeWriter{Written: make(map[string][]byte)}
	m := buildEnvWriteModel(fw, nil)
	view := m.View()
	if !strings.Contains(view, "Writing") && !strings.Contains(view, "writing") && !strings.Contains(view, ".env") {
		t.Errorf("View() before done should mention writing/env, got: %q", view)
	}
}

// TestEnvWriteModelViewAfterDoneContainsWritten verifies the completion view.
func TestEnvWriteModelViewAfterDoneContainsWritten(t *testing.T) {
	fw := &envgen.FakeWriter{Written: make(map[string][]byte)}
	m := buildEnvWriteModel(fw, nil)
	// Simulate done state.
	m.done = true
	m.writtenPath = "/tmp/.env"
	view := m.View()
	if !strings.Contains(view, "Written") && !strings.Contains(view, "written") && !strings.Contains(view, "✓") {
		t.Errorf("View() after done should mention written/checkmark, got: %q", view)
	}
}
