package tui

// fullflow_docker_bootstrap_test.go: deterministic integration tests for the
// Docker-specific bootstrap paths added in installer-docker-bootstrap.
//
// Each test controls:
//   - BootstrapEnv: injected per scenario
//   - FakeDockerClient: controls Probe() result (fails first call, passes on second)
//   - FakeExecutor: pre-loaded with success results for each expected action
//   - DirectoryChecker: always writable (these tests focus on Docker, not dirs)

import (
	"errors"
	"strings"
	"testing"

	"context"

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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// countingDockerClient fails Probe on the first call, succeeds on subsequent calls.
// This simulates the bootstrap action having installed/started/configured Docker.
type countingDockerClient struct {
	probeErr error // error to return on first Probe call
	calls    int
	// Delegate to a real FakeDockerClient for Version/Info etc.
	inner *docker.FakeDockerClient
}

func newCountingDockerClient(firstErr error) *countingDockerClient {
	return &countingDockerClient{
		probeErr: firstErr,
		inner:    &docker.FakeDockerClient{VersionVal: docker.Version{Client: "25.0.0", Server: "25.0.0"}},
	}
}

func (c *countingDockerClient) Probe(_ context.Context) error {
	c.calls++
	if c.calls == 1 {
		return c.probeErr
	}
	return nil
}

func (c *countingDockerClient) Info(ctx context.Context) (docker.Info, error) {
	return c.inner.Info(ctx)
}

func (c *countingDockerClient) Version(ctx context.Context) (docker.Version, error) {
	return c.inner.Version(ctx)
}

func (c *countingDockerClient) HasRuntime(ctx context.Context, name string) (bool, error) {
	return c.inner.HasRuntime(ctx, name)
}

// fakeDirCheckerAlwaysWritable is a DirectoryChecker that always succeeds.
type fakeDirCheckerAlwaysWritable struct{}

func (fakeDirCheckerAlwaysWritable) IsWritable(_ string) (bool, string) { return true, "" }

// buildDockerBootstrapDeps builds Dependencies for Docker-specific bootstrap tests.
// dockerClient controls Docker Probe behavior.
// env controls BootstrapEnv for classification.
// exec is the pre-loaded FakeExecutor.
func buildDockerBootstrapDeps(
	dockerClient docker.DockerClient,
	env BootstrapEnv,
	exec Executor,
) Dependencies {
	coord := preflight.Coordinator{
		OS:                &platform.FakeOSGuard{Linux: true, Name: "linux"},
		Arch:              &platform.FakeArchDetector{Arch: platform.ArchAMD64},
		Docker:            dockerClient,
		Compose:           &compose.FakeComposeRunner{VersionVal: compose.Version{V2Plugin: true, Raw: "2.21.0"}},
		GPU:               &platform.FakeGPUDetector{Info: platform.GPUInfo{ToolkitInstalled: true}},
		Ports:             &ports.FakePortScanner{},
		Dirs:              fakeDirCheckerAlwaysWritable{},
		MediaDir:          "/tmp/media",
		ConfigDir:         "/tmp/config",
		MinDockerVersion:  "20.10.0",
		MinComposeVersion: "2.0.0",
	}
	runner := &compose.FakeComposeRunner{
		VersionVal: compose.Version{V2Plugin: true, Raw: "2.21.0"},
		PullProgressMsgs: []compose.PullProgressMsg{
			{Service: "backend", Status: "Pulled"},
		},
		UpProgressMsgs: []compose.UpProgressMsg{
			{Service: "backend", Status: "Started"},
		},
		Healths: []compose.ServiceHealth{
			{Service: "backend", Status: "healthy"},
		},
	}
	return Dependencies{
		Theme:   theme.Default(),
		OS:      &platform.FakeOSGuard{Linux: true, Name: "linux"},
		Arch:    &platform.FakeArchDetector{Arch: platform.ArchAMD64},
		GPU:     &platform.FakeGPUDetector{Info: platform.GPUInfo{ToolkitInstalled: true}},
		Ports:   &ports.FakePortScanner{},
		Docker:  dockerClient,
		Compose: runner,
		Envgen:  &envgen.Templater{PasswordGen: &secrets.FakeGenerator{Val: "docker-password"}},
		Writer:  &envgen.FakeWriter{Written: make(map[string][]byte)},
		Assets: TemplateAssets{
			EnvExample: []byte("WORKSPACE=\nPOSTGRES_PASSWORD=\n"),
		},
		PreflightCoordinator: coord,
		Executor:             exec,
		Env:                  env,
		MediaDir:             "/tmp/media",
		ConfigDir:            "/tmp/config",
	}
}

// bootstrapToComplete drives a model from StateBootstrap through action execution
// to BootstrapCompleteMsg or the banner screen. Returns the model after BootstrapCompleteMsg
// is processed (state = StatePreflight, rearmed) AND the rearmed preflight cmd.
// If the bootstrap has banners, presses Enter to dismiss them first.
func bootstrapToComplete(t *testing.T, m Model, actionCount int) (Model, tea.Cmd, []string) {
	t.Helper()

	// Press 'y' to start execution.
	m, cmd := sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("pressing y should return an executor cmd")
	}

	// Drain all action results.
	var actionIDs []string
	for i := 0; i < actionCount; i++ {
		actionResultMsg := drainCmd(cmd)
		result, ok := actionResultMsg.(BootstrapActionResultMsg)
		if !ok {
			t.Fatalf("action %d: expected BootstrapActionResultMsg, got %T", i+1, actionResultMsg)
		}
		actionIDs = append(actionIDs, result.ActionID)
		m, cmd = sendMsg(m, result)
	}

	// After last action: either BootstrapCompleteMsg or banner screen.
	if m.bootstrap.showingBanner {
		// Banner screen active — press Enter to dismiss.
		m, cmd = sendMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
	}

	// Now cmd should produce BootstrapCompleteMsg.
	if cmd == nil {
		t.Fatal("expected cmd after banner/action completion")
	}
	completeMsg := drainCmd(cmd)
	if _, ok := completeMsg.(BootstrapCompleteMsg); !ok {
		t.Fatalf("expected BootstrapCompleteMsg, got %T", completeMsg)
	}

	// Apply BootstrapCompleteMsg → StatePreflight rearmed. Keep the re-arm cmd.
	var rearmCmd tea.Cmd
	m, rearmCmd = sendMsg(m, completeMsg)
	if m.state != StatePreflight {
		t.Fatalf("after BootstrapCompleteMsg state = %v, want StatePreflight", m.state)
	}
	return m, rearmCmd, actionIDs
}

