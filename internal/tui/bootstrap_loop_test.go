package tui

// bootstrap_loop_test.go: adversarial test that verifies the attemptedActions guard
// prevents an infinite bootstrap loop when preflight keeps returning the same failure
// even after the remediation action has already been attempted.
//
// Scenario:
//   - FakePreflight coordinator always returns docker_daemon FAIL.
//   - FakeExecutor reports success for every action.
//   - After bootstrap completes + preflight reruns, the second PreflightResultMsg
//     must NOT route to StateBootstrap again (the guard must block it).
//   - The model should stay on StatePreflight.

import (
	"context"
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

// alwaysFailingDockerClient is a DockerClient that always fails Probe — simulating
// a daemon that didn't come up even after systemctl returned 0.
type alwaysFailingDockerClient struct {
	inner *docker.FakeDockerClient
}

func newAlwaysFailingDockerClient() *alwaysFailingDockerClient {
	return &alwaysFailingDockerClient{
		inner: &docker.FakeDockerClient{VersionVal: docker.Version{Client: "25.0.0", Server: "25.0.0"}},
	}
}

func (c *alwaysFailingDockerClient) Probe(_ context.Context) error {
	return context.DeadlineExceeded // always fails
}

func (c *alwaysFailingDockerClient) Info(ctx context.Context) (docker.Info, error) {
	return c.inner.Info(ctx)
}

func (c *alwaysFailingDockerClient) Version(ctx context.Context) (docker.Version, error) {
	return c.inner.Version(ctx)
}

func (c *alwaysFailingDockerClient) HasRuntime(ctx context.Context, name string) (bool, error) {
	return c.inner.HasRuntime(ctx, name)
}

// buildAdversarialLoopDeps builds Dependencies for the adversarial bootstrap loop test.
// Docker always fails Probe; executor always reports action success.
func buildAdversarialLoopDeps(exec Executor) Dependencies {
	dockerClient := newAlwaysFailingDockerClient()

	// Env: binary present, user in docker group, systemd present.
	// → CheckDockerDaemon FAIL classified as fixable via systemd_start_docker.
	env := BootstrapEnv{
		UserName:            "alice",
		DockerBinaryPresent: true,
		UserInDockerGroup:   true,
		SystemdPresent:      true,
	}

	coord := preflight.Coordinator{
		OS:                &platform.FakeOSGuard{Linux: true, Name: "linux"},
		Arch:              &platform.FakeArchDetector{Arch: platform.ArchAMD64},
		Docker:            dockerClient,
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
		Theme:   theme.Default(),
		OS:      &platform.FakeOSGuard{Linux: true, Name: "linux"},
		Arch:    &platform.FakeArchDetector{Arch: platform.ArchAMD64},
		GPU:     &platform.FakeGPUDetector{},
		Ports:   &ports.FakePortScanner{},
		Docker:  dockerClient,
		Compose: &compose.FakeComposeRunner{},
		Envgen:  &envgen.Templater{PasswordGen: &secrets.FakeGenerator{Val: "loop-password"}},
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

// TestBootstrapLoopGuardPreventsInfiniteRetry is the adversarial test:
//
//  1. Splash → Enter → PreflightStartedMsg → StatePreflight
//  2. Preflight runs → docker_daemon FAIL → fixable (systemd_start) → StateBootstrap
//  3. User presses 'y' → executor reports success → BootstrapCompleteMsg
//  4. Root records attemptedActions, rearmes preflight
//  5. Second preflight run: docker_daemon STILL fails
//  6. ClassifyBlockers returns systemd_start again — but attemptedActions contains it
//  7. Model should stay on StatePreflight (not StateBootstrap) — guard blocks loop
func TestBootstrapLoopGuardPreventsInfiniteRetry(t *testing.T) {
	fe := &FakeExecutor{
		Results: []BootstrapActionResultMsg{
			// systemctl returned 0 (success) — but daemon never came up.
			{ActionID: ActionIDSystemdStart, Err: nil},
		},
	}

	m := NewModel(buildAdversarialLoopDeps(fe))

	// --- Step 1: Splash → Enter → PreflightStartedMsg ---
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

	// --- Step 3: First preflight runs → docker_daemon FAIL → StateBootstrap ---
	firstPreflightResult := drainCmd(cmd)
	m, _ = sendMsg(m, firstPreflightResult)
	if m.state != StateBootstrap {
		t.Fatalf("docker_daemon FAIL with systemd env → state = %v, want StateBootstrap", m.state)
	}

	// Verify the action is systemd_start_docker.
	if len(m.bootstrap.actions) != 1 {
		t.Fatalf("bootstrap actions count = %d, want 1", len(m.bootstrap.actions))
	}
	if m.bootstrap.actions[0].ID != ActionIDSystemdStart {
		t.Errorf("bootstrap action[0].ID = %q, want %q", m.bootstrap.actions[0].ID, ActionIDSystemdStart)
	}

	// --- Step 4: User presses 'y' → executor runs → BootstrapActionResultMsg ---
	m, cmd = sendMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	actionResultMsg := drainCmd(cmd)
	if _, ok := actionResultMsg.(BootstrapActionResultMsg); !ok {
		t.Fatalf("executor cmd should produce BootstrapActionResultMsg, got %T", actionResultMsg)
	}

	// --- Step 5: Apply BootstrapActionResultMsg → BootstrapCompleteMsg ---
	m, cmd = sendMsg(m, actionResultMsg)
	completeMsg := drainCmd(cmd)
	if _, ok := completeMsg.(BootstrapCompleteMsg); !ok {
		t.Fatalf("all actions done → BootstrapCompleteMsg, got %T", completeMsg)
	}

	// --- Step 6: Apply BootstrapCompleteMsg → StatePreflight (rearmed) ---
	m, rearmCmd := sendMsg(m, completeMsg)
	if m.state != StatePreflight {
		t.Fatalf("BootstrapCompleteMsg → state = %v, want StatePreflight", m.state)
	}

	// Verify attemptedActions now contains the systemd action.
	if !m.attemptedActions[ActionIDSystemdStart] {
		t.Error("attemptedActions should contain ActionIDSystemdStart after bootstrap complete")
	}

	// --- Step 7: Second preflight runs → docker_daemon STILL fails ---
	if rearmCmd == nil {
		t.Fatal("BootstrapCompleteMsg should return a re-arm cmd")
	}
	secondPreflightResult := drainCmd(rearmCmd)
	m, _ = sendMsg(m, secondPreflightResult)

	// CRITICAL: The model must stay on StatePreflight, NOT go to StateBootstrap.
	// The guard (attemptedActions) must block the re-queue of systemd_start_docker.
	if m.state != StatePreflight {
		t.Errorf("after second preflight with already-attempted action: state = %v, want StatePreflight (guard must block loop)", m.state)
	}

	// Verify the preflight report shows the blocking failure (user can see the error).
	if m.preflight.report == nil {
		t.Fatal("preflight report should be populated after second run")
	}
	if !m.preflight.report.HasBlockingFailure() {
		t.Error("second preflight report should still have blocking failure")
	}
}
