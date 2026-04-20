package tui

// fullflow_bootstrap_test.go: deterministic integration test for the bootstrap path.
//
// Scenario:
//   1. Preflight fails: ConfigDir is not writable (first call).
//   2. Root model routes to StateBootstrap.
//   3. User presses 'y' → FakeExecutor runs action → Err=nil → BootstrapCompleteMsg.
//   4. Root re-arms preflight; second call passes → StateWorkspaceInput after Enter.
//   5. Rest of happy path identical to fullflow_test.go → StateResult success.
//
// Also tests the BootstrapSkippedMsg path:
//   - After bootstrap is triggered, pressing 'n' returns to preflight with the
//     original failing report preserved.

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

// countingDirChecker is a test double that fails on the first call for a
// specific path and succeeds on subsequent calls. This simulates the bootstrap
// action having created the directory.
type countingDirChecker struct {
	failPath string // path that should fail on first call
	calls    map[string]int
}

func newCountingDirChecker(failPath string) *countingDirChecker {
	return &countingDirChecker{
		failPath: failPath,
		calls:    make(map[string]int),
	}
}

func (c *countingDirChecker) IsWritable(path string) (bool, string) {
	c.calls[path]++
	if path == c.failPath && c.calls[path] == 1 {
		return false, "permission denied (simulated)"
	}
	return true, ""
}

// buildBootstrapFlowDeps builds Dependencies where ConfigDir fails once, then passes.
func buildBootstrapFlowDeps(
	fw *envgen.FakeWriter,
	runner *compose.FakeComposeRunner,
	dirChecker preflight.DirectoryChecker,
	exec Executor,
) Dependencies {
	coord := preflight.Coordinator{
		OS:                &platform.FakeOSGuard{Linux: true, Name: "linux"},
		Arch:              &platform.FakeArchDetector{Arch: platform.ArchAMD64},
		Docker:            &docker.FakeDockerClient{VersionVal: docker.Version{Client: "25.0.0", Server: "25.0.0"}},
		Compose:           &compose.FakeComposeRunner{VersionVal: compose.Version{V2Plugin: true, Raw: "2.21.0"}},
		GPU:               &platform.FakeGPUDetector{Info: platform.GPUInfo{ToolkitInstalled: true}},
		Ports:             &ports.FakePortScanner{},
		Dirs:              dirChecker,
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
		Envgen:  &envgen.Templater{PasswordGen: &secrets.FakeGenerator{Val: "bootstrap-password"}},
		Writer:  fw,
		Assets: TemplateAssets{
			EnvExample: []byte("WORKSPACE=\nPOSTGRES_PASSWORD=\n"),
		},
		PreflightCoordinator: coord,
		Executor:             exec,
		// Default env for bootstrap flow tests: docker present, in group, no systemd.
		// Individual tests that need Docker-specific classification override this.
		Env: BootstrapEnv{
			UserName:            "testuser",
			DockerBinaryPresent: true,
			UserInDockerGroup:   true,
			SystemdPresent:      false,
		},
		MediaDir:  "/tmp/media",
		ConfigDir: "/tmp/config",
	}
}

