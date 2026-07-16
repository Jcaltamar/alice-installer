package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jcaltamar/alice-installer/internal/asterisk"
	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/docker"
	"github.com/jcaltamar/alice-installer/internal/envgen"
	"github.com/jcaltamar/alice-installer/internal/installation"
	"github.com/jcaltamar/alice-installer/internal/migration"
	"github.com/jcaltamar/alice-installer/internal/platform"
	"github.com/jcaltamar/alice-installer/internal/ports"
	"github.com/jcaltamar/alice-installer/internal/preflight"
	"github.com/jcaltamar/alice-installer/internal/runlog"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

// State represents the active screen in the TUI state machine.
type State int

const (
	StateSplash                 State = iota
	StatePreflight              State = iota
	StateBootstrap              State = iota // auto-elevation: sits between preflight and workspace-input
	StateWorkspaceInput         State = iota
	StateOptionalPackages       State = iota
	StatePortScan               State = iota
	StateEnvWrite               State = iota
	StateAsteriskSetup          State = iota
	StatePull                   State = iota
	StateDeploy                 State = iota
	StateVerify                 State = iota
	StateResult                 State = iota
	StateDetecting              State = iota
	StateContextMenu            State = iota
	StateUpdating               State = iota
	StateBlockedOperation       State = iota
	StateActionResult           State = iota
	StateMigrationEnv           State = iota
	StateMigrationAuth          State = iota
	StateMigrationAuthFailed    State = iota
	StateBackupPreflight        State = iota
	StateBackupConfirm          State = iota
	StateBackupRunning          State = iota
	StateBackupResult           State = iota
	StateMigrationBlocked       State = iota
	StateDatabaseRestore        State = iota
	StateMigrationResult        State = iota
	StateMigrationQuiescence    State = iota
	StateMigrationRecovery      State = iota
	StateMigrationSuccess       State = iota
	StateMigrationConfirm       State = iota
	StateMigrationCancelled     State = iota
	StateMigrationDisposition   State = iota
	StateMigrationRemoveConfirm State = iota
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
	Theme                  theme.Theme
	Version                string
	Debug                  bool
	RunLog                 runlog.Logger
	LogPath                string
	OS                     platform.OSGuard
	Arch                   platform.ArchDetector
	GPU                    platform.GPUDetector
	Ports                  ports.PortScanner
	Docker                 docker.DockerClient
	Compose                compose.ComposeRunner
	Envgen                 *envgen.Templater
	Writer                 envgen.FileWriter
	Assets                 TemplateAssets
	AsteriskInstaller      AsteriskInstaller
	AsteriskOptions        asterisk.Options
	AsteriskAvailable      func() bool
	Detector               installation.Detector
	UpdateAction           UpdateAction
	LegacyBackupAction     LegacyBackupAction
	LegacyBackupRequest    migration.BackupRequest
	LegacyRestoreAction    migration.LegacyRestoreAction
	MigrationHandoff       MigrationHandoff
	MigrationAuthenticator MigrationAuthenticator

	PreflightCoordinator preflight.Coordinator

	// Executor is used by the bootstrap state to run elevated commands.
	// In production: NewExecutor(). In tests: *FakeExecutor.
	Executor Executor

	// Env holds the detected host environment used by ClassifyBlockers.
	// In production: populated via DetectEnv(). In tests: inject as needed.
	Env BootstrapEnv

	// Runtime config
	MediaDir         string
	ConfigDir        string
	WorkspaceDir     string         // default: ~/.config/alice-guardian — user-editable artifacts
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
	splash                SplashModel
	preflight             PreflightModel
	bootstrap             BootstrapModel
	workspace             WorkspaceInputModel
	optionalPackagesModel OptionalPackagesModel
	portscan              PortScanModel
	envwrite              EnvWriteModel
	asteriskSetup         AsteriskSetupModel
	pull                  PullModel
	deploy                DeployModel
	verify                VerifyModel
	result                ResultModel
	contextMenu           ContextMenuModel
	blockedOperation      blockedOperationModel
	actionResult          actionResultModel
	migrationEnvChoice    migrationEnvironmentModel
	migrationDisposition  migrationDispositionModel
	containerDisposition  migration.ContainerDisposition
	backupPlan            migration.BackupPlan
	backupResult          migration.BackupResult
	backupCancel          context.CancelFunc
	backupCancelling      bool
	backupSpinner         spinner.Model
	backupProgress        <-chan tea.Msg
	backupStage           migration.BackupProgressStage
	backupElapsed         time.Duration
	restoreCancel         context.CancelFunc
	quiescenceCancel      context.CancelFunc
	restoreResult         migration.RestoreResult
	migrationLease        *migration.PreInstallMigrationLease
	migrationRecoveryCode string
	migrationDiagnostic   *installation.PM2ObservationDiagnostic
	originalFailure       *InstallFailureMsg

	// Accumulated state carried across sub-models.
	workspaceName    string
	optionalPackages map[OptionalPackage]bool
	asteriskOptions  asterisk.Options
	confirmedPorts   map[string]int
	envPath          string   // absolute path to the written .env
	composeFiles     []string // computed once at env-write → pull transition
	gpuDetected      bool
	migrationPending bool
	migrationEnv     migration.EnvironmentName

	// attemptedActions tracks bootstrap Action IDs that have already been
	// executed successfully in this session. Prevents bootstrap from looping
	// when a preflight rerun still fails (e.g. systemctl enable returned 0
	// but the daemon didn't actually come up, so docker_daemon still fails).
	attemptedActions map[string]bool
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
		HLSPort:          ports["HLS_PORT"],
		HLSPort2:         ports["HLS_PORT2"],
		HLSPort3:         ports["HLS_PORT3"],
		WebRTCICEPort:    ports["WEBRTC_ICE_PORT"],
		RTMPPort:         ports["RTMP_PORT"],
		MilvusPort:       ports["MILVUS_PORT"],
		MilvusWebPort:    ports["MILVUS_WEB_PORT"],
		MinioAPIPort:     ports["MINIO_API_PORT"],
		MinioConsolePort: ports["MINIO_CONSOLE_PORT"],
	}
}

