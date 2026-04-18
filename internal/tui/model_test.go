package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/docker"
	"github.com/jcaltamar/alice-installer/internal/envgen"
	"github.com/jcaltamar/alice-installer/internal/platform"
	"github.com/jcaltamar/alice-installer/internal/ports"
	"github.com/jcaltamar/alice-installer/internal/preflight"
	"github.com/jcaltamar/alice-installer/internal/secrets"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

// buildTestDeps builds a Dependencies with all fakes for tests.
func buildTestDeps() Dependencies {
	coord := preflight.Coordinator{
		OS:                &platform.FakeOSGuard{Linux: true, Name: "linux"},
		Arch:              &platform.FakeArchDetector{Arch: platform.ArchAMD64},
		Docker:            &docker.FakeDockerClient{},
		Compose:           &compose.FakeComposeRunner{VersionVal: compose.Version{V2Plugin: true, Raw: "2.21.0"}},
		GPU:               &platform.FakeGPUDetector{},
		Ports:             &ports.FakePortScanner{},
		Dirs:              &fakeDirChecker{writable: true},
		MediaDir:          "/tmp/media",
		ConfigDir:         "/tmp/config",
		MinDockerVersion:  "20.10.0",
		MinComposeVersion: "2.0.0",
	}
	return Dependencies{
		Theme:                theme.Default(),
		OS:                   &platform.FakeOSGuard{Linux: true, Name: "linux"},
		Arch:                 &platform.FakeArchDetector{Arch: platform.ArchAMD64},
		GPU:                  &platform.FakeGPUDetector{},
		Ports:                &ports.FakePortScanner{},
		Docker:               &docker.FakeDockerClient{},
		Compose:              &compose.FakeComposeRunner{},
		Envgen:               &envgen.Templater{PasswordGen: &secrets.FakeGenerator{Val: "secret"}},
		Writer:               &envgen.FakeWriter{Written: make(map[string][]byte)},
		PreflightCoordinator: coord,
		MediaDir:             "/tmp/media",
		ConfigDir:            "/tmp/config",
	}
}

// TestNewModelInitializesStateSplash verifies the initial state is StateSplash.
func TestNewModelInitializesStateSplash(t *testing.T) {
	m := NewModel(buildTestDeps())
	if m.state != StateSplash {
		t.Errorf("initial state = %v, want StateSplash", m.state)
	}
}

// TestWindowSizeMsgUpdatesWidthHeight verifies that a WindowSizeMsg updates the model dimensions.
func TestWindowSizeMsgUpdatesWidthHeight(t *testing.T) {
	m := NewModel(buildTestDeps())
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(Model)
	if m.width != 100 || m.height != 40 {
		t.Errorf("width=%d height=%d, want 100/40", m.width, m.height)
	}
}

// TestCtrlCReturnsQuitCmd verifies that Ctrl+C produces tea.Quit.
func TestCtrlCReturnsQuitCmd(t *testing.T) {
	m := NewModel(buildTestDeps())
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("Ctrl+C should return a non-nil command")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("Ctrl+C cmd should produce tea.QuitMsg, got %T", msg)
	}
}

// TestQKeyOutsideTextInputReturnsQuit verifies that "q" on non-text-input states quits.
func TestQKeyOutsideTextInputReturnsQuit(t *testing.T) {
	m := NewModel(buildTestDeps())
	// state = StateSplash by default
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("'q' outside text input should return a cmd")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("'q' outside text-input should produce tea.QuitMsg, got %T", msg)
	}
}

// TestQKeyInWorkspaceInputDoesNotQuit verifies that "q" on StateWorkspaceInput
// is NOT intercepted as quit (it should be typed into the text field).
func TestQKeyInWorkspaceInputDoesNotQuit(t *testing.T) {
	m := NewModel(buildTestDeps())
	m.state = StateWorkspaceInput
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	// cmd may be nil or a textinput blink — but NOT tea.Quit.
	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); ok {
			t.Error("'q' in StateWorkspaceInput should NOT quit")
		}
	}
}

// TestPreflightStartedMsgTransitionsToPreflight verifies the state transition.
func TestPreflightStartedMsgTransitionsToPreflight(t *testing.T) {
	m := NewModel(buildTestDeps())
	updated, _ := m.Update(PreflightStartedMsg{})
	m = updated.(Model)
	if m.state != StatePreflight {
		t.Errorf("PreflightStartedMsg → state = %v, want StatePreflight", m.state)
	}
}

// TestPreflightPassedMsgTransitionsToWorkspaceInput verifies state transition on pass.
func TestPreflightPassedMsgTransitionsToWorkspaceInput(t *testing.T) {
	m := NewModel(buildTestDeps())
	m.state = StatePreflight
	updated, _ := m.Update(PreflightPassedMsg{})
	m = updated.(Model)
	if m.state != StateWorkspaceInput {
		t.Errorf("PreflightPassedMsg → state = %v, want StateWorkspaceInput", m.state)
	}
}