// ---------------------------------------------------------------------------
// Test 1: Docker-missing path
// ---------------------------------------------------------------------------

// TestFullFlowDockerMissingBootstrapsInstall verifies that when Docker binary is
// absent, the installer offers docker_install action and continues after it runs.
func TestFullFlowDockerMissingBootstrapsInstall(t *testing.T) {
	dockerClient := newCountingDockerClient(errors.New("docker: command not found"))
	env := BootstrapEnv{
		UserName:            "alice",
		DockerBinaryPresent: false,
		UserInDockerGroup:   false,
		SystemdPresent:      false,
	}
	fe := &FakeExecutor{
		Results: []BootstrapActionResultMsg{
			{ActionID: ActionIDDockerInstall, Err: nil},
		},
	}

	m := NewModel(buildDockerBootstrapDeps(dockerClient, env, fe))

	// Splash → preflight start.
	m, cmd := sendMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
	m, cmd = sendMsg(m, drainCmd(cmd).(PreflightStartedMsg))

	// First preflight: Docker Probe fails → CheckDockerDaemon FAIL.
	preflightResult := drainCmd(cmd)
	m, _ = sendMsg(m, preflightResult)

	// Should be in StateBootstrap with docker_install action.
	if m.state != StateBootstrap {
		t.Fatalf("docker missing → state = %v, want StateBootstrap", m.state)
	}

	// Confirm bootstrap model has docker_install action.
	if len(m.bootstrap.actions) != 1 {
		t.Fatalf("bootstrap actions count = %d, want 1", len(m.bootstrap.actions))
	}
	if m.bootstrap.actions[0].ID != ActionIDDockerInstall {
		t.Errorf("bootstrap action[0].ID = %q, want %q", m.bootstrap.actions[0].ID, ActionIDDockerInstall)
	}

	// View should mention get.docker.com.
	view := m.bootstrap.View()
	if !strings.Contains(view, "get.docker.com") {
		t.Errorf("bootstrap view should mention get.docker.com, got:\n%s", view)
	}

	// Execute bootstrap → complete; get the rearmed preflight cmd.
	m, rearmCmd, actionIDs := bootstrapToComplete(t, m, 1)

	// Verify the docker_install action was executed.
	if len(actionIDs) != 1 || actionIDs[0] != ActionIDDockerInstall {
		t.Errorf("expected docker_install action, got: %v", actionIDs)
	}

	// Second preflight runs → now passes (Docker Probe succeeds on 2nd call).
	if rearmCmd == nil {
		t.Fatal("BootstrapCompleteMsg should return a re-arm cmd")
	}
	rePreflightResult := drainCmd(rearmCmd)
	m, cmd = sendMsg(m, rePreflightResult)
	if m.state != StatePreflight {
		t.Fatalf("after re-preflight result state = %v, want StatePreflight", m.state)
	}

	// Press Enter → PreflightPassedMsg → StateWorkspaceInput.
	m, cmd = sendMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
	passedMsg := drainCmd(cmd)
	if _, ok := passedMsg.(PreflightPassedMsg); !ok {
		t.Fatalf("Enter on passing preflight should emit PreflightPassedMsg, got %T", passedMsg)
	}
	m, _ = sendMsg(m, passedMsg.(PreflightPassedMsg))
	if m.state != StateWorkspaceInput {
		t.Fatalf("PreflightPassedMsg → state = %v, want StateWorkspaceInput", m.state)
	}
}

