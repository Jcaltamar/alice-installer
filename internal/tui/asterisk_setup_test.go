package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/asterisk"
)

type recordingAsteriskInstaller struct {
	calls   int
	options asterisk.Options
	err     error
}

func (r *recordingAsteriskInstaller) Install(_ context.Context, opts asterisk.Options) (asterisk.Result, error) {
	r.calls++
	r.options = opts
	if r.err != nil {
		return asterisk.Result{}, r.err
	}
	return asterisk.Result{Installed: true, AMIEndpoint: "127.0.0.1:5038", Resources: opts.ConfigRoot}, nil
}

func TestAsteriskSetupModelRunsInstallerWithSharedOptions(t *testing.T) {
	t.Parallel()

	installer := &recordingAsteriskInstaller{}
	opts := asterisk.Options{
		Enabled:    true,
		ConfigRoot: "/opt/alice-config/asterisk",
		AMI: asterisk.AMIContract{
			Enabled:  true,
			Host:     "127.0.0.1",
			Port:     5038,
			Username: "alice-guardian",
			Password: "generated-secret",
		},
	}
	model := NewAsteriskSetupModel(themeDefaultForTest(), installer, opts)

	msg := model.Init()()
	complete, ok := msg.(AsteriskSetupCompleteMsg)
	if !ok {
		t.Fatalf("Init emitted %T, want AsteriskSetupCompleteMsg", msg)
	}
	if !complete.Result.Installed {
		t.Fatalf("expected installed result, got %#v", complete.Result)
	}
	if installer.calls != 1 {
		t.Fatalf("installer calls = %d, want 1", installer.calls)
	}
	if installer.options.AMI.Username != opts.AMI.Username || installer.options.AMI.Password != opts.AMI.Password {
		t.Fatalf("installer did not receive shared AMI credentials: %#v", installer.options.AMI)
	}
}

func TestAsteriskSetupModelReportsOptionalFailure(t *testing.T) {
	t.Parallel()

	installer := &recordingAsteriskInstaller{err: errors.New("ami unavailable")}
	model := NewAsteriskSetupModel(themeDefaultForTest(), installer, asterisk.Options{Enabled: true})

	msg := model.Init()()
	failure, ok := msg.(InstallFailureMsg)
	if !ok {
		t.Fatalf("Init emitted %T, want InstallFailureMsg", msg)
	}
	if failure.Stage != "asterisk-setup" {
		t.Fatalf("failure stage = %q, want asterisk-setup", failure.Stage)
	}
	if !strings.Contains(failure.Err.Error(), "ami unavailable") {
		t.Fatalf("failure error should include installer error, got %v", failure.Err)
	}
}

func TestAsteriskSetupModelViews(t *testing.T) {
	model := NewAsteriskSetupModel(themeDefaultForTest(), &recordingAsteriskInstaller{}, asterisk.Options{Enabled: true})
	if view := model.View(); !strings.Contains(view, "Preparing host Asterisk") {
		t.Fatalf("pending view = %q", view)
	}
	model.err = errors.New("setup failed")
	if view := model.View(); !strings.Contains(view, "setup failed") {
		t.Fatalf("failure view = %q", view)
	}
	model.err = nil
	model.done = true
	model.result = asterisk.Result{AMIEndpoint: "127.0.0.1:5038"}
	if view := model.View(); !strings.Contains(view, "127.0.0.1:5038") {
		t.Fatalf("success view = %q", view)
	}
}

func TestRootModelInitDelegatesToSplash(t *testing.T) {
	if cmd := NewModel(buildTestDeps()).Init(); cmd != nil {
		t.Fatalf("root Init command = %v, want nil splash command", cmd)
	}
}

func TestAsteriskSetupModelCompletionAdvancesToPull(t *testing.T) {
	t.Parallel()

	model := NewAsteriskSetupModel(themeDefaultForTest(), &recordingAsteriskInstaller{}, asterisk.Options{Enabled: true})

	updated, cmd := model.Update(AsteriskSetupCompleteMsg{})
	if !updated.done {
		t.Fatal("completion should mark Asterisk setup done")
	}
	if cmd == nil {
		t.Fatal("completion should emit PullStartedMsg")
	}
	if _, ok := cmd().(PullStartedMsg); !ok {
		t.Fatalf("completion emitted %T, want PullStartedMsg", cmd())
	}

	updated, _ = updated.Update(tea.WindowSizeMsg{})
	if !updated.done {
		t.Fatal("unrelated messages should not clear done state")
	}
}