// TestPreflightResultMsgWithFailureStaysOnPreflight verifies that a blocking
// failure keeps the state on StatePreflight.
func TestPreflightResultMsgWithFailureStaysOnPreflight(t *testing.T) {
	m := NewModel(buildTestDeps())
	m.state = StatePreflight
	failReport := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckDockerDaemon, Status: preflight.StatusFail, Title: "Docker"},
		},
	}
	updated, _ := m.Update(PreflightResultMsg{Report: failReport})
	m = updated.(Model)
	if m.state != StatePreflight {
		t.Errorf("PreflightResultMsg with failure → state = %v, want StatePreflight", m.state)
	}
}

// TestWorkspaceEnteredMsgTransitionsToPortScan verifies workspace → port scan.
func TestWorkspaceEnteredMsgTransitionsToPortScan(t *testing.T) {
	m := NewModel(buildTestDeps())
	m.state = StateWorkspaceInput
	updated, _ := m.Update(WorkspaceEnteredMsg{Value: "my-site"})
	m = updated.(Model)
	if m.state != StatePortScan {
		t.Errorf("WorkspaceEnteredMsg → state = %v, want StatePortScan", m.state)
	}
}

// TestPortsConfirmedMsgTransitionsToEnvWrite verifies portscan → env write.
func TestPortsConfirmedMsgTransitionsToEnvWrite(t *testing.T) {
	m := NewModel(buildTestDeps())
	m.state = StatePortScan
	updated, _ := m.Update(PortsConfirmedMsg{FinalPorts: map[string]int{"POSTGRES_PORT": 5432}})
	m = updated.(Model)
	if m.state != StateEnvWrite {
		t.Errorf("PortsConfirmedMsg → state = %v, want StateEnvWrite", m.state)
	}
}

// TestEnvWrittenMsgTransitionsToPull verifies env-write → pull.
func TestEnvWrittenMsgTransitionsToPull(t *testing.T) {
	m := NewModel(buildTestDeps())
	m.state = StateEnvWrite
	updated, _ := m.Update(EnvWrittenMsg{Path: "/tmp/.env"})
	m = updated.(Model)
	if m.state != StatePull {
		t.Errorf("EnvWrittenMsg → state = %v, want StatePull", m.state)
	}
	if m.envPath != "/tmp/.env" {
		t.Errorf("envPath = %q, want /tmp/.env", m.envPath)
	}
}

// TestDeployStartedMsgTransitionsToDeploy verifies pull-done → deploy.
func TestDeployStartedMsgTransitionsToDeploy(t *testing.T) {
	m := NewModel(buildTestDeps())
	m.state = StatePull
	updated, _ := m.Update(DeployStartedMsg{})
	m = updated.(Model)
	if m.state != StateDeploy {
		t.Errorf("DeployStartedMsg → state = %v, want StateDeploy", m.state)
	}
}

// TestHealthTickMsgFromDeployTransitionsToVerify verifies deploy → verify on first HealthTickMsg.
func TestHealthTickMsgFromDeployTransitionsToVerify(t *testing.T) {
	m := NewModel(buildTestDeps())
	m.state = StateDeploy
	updated, _ := m.Update(HealthTickMsg{})
	m = updated.(Model)
	if m.state != StateVerify {
		t.Errorf("HealthTickMsg (from deploy) → state = %v, want StateVerify", m.state)
	}
}

// TestInstallSuccessMsgTransitionsToResult verifies verify → result (success).
func TestInstallSuccessMsgTransitionsToResult(t *testing.T) {
	m := NewModel(buildTestDeps())
	m.state = StateVerify
	updated, _ := m.Update(InstallSuccessMsg{EnvPath: "/tmp/.env"})
	m = updated.(Model)
	if m.state != StateResult {
		t.Errorf("InstallSuccessMsg → state = %v, want StateResult", m.state)
	}
	if !m.result.success {
		t.Error("result model should be success=true")
	}
}

// TestInstallFailureMsgTransitionsToResult verifies failure at any stage → result.
func TestInstallFailureMsgTransitionsToResult(t *testing.T) {
	tests := []struct {
		name  string
		state State
	}{
		{"from pull", StatePull},
		{"from deploy", StateDeploy},
		{"from verify", StateVerify},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(buildTestDeps())
			m.state = tt.state
			updated, _ := m.Update(InstallFailureMsg{Stage: "test", Err: nil})
			m = updated.(Model)
			if m.state != StateResult {
				t.Errorf("InstallFailureMsg → state = %v, want StateResult", m.state)
			}
			if m.result.success {
				t.Error("result model should be success=false on failure")
			}
		})
	}
}
