package tui

// fullflow_test.go: deterministic happy-path integration test for the full
// installer flow. Rather than using teatest (which requires goroutine
// coordination across a real tea.Program), this test drives the root Model
// directly via Update() calls in a deterministic sequence. This is the
// approach recommended by the go-testing skill when teatest is finicky.
//
// The test walks through ALL state transitions and verifies:
//   1. State machine advances correctly at each step.
//   2. FakeWriter received .env content.
//   3. FakeComposeRunner.Pull and Up were called (via error-free execution).
//   4. Final state is StateResult with success=true.

import (
	"strings"
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

// drainCmd executes a tea.Cmd and returns its Msg.
// Returns nil if cmd is nil.
func drainCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

// buildFullFlowDeps builds a Dependencies with all fakes for integration tests.
func buildFullFlowDeps(fw *envgen.FakeWriter, runner *compose.FakeComposeRunner) Dependencies {
	coord := preflight.Coordinator{
		OS:                &platform.FakeOSGuard{Linux: true, Name: "linux"},
		Arch:              &platform.FakeArchDetector{Arch: platform.ArchAMD64},
		Docker:            &docker.FakeDockerClient{VersionVal: docker.Version{Client: "25.0.0", Server: "25.0.0"}},
		Compose:           &compose.FakeComposeRunner{VersionVal: compose.Version{V2Plugin: true, Raw: "2.21.0"}},
		GPU:               &platform.FakeGPUDetector{Info: platform.GPUInfo{ToolkitInstalled: true}},
		Ports:             &ports.FakePortScanner{},
		Dirs:              &fakeDirChecker{writable: true},
		MediaDir:          "/tmp/media",
		ConfigDir:         "/tmp/config",
		MinDockerVersion:  "20.10.0",
		MinComposeVersion: "2.0.0",
	}
	return Dependencies{
		Theme:   theme.Default(),
		OS:      &platform.FakeOSGuard{Linux: true, Name: "linux"},
		Arch:    &platform.FakeArchDetector{Arch: platform.ArchAMD64},
		GPU:     &platform.FakeGPUDetector{Info: platform.GPUInfo{ToolkitInstalled: true}},
		Ports:   &ports.FakePortScanner{},
		Docker:  &docker.FakeDockerClient{VersionVal: docker.Version{Client: "25.0.0", Server: "25.0.0"}},
		Compose: runner,
		Envgen:  &envgen.Templater{PasswordGen: &secrets.FakeGenerator{Val: "integration-password"}},
		Writer:  fw,
		Assets: TemplateAssets{
			EnvExample: []byte("WORKSPACE=\nPOSTGRES_PASSWORD=\n"),
		},
		PreflightCoordinator: coord,
		MediaDir:             "/tmp/media",
		ConfigDir:            "/tmp/config",
	}
}

// runBatched executes a tea.Batch cmd and runs each result cmd, collecting
// any terminal messages that are state-transition messages.
// It iterates the batch by running the cmd and recursing on any returned Cmd.
// For simplicity, it runs the batch cmd once and returns the message.
// (Batches are handled by the real tea.Program; here we call the batch function
// directly, which returns the first message.)
func sendMsg(m Model, msg tea.Msg) (Model, tea.Cmd) {
	updated, cmd := m.Update(msg)
	return updated.(Model), cmd
}

// TestFullFlowHappyPath drives the entire installer flow deterministically.
func TestFullFlowHappyPath(t *testing.T) {
	fw := &envgen.FakeWriter{Written: make(map[string][]byte)}
	runner := &compose.FakeComposeRunner{
		VersionVal: compose.Version{V2Plugin: true, Raw: "2.21.0"},
		PullProgressMsgs: []compose.PullProgressMsg{
			{Service: "backend", Status: "Pulling"},
			{Service: "web", Status: "Pulled"},
		},
		UpProgressMsgs: []compose.UpProgressMsg{
			{Service: "backend", Status: "Started"},
			{Service: "web", Status: "Started"},
		},
		Healths: []compose.ServiceHealth{
			{Service: "backend", Status: "healthy", State: "running"},
			{Service: "web", Status: "healthy", State: "running"},
		},
	}

	m := NewModel(buildFullFlowDeps(fw, runner))

	// --- Step 1: Splash → press Enter → PreflightStartedMsg ---
	m, cmd := sendMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != StateSplash {
		// Enter on splash emits PreflightStartedMsg; apply that msg next.
	}
	msg := drainCmd(cmd)
	if _, ok := msg.(PreflightStartedMsg); !ok {
		t.Fatalf("splash Enter should produce PreflightStartedMsg, got %T", msg)
	}

	// --- Step 2: Apply PreflightStartedMsg → StatePreflight ---
	m, cmd = sendMsg(m, msg.(PreflightStartedMsg))
	if m.state != StatePreflight {
		t.Fatalf("after PreflightStartedMsg state = %v, want StatePreflight", m.state)
	}

	// --- Step 3: Preflight runs → PreflightResultMsg (no blocking failures) ---
	preflightResultMsg := drainCmd(cmd)
	m, cmd = sendMsg(m, preflightResultMsg)
	_ = cmd // result is nil for a passing preflight

	// --- Step 4: User presses Enter on preflight → PreflightPassedMsg ---
	m, cmd = sendMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
	passedMsg := drainCmd(cmd)
	if _, ok := passedMsg.(PreflightPassedMsg); !ok {
		t.Fatalf("Enter on passing preflight should produce PreflightPassedMsg, got %T", passedMsg)
	}

	// --- Step 5: Apply PreflightPassedMsg → StateWorkspaceInput ---
	m, cmd = sendMsg(m, passedMsg.(PreflightPassedMsg))
	if m.state != StateWorkspaceInput {
		t.Fatalf("after PreflightPassedMsg state = %v, want StateWorkspaceInput", m.state)
	}
	_ = cmd

	// --- Step 6: Type "mysite" + Enter → WorkspaceEnteredMsg ---
	for _, ch := range "mysite" {
		m, _ = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	m, cmd = sendMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
	enteredMsg := drainCmd(cmd)
	wsMsg, ok := enteredMsg.(WorkspaceEnteredMsg)
	if !ok {
		t.Fatalf("Enter after workspace text should produce WorkspaceEnteredMsg, got %T", enteredMsg)
	}
	if wsMsg.Value != "mysite" {
		t.Errorf("WorkspaceEnteredMsg.Value = %q, want mysite", wsMsg.Value)
	}

	// --- Step 7: Apply WorkspaceEnteredMsg → StatePortScan ---
	m, cmd = sendMsg(m, wsMsg)
	if m.state != StatePortScan {
		t.Fatalf("after WorkspaceEnteredMsg state = %v, want StatePortScan", m.state)
	}

	// --- Step 8: Port scan runs → PortScanResultMsg → PortsConfirmedMsg (no conflicts) ---
	scanMsg := drainCmd(cmd) // cmd = scanAll
	m, cmd = sendMsg(m, scanMsg)
	// FakePortScanner has no occupied ports, so PortScanResultMsg has no conflicts →
	// portscan sub-model immediately emits PortsConfirmedMsg.
	confirmedMsg := drainCmd(cmd)
	ports, ok := confirmedMsg.(PortsConfirmedMsg)
	if !ok {
		t.Fatalf("PortScanResultMsg with no conflicts should produce PortsConfirmedMsg, got %T", confirmedMsg)
	}

	// --- Step 9: Apply PortsConfirmedMsg → StateEnvWrite ---
	m, cmd = sendMsg(m, ports)
	if m.state != StateEnvWrite {
		t.Fatalf("after PortsConfirmedMsg state = %v, want StateEnvWrite", m.state)
	}

	// --- Step 10: EnvWrite runs → EnvWrittenMsg ---
	envMsg := drainCmd(cmd)
	written, ok := envMsg.(EnvWrittenMsg)
	if !ok {
		t.Fatalf("EnvWrite cmd should produce EnvWrittenMsg, got %T", envMsg)
	}
	// Verify FakeWriter has the .env content.
	envData, found := fw.Written[written.Path]
	if !found {
		t.Fatalf("FakeWriter should have written to %q", written.Path)
	}
	if !strings.Contains(string(envData), "WORKSPACE=mysite") {
		t.Errorf("written .env should contain WORKSPACE=mysite, got:\n%s", string(envData))
	}

	// --- Step 11: Apply EnvWrittenMsg → StatePull ---
	m, cmd = sendMsg(m, written)
	if m.state != StatePull {
		t.Fatalf("after EnvWrittenMsg state = %v, want StatePull", m.state)
	}

	// --- Step 12: Pull runs → PullCompleteMsg (via runPull helper) ---
	// The real batch cmd opens a goroutine; for deterministic testing we use runPull.
	_ = cmd // discard the batched Init cmd
	pullResult := m.pull.runPull()
	m, cmd = sendMsg(m, pullResult)
	// runPull returns PullCompleteMsg; Update on PullCompleteMsg → done=true, emits DeployStartedMsg.
	if !m.pull.done {
		t.Error("pull should be done after PullCompleteMsg")
	}
	deployMsg := drainCmd(cmd)
	if _, ok := deployMsg.(DeployStartedMsg); !ok {
		t.Fatalf("PullCompleteMsg → Cmd should produce DeployStartedMsg, got %T", deployMsg)
	}

	// --- Step 13: Apply DeployStartedMsg → StateDeploy ---
	m, cmd = sendMsg(m, deployMsg.(DeployStartedMsg))
	if m.state != StateDeploy {
		t.Fatalf("after DeployStartedMsg state = %v, want StateDeploy", m.state)
	}

	// --- Step 14: Deploy runs → DeployCompleteMsg → HealthTickMsg ---
	_ = cmd // discard batch cmd
	deployResult := m.deploy.runDeploy()
	m, cmd = sendMsg(m, deployResult)
	if !m.deploy.done {
		t.Error("deploy should be done after DeployCompleteMsg")
	}
	healthTickMsg := drainCmd(cmd)
	if _, ok := healthTickMsg.(HealthTickMsg); !ok {
		t.Fatalf("DeployCompleteMsg → Cmd should produce HealthTickMsg, got %T", healthTickMsg)
	}

	// --- Step 15: Apply HealthTickMsg (from deploy) → StateVerify ---
	m, cmd = sendMsg(m, healthTickMsg.(HealthTickMsg))
	if m.state != StateVerify {
		t.Fatalf("HealthTickMsg from deploy → state = %v, want StateVerify", m.state)
	}

	// --- Step 16: Verify polls → all healthy → InstallSuccessMsg ---
	// The verify Init returns a tea.Tick; for deterministic testing send HealthTickMsg directly.
	_ = cmd
	m, cmd = sendMsg(m, HealthTickMsg{})
	if !m.verify.done {
		t.Error("verify should be done when all services are healthy")
	}
	successMsg := drainCmd(cmd)
	if _, ok := successMsg.(InstallSuccessMsg); !ok {
		t.Fatalf("all-healthy verify → Cmd should produce InstallSuccessMsg, got %T", successMsg)
	}

	// --- Step 17: Apply InstallSuccessMsg → StateResult ---
	m, _ = sendMsg(m, successMsg.(InstallSuccessMsg))
	if m.state != StateResult {
		t.Fatalf("after InstallSuccessMsg state = %v, want StateResult", m.state)
	}
	if !m.result.success {
		t.Error("result should be success=true")
	}

	// --- Step 18: Press Enter → tea.Quit ---
	_, quitCmd := sendMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
	if quitCmd == nil {
		t.Fatal("Enter on result should return a Cmd")
	}
	quitMsg := quitCmd()
	if _, ok := quitMsg.(tea.QuitMsg); !ok {
		t.Errorf("Enter on result → Cmd should produce tea.QuitMsg, got %T", quitMsg)
	}

	// Verify view contains "complete".
	view := m.result.View()
	if !strings.Contains(strings.ToLower(view), "complete") {
		t.Errorf("result view should contain 'complete', got:\n%q", view)
	}
}
