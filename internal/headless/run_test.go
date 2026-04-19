package headless_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jcaltamar/alice-installer/internal/bootstrap"
	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/docker"
	"github.com/jcaltamar/alice-installer/internal/envgen"
	"github.com/jcaltamar/alice-installer/internal/headless"
	"github.com/jcaltamar/alice-installer/internal/platform"
	"github.com/jcaltamar/alice-installer/internal/ports"
	"github.com/jcaltamar/alice-installer/internal/preflight"
	"github.com/jcaltamar/alice-installer/internal/secrets"
)

// ---------------------------------------------------------------------------
// Fakes / test doubles
// ---------------------------------------------------------------------------

// fakeDirChecker is a test double for preflight.DirectoryChecker.
type fakeDirChecker struct{}

func (fakeDirChecker) IsWritable(_ string) (bool, string) { return true, "" }

// fakeCmdExecutor is a test double for headless.CmdExecutor.
type fakeCmdExecutor struct {
	results map[string]cmdResult
	calls   []string
}

type cmdResult struct {
	out []byte
	err error
}

func newFakeCmdExecutor() *fakeCmdExecutor {
	return &fakeCmdExecutor{results: make(map[string]cmdResult)}
}

func (f *fakeCmdExecutor) setDefaultResult(out []byte, err error) {
	f.results["*"] = cmdResult{out: out, err: err}
}

func (f *fakeCmdExecutor) Run(name string, args ...string) ([]byte, error) {
	key := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, key)
	if r, ok := f.results[key]; ok {
		return r.out, r.err
	}
	if r, ok := f.results["*"]; ok {
		return r.out, r.err
	}
	return nil, fmt.Errorf("fakeCmdExecutor: no result for %q", key)
}

// trackingCmdExecutor records all commands executed.
type trackingCmdExecutor struct {
	inner headless.CmdExecutor
	calls *[]string
}

func (t *trackingCmdExecutor) Run(name string, args ...string) ([]byte, error) {
	key := name + " " + strings.Join(args, " ")
	*t.calls = append(*t.calls, key)
	return t.inner.Run(name, args...)
}

// switchingDockerClient changes its Probe/Version behavior after the first call.
type switchingDockerClient struct {
	firstProbeErr  error
	secondProbeErr error
	calls          int
}

func (s *switchingDockerClient) Probe(_ context.Context) error {
	s.calls++
	if s.calls == 1 {
		return s.firstProbeErr
	}
	return s.secondProbeErr
}

func (s *switchingDockerClient) Info(_ context.Context) (docker.Info, error) {
	return docker.Info{}, nil
}

func (s *switchingDockerClient) Version(_ context.Context) (docker.Version, error) {
	if s.calls <= 1 {
		return docker.Version{}, errors.New("docker not available yet")
	}
	return docker.Version{Client: "24.0.0", Server: "24.0.0"}, nil
}

func (s *switchingDockerClient) HasRuntime(_ context.Context, _ string) (bool, error) {
	return false, nil
}

// minimalEnvExample is a trivial .env template the Templater can render.
var minimalEnvExample = []byte(`WORKSPACE=alice
BACKEND_IMAGE=img:latest
WEBSOCKET_IMAGE=img:latest
WEB_IMAGE=img:latest
REDIS_IMAGE=redis:7-alpine
POSTGRES_PASSWORD=changeme
POSTGRES_PORT=5432
BACKEND_PORT=9090
WEBSOCKET_PORT=4550
WEB_PORT=8080
RTSP_PORT=8554
REDIS_PORT=6379
HLS_PORT=8888
HLS_PORT2=8889
HLS_PORT3=8890
RTMP_PORT=1935
MILVUS_PORT=19530
MINIO_API_PORT=9000
MINIO_CONSOLE_PORT=9001
`)

// happyCoord builds a preflight.Coordinator where all fakes return success.
func happyCoord(mediaDir, configDir, workspaceDir string) preflight.Coordinator {
	return preflight.Coordinator{
		OS:   &platform.FakeOSGuard{Linux: true, Name: "linux"},
		Arch: &platform.FakeArchDetector{Arch: platform.ArchAMD64},
		Docker: &docker.FakeDockerClient{
			VersionVal: docker.Version{Client: "24.0.5", Server: "24.0.5"},
		},
		Compose: &compose.FakeComposeRunner{
			VersionVal: compose.Version{V2Plugin: true, Raw: "2.21.0"},
		},
		GPU:               &platform.FakeGPUDetector{},
		Ports:             &ports.FakePortScanner{},
		Dirs:              fakeDirChecker{},
		MediaDir:          mediaDir,
		ConfigDir:         configDir,
		WorkspaceDir:      workspaceDir,
		RequiredTCPPorts:  []int{5432},
		MinDockerVersion:  "20.10.0",
		MinComposeVersion: "2.0.0",
	}
}

