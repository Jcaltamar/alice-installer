package tui

import (
	"context"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/docker"
	"github.com/jcaltamar/alice-installer/internal/envgen"
	"github.com/jcaltamar/alice-installer/internal/platform"
	"github.com/jcaltamar/alice-installer/internal/ports"
	"github.com/jcaltamar/alice-installer/internal/preflight"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

// State represents the active screen in the TUI state machine.
type State int

const (
	StateSplash        State = iota
	StatePreflight     State = iota
	StateWorkspaceInput State = iota
	StatePortScan      State = iota
	StateEnvWrite      State = iota
	StatePull          State = iota
	StateDeploy        State = iota
	StateVerify        State = iota
	StateResult        State = iota
)

// TemplateAssets bundles the embedded installer assets.
type TemplateAssets struct {
	BaselineYAML []byte
	OverlayYAML  []byte
	EnvExample   []byte
}

// Dependencies holds all injectable dependencies for the root Model.
// Every field is an interface so tests can inject fakes without globals.
type Dependencies struct {
	Theme   theme.Theme
	OS      platform.OSGuard
	Arch    platform.ArchDetector
	GPU     platform.GPUDetector
	Ports   ports.PortScanner
	Docker  docker.DockerClient
	Compose compose.ComposeRunner
	Envgen  *envgen.Templater
	Writer  envgen.FileWriter
	Assets  TemplateAssets

	PreflightCoordinator preflight.Coordinator

	// Runtime config
	MediaDir         string
	ConfigDir        string
	RequiredTCPPorts map[string]int // env-key → default port
	RequiredUDPPorts map[string]int // env-key → default UDP port
}

// Model is the root Bubbletea model. It owns the state machine and delegates
// rendering and event handling to the active sub-model.
type Model struct {
	deps          Dependencies
	state         State
	width, height int

	// Sub-models (only the active one matters at any given time).
	splash    SplashModel
	preflight PreflightModel
	workspace WorkspaceInputModel
	portscan  PortScanModel
	envwrite  EnvWriteModel
	pull      PullModel
	deploy    DeployModel
	verify    VerifyModel
	result    ResultModel

	// Accumulated state carried across sub-models.
	workspaceName string
	confirmedPorts map[string]int
	envPath        string   // absolute path to the written .env
	composeFiles   []string // computed once at env-write → pull transition
	gpuDetected    bool
}

// portsConfigFromMap converts the flat env-key → port map from PortScan into
// the typed PortsConfig struct expected by envgen.Input.
// Unknown keys are silently ignored; unset keys default to 0.
func portsConfigFromMap(ports map[string]int) envgen.PortsConfig {
	return envgen.PortsConfig{
		PostgresPort:     ports["POSTGRES_PORT"],
		BackendPort:      ports["BACKEND_PORT"],
		WebsocketPort:    ports["WEBSOCKET_PORT"],
		WebPort:          ports["WEB_PORT"],
		RTSPPort:         ports["RTSP_PORT"],
		RedisPort:        ports["REDIS_PORT"],
		QueuePort:        ports["QUEUE_PORT"],
		HLSPort:          ports["HLS_PORT"],
		HLSPort2:         ports["HLS_PORT2"],
		HLSPort3:         ports["HLS_PORT3"],
		RTMPPort:         ports["RTMP_PORT"],
		MilvusPort:       ports["MILVUS_PORT"],
		MinioAPIPort:     ports["MINIO_API_PORT"],
		MinioConsolePort: ports["MINIO_CONSOLE_PORT"],
	}
}

// NewModel constructs the root Model with all sub-models pre-initialised.
func NewModel(deps Dependencies) Model {
	// Detect GPU once at construction time so it's available for overlay selection.
	gpuInfo := deps.GPU.Detect(context.Background())
	gpuDetected := gpuInfo.ToolkitInstalled

	return Model{
		deps:        deps,
		state:       StateSplash,
		gpuDetected: gpuDetected,
		splash:      NewSplashModel(deps.Theme),
		preflight:   NewPreflightModel(deps.Theme, deps.PreflightCoordinator),
		workspace:   NewWorkspaceInputModel(deps.Theme),
		portscan: NewPortScanModel(
			deps.Theme,
			deps.Ports,
			deps.RequiredTCPPorts,
			deps.RequiredUDPPorts,
		),
		// envwrite, pull, deploy, verify, result are initialised lazily at each
		// state transition so they receive the correct runtime data.
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return m.splash.Init()
}

// Update implements tea.Model.
// It handles global messages first (window size, quit keys), then dispatches
// to the active sub-model and processes state-transition messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// -----------------------------------------------------------------------
	// Global handlers
	// -----------------------------------------------------------------------
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case tea.KeyMsg:
		switch {
		case msg.Type == tea.KeyCtrlC:
			return m, tea.Quit

		case msg.Type == tea.KeyRunes && string(msg.Runes) == "q":
			// "q" quits from any state EXCEPT the workspace text input.
			if m.state != StateWorkspaceInput {
				return m, tea.Quit
			}
		}
	}

	// -----------------------------------------------------------------------
	// State-transition messages (handled before delegating to sub-models so
	// the root can intercept and switch state).
	// -----------------------------------------------------------------------
	switch msg := msg.(type) {
	case PreflightStartedMsg:
		m.state = StatePreflight
		return m, m.preflight.Init()

	case PreflightResultMsg:
		// Forward to preflight sub-model; state stays StatePreflight.
		updated, cmd := m.preflight.Update(msg)
		m.preflight = updated
		return m, cmd

	case PreflightPassedMsg:
		m.state = StateWorkspaceInput
		return m, m.workspace.Init()

	case WorkspaceEnteredMsg:
		m.workspaceName = msg.Value
		m.state = StatePortScan
		return m, m.portscan.Init()

	case PortsConfirmedMsg:
		m.confirmedPorts = msg.FinalPorts
		m.state = StateEnvWrite
		// Build the envgen.Input from accumulated state.
		envInput := envgen.Input{
			Workspace:        m.workspaceName,
			Arch:             m.deps.Arch.Detect(),
			Ports:            portsConfigFromMap(m.confirmedPorts),
			GeneratePassword: true,
		}
		envTarget := filepath.Join(m.deps.MediaDir, ".env")
		m.envwrite = NewEnvWriteModel(
			m.deps.Theme,
			m.deps.Envgen,
			m.deps.Writer,
			m.deps.Assets,
			envTarget,
			envInput,
		)
		return m, m.envwrite.Init()

	case EnvWrittenMsg:
		m.envPath = msg.Path
		// Compute compose files for pull/deploy/verify.
		m.composeFiles = compose.ComposeFiles(
			m.gpuDetected,
			filepath.Join(m.deps.MediaDir, "docker-compose.yml"),
			filepath.Join(m.deps.MediaDir, "docker-compose.gpu.yml"),
		)
		m.state = StatePull
		m.pull = NewPullModel(m.deps.Theme, m.deps.Compose, m.composeFiles, m.envPath)
		return m, m.pull.Init()

	case DeployStartedMsg:
		m.state = StateDeploy
		m.deploy = NewDeployModel(m.deps.Theme, m.deps.Compose, m.composeFiles, m.envPath)
		return m, m.deploy.Init()

	case HealthTickMsg:
		// Transition to verify only if we're still in deploy state (first tick).
		if m.state == StateDeploy {
			m.state = StateVerify
			m.verify = NewVerifyModel(m.deps.Theme, m.deps.Compose, m.composeFiles, m.envPath)
			return m, m.verify.Init()
		}

	case InstallSuccessMsg:
		m.state = StateResult
		m.result = NewResultModel(m.deps.Theme, &msg, nil)
		return m, nil

	case InstallFailureMsg:
		m.state = StateResult
		m.result = NewResultModel(m.deps.Theme, nil, &msg)
		return m, nil
	}

	// -----------------------------------------------------------------------
	// Delegate to the active sub-model.
	// -----------------------------------------------------------------------
	var cmd tea.Cmd
	switch m.state {
	case StateSplash:
		var updated SplashModel
		updated, cmd = m.splash.Update(msg)
		m.splash = updated

	case StatePreflight:
		var updated PreflightModel
		updated, cmd = m.preflight.Update(msg)
		m.preflight = updated

	case StateWorkspaceInput:
		var updated WorkspaceInputModel
		updated, cmd = m.workspace.Update(msg)
		m.workspace = updated

	case StatePortScan:
		var updated PortScanModel
		updated, cmd = m.portscan.Update(msg)
		m.portscan = updated

	case StateEnvWrite:
		var updated EnvWriteModel
		updated, cmd = m.envwrite.Update(msg)
		m.envwrite = updated

	case StatePull:
		var updated PullModel
		updated, cmd = m.pull.Update(msg)
		m.pull = updated

	case StateDeploy:
		var updated DeployModel
		updated, cmd = m.deploy.Update(msg)
		m.deploy = updated

	case StateVerify:
		var updated VerifyModel
		updated, cmd = m.verify.Update(msg)
		m.verify = updated

	case StateResult:
		var updated ResultModel
		updated, cmd = m.result.Update(msg)
		m.result = updated
	}

	return m, cmd
}

// View implements tea.Model.
func (m Model) View() string {
	// Terminal-too-small guard (REQ-TUI-6 — handled here for all states).
	if m.width > 0 && m.height > 0 {
		if m.width < 80 || m.height < 24 {
			return "Terminal too small. Resize to at least 80×24.\n"
		}
	}

	switch m.state {
	case StateSplash:
		return m.splash.View()
	case StatePreflight:
		return m.preflight.View()
	case StateWorkspaceInput:
		return m.workspace.View()
	case StatePortScan:
		return m.portscan.View()
	case StateEnvWrite:
		return m.envwrite.View()
	case StatePull:
		return m.pull.View()
	case StateDeploy:
		return m.deploy.View()
	case StateVerify:
		return m.verify.View()
	case StateResult:
		return m.result.View()
	default:
		return m.deps.Theme.TextMuted.Render("Loading…")
	}
}