// ---------------------------------------------------------------------------
// Test 2: User-not-in-group path
// ---------------------------------------------------------------------------

// TestFullFlowUserNotInGroupBootstrapsGroupAdd verifies that when Docker binary
// is present but user is not in docker group, the installer offers docker_group_add
// action with a post-action banner.
func TestFullFlowUserNotInGroupBootstrapsGroupAdd(t *testing.T) {
	dockerClient := newCountingDockerClient(errors.New("permission denied while trying to connect to Docker"))
	env := BootstrapEnv{
		UserName:            "alice",
		DockerBinaryPresent: true,
		UserInDockerGroup:   false,
		SystemdPresent:      true,
	}
	fe := &FakeExecutor{
		Results: []BootstrapActionResultMsg{
			{ActionID: ActionIDDockerGroup, Err: nil},
		},
	}

	m := NewModel(buildDockerBootstrapDeps(dockerClient, env, fe))

	// Splash → preflight.
	m, cmd := sendMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
	m, cmd = sendMsg(m, drainCmd(cmd).(PreflightStartedMsg))

	// First preflight: Docker Probe fails → CheckDockerDaemon FAIL.
	preflightResult := drainCmd(cmd)
	m, _ = sendMsg(m, preflightResult)

	if m.state != StateBootstrap {
		t.Fatalf("user-not-in-group → state = %v, want StateBootstrap", m.state)
	}

	// Should have docker_group_add action with a PostActionBanner.
	if len(m.bootstrap.actions) != 1 {
		t.Fatalf("bootstrap actions count = %d, want 1", len(m.bootstrap.actions))
	}
	if m.bootstrap.actions[0].ID != ActionIDDockerGroup {
		t.Errorf("bootstrap action[0].ID = %q, want %q", m.bootstrap.actions[0].ID, ActionIDDockerGroup)
	}
	if m.bootstrap.actions[0].PostActionBanner == "" {
		t.Error("docker_group_add action should have PostActionBanner")
	}

	// Press 'y' to start.
	m, cmd = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	actionResult := drainCmd(cmd)
	m, cmd = sendMsg(m, actionResult)

	// Should enter banner screen (not immediately emit BootstrapCompleteMsg).
	if !m.bootstrap.showingBanner {
		t.Fatal("after docker_group_add with PostActionBanner, model should show banner screen")
	}

	// View should contain banner text.
	bannerView := m.bootstrap.View()
	if !strings.Contains(bannerView, "Log out") && !strings.Contains(bannerView, "newgrp") {
		t.Errorf("banner view should contain 'Log out' or 'newgrp', got:\n%s", bannerView)
	}
	if !strings.Contains(bannerView, "Enter") {
		t.Errorf("banner view should prompt for Enter, got:\n%s", bannerView)
	}

	// Press Enter on the banner screen. Because docker_group_add emitted a
	// banner (the "log out and back in" message), the installer CANNOT rerun
	// preflight in this session — group membership only applies after re-login.
	// The new behaviour is to print the banner to scrollback and quit cleanly,
	// so the user sees the instruction after the TUI exits.
	_, cmd = sendMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on banner screen should return cmd")
	}
	// The cmd is tea.Sequence(tea.Println..., tea.Quit). We don't introspect
	// the unexported sequenceMsg type; instead we assert that the cmd does NOT
	// produce a BootstrapCompleteMsg (which would incorrectly rerun preflight).
	msg := drainCmd(cmd)
	if _, ok := msg.(BootstrapCompleteMsg); ok {
		t.Fatalf("Enter on banner with docker_group_add should NOT emit BootstrapCompleteMsg (would loop); got %T", msg)
	}
}