// NewModel constructs the root Model with all sub-models pre-initialised.
func NewModel(deps Dependencies) Model {
	// Detect GPU once at construction time so it's available for overlay selection.
	gpuInfo := deps.GPU.Detect(context.Background())
	gpuDetected := gpuInfo.ToolkitInstalled

	backupSpinner := spinner.New()
	backupSpinner.Spinner = spinner.Dot
	backupSpinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(string(theme.ColorPrimary)))
	return Model{
		deps:                  deps,
		state:                 StateSplash,
		gpuDetected:           gpuDetected,
		attemptedActions:      map[string]bool{},
		splash:                NewSplashModel(deps.Theme, deps.Version),
		preflight:             NewPreflightModel(deps.Theme, deps.PreflightCoordinator),
		backupSpinner:         backupSpinner,
		workspace:             NewWorkspaceInputModel(deps.Theme),
		optionalPackagesModel: NewOptionalPackagesModel(deps.Theme, asteriskAvailable(deps)),
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

func (m *Model) log(event runlog.Event) {
	if m.deps.RunLog != nil {
		m.deps.RunLog.Log(event)
	}
}

func boolPtr(value bool) *bool { return &value }

func (m *Model) applyMigrationPortPolicy() {
	var exemptTCP, exemptUDP map[int]struct{}
	if m.hasLiveMigrationLease() {
		exemptTCP, exemptUDP = ports.MigrationPortExemptions()
	}
	m.deps.PreflightCoordinator.ExemptTCPPorts = exemptTCP
	m.deps.PreflightCoordinator.ExemptUDPPorts = exemptUDP
	m.preflight.coord = m.deps.PreflightCoordinator
	m.portscan.setExemptPorts(exemptTCP, exemptUDP)
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return m.splash.Init()
}

// Update implements tea.Model.
// It handles global messages first (window size, quit keys), then dispatches
// to the active sub-model and processes state-transition messages.
func (m Model) Update(msg tea.Msg) (model tea.Model, command tea.Cmd) {
	previousState := m.state
	defer func() {
		next, ok := model.(Model)
		if !ok || next.state == previousState {
			return
		}
		next.log(runlog.Event{Event: "tui-transition", Stage: stateName(next.state), Status: "entered"})
		model = next
	}()
	// -----------------------------------------------------------------------
	// Global handlers
	// -----------------------------------------------------------------------
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case tea.KeyMsg:
		switch {
		case msg.Type == tea.KeyCtrlC:
			if m.state == StateBackupRunning && m.backupCancel != nil {
				m.backupCancelling = true
				m.backupCancel()
				return m, nil
			}
			if m.state == StateMigrationQuiescence && m.quiescenceCancel != nil {
				m.quiescenceCancel()
				return m, nil
			}
			if m.state == StateDatabaseRestore && m.restoreCancel != nil {
				m.restoreCancel()
				return m, nil
			}
			if m.hasLiveMigrationLease() {
				return m.beginMigrationRecovery()
			}
			m.log(runlog.Event{Event: "terminal", Stage: stateName(m.state), Status: "quit", Code: "user-quit"})
			return m, tea.Quit

		case msg.Type == tea.KeyEsc && m.state == StateBackupRunning:
			if m.backupCancel != nil {
				m.backupCancelling = true
				m.backupCancel()
			}
			return m, nil
		case msg.Type == tea.KeyEsc && m.state == StateMigrationConfirm:
			m.state = StateMigrationCancelled
			m.log(runlog.Event{Event: "terminal", Stage: "migration", Status: "cancelled", Code: "migration-cancelled"})
			return m, nil
		case msg.Type == tea.KeyEsc && (m.state == StateMigrationDisposition || m.state == StateMigrationRemoveConfirm):
			m.state = StateMigrationCancelled
			m.log(runlog.Event{Event: "terminal", Stage: "migration", Status: "cancelled", Code: "migration-cancelled"})
			return m, nil
		case msg.Type == tea.KeyEsc && m.state == StateMigrationQuiescence && m.quiescenceCancel != nil:
			m.quiescenceCancel()
			return m, nil
		case msg.Type == tea.KeyEsc && m.state == StateDatabaseRestore:
			if m.restoreCancel != nil {
				m.restoreCancel()
			}
			return m, nil
		case msg.Type == tea.KeyEsc && m.hasLiveMigrationLease():
			return m.beginMigrationRecovery()
		case msg.Type == tea.KeyEsc && (m.state == StateDetecting || m.state == StateContextMenu):
			m.log(runlog.Event{Event: "terminal", Stage: stateName(m.state), Status: "quit", Code: "user-quit"})
			return m, tea.Quit

		case msg.Type == tea.KeyRunes && string(msg.Runes) == "q":
			if m.state == StateMigrationConfirm {
				m.state = StateMigrationCancelled
				m.log(runlog.Event{Event: "terminal", Stage: "migration", Status: "cancelled", Code: "migration-cancelled"})
				return m, nil
			}
			if m.state == StateBackupRunning && m.backupCancel != nil {
				m.backupCancelling = true
				m.backupCancel()
				return m, nil
			}
			if m.state == StateMigrationQuiescence && m.quiescenceCancel != nil {
				m.quiescenceCancel()
				return m, nil
			}
			if m.state == StateDatabaseRestore && m.restoreCancel != nil {
				m.restoreCancel()
				return m, nil
			}
			if m.hasLiveMigrationLease() {
				return m.beginMigrationRecovery()
			}
			// "q" quits from any state EXCEPT the workspace text input.
			if m.state != StateWorkspaceInput {
				m.log(runlog.Event{Event: "terminal", Stage: stateName(m.state), Status: "quit", Code: "user-quit"})
				return m, tea.Quit
			}
		}
	}

	// -----------------------------------------------------------------------
	// State-transition messages (handled before delegating to sub-models so
	// the root can intercept and switch state).
	// -----------------------------------------------------------------------
	switch msg := msg.(type) {
	case DetectionStartedMsg:
		m.state = StateDetecting
		if m.deps.Detector == nil {
			return m, func() tea.Msg {
				return DetectionCompletedMsg{Detection: installation.Detection{State: installation.StateUnknown}}
			}
		}
		return m, func() tea.Msg { return DetectionCompletedMsg{Detection: m.deps.Detector.Detect(context.Background())} }

	case DetectionCompletedMsg:
		m.state = StateContextMenu
		m.contextMenu = NewContextMenuModel(m.deps.Theme, msg.Detection)
		m.contextMenu.migrationAvailable = hasMigrationCapability(m.deps)
		return m, nil

	case ContextActionSelectedMsg:
		if m.state != StateContextMenu || !m.contextMenu.hasAction(msg.Action) {
			return m, nil
		}
		switch msg.Action {
		case ContextActionInstall:
			m.state = StatePreflight
			return m, m.preflight.Init()
		case ContextActionUpdate:
			m.state = StateUpdating
			if m.deps.UpdateAction == nil {
				return m, func() tea.Msg { return UpdateCompletedMsg{Err: fmt.Errorf("update action is unavailable")} }
			}
			return m, func() tea.Msg { return UpdateCompletedMsg{Err: m.deps.UpdateAction.Run(context.Background())} }
		case ContextActionUninstall:
			m.state = StateBlockedOperation
			m.blockedOperation = blockedOperationModel{theme: m.deps.Theme, action: msg.Action}
			return m, nil
		case ContextActionMigration:
			if !hasMigrationCapability(m.deps) {
				m.state = StateBlockedOperation
				m.blockedOperation = blockedOperationModel{theme: m.deps.Theme, action: msg.Action}
				return m, nil
			}
			m.state = StateMigrationEnv
			m.migrationEnvChoice = migrationEnvironmentModel{theme: m.deps.Theme}
			return m, nil
		}

	case MigrationEnvironmentSelectedMsg:
		if m.state != StateMigrationEnv || msg.Environment != migration.EnvironmentDevelopment && msg.Environment != migration.EnvironmentProduction {
			return m, nil
		}
		m.migrationEnv = msg.Environment
		m.log(runlog.Event{Event: "migration-environment", Stage: "environment", Status: "selected", Fields: runlog.Fields{Mode: string(msg.Environment)}})
		m.state = StateMigrationAuth
		return m, m.deps.MigrationAuthenticator.Authenticate()

	case MigrationAuthenticationCompletedMsg:
		if m.state != StateMigrationAuth {
			return m, nil
		}
		if msg.Err != nil {
			m.log(runlog.Event{Event: "migration-auth", Stage: "authentication", Status: "failed", Code: "migration-auth-failed", Fields: runlog.Fields{ErrorClass: fmt.Sprintf("%T", msg.Err)}})
			m.state = StateMigrationAuthFailed
			return m, nil
		}
		m.state = StateBackupPreflight
		m.log(runlog.Event{Event: "migration-auth", Stage: "authentication", Status: "succeeded"})
		return m, func() tea.Msg {
			request := m.deps.LegacyBackupRequest
			request.ConfigEnvironment = string(m.migrationEnv)
			plan, err := m.deps.LegacyBackupAction.Preflight(context.Background(), request)
			return BackupPreflightCompletedMsg{Plan: plan, Err: err}
		}

	case BackupPreflightCompletedMsg:
		if m.state != StateBackupPreflight {
			return m, nil
		}
		if msg.Err != nil {
			m.log(runlog.Event{Event: "backup-preflight", Stage: "backup-preflight", Status: "failed", Code: "backup-preflight-failed", Fields: runlog.Fields{ErrorClass: fmt.Sprintf("%T", msg.Err)}})
			m.state = StateBackupResult
			m.backupResult = migration.BackupPreflightFailureResult(msg.Err)
			return m, nil
		}
		m.backupPlan = msg.Plan
		m.log(runlog.Event{Event: "backup-preflight", Stage: "backup-preflight", Status: "succeeded"})
		m.state = StateBackupConfirm
		return m, nil

	case BackupConfirmedMsg:
		if m.state != StateBackupConfirm || m.deps.LegacyBackupAction == nil {
			return m, nil
		}
		m.state = StateBackupRunning
		ctx, cancel := context.WithCancel(context.Background())
		m.backupCancel = cancel
		m.backupStage = migration.BackupProgressPreparing
		m.backupElapsed = 0
		m.backupCancelling = false
		progress := make(chan tea.Msg, 6)
		m.backupProgress = progress
		run := func() tea.Msg {
			defer close(progress)
			if action, ok := m.deps.LegacyBackupAction.(legacyBackupProgressAction); ok {
				result := action.RunWithProgress(ctx, m.backupPlan, func(stage migration.BackupProgressStage) { progress <- BackupProgressMsg{Stage: stage} })
				return BackupCompletedMsg{Result: result}
			}
			return BackupCompletedMsg{Result: m.deps.LegacyBackupAction.Run(ctx, m.backupPlan)}
		}
		return m, tea.Batch(run, m.backupSpinner.Tick, backupElapsedTick(), waitBackupProgress(progress))

	case BackupProgressMsg:
		if m.state != StateBackupRunning || m.backupCancelling {
			return m, nil
		}
		m.backupStage = msg.Stage
		return m, waitBackupProgress(m.backupProgress)

	case backupProgressClosedMsg:
		return m, nil

	case backupElapsedMsg:
		if m.state != StateBackupRunning || m.backupCancelling {
			return m, nil
		}
		m.backupElapsed += time.Second
		return m, backupElapsedTick()

	case spinner.TickMsg:
		if m.state == StateBackupRunning && !m.backupCancelling {
			var spinnerCmd tea.Cmd
			m.backupSpinner, spinnerCmd = m.backupSpinner.Update(msg)
			return m, spinnerCmd
		}

	case BackupCompletedMsg:
		if m.state != StateBackupRunning {
			return m, nil
		}
		m.backupCancel = nil
		m.backupCancelling = false
		m.backupProgress = nil
		m.backupResult = msg.Result
		m.log(runlog.Event{Event: "backup-result", Stage: "backup", Status: fmt.Sprint(msg.Result.Outcome), Code: msg.Result.FailureCode.String()})
		if msg.Result.Outcome == migration.BackupValidated {
			if m.deps.LegacyRestoreAction == nil || m.deps.MigrationHandoff == nil {
				m.state = StateMigrationBlocked
				return m, nil
			}
			m.state = StateMigrationConfirm
			return m, nil
		}
		m.state = StateBackupResult
		return m, nil

	case MigrationDispositionSelectedMsg:
		if m.state != StateMigrationDisposition || msg.Disposition > migration.DispositionRemove {
			return m, nil
		}
		m.containerDisposition = msg.Disposition
		disposition := "stop"
		if msg.Disposition == migration.DispositionRemove {
			disposition = "remove"
		}
		m.log(runlog.Event{Event: "container-disposition", Stage: "quiescence", Status: "selected", Fields: runlog.Fields{Disposition: disposition}})
		if msg.Disposition == migration.DispositionRemove {
			m.state = StateMigrationRemoveConfirm
			return m, nil
		}
		return m.beginMigrationQuiescence()

	case MigrationRemoveConfirmedMsg:
		if m.state != StateMigrationRemoveConfirm || m.containerDisposition != migration.DispositionRemove {
			return m, nil
		}
		return m.beginMigrationQuiescence()

	case MigrationQuiescenceCompletedMsg:
		if m.state != StateMigrationQuiescence {
			return m, nil
		}
		m.quiescenceCancel = nil
		if msg.Err != nil || msg.Lease == nil {
			m.state = StateMigrationResult
			m.migrationRecoveryCode = boundedQuiescenceCode(msg.Err)
			fields := runlog.Fields{ErrorClass: fmt.Sprintf("%T", msg.Err)}
			if diagnostic := boundedQuiescenceDiagnostic(msg.Err); diagnostic != nil {
				fields.Operation, fields.Cause = diagnostic.Operation, diagnostic.Cause
			}
			m.log(runlog.Event{Event: "pm2-quiescence", Stage: "quiescence", Status: "failed", Code: m.migrationRecoveryCode, Fields: fields})
			if m.deps.Debug {
				m.migrationDiagnostic = boundedQuiescenceDiagnostic(msg.Err)
			}
			return m, nil
		}
		m.migrationLease = msg.Lease
		targets := make([]runlog.Target, 0, len(msg.Lease.PM2Targets()))
		for _, target := range msg.Lease.PM2Targets() {
			targets = append(targets, runlog.Target{PMID: target.PMID, Name: target.Name, Port: target.Port})
		}
		m.log(runlog.Event{Event: "pm2-quiescence", Stage: "quiescence", Status: "verified", Code: "pm2-stop-verified", Fields: runlog.Fields{Targets: targets, Attempted: len(targets), Verified: boolPtr(true)}})
		dispositionCode := migration.DispositionStoppedCode
		if m.containerDisposition == migration.DispositionRemove {
			dispositionCode = migration.DispositionRemovedCode
		}
		m.log(runlog.Event{Event: "container-disposition", Stage: "quiescence", Status: "verified", Code: dispositionCode, Fields: runlog.Fields{Verified: boolPtr(true)}})
		m.migrationPending = true
		m.applyMigrationPortPolicy()
		m.state = StatePreflight
		return m, m.preflight.Init()

	case MigrationRecoveryCompletedMsg:
		if m.state != StateMigrationRecovery {
			return m, nil
		}
		m.migrationLease = nil
		m.migrationPending = false
		m.applyMigrationPortPolicy()
		m.migrationRecoveryCode = boundedRecoveryCode(msg.Recovery, msg.Err)
		recoveryFields := runlog.Fields{Attempted: msg.Recovery.Attempted, Recovered: msg.Recovery.Recovered, Verified: boolPtr(msg.Recovery.Verified), ErrorClass: fmt.Sprintf("%T", msg.Err)}
		if msg.Recovery.Diagnostic != nil {
			recoveryFields.Operation, recoveryFields.Cause = msg.Recovery.Diagnostic.Operation, msg.Recovery.Diagnostic.Cause
		}
		m.log(runlog.Event{Event: "pm2-recovery", Stage: "recovery", Status: "completed", Code: m.migrationRecoveryCode, Fields: recoveryFields})
		m.migrationDiagnostic = nil
		if m.deps.Debug && msg.Recovery.Diagnostic != nil {
			diagnostic := *msg.Recovery.Diagnostic
			m.migrationDiagnostic = &diagnostic
		}
		m.state = StateMigrationResult
		return m, nil

	case MigrationSuccessCompletedMsg:
		if m.state != StateMigrationSuccess {
			return m, nil
		}
		m.migrationLease = nil
		m.migrationPending = false
		m.applyMigrationPortPolicy()
		if msg.Err != nil {
			m.migrationRecoveryCode = "pm2-lease-completion-failed"
			m.state = StateMigrationResult
			return m, nil
		}
		m.state = StateResult
		m.result = NewResultModel(m.deps.Theme, &msg.Success, nil)
		return m, nil

	case MigrationAbandonedMsg:
		if m.state == StateMigrationQuiescence && m.quiescenceCancel != nil {
			m.quiescenceCancel()
			return m, nil
		}
		if m.state == StateDatabaseRestore && m.restoreCancel != nil {
			m.restoreCancel()
			return m, nil
		}
		if m.hasLiveMigrationLease() {
			return m.beginMigrationRecovery()
		}
		return m, nil

	case UpdateCompletedMsg:
		m.state = StateActionResult
		m.actionResult = actionResultModel{theme: m.deps.Theme, err: msg.Err}
		return m, nil

	case BlockedOperationDismissedMsg:
		m.state = StateContextMenu
		return m, nil

	case PreflightStartedMsg:
		m.state = StatePreflight
		return m, m.preflight.Init()

	case PreflightResultMsg:
		// Classify blockers: if all are fixable → bootstrap; any non-fixable → stay preflight.
		fixable, nonFixable := ClassifyBlockers(msg.Report, m.deps.Env, m.deps.MediaDir, m.deps.ConfigDir, m.deps.WorkspaceDir)
		// Filter out actions we've already attempted this session. An action
		// that was executed, returned success, but didn't actually fix the
		// root cause (common: systemctl enable returned 0 but the daemon
		// failed to start) must NOT be retried — that's an infinite loop.
		var freshFixable []Action
		for _, a := range fixable {
			if !m.attemptedActions[a.ID] {
				freshFixable = append(freshFixable, a)
			}
		}
		if len(nonFixable) > 0 || (len(fixable) > 0 && len(freshFixable) == 0) {
			// Either classifier says non-fixable, or every fixable action has
			// already been tried. Delegate to preflight so the user sees the
			// failing report with the remediation hints.
			updated, cmd := m.preflight.Update(msg)
			m.preflight = updated
			return m, cmd
		}
		if len(freshFixable) > 0 {
			updatedPf, _ := m.preflight.Update(msg)
			m.preflight = updatedPf
			exec := m.deps.Executor
			if exec == nil {
				exec = NewExecutor()
			}
			m.state = StateBootstrap
			m.bootstrap = NewBootstrapModel(m.deps.Theme, exec, freshFixable)
			return m, m.bootstrap.Init()
		}
		// No blockers at all → delegate to preflight (will emit PreflightPassedMsg on Enter).
		updated, cmd := m.preflight.Update(msg)
		m.preflight = updated
		return m, cmd

	case BootstrapCompleteMsg:
		// Record every action that was executed so the classifier won't re-queue
		// them on the next preflight rerun. Re-detect env so stale snapshot data
		// (DockerBinaryPresent=false after install, UserInDockerGroup=false after
		// usermod) doesn't mislead the classifier.
		for _, a := range m.bootstrap.actions {
			m.attemptedActions[a.ID] = true
		}
		m.deps.Env = DetectEnv()
		m.state = StatePreflight
		return m, m.preflight.Rearm()

	case BootstrapSkippedMsg:
		// User declined bootstrap — return to preflight with original report frozen.
		m.state = StatePreflight
		return m, nil

	case PreflightPassedMsg:
		m.state = StateWorkspaceInput
		return m, m.workspace.Init()

	case WorkspaceEnteredMsg:
		m.workspaceName = msg.Value
		m.state = StateOptionalPackages
		m.optionalPackagesModel = NewOptionalPackagesModel(m.deps.Theme, asteriskAvailable(m.deps))
		return m, m.optionalPackagesModel.Init()

	case OptionalPackagesConfirmedMsg:
		m.optionalPackages = copyOptionalPackageMap(msg.Selected)
		if !asteriskAvailable(m.deps) {
			delete(m.optionalPackages, OptionalPackageAsterisk)
		}
		if m.optionalPackages[OptionalPackageAsterisk] {
			opts := m.deps.AsteriskOptions
			opts.Enabled = true
			if opts.ConfigRoot == "" {
				opts.ConfigRoot = asterisk.DefaultConfigRoot
			}
			opts.AMI = asterisk.NormalizeContract(opts.AMI, opts.ConfigRoot)
			m.asteriskOptions = opts
		} else {
			m.asteriskOptions = asterisk.Options{Enabled: false}
		}
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
		if m.optionalPackages[OptionalPackageAsterisk] {
			envInput.Asterisk = m.asteriskOptions.AMI
		}
		workspaceDir := m.deps.WorkspaceDir
		if workspaceDir == "" {
			workspaceDir = m.deps.MediaDir // fallback for tests that don't set WorkspaceDir
		}
		envTarget := filepath.Join(workspaceDir, ".env")
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
		workspaceDir := m.deps.WorkspaceDir
		if workspaceDir == "" {
			workspaceDir = m.deps.MediaDir // fallback for tests that don't set WorkspaceDir
		}
		m.composeFiles = compose.ComposeFiles(
			m.gpuDetected,
			filepath.Join(workspaceDir, "docker-compose.yml"),
			filepath.Join(workspaceDir, "docker-compose.gpu.yml"),
		)
		if m.optionalPackages[OptionalPackageAsterisk] {
			m.state = StateAsteriskSetup
			m.asteriskSetup = NewAsteriskSetupModel(m.deps.Theme, m.deps.AsteriskInstaller, m.asteriskOptions)
			return m, m.asteriskSetup.Init()
		}
		m.state = StatePull
		m.pull = NewPullModel(m.deps.Theme, m.deps.Compose, m.composeFiles, m.envPath)
		return m, m.pull.Init()

	case AsteriskSetupCompleteMsg:
		updated, cmd := m.asteriskSetup.Update(msg)
		m.asteriskSetup = updated
		return m, cmd

	case PullStartedMsg:
		m.state = StatePull
		m.pull = NewPullModel(m.deps.Theme, m.deps.Compose, m.composeFiles, m.envPath)
		return m, m.pull.Init()

	case DeployStartedMsg:
		m.state = StateDeploy
		m.deploy = NewDeployModel(m.deps.Theme, m.deps.Compose, m.composeFiles, m.envPath)
		return m, m.deploy.Init()

	case DeployCompleteMsg:
		if m.state == StateDeploy && m.migrationPending {
			m.state = StateDatabaseRestore
			ctx, cancel := context.WithCancel(context.Background())
			m.restoreCancel = cancel
			request := migration.RestoreRequest{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, ComposeFiles: m.composeFiles, EnvFile: m.envPath, BackupDestination: "/opt/alice/backups/", Legacy: migration.BackupRef{DumpPath: m.backupResult.DumpPath, ManifestPath: m.backupResult.ManifestPath, SHA256: m.backupResult.SHA256, Size: m.backupResult.Size}}
			return m, func() tea.Msg { return RestoreCompletedMsg{Result: m.deps.LegacyRestoreAction.Run(ctx, request)} }
		}
	case RestoreCompletedMsg:
		if m.state != StateDatabaseRestore {
			return m, nil
		}
		m.restoreCancel = nil
		m.restoreResult = msg.Result
		m.log(runlog.Event{Event: "restore-result", Stage: restoreStageName(msg.Result.FailedStage), Status: restoreOutcomeName(msg.Result.Outcome), Code: msg.Result.Code, Fields: runlog.Fields{Mutated: boolPtr(msg.Result.Mutated), Rollback: rollbackName(msg.Result.Rollback), BackendHealthy: boolPtr(msg.Result.BackendHealthy), LegacyValidated: boolPtr(msg.Result.LegacyBackup.Validated), TargetValidated: boolPtr(msg.Result.TargetBackup.Validated), RestoreExitOK: boolPtr(msg.Result.Database.RestoreExitOK), ConnectionOK: boolPtr(msg.Result.Database.ConnectionOK), PostgreSQLReachable: boolPtr(msg.Result.Database.PostgreSQLReachable)}})
		if msg.Result.Outcome == migration.RestoreSucceeded {
			m.state = StateDeploy
			return m, func() tea.Msg { return HealthTickMsg{} }
		}
		return m.beginMigrationRecovery()
	case HealthTickMsg:
		// Transition to verify only if we're still in deploy state (first tick).
		if m.state == StateDeploy {
			m.state = StateVerify
			m.verify = NewVerifyModel(m.deps.Theme, m.deps.Compose, m.composeFiles, m.envPath)
			return m, m.verify.Init()
		}

	case InstallSuccessMsg:
		m.log(runlog.Event{Event: "terminal", Stage: "install", Status: "success", Code: "install-succeeded"})
		if m.hasLiveMigrationLease() {
			m.state = StateMigrationSuccess
			return m, m.completeMigrationSuccess(msg)
		}
		m.state = StateResult
		m.result = NewResultModel(m.deps.Theme, &msg, nil, m.deps.LogPath)
		return m, nil

	case InstallFailureMsg:
		failure := msg
		m.originalFailure = &failure
		code := "install-failed"
		if msg.Stage != "" {
			code = msg.Stage + "-failed"
		}
		m.log(runlog.Event{Event: "original-failure", Stage: msg.Stage, Status: "failed", Code: code, Fields: runlog.Fields{ErrorClass: fmt.Sprintf("%T", msg.Err)}})
		if m.hasLiveMigrationLease() {
			return m.beginMigrationRecovery()
		}
		m.state = StateResult
		m.result = NewResultModel(m.deps.Theme, nil, &msg, m.deps.LogPath)
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

	case StateContextMenu:
		var updated ContextMenuModel
		updated, cmd = m.contextMenu.Update(msg)
		m.contextMenu = updated

	case StateBlockedOperation:
		var updated blockedOperationModel
		updated, cmd = m.blockedOperation.Update(msg)
		m.blockedOperation = updated

	case StateMigrationEnv:
		var updated migrationEnvironmentModel
		updated, cmd = m.migrationEnvChoice.Update(msg)
		m.migrationEnvChoice = updated

	case StateMigrationDisposition:
		var updated migrationDispositionModel
		updated, cmd = m.migrationDisposition.Update(msg)
		m.migrationDisposition = updated

	case StatePreflight:
		var updated PreflightModel
		updated, cmd = m.preflight.Update(msg)
		m.preflight = updated

	case StateBootstrap:
		var updated BootstrapModel
		updated, cmd = m.bootstrap.Update(msg)
		m.bootstrap = updated

	case StateWorkspaceInput:
		var updated WorkspaceInputModel
		updated, cmd = m.workspace.Update(msg)
		m.workspace = updated

	case StateOptionalPackages:
		var updated OptionalPackagesModel
		updated, cmd = m.optionalPackagesModel.Update(msg)
		m.optionalPackagesModel = updated

	case StatePortScan:
		var updated PortScanModel
		updated, cmd = m.portscan.Update(msg)
		m.portscan = updated

	case StateEnvWrite:
		var updated EnvWriteModel
		updated, cmd = m.envwrite.Update(msg)
		m.envwrite = updated

	case StateAsteriskSetup:
		var updated AsteriskSetupModel
		updated, cmd = m.asteriskSetup.Update(msg)
		m.asteriskSetup = updated

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

	case StateBackupConfirm:
		if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEnter {
			return m.Update(BackupConfirmedMsg{})
		}

	case StateMigrationConfirm:
		if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEnter {
			m.state = StateMigrationDisposition
			m.migrationDisposition = migrationDispositionModel{theme: m.deps.Theme}
			return m, nil
		}
	case StateMigrationRemoveConfirm:
		if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEnter {
			return m.Update(MigrationRemoveConfirmedMsg{})
		}
	}

	return m, cmd
}

