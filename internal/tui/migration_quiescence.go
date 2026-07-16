package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jcaltamar/alice-installer/internal/installation"
	"github.com/jcaltamar/alice-installer/internal/migration"
	"github.com/jcaltamar/alice-installer/internal/runlog"
)

// MigrationHandoff owns the PM2 lease outside the TUI policy layer.
type MigrationHandoff interface {
	Begin(context.Context, migration.BackupRef, string, migration.ContainerDisposition) (*migration.PreInstallMigrationLease, error)
	CompleteSuccess(*migration.PreInstallMigrationLease) error
	CompleteFailure(*migration.PreInstallMigrationLease) (installation.PM2Recovery, error)
}

type MigrationQuiescenceCompletedMsg struct {
	Lease *migration.PreInstallMigrationLease
	Err   error
}

type MigrationRecoveryCompletedMsg struct {
	Recovery installation.PM2Recovery
	Err      error
}

type MigrationSuccessCompletedMsg struct {
	Success InstallSuccessMsg
	Err     error
}

// MigrationAbandonedMsg is the explicit terminal signal for a migration that
// cannot continue through the ordinary installer state machine.
type MigrationAbandonedMsg struct{}

func (m Model) beginMigrationQuiescence() (Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	m.quiescenceCancel = cancel
	m.state = StateMigrationQuiescence
	handoff := m.deps.MigrationHandoff
	backup := migration.BackupRef{DumpPath: m.backupResult.DumpPath, ManifestPath: m.backupResult.ManifestPath, SHA256: m.backupResult.SHA256, Size: m.backupResult.Size}
	return m, func() (msg tea.Msg) {
		defer func() {
			if recover() != nil {
				msg = MigrationQuiescenceCompletedMsg{Err: errors.New("migration quiescence failed")}
			}
		}()
		lease, err := handoff.Begin(ctx, backup, m.backupPlan.ContainerID(), m.containerDisposition)
		return MigrationQuiescenceCompletedMsg{Lease: lease, Err: err}
	}
}

func (m Model) beginMigrationRecovery() (Model, tea.Cmd) {
	if !m.hasLiveMigrationLease() {
		m.state = StateMigrationResult
		return m, nil
	}
	if m.restoreCancel != nil {
		m.restoreCancel()
		m.restoreCancel = nil
	}
	if m.quiescenceCancel != nil {
		m.quiescenceCancel()
		m.quiescenceCancel = nil
	}
	m.state = StateMigrationRecovery
	m.log(runlog.Event{Event: "recovery-start", Stage: "recovery", Status: "started"})
	handoff, lease := m.deps.MigrationHandoff, m.migrationLease
	return m, func() (msg tea.Msg) {
		defer func() {
			if recover() != nil {
				msg = MigrationRecoveryCompletedMsg{Err: errors.New("migration recovery failed")}
			}
		}()
		recovery, err := handoff.CompleteFailure(lease)
		return MigrationRecoveryCompletedMsg{Recovery: recovery, Err: err}
	}
}

func (m Model) completeMigrationSuccess(success InstallSuccessMsg) tea.Cmd {
	handoff, lease := m.deps.MigrationHandoff, m.migrationLease
	return func() (msg tea.Msg) {
		defer func() {
			if recover() != nil {
				msg = MigrationSuccessCompletedMsg{Success: success, Err: errors.New("migration completion failed")}
			}
		}()
		return MigrationSuccessCompletedMsg{Success: success, Err: handoff.CompleteSuccess(lease)}
	}
}

func (m Model) hasLiveMigrationLease() bool {
	return m.migrationPending && m.migrationLease != nil && m.deps.MigrationHandoff != nil
}

func boundedRecoveryCode(recovery installation.PM2Recovery, err error) string {
	if recovery.Code == migration.DispositionManualRecoveryCode {
		return recovery.Code
	}
	if recovery.Code == migration.DispositionRecoveryUnprovenCode {
		return recovery.Code
	}
	if err != nil {
		return "pm2-recovery-unproven"
	}
	if recovery.Verified && recovery.Code == "pm2-recovery-verified" {
		return recovery.Code
	}
	return "pm2-recovery-unproven"
}

func boundedQuiescenceCode(err error) string {
	var failure installation.QuiescenceError
	if errors.As(err, &failure) {
		switch failure.Code {
		case "pm2-observation-unavailable", "pm2-correlation-failed", "pm2-state-changed", "pm2-stop-failed", "pm2-stop-unproven", "pm2-final-state-unproven":
			return failure.Code
		}
	}
	if errors.Is(err, migration.ErrSudoDockerPermission) {
		return migration.SudoDockerPermissionCode
	}
	return "pm2-quiescence-unavailable"
}

func boundedQuiescenceDiagnostic(err error) *installation.PM2ObservationDiagnostic {
	var failure installation.QuiescenceError
	if !errors.As(err, &failure) || failure.Diagnostic == nil || failure.Code != "pm2-observation-unavailable" && failure.Code != "pm2-stop-unproven" {
		return nil
	}
	diagnostic := *failure.Diagnostic
	return &diagnostic
}

func (m Model) migrationQuiescenceView() string {
	return m.deps.Theme.TextMuted.Render("Quiescing confirmed legacy services and applying the selected PostgreSQL disposition before installation. Press Escape to cancel safely.\n")
}

func (m Model) migrationRecoveryView() string {
	return m.deps.Theme.TextMuted.Render("Completing bounded legacy PM2 recovery. Do not close the installer.\n")
}

func (m Model) migrationTerminalView() string {
	var message strings.Builder
	message.WriteString("Migration did not complete.\n")
	message.WriteString("The installer did not report installation success.\n")
	if m.originalFailure != nil {
		code := "install-failed"
		if m.originalFailure.Stage != "" {
			code = m.originalFailure.Stage + "-failed"
		}
		message.WriteString("Original failure: stage=" + m.originalFailure.Stage + " code=" + code + "\n")
	}
	message.WriteString("Recovery status: " + m.migrationRecoveryCode + "\n")
	if m.deps.LogPath != "" {
		message.WriteString("Log: " + m.deps.LogPath + "\n")
	}
	if diagnostic := m.migrationDiagnostic; diagnostic != nil {
		stage, operation, command, cause := diagnostic.Stage, diagnostic.Operation, diagnostic.Command, diagnostic.Cause
		if diagnostic.StopProofTimedOut || diagnostic.StopProofCancelled {
			stage = "stop-proof"
			operation = "stopped-and-released"
			command = fmt.Sprintf("sudo -n pm2 stop %d", diagnostic.PMID)
			cause = diagnostic.String()
		} else if diagnostic.RecoveryProofTimedOut || diagnostic.RecoveryProofCancelled {
			stage = "recovery-proof"
			operation = "online-port-proc"
			command = fmt.Sprintf("sudo -n pm2 start %d", diagnostic.PMID)
			cause = diagnostic.String()
		}
		message.WriteString("\nDebug:\n")
		for _, field := range []struct{ label, value string }{{"Stage", stage}, {"Operation", operation}, {"Command", command}, {"Cause", cause}} {
			if field.value != "" {
				message.WriteString(field.label + ": " + field.value + "\n")
			}
		}
	}
	if m.migrationRecoveryCode == migration.DispositionManualRecoveryCode {
		message.WriteString("\nThe legacy container was removed and cannot be recreated automatically; use the preserved volumes and validated backup for bounded manual recovery.\n")
	}
	view := strings.TrimSuffix(message.String(), "\n")
	if m.width > 0 {
		view = lipgloss.NewStyle().Width(m.width).Render(view)
	}
	return m.deps.Theme.Danger.Render(view + "\n")
}