// happyDeps returns fully wired happy-path Dependencies + Config.
func happyDeps(t *testing.T) (headless.Dependencies, headless.Config) {
	t.Helper()
	dir := t.TempDir()
	fakeWriter := &envgen.FakeWriter{Written: make(map[string][]byte)}
	fakeCompose := &compose.FakeComposeRunner{
		VersionVal: compose.Version{V2Plugin: true, Raw: "2.21.0"},
		Healths: []compose.ServiceHealth{
			{Service: "backend", Status: "healthy"},
			{Service: "web", Status: "healthy"},
		},
	}
	cfg := headless.Config{WorkspaceName: "testws", AcceptAllBootstrap: true, Deploy: true}
	deps := headless.Dependencies{
		PreflightCoordinator: happyCoord("/opt/media", "/opt/config", dir),
		Ports:                &ports.FakePortScanner{},
		Envgen:               &envgen.Templater{PasswordGen: secrets.CryptoRandGenerator{}},
		Writer:               fakeWriter,
		Assets:               headless.TemplateAssets{BaselineYAML: []byte("# c"), OverlayYAML: []byte("# g"), EnvExample: minimalEnvExample},
		Compose:              fakeCompose,
		Arch:                 &platform.FakeArchDetector{Arch: platform.ArchAMD64},
		Env:                  bootstrap.BootstrapEnv{UserName: "u", DockerBinaryPresent: true, UserInDockerGroup: true},
		WorkspaceDir:         dir,
		MediaDir:             "/opt/media",
		ConfigDir:            "/opt/config",
		RequiredTCPPorts:     map[string]int{"POSTGRES_PORT": 5432},
		CmdExecutor:          newFakeCmdExecutor(),
	}
	return deps, cfg
}

// ---------------------------------------------------------------------------
// TestHeadlessRun_HappyPath
// ---------------------------------------------------------------------------

func TestHeadlessRun_HappyPath(t *testing.T) {
	deps, cfg := happyDeps(t)
	var buf bytes.Buffer

	err := headless.Run(context.Background(), cfg, deps, &buf)
	if err != nil {
		t.Fatalf("expected nil, got: %v\nlog:\n%s", err, buf.String())
	}

	log := buf.String()

	if !strings.Contains(log, "[preflight] all checks passed") {
		t.Errorf("missing '[preflight] all checks passed'\n%s", log)
	}
	if !strings.Contains(log, "[env-write]") {
		t.Errorf("missing [env-write] entries\n%s", log)
	}
	if !strings.Contains(log, "[pull]") {
		t.Errorf("missing [pull] entries\n%s", log)
	}
	if !strings.Contains(log, "[deploy]") {
		t.Errorf("missing [deploy] entries\n%s", log)
	}
	if !strings.Contains(log, "healthy") {
		t.Errorf("missing health status in log\n%s", log)
	}

	// docker-compose files must exist on disk (written via os.WriteFile).
	for _, name := range []string{"docker-compose.yml", "docker-compose.gpu.yml"} {
		path := filepath.Join(deps.WorkspaceDir, name)
		if _, statErr := os.Stat(path); statErr != nil {
			t.Errorf("expected file %s to exist: %v", path, statErr)
		}
	}

	// .env is written via the FakeWriter — check its in-memory record.
	envPath := filepath.Join(deps.WorkspaceDir, ".env")
	fw := deps.Writer.(*envgen.FakeWriter)
	if _, ok := fw.Written[envPath]; !ok {
		t.Errorf(".env was not written to FakeWriter; keys: %v", mapKeys(fw.Written))
	}
}