func stateName(state State) string {
	names := [...]string{"splash", "preflight", "bootstrap", "workspace-input", "optional-packages", "port-scan", "env-write", "asterisk-setup", "pull", "deploy", "verify", "result", "detecting", "context-menu", "updating", "blocked-operation", "action-result", "migration-environment", "migration-auth", "migration-auth-failed", "backup-preflight", "backup-confirm", "backup-running", "backup-result", "migration-blocked", "database-restore", "migration-result", "migration-quiescence", "migration-recovery", "migration-success", "migration-confirm", "migration-cancelled", "migration-disposition", "migration-remove-confirm"}
	if int(state) < 0 || int(state) >= len(names) {
		return "unknown"
	}
	return names[state]
}

func restoreStageName(stage migration.RestoreStage) string {
	names := [...]string{"platform-gate", "wait", "credentials", "legacy-revalidation", "backend-stop", "postgres-check", "target-backup", "target-replacement", "restore-validation", "backend-start", "backend-health", "rollback"}
	if int(stage) < 0 || int(stage) >= len(names) {
		return "unknown"
	}
	return names[stage]
}

func restoreOutcomeName(outcome migration.RestoreOutcome) string {
	names := [...]string{"succeeded", "failed-before-cutover", "cancelled-before-cutover", "partial-cutover", "unsupported"}
	if int(outcome) < 0 || int(outcome) >= len(names) {
		return "unknown"
	}
	return names[outcome]
}