// TestFullFlowBootstrapHappyPath drives the bootstrap path end-to-end:
// preflight FAIL (ConfigDir) → bootstrap → user confirms → re-preflight PASS → continues.
func TestFullFlowBootstrapHappyPath(t *testing.T) {
	fw := &envgen.FakeWriter{Written: make(map[string][]byte)}
	runner := &compose.FakeComposeRunner{
		VersionVal: compose.Version{V2Plugin: true, Raw: "2.21.0"},
		PullProgressMsgs: []compose.PullProgressMsg{
			{Service: "backend", Status: "Pulled"},
		},
		UpProgressMsgs: []compose.UpProgressMsg{
			{Service: "backend", Status: "Started"},
		},
		Healths: []compose.ServiceHealth{
			{Service: "backend", Status: "healthy", State: "running"},
		},
	}

	// ConfigDir fails on first check, passes on second (simulating bootstrap fix).
	dirChecker := newCountingDirChecker("/tmp/config")

	// Executor that simulates a successful sudo command.
	fe := &FakeExecutor{
		Results: []BootstrapActionResultMsg{
			{ActionID: string(preflight.CheckConfigWritable), Err: nil},
		},
	}

	m := NewModel(buildBootstrapFlowDeps(fw, runner, dirChecker, fe))

	// --- Step 1: Splash → press Enter → PreflightStartedMsg ---
	m, cmd := sendMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
	msg := drainCmd(cmd)
	if _, ok := msg.(PreflightStartedMsg); !ok {
		t.Fatalf("splash Enter should produce PreflightStartedMsg, got %T", msg)
	}

	// --- Step 2: Apply PreflightStartedMsg → StatePreflight ---
	m, cmd = sendMsg(m, msg.(PreflightStartedMsg))
	if m.state != StatePreflight {
		t.Fatalf("after PreflightStartedMsg state = %v, want StatePreflight", m.state)
	}

	// --- Step 3: Preflight runs → ConfigDir FAIL → StateBootstrap ---
	preflightResultMsg := drainCmd(cmd)
	m, cmd = sendMsg(m, preflightResultMsg)
	if m.state != StateBootstrap {
		t.Fatalf("ConfigDir FAIL should route to StateBootstrap, got state = %v", m.state)
	}
	_ = cmd // bootstrap Init() returns nil

	// --- Step 4: Bootstrap confirming view contains the action ---
	view := m.bootstrap.View()
	if !strings.Contains(view, "/tmp/config") {
		t.Errorf("bootstrap view should mention /tmp/config, got:\n%s", view)
	}

	// --- Step 5: User presses 'y' → executor runs → BootstrapActionResultMsg ---
	m, cmd = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if m.state != StateBootstrap {
		t.Fatalf("after pressing y state should still be StateBootstrap, got %v", m.state)
	}
	if cmd == nil {
		t.Fatal("pressing y should return a cmd (executor)")
	}
	actionResultMsg := drainCmd(cmd)
	if _, ok := actionResultMsg.(BootstrapActionResultMsg); !ok {
		t.Fatalf("executor cmd should produce BootstrapActionResultMsg, got %T", actionResultMsg)
	}

	// --- Step 6: Apply BootstrapActionResultMsg → BootstrapCompleteMsg ---
	m, cmd = sendMsg(m, actionResultMsg)
	completeMsg := drainCmd(cmd)
	if _, ok := completeMsg.(BootstrapCompleteMsg); !ok {
		t.Fatalf("all actions done should emit BootstrapCompleteMsg, got %T", completeMsg)
	}

	// --- Step 7: Apply BootstrapCompleteMsg → StatePreflight (rearmed) ---
	m, cmd = sendMsg(m, completeMsg)
	if m.state != StatePreflight {
		t.Fatalf("BootstrapCompleteMsg → state = %v, want StatePreflight", m.state)
	}
	if cmd == nil {
		t.Fatal("BootstrapCompleteMsg should return re-arm cmd")
	}

	// --- Step 8: Re-preflight runs → second call passes → no blockers ---
	rePreflightResultMsg := drainCmd(cmd)
	m, cmd = sendMsg(m, rePreflightResultMsg)
	if m.state != StatePreflight {
		t.Fatalf("after re-preflight result state = %v, want StatePreflight", m.state)
	}
	// Preflight should now have no blocking failures.
	if m.preflight.report == nil {
		t.Fatal("preflight report should be set after re-preflight result")
	}
	if m.preflight.report.HasBlockingFailure() {
		t.Error("re-preflight report should have no blocking failures")
	}

	// --- Step 9: Press Enter → PreflightPassedMsg → StateWorkspaceInput ---
	m, cmd = sendMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
	passedMsg := drainCmd(cmd)
	if _, ok := passedMsg.(PreflightPassedMsg); !ok {
		t.Fatalf("Enter on passing re-preflight should produce PreflightPassedMsg, got %T", passedMsg)
	}
	m, cmd = sendMsg(m, passedMsg.(PreflightPassedMsg))
	if m.state != StateWorkspaceInput {
		t.Fatalf("PreflightPassedMsg → state = %v, want StateWorkspaceInput", m.state)
	}
	_ = cmd

	// --- Step 10: Type workspace name + Enter ---
	for _, ch := range "bootstrap-site" {
		m, _ = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	m, cmd = sendMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
	enteredMsg := drainCmd(cmd)
	wsMsg, ok := enteredMsg.(WorkspaceEnteredMsg)
	if !ok {
		t.Fatalf("Enter after workspace text should produce WorkspaceEnteredMsg, got %T", enteredMsg)
	}
	if wsMsg.Value != "bootstrap-site" {
		t.Errorf("WorkspaceEnteredMsg.Value = %q, want bootstrap-site", wsMsg.Value)
	}

	// --- Step 11: Apply WorkspaceEnteredMsg → StatePortScan ---
	m, cmd = sendMsg(m, wsMsg)
	if m.state != StatePortScan {
		t.Fatalf("WorkspaceEnteredMsg → state = %v, want StatePortScan", m.state)
	}

	// --- Step 12: Port scan → PortsConfirmedMsg → StateEnvWrite ---
	scanMsg := drainCmd(cmd)
	m, cmd = sendMsg(m, scanMsg)
	confirmedMsg := drainCmd(cmd)
	ports, ok := confirmedMsg.(PortsConfirmedMsg)
	if !ok {
		t.Fatalf("scan result with no conflicts should produce PortsConfirmedMsg, got %T", confirmedMsg)
	}
	m, cmd = sendMsg(m, ports)
	if m.state != StateEnvWrite {
		t.Fatalf("PortsConfirmedMsg → state = %v, want StateEnvWrite", m.state)
	}

	// --- Step 13: EnvWrite → EnvWrittenMsg → StatePull ---
	envMsg := drainCmd(cmd)
	written, ok := envMsg.(EnvWrittenMsg)
	if !ok {
		t.Fatalf("EnvWrite cmd should produce EnvWrittenMsg, got %T", envMsg)
	}
	m, cmd = sendMsg(m, written)
	if m.state != StatePull {
		t.Fatalf("EnvWrittenMsg → state = %v, want StatePull", m.state)
	}

	// --- Step 14: Pull → DeployStartedMsg → StateDeploy ---
	_ = cmd
	pullResult := m.pull.runPull()
	m, cmd = sendMsg(m, pullResult)
	deployMsg := drainCmd(cmd)
	if _, ok := deployMsg.(DeployStartedMsg); !ok {
		t.Fatalf("PullCompleteMsg → should produce DeployStartedMsg, got %T", deployMsg)
	}
	m, cmd = sendMsg(m, deployMsg.(DeployStartedMsg))
	if m.state != StateDeploy {
		t.Fatalf("DeployStartedMsg → state = %v, want StateDeploy", m.state)
	}

	// --- Step 15: Deploy → HealthTickMsg → StateVerify ---
	_ = cmd
	deployResult := m.deploy.runDeploy()
	m, cmd = sendMsg(m, deployResult)
	healthTickMsg := drainCmd(cmd)
	if _, ok := healthTickMsg.(HealthTickMsg); !ok {
		t.Fatalf("DeployCompleteMsg → should produce HealthTickMsg, got %T", healthTickMsg)
	}
	m, cmd = sendMsg(m, healthTickMsg.(HealthTickMsg))
	if m.state != StateVerify {
		t.Fatalf("HealthTickMsg → state = %v, want StateVerify", m.state)
	}

	// --- Step 16: Verify → InstallSuccessMsg → StateResult ---
	_ = cmd
	m, cmd = sendMsg(m, HealthTickMsg{})
	successMsg := drainCmd(cmd)
	if _, ok := successMsg.(InstallSuccessMsg); !ok {
		t.Fatalf("all-healthy verify → should produce InstallSuccessMsg, got %T", successMsg)
	}
	m, _ = sendMsg(m, successMsg.(InstallSuccessMsg))
	if m.state != StateResult {
		t.Fatalf("InstallSuccessMsg → state = %v, want StateResult", m.state)
	}
	if !m.result.success {
		t.Error("result should be success=true after bootstrap path")
	}
}

// TestFullFlowBootstrapSkippedPreservesReport verifies that pressing 'n' in
// bootstrap returns to preflight with the original failing report intact.
func TestFullFlowBootstrapSkippedPreservesReport(t *testing.T) {
	fw := &envgen.FakeWriter{Written: make(map[string][]byte)}
	runner := &compose.FakeComposeRunner{
		VersionVal: compose.Version{V2Plugin: true, Raw: "2.21.0"},
	}

	// MediaDir will fail (first and only call — user cancels).
	dirChecker := newCountingDirChecker("/tmp/media")
	fe := &FakeExecutor{
		Results: []BootstrapActionResultMsg{
			{ActionID: string(preflight.CheckMediaWritable), Err: nil},
		},
	}

	m := NewModel(buildBootstrapFlowDeps(fw, runner, dirChecker, fe))

	// Splash → Enter → PreflightStartedMsg → StatePreflight → run preflight → StateBootstrap.
	m, cmd := sendMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
	m, cmd = sendMsg(m, drainCmd(cmd).(PreflightStartedMsg))
	preflightResultMsg := drainCmd(cmd)
	m, _ = sendMsg(m, preflightResultMsg)
	if m.state != StateBootstrap {
		t.Fatalf("expected StateBootstrap after MediaDir FAIL, got %v", m.state)
	}

	// Press 'n' → BootstrapSkippedMsg.
	m, cmd = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	skippedMsg := drainCmd(cmd)
	if _, ok := skippedMsg.(BootstrapSkippedMsg); !ok {
		t.Fatalf("pressing n should produce BootstrapSkippedMsg, got %T", skippedMsg)
	}

	// Apply BootstrapSkippedMsg → StatePreflight with report frozen.
	m, _ = sendMsg(m, skippedMsg.(BootstrapSkippedMsg))
	if m.state != StatePreflight {
		t.Fatalf("BootstrapSkippedMsg → state = %v, want StatePreflight", m.state)
	}
	if m.preflight.report == nil {
		t.Fatal("preflight report should still be set after user declined bootstrap")
	}
	if !m.preflight.report.HasBlockingFailure() {
		t.Error("preflight report should still have the blocking failure after skip")
	}

	// Verify the view shows the blocking issue (not a "passed" screen).
	view := m.preflight.View()
	if !strings.Contains(view, "Blocking") && !strings.Contains(view, "error") && !strings.Contains(view, "✗") {
		t.Errorf("preflight view after skip should show blocking issue indicator, got:\n%s", view)
	}
}