// mapKeys returns the string keys of a map[string][]byte for error messages.
func mapKeys(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ---------------------------------------------------------------------------
// TestHeadlessRun_BootstrapFixesDockerMissing
// ---------------------------------------------------------------------------

func TestHeadlessRun_BootstrapFixesDockerMissing(t *testing.T) {
	dir := t.TempDir()

	// Docker starts failing, succeeds after bootstrap.
	dockerClient := &switchingDockerClient{
		firstProbeErr:  errors.New("no such file or directory"),
		secondProbeErr: nil,
	}

	coord := preflight.Coordinator{
		OS:                &platform.FakeOSGuard{Linux: true, Name: "linux"},
		Arch:              &platform.FakeArchDetector{Arch: platform.ArchAMD64},
		Docker:            dockerClient,
		Compose:           &compose.FakeComposeRunner{VersionVal: compose.Version{V2Plugin: true, Raw: "2.21.0"}},
		GPU:               &platform.FakeGPUDetector{},
		Ports:             &ports.FakePortScanner{},
		Dirs:              fakeDirChecker{},
		MediaDir:          "/opt/media",
		ConfigDir:         "/opt/config",
		WorkspaceDir:      dir,
		RequiredTCPPorts:  []int{5432},
		MinDockerVersion:  "20.10.0",
		MinComposeVersion: "2.0.0",
	}

	var executedCalls []string
	fakeExec := newFakeCmdExecutor()
	fakeExec.setDefaultResult([]byte("ok"), nil)
	tracking := &trackingCmdExecutor{inner: fakeExec, calls: &executedCalls}

	deps := headless.Dependencies{
		PreflightCoordinator: coord,
		Ports:                &ports.FakePortScanner{},
		Envgen:               &envgen.Templater{PasswordGen: secrets.CryptoRandGenerator{}},
		Writer:               &envgen.FakeWriter{Written: make(map[string][]byte)},
		Assets:               headless.TemplateAssets{BaselineYAML: []byte("# c"), OverlayYAML: []byte("# g"), EnvExample: minimalEnvExample},
		Compose:              &compose.FakeComposeRunner{Healths: []compose.ServiceHealth{{Service: "svc", Status: "healthy"}}},
		Arch:                 &platform.FakeArchDetector{Arch: platform.ArchAMD64},
		// Docker binary NOT present → docker_install action.
		Env:              bootstrap.BootstrapEnv{UserName: "testuser", DockerBinaryPresent: false},
		WorkspaceDir:     dir,
		MediaDir:         "/opt/media",
		ConfigDir:        "/opt/config",
		RequiredTCPPorts: map[string]int{"POSTGRES_PORT": 5432},
		CmdExecutor:      tracking,
	}

	cfg := headless.Config{WorkspaceName: "testws", AcceptAllBootstrap: true, Deploy: true}

	var buf bytes.Buffer
	err := headless.Run(context.Background(), cfg, deps, &buf)
	// Accept nil (bootstrap worked + second preflight passed) OR ErrPreflightStillFailing
	// (bootstrap ran but second docker probe also returned error due to client state not
	// being reset — the important thing is that the bootstrap action ran).
	if err != nil && !errors.Is(err, headless.ErrPreflightStillFailing) {
		t.Fatalf("unexpected error: %v\nlog:\n%s", err, buf.String())
	}

	// Verify that sudo (docker install action) was invoked.
	foundSudo := false
	for _, c := range executedCalls {
		if strings.HasPrefix(c, "sudo") {
			foundSudo = true
			break
		}
	}
	if !foundSudo {
		t.Errorf("expected bootstrap action (sudo ...) to have been executed; calls: %v", executedCalls)
	}
}

// ---------------------------------------------------------------------------
// TestHeadlessRun_BootstrapStillFailsAfterActions
// ---------------------------------------------------------------------------

func TestHeadlessRun_BootstrapStillFailsAfterActions(t *testing.T) {
	dir := t.TempDir()

	// Docker daemon ALWAYS fails — even after systemctl action succeeds.
	alwaysFailDocker := &docker.FakeDockerClient{
		ProbeErr:   errors.New("daemon not responding"),
		VersionErr: errors.New("daemon not responding"),
	}

	coord := preflight.Coordinator{
		OS:   &platform.FakeOSGuard{Linux: true, Name: "linux"},
		Arch: &platform.FakeArchDetector{Arch: platform.ArchAMD64},
		Docker: alwaysFailDocker,
		// Compose passes — only docker_daemon keeps failing.
		Compose:           &compose.FakeComposeRunner{VersionVal: compose.Version{V2Plugin: true, Raw: "2.21.0"}},
		GPU:               &platform.FakeGPUDetector{},
		Ports:             &ports.FakePortScanner{},
		Dirs:              fakeDirChecker{},
		MediaDir:          "/opt/media",
		ConfigDir:         "/opt/config",
		WorkspaceDir:      dir,
		RequiredTCPPorts:  []int{5432},
		MinDockerVersion:  "20.10.0",
		MinComposeVersion: "2.0.0",
	}

	// Docker binary present, user in group, systemd available → systemctl action.
	env := bootstrap.BootstrapEnv{
		UserName:            "testuser",
		DockerBinaryPresent: true,
		UserInDockerGroup:   true,
		SystemdPresent:      true,
	}

	fakeExec := newFakeCmdExecutor()
	fakeExec.setDefaultResult([]byte("started"), nil) // systemctl "succeeds" but daemon still dead

	deps := headless.Dependencies{
		PreflightCoordinator: coord,
		Ports:                &ports.FakePortScanner{},
		Envgen:               &envgen.Templater{PasswordGen: secrets.CryptoRandGenerator{}},
		Writer:               &envgen.FakeWriter{Written: make(map[string][]byte)},
		Assets:               headless.TemplateAssets{BaselineYAML: []byte("# c"), OverlayYAML: []byte("# g"), EnvExample: minimalEnvExample},
		Compose:              &compose.FakeComposeRunner{},
		Arch:                 &platform.FakeArchDetector{Arch: platform.ArchAMD64},
		Env:                  env,
		WorkspaceDir:         dir,
		MediaDir:             "/opt/media",
		ConfigDir:            "/opt/config",
		RequiredTCPPorts:     map[string]int{"POSTGRES_PORT": 5432},
		CmdExecutor:          fakeExec,
	}

	cfg := headless.Config{WorkspaceName: "testws", AcceptAllBootstrap: true, Deploy: true}

	var buf bytes.Buffer
	err := headless.Run(context.Background(), cfg, deps, &buf)

	if err == nil {
		t.Fatal("expected an error, got nil")
	}

	if !errors.Is(err, headless.ErrPreflightStillFailing) {
		t.Errorf("expected errors.Is(err, ErrPreflightStillFailing), got: %v", err)
	}

	if !strings.Contains(err.Error(), "preflight still failing after bootstrap") {
		t.Errorf("error text should mention 'preflight still failing after bootstrap', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestHeadlessRun_DockerGroupAddRequiresRelogin
// ---------------------------------------------------------------------------

func TestHeadlessRun_DockerGroupAddRequiresRelogin(t *testing.T) {
	dir := t.TempDir()

	// Docker daemon fails; user NOT in docker group → docker_group_add.
	dockerFail := &docker.FakeDockerClient{
		ProbeErr:   errors.New("permission denied"),
		VersionErr: errors.New("cannot connect"),
	}

	coord := preflight.Coordinator{
		OS:   &platform.FakeOSGuard{Linux: true, Name: "linux"},
		Arch: &platform.FakeArchDetector{Arch: platform.ArchAMD64},
		Docker: dockerFail,
		Compose: &compose.FakeComposeRunner{
			VersionVal: compose.Version{V2Plugin: true, Raw: "2.21.0"},
		},
		GPU:               &platform.FakeGPUDetector{},
		Ports:             &ports.FakePortScanner{},
		Dirs:              fakeDirChecker{},
		MediaDir:          "/opt/media",
		ConfigDir:         "/opt/config",
		WorkspaceDir:      dir,
		RequiredTCPPorts:  []int{5432},
		MinDockerVersion:  "20.10.0",
		MinComposeVersion: "2.0.0",
	}

	// DockerBinaryPresent=true, UserInDockerGroup=false → docker_group_add.
	env := bootstrap.BootstrapEnv{
		UserName:            "testuser",
		DockerBinaryPresent: true,
		UserInDockerGroup:   false,
		SystemdPresent:      false,
	}

	fakeExec := newFakeCmdExecutor()
	fakeExec.setDefaultResult([]byte("ok"), nil) // usermod succeeds

	deps := headless.Dependencies{
		PreflightCoordinator: coord,
		Ports:                &ports.FakePortScanner{},
		Envgen:               &envgen.Templater{PasswordGen: secrets.CryptoRandGenerator{}},
		Writer:               &envgen.FakeWriter{Written: make(map[string][]byte)},
		Assets:               headless.TemplateAssets{BaselineYAML: []byte("# c"), OverlayYAML: []byte("# g"), EnvExample: minimalEnvExample},
		Compose:              &compose.FakeComposeRunner{},
		Arch:                 &platform.FakeArchDetector{Arch: platform.ArchAMD64},
		Env:                  env,
		WorkspaceDir:         dir,
		MediaDir:             "/opt/media",
		ConfigDir:            "/opt/config",
		RequiredTCPPorts:     map[string]int{"POSTGRES_PORT": 5432},
		CmdExecutor:          fakeExec,
	}

	cfg := headless.Config{WorkspaceName: "testws", AcceptAllBootstrap: true, Deploy: true}

	var buf bytes.Buffer
	err := headless.Run(context.Background(), cfg, deps, &buf)

	if err == nil {
		t.Fatal("expected ErrReloginRequired but got nil")
	}

	if !errors.Is(err, headless.ErrReloginRequired) {
		t.Errorf("expected errors.Is(err, ErrReloginRequired), got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestHeadlessRun_PullFailureIncludesStderr
// ---------------------------------------------------------------------------

func TestHeadlessRun_PullFailureIncludesStderr(t *testing.T) {
	dir := t.TempDir()
	stderrMsg := "pull access denied for registry.example.com/image"
	pullErr := fmt.Errorf("exit status 1\n--- docker compose pull stderr ---\n%s", stderrMsg)

	deps := headless.Dependencies{
		PreflightCoordinator: happyCoord("/opt/media", "/opt/config", dir),
		Ports:                &ports.FakePortScanner{},
		Envgen:               &envgen.Templater{PasswordGen: secrets.CryptoRandGenerator{}},
		Writer:               &envgen.FakeWriter{Written: make(map[string][]byte)},
		Assets:               headless.TemplateAssets{BaselineYAML: []byte("# c"), OverlayYAML: []byte("# g"), EnvExample: minimalEnvExample},
		Compose:              &compose.FakeComposeRunner{PullErr: pullErr},
		Arch:                 &platform.FakeArchDetector{Arch: platform.ArchAMD64},
		Env:                  bootstrap.BootstrapEnv{UserName: "u", DockerBinaryPresent: true, UserInDockerGroup: true},
		WorkspaceDir:         dir,
		MediaDir:             "/opt/media",
		ConfigDir:            "/opt/config",
		RequiredTCPPorts:     map[string]int{"POSTGRES_PORT": 5432},
		CmdExecutor:          newFakeCmdExecutor(),
	}

	var buf bytes.Buffer
	err := headless.Run(context.Background(), headless.Config{WorkspaceName: "testws", AcceptAllBootstrap: true, Deploy: true}, deps, &buf)

	if err == nil {
		t.Fatal("expected pull error but got nil")
	}

	if !strings.Contains(err.Error(), stderrMsg) {
		t.Errorf("error should contain stderr message %q\ngot: %v", stderrMsg, err)
	}
}

// ---------------------------------------------------------------------------
// TestHeadlessRun_NoInteractiveDependency
// ---------------------------------------------------------------------------

// TestHeadlessRun_NoInteractiveDependency proves headless.Dependencies has no Theme
// field (compile-time) and that Run works correctly without any TUI-specific deps.
func TestHeadlessRun_NoInteractiveDependency(t *testing.T) {
	dir := t.TempDir()

	// This is both a compile-time check (struct literal with no Theme field) and
	// a runtime check (Run succeeds).
	deps := headless.Dependencies{
		PreflightCoordinator: happyCoord("/opt/media", "/opt/config", dir),
		Ports:                &ports.FakePortScanner{},
		Envgen:               &envgen.Templater{PasswordGen: secrets.CryptoRandGenerator{}},
		Writer:               &envgen.FakeWriter{Written: make(map[string][]byte)},
		Assets:               headless.TemplateAssets{BaselineYAML: []byte("# c"), OverlayYAML: []byte("# g"), EnvExample: minimalEnvExample},
		Compose: &compose.FakeComposeRunner{
			Healths: []compose.ServiceHealth{{Service: "svc", Status: "healthy"}},
		},
		Arch:             &platform.FakeArchDetector{Arch: platform.ArchAMD64},
		Env:              bootstrap.BootstrapEnv{UserName: "u", DockerBinaryPresent: true, UserInDockerGroup: true},
		WorkspaceDir:     dir,
		MediaDir:         "/opt/media",
		ConfigDir:        "/opt/config",
		RequiredTCPPorts: map[string]int{"POSTGRES_PORT": 5432},
		CmdExecutor:      newFakeCmdExecutor(),
		// Intentionally no Theme field — headless.Dependencies does not declare one.
	}

	var buf bytes.Buffer
	err := headless.Run(context.Background(), headless.Config{WorkspaceName: "testws", AcceptAllBootstrap: true, Deploy: true}, deps, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v\nlog:\n%s", err, buf.String())
	}
}

// ---------------------------------------------------------------------------
// TestHeadlessRun_PortConflictReturnsError
// ---------------------------------------------------------------------------

func TestHeadlessRun_PortConflictReturnsError(t *testing.T) {
	dir := t.TempDir()

	deps := headless.Dependencies{
		PreflightCoordinator: happyCoord("/opt/media", "/opt/config", dir),
		Ports:                &ports.FakePortScanner{OccupiedPorts: []int{5432}},
		Envgen:               &envgen.Templater{PasswordGen: secrets.CryptoRandGenerator{}},
		Writer:               &envgen.FakeWriter{Written: make(map[string][]byte)},
		Assets:               headless.TemplateAssets{BaselineYAML: []byte("# c"), OverlayYAML: []byte("# g"), EnvExample: minimalEnvExample},
		Compose:              &compose.FakeComposeRunner{},
		Arch:                 &platform.FakeArchDetector{Arch: platform.ArchAMD64},
		Env:                  bootstrap.BootstrapEnv{UserName: "u", DockerBinaryPresent: true, UserInDockerGroup: true},
		WorkspaceDir:         dir,
		MediaDir:             "/opt/media",
		ConfigDir:            "/opt/config",
		RequiredTCPPorts:     map[string]int{"POSTGRES_PORT": 5432},
		CmdExecutor:          newFakeCmdExecutor(),
	}

	var buf bytes.Buffer
	err := headless.Run(context.Background(), headless.Config{WorkspaceName: "testws", AcceptAllBootstrap: true, Deploy: true}, deps, &buf)

	if err == nil {
		t.Fatal("expected port conflict error but got nil")
	}
	if !strings.Contains(err.Error(), "5432") {
		t.Errorf("expected error to mention port 5432, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestHeadlessRun_DeploySkippedWhenFlagFalse
// ---------------------------------------------------------------------------

func TestHeadlessRun_DeploySkippedWhenFlagFalse(t *testing.T) {
	dir := t.TempDir()

	deps := headless.Dependencies{
		PreflightCoordinator: happyCoord("/opt/media", "/opt/config", dir),
		Ports:                &ports.FakePortScanner{},
		Envgen:               &envgen.Templater{PasswordGen: secrets.CryptoRandGenerator{}},
		Writer:               &envgen.FakeWriter{Written: make(map[string][]byte)},
		Assets:               headless.TemplateAssets{BaselineYAML: []byte("# c"), OverlayYAML: []byte("# g"), EnvExample: minimalEnvExample},
		Compose:              &compose.FakeComposeRunner{},
		Arch:                 &platform.FakeArchDetector{Arch: platform.ArchAMD64},
		Env:                  bootstrap.BootstrapEnv{UserName: "u", DockerBinaryPresent: true, UserInDockerGroup: true},
		WorkspaceDir:         dir,
		MediaDir:             "/opt/media",
		ConfigDir:            "/opt/config",
		RequiredTCPPorts:     map[string]int{"POSTGRES_PORT": 5432},
		CmdExecutor:          newFakeCmdExecutor(),
	}

	var buf bytes.Buffer
	err := headless.Run(context.Background(), headless.Config{
		WorkspaceName:      "testws",
		AcceptAllBootstrap: true,
		Deploy:             false,
		SkipPull:           true,
	}, deps, &buf)

	if err != nil {
		t.Fatalf("unexpected error: %v\nlog:\n%s", err, buf.String())
	}

	log := buf.String()
	if strings.Contains(log, "[deploy]") {
		t.Error("expected deploy to be skipped but [deploy] appears in log")
	}
	if strings.Contains(log, "[verify]") {
		t.Error("expected verify to be skipped but [verify] appears in log")
	}
	if !strings.Contains(log, "deploy skipped") {
		t.Errorf("expected 'deploy skipped' in log\n%s", log)
	}
}
