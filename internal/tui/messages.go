// Package tui implements the Bubbletea terminal user interface for alice-installer.
package tui

import (
	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/preflight"
)

// ---------------------------------------------------------------------------
// Global messages
// ---------------------------------------------------------------------------

// ErrorMsg carries an error that may be fatal (abort program) or recoverable.
type ErrorMsg struct {
	Err   error
	Fatal bool
}

// AbortMsg signals that the user has confirmed they want to abort the install.
type AbortMsg struct{}

// QuitMsg signals a clean exit with no rollback needed.
type QuitMsg struct{}

// ---------------------------------------------------------------------------
// Preflight messages
// ---------------------------------------------------------------------------

// PreflightStartedMsg is emitted when the preflight state begins running checks.
type PreflightStartedMsg struct{}

// PreflightResultMsg carries the completed preflight report.
type PreflightResultMsg struct {
	Report preflight.Report
}

// PreflightPassedMsg is emitted when the user presses Enter on a passing preflight.
// The root model uses this to advance to the workspace-input state.
type PreflightPassedMsg struct{}

// ---------------------------------------------------------------------------
// Workspace messages
// ---------------------------------------------------------------------------

// WorkspaceEnteredMsg is emitted when the user submits a valid workspace name.
type WorkspaceEnteredMsg struct {
	Value string
}

// ---------------------------------------------------------------------------
// Port scan messages
// ---------------------------------------------------------------------------

// PortConflict describes a single port that is already in use.
type PortConflict struct {
	Key       string // env-var name, e.g. "POSTGRES_PORT"
	Requested int    // the port the installer wanted to use
	Reason    string // human-readable reason, e.g. "occupied"
}

// PortScanResultMsg is emitted when the port scan completes.
// Conflicts lists every port that is unavailable.
// FreePlan maps env-var names to currently-available ports (conflicts excluded).
type PortScanResultMsg struct {
	Conflicts []PortConflict
	FreePlan  map[string]int // "POSTGRES_PORT" → 5432
}

// PortResolvedMsg is emitted when the user picks an alternate port for one conflict.
type PortResolvedMsg struct {
	Key    string // env-var name
	Chosen int    // the new port the user selected
}

// PortsConfirmedMsg is emitted when all port conflicts are resolved.
// FinalPorts is the complete env-var → port mapping to write into .env.
type PortsConfirmedMsg struct {
	FinalPorts map[string]int
}

// ---------------------------------------------------------------------------
// Env-write messages
// ---------------------------------------------------------------------------

// EnvWrittenMsg is emitted after the .env file has been written successfully.
type EnvWrittenMsg struct {
	Path string // absolute path of the written file
}

// ---------------------------------------------------------------------------
// Compose pull messages
// ---------------------------------------------------------------------------

// PullStartedMsg is emitted when the compose pull phase begins.
type PullStartedMsg struct{}

// PullProgressMsg is an alias for compose.PullProgressMsg so callers import
// only the tui package for message dispatch.
type PullProgressMsg = compose.PullProgressMsg

// PullCompleteMsg is emitted when all images have been pulled successfully.
type PullCompleteMsg struct{}

// ---------------------------------------------------------------------------
// Compose deploy messages
// ---------------------------------------------------------------------------

// DeployStartedMsg is emitted when `docker compose up` is invoked.
type DeployStartedMsg struct{}

// DeployProgressMsg is an alias for compose.UpProgressMsg.
type DeployProgressMsg = compose.UpProgressMsg

// DeployCompleteMsg is emitted when all services are reported as started.
type DeployCompleteMsg struct{}

// ---------------------------------------------------------------------------
// Health-check messages
// ---------------------------------------------------------------------------

// HealthTickMsg is emitted on each polling interval during the health-check phase.
type HealthTickMsg struct{}

// HealthReportMsg carries the current health snapshot.
// Done is true when all services are healthy or the timeout has elapsed.
type HealthReportMsg struct {
	Services []compose.ServiceHealth
	Done     bool
}

// ---------------------------------------------------------------------------
// Final result messages
// ---------------------------------------------------------------------------

// InstallSuccessMsg is emitted when the full installation completes successfully.
type InstallSuccessMsg struct {
	WorkspaceDir string
	EnvPath      string
	Services     []compose.ServiceHealth
}

// InstallFailureMsg is emitted when installation fails at any stage.
type InstallFailureMsg struct {
	Err   error
	Stage string // e.g. "preflight", "pull", "deploy", "verify"
}

// ---------------------------------------------------------------------------
// Bootstrap messages
// ---------------------------------------------------------------------------

// Action is defined in bootstrap.go (imported from internal/bootstrap).

// BootstrapNeededMsg is emitted by the root model to signal that the bootstrap
// state should be entered with the given action list.
type BootstrapNeededMsg struct {
	Actions []Action
}

// BootstrapConfirmedMsg is emitted when the user presses Y to approve all actions.
type BootstrapConfirmedMsg struct{}

// BootstrapSkippedMsg is emitted when the user declines the bootstrap (N or Esc).
type BootstrapSkippedMsg struct{}

// BootstrapActionResultMsg is posted by the executor after a single action finishes.
type BootstrapActionResultMsg struct {
	ActionID string
	Err      error
}

// BootstrapCompleteMsg is emitted when all bootstrap actions have succeeded.
type BootstrapCompleteMsg struct{}

// BootstrapFailedMsg is emitted when a bootstrap action returns a non-nil error.
type BootstrapFailedMsg struct {
	ActionID string
	Err      error
}

// PreflightReRunMsg signals that the preflight should be re-armed and re-run.
type PreflightReRunMsg struct{}