// ---------------------------------------------------------------------------
// Test 3: Systemctl path
// ---------------------------------------------------------------------------

// TestFullFlowSystemctlBootstrapsDaemonStart verifies that when Docker binary is
// present, user is in docker group, and systemd is available, the installer offers
// systemctl enable --now docker.
func TestFullFlowSystemctlBootstrapsDaemonStart(t *testing.T) {
	dockerClient := newCountingDockerClient(errors.New("docker daemon is not running"))
	env := BootstrapEnv{
		UserName:            "alice",
		DockerBinaryPresent: true,
		UserInDockerGroup:   true,
		SystemdPresent:      true,
	}
	fe := &FakeExecutor{
		Results: []BootstrapActionResultMsg{
			{ActionID: ActionIDSystemdStart, Err: nil},
		},
	}

	m := NewModel(buildDockerBootstrapDeps(dockerClient, env, fe))

	// Splash → preflight.
	m, cmd := sendMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
	m, cmd = sendMsg(m, drainCmd(cmd).(PreflightStartedMsg))

	preflightResult := drainCmd(cmd)
	m, _ = sendMsg(m, preflightResult)

	if m.state != StateBootstrap {
		t.Fatalf("systemctl path → state = %v, want StateBootstrap", m.state)
	}

	// Should have systemd_start_docker action.
	if len(m.bootstrap.actions) != 1 {
		t.Fatalf("bootstrap actions count = %d, want 1", len(m.bootstrap.actions))
	}
	if m.bootstrap.actions[0].ID != ActionIDSystemdStart {
		t.Errorf("bootstrap action[0].ID = %q, want %q", m.bootstrap.actions[0].ID, ActionIDSystemdStart)
	}
	// No banner for systemctl action.
	if m.bootstrap.actions[0].PostActionBanner != "" {
		t.Error("systemctl action should NOT have a PostActionBanner")
	}

	// Execute and complete (no banner).
	m, _, actionIDs := bootstrapToComplete(t, m, 1)
	if len(actionIDs) != 1 || actionIDs[0] != ActionIDSystemdStart {
		t.Errorf("expected systemd_start action, got: %v", actionIDs)
	}

	if m.state != StatePreflight {
		t.Fatalf("after complete → state = %v, want StatePreflight", m.state)
	}
}

// ---------------------------------------------------------------------------
// Test 4: Non-systemd stuck path
// ---------------------------------------------------------------------------

// TestFullFlowNonSystemdStuckNonFixable verifies that when Docker binary is present,
// user is in docker group, but systemd is NOT available, the check is non-fixable
// and the model stays on StatePreflight showing the error.
func TestFullFlowNonSystemdStuckNonFixable(t *testing.T) {
	dockerClient := newCountingDockerClient(errors.New("docker daemon is not running"))
	env := BootstrapEnv{
		UserName:            "alice",
		DockerBinaryPresent: true,
		UserInDockerGroup:   true,
		SystemdPresent:      false, // no systemd → non-fixable
	}
	fe := &FakeExecutor{}

	m := NewModel(buildDockerBootstrapDeps(dockerClient, env, fe))

	// Splash → preflight.
	m, cmd := sendMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
	m, cmd = sendMsg(m, drainCmd(cmd).(PreflightStartedMsg))

	preflightResult := drainCmd(cmd)
	m, _ = sendMsg(m, preflightResult)

	// Should stay on StatePreflight (non-fixable: can't fix Docker daemon without systemd).
	if m.state != StatePreflight {
		t.Fatalf("non-systemd stuck → state = %v, want StatePreflight (non-fixable)", m.state)
	}

	// Preflight view should show the blocking error.
	view := m.preflight.View()
	if !strings.Contains(view, "✗") && !strings.Contains(view, "Blocking") && !strings.Contains(view, "FAIL") {
		t.Errorf("preflight view should show blocking failure indicator, got:\n%s", view)
	}
}

// ---------------------------------------------------------------------------
// Test 5: Mixed path (docker install + dirs)
// ---------------------------------------------------------------------------