func rollbackName(status migration.RollbackStatus) string {
	names := [...]string{"not-required", "succeeded", "failed", "cancelled"}
	if int(status) < 0 || int(status) >= len(names) {
		return "unknown"
	}
	return names[status]
}

func (m Model) backupConfirmationView() string {
	review := m.backupPlan.Review()
	return m.deps.Theme.Primary.Bold(true).Render("Review PostgreSQL backup") + fmt.Sprintf(
		"\n\nSelected environment: %s\nEndpoint: %s\nDatabase: %s\nUser: %s\nContainer ID: %s\nImage: %s\nDestination: %s\n\nNo backup artifact has been created. Confirm to create a validated backup in the protected destination.\n\nPress Enter to confirm or Escape to cancel.\n",
		review.Environment,
		review.Endpoint,
		review.Database,
		review.User,
		review.ContainerID,
		review.Image,
		review.Destination,
	)
}

func asteriskAvailable(deps Dependencies) bool {
	if deps.AsteriskAvailable != nil {
		return deps.AsteriskAvailable()
	}
	return deps.OS == nil || deps.OS.IsLinux()
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
	case StateDetecting:
		return m.deps.Theme.TextMuted.Render("Detecting existing installation…")
	case StateContextMenu:
		return m.contextMenu.View()
	case StateUpdating:
		return m.deps.Theme.TextMuted.Render("Updating existing installation…")
	case StateBlockedOperation:
		return m.blockedOperation.View()
	case StateActionResult:
		return m.actionResult.View()
	case StateMigrationEnv:
		return m.migrationEnvChoice.View()
	case StateMigrationAuth:
		return m.deps.Theme.TextMuted.Render("Requesting migration authorization in the terminal…")
	case StateMigrationAuthFailed:
		return m.deps.Theme.Primary.Bold(true).Render("Migration authorization failed") + "\n\nMigration remains blocked. No backup preflight or filesystem mutation was started.\n"
	case StateBackupPreflight:
		return m.deps.Theme.TextMuted.Render("Reviewing legacy backup prerequisites…")
	case StateBackupConfirm:
		return m.backupConfirmationView()
	case StateBackupRunning:
		return m.backupRunningView()
	case StateMigrationConfirm:
		return m.deps.Theme.Primary.Bold(true).Render("Backup validated") + fmt.Sprintf("\n\n%s\nDump: %s\nManifest: %s\nSHA-256: %s\nSize: %d bytes\n\nThe validated backup is complete. Continuing will choose the legacy PostgreSQL disposition before stopping confirmed legacy PM2 services and installing new services.\n\nPress Enter to continue or Escape to stop here and preserve the backup.\n", m.backupStagesView(), m.backupResult.DumpPath, m.backupResult.ManifestPath, m.backupResult.SHA256, m.backupResult.Size)
	case StateMigrationDisposition:
		return m.migrationDisposition.View()
	case StateMigrationRemoveConfirm:
		return m.migrationRemoveConfirmationView()
	case StateMigrationCancelled:
		return m.deps.Theme.Primary.Bold(true).Render("Migration stopped safely") + fmt.Sprintf("\n\nDump: %s\nManifest: %s\nSHA-256: %s\nSize: %d bytes\n\nPM2 services were not stopped and new services were not installed. The validated backup was preserved.\n", m.backupResult.DumpPath, m.backupResult.ManifestPath, m.backupResult.SHA256, m.backupResult.Size)
	case StateBackupResult:
		return m.backupResultView()
	case StateDatabaseRestore:
		return m.migrationRestoreView()
	case StateMigrationQuiescence:
		return m.migrationQuiescenceView()
	case StateMigrationRecovery:
		return m.migrationRecoveryView()
	case StateMigrationResult:
		if m.migrationRecoveryCode != "" {
			return m.migrationTerminalView()
		}
		return m.migrationResultView()
	case StateMigrationBlocked:
		return m.deps.Theme.Primary.Bold(true).Render("Backup validated") + fmt.Sprintf("\n\nDump: %s\nManifest: %s\nSHA-256: %s\nSize: %d bytes\n\nLater migration steps are not implemented and remain blocked. No restore, cutover, or source mutation was performed.\n", m.backupResult.DumpPath, m.backupResult.ManifestPath, m.backupResult.SHA256, m.backupResult.Size)
	case StatePreflight:
		return m.preflight.View()
	case StateBootstrap:
		return m.bootstrap.View()
	case StateWorkspaceInput:
		return m.workspace.View()
	case StateOptionalPackages:
		return m.optionalPackagesModel.View()
	case StatePortScan:
		return m.portscan.View()
	case StateEnvWrite:
		return m.envwrite.View()
	case StateAsteriskSetup:
		return m.asteriskSetup.View()
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

func (m Model) backupRunningView() string {
	if m.backupCancelling {
		return m.deps.Theme.Primary.Bold(true).Render("Cancelling backup safely") + "\n\n" + m.deps.Theme.TextMuted.Render("Waiting for the backup helper and staged files to be cleaned up.") + "\n"
	}
	stages := [...]string{
		"Preparing destination and credentials",
		"Creating database dump",
		"Syncing staged file",
		"Validating archive",
		"Publishing backup, checksum, and manifest",
	}
	stage := "Preparing destination and credentials"
	if int(m.backupStage) < len(stages) {
		stage = stages[m.backupStage]
	}
	return m.deps.Theme.Primary.Bold(true).Render("Creating validated backup") + "\n\n" + m.backupSpinner.View() + " " + m.deps.Theme.TextMuted.Render(fmt.Sprintf("%s (%s elapsed)", stage, m.backupElapsed)) + "\n\n" + m.deps.Theme.TextMuted.Render("Press Escape to cancel safely.") + "\n"
}

type BackupProgressMsg struct{ Stage migration.BackupProgressStage }
type backupElapsedMsg struct{}
type backupProgressClosedMsg struct{}

func backupElapsedTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return backupElapsedMsg{} })
}

func waitBackupProgress(progress <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-progress
		if !ok {
			return backupProgressClosedMsg{}
		}
		return msg
	}
}

func (m Model) backupResultView() string {
	var b strings.Builder
	b.WriteString(m.deps.Theme.Primary.Bold(true).Render("Backup did not validate"))
	b.WriteString("\n\nLater migration steps remain blocked.\n\n")
	b.WriteString(m.backupStagesView())
	b.WriteString(fmt.Sprintf("\nFailure code: %s\nRemediation: %s\n", m.backupResult.FailureCode.String(), m.backupResult.Remediation.String()))
	return b.String()
}

func (m Model) backupStagesView() string {
	var b strings.Builder
	b.WriteString("Stages:\n")
	for _, stage := range m.backupResult.Stages {
		b.WriteString(fmt.Sprintf("  %-43s %s\n", stage.Stage.String()+":", stage.Status.String()))
	}
	return b.String()
}

func hasMigrationCapability(deps Dependencies) bool {
	return deps.MigrationAuthenticator != nil && deps.LegacyBackupAction != nil && deps.LegacyRestoreAction != nil && deps.MigrationHandoff != nil
}