// TestFullFlowMixedDockerAndDirsActionsOrderedCorrectly verifies that when both
// Docker is missing AND a directory fails, actions execute in priority order:
// docker_install first, then dir-creation.
func TestFullFlowMixedDockerAndDirsActionsOrderedCorrectly(t *testing.T) {
	// Docker fails on first probe; dirs fail on first check.
	dockerClient := newCountingDockerClient(errors.New("docker: command not found"))
	dirChecker := newCountingDirChecker("/tmp/config")

	env := BootstrapEnv{
		UserName:            "alice",
		DockerBinaryPresent: false,
		UserInDockerGroup:   false,
		SystemdPresent:      false,
	}
	fe := &FakeExecutor{
		Results: []BootstrapActionResultMsg{
			{ActionID: ActionIDDockerInstall, Err: nil},
			{ActionID: string(preflight.CheckConfigWritable), Err: nil},
		},
	}

	coord := preflight.Coordinator{
		OS:                &platform.FakeOSGuard{Linux: true, Name: "linux"},
		Arch:              &platform.FakeArchDetector{Arch: platform.ArchAMD64},
		Docker:            dockerClient,
		Compose:           &compose.FakeComposeRunner{VersionVal: compose.Version{V2Plugin: true, Raw: "2.21.0"}},
		GPU:               &platform.FakeGPUDetector{Info: platform.GPUInfo{ToolkitInstalled: true}},
		Ports:             &ports.FakePortScanner{},
		Dirs:              dirChecker,
		MediaDir:          "/tmp/media",
		ConfigDir:         "/tmp/config",
		MinDockerVersion:  "20.10.0",
		MinComposeVersion: "2.0.0",
	}
	runner := &compose.FakeComposeRunner{
		VersionVal:       compose.Version{V2Plugin: true, Raw: "2.21.0"},
		PullProgressMsgs: []compose.PullProgressMsg{{Service: "backend", Status: "Pulled"}},
		UpProgressMsgs:   []compose.UpProgressMsg{{Service: "backend", Status: "Started"}},
		Healths:          []compose.ServiceHealth{{Service: "backend", Status: "healthy"}},
	}

	deps := Dependencies{
		Theme:                theme.Default(),
		OS:                   &platform.FakeOSGuard{Linux: true, Name: "linux"},
		Arch:                 &platform.FakeArchDetector{Arch: platform.ArchAMD64},
		GPU:                  &platform.FakeGPUDetector{Info: platform.GPUInfo{ToolkitInstalled: true}},
		Ports:                &ports.FakePortScanner{},
		Docker:               dockerClient,
		Compose:              runner,
		Envgen:               &envgen.Templater{PasswordGen: &secrets.FakeGenerator{Val: "mixed-password"}},
		Writer:               &envgen.FakeWriter{Written: make(map[string][]byte)},
		Assets:               TemplateAssets{EnvExample: []byte("WORKSPACE=\nPOSTGRES_PASSWORD=\n")},
		PreflightCoordinator: coord,
		Executor:             fe,
		Env:                  env,
		MediaDir:             "/tmp/media",
		ConfigDir:            "/tmp/config",
	}

	m := NewModel(deps)

	// Splash → preflight.
	m, cmd := sendMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
	m, cmd = sendMsg(m, drainCmd(cmd).(PreflightStartedMsg))

	preflightResult := drainCmd(cmd)
	m, _ = sendMsg(m, preflightResult)

	if m.state != StateBootstrap {
		t.Fatalf("mixed docker+dir fail → state = %v, want StateBootstrap", m.state)
	}

	// Verify action ordering: docker_install must be first.
	if len(m.bootstrap.actions) < 2 {
		t.Fatalf("bootstrap actions count = %d, want >= 2", len(m.bootstrap.actions))
	}
	if m.bootstrap.actions[0].ID != ActionIDDockerInstall {
		t.Errorf("first action should be docker_install, got %q", m.bootstrap.actions[0].ID)
	}
	if m.bootstrap.actions[1].ID != string(preflight.CheckConfigWritable) {
		t.Errorf("second action should be config_writable, got %q", m.bootstrap.actions[1].ID)
	}

	// Execute both actions.
	m, _, actionIDs := bootstrapToComplete(t, m, 2)

	if len(actionIDs) != 2 {
		t.Fatalf("expected 2 action IDs executed, got %d: %v", len(actionIDs), actionIDs)
	}
	if actionIDs[0] != ActionIDDockerInstall {
		t.Errorf("executed action[0] = %q, want docker_install", actionIDs[0])
	}
	if actionIDs[1] != string(preflight.CheckConfigWritable) {
		t.Errorf("executed action[1] = %q, want config_writable", actionIDs[1])
	}

	if m.state != StatePreflight {
		t.Fatalf("after mixed bootstrap complete → state = %v, want StatePreflight", m.state)
	}
}
