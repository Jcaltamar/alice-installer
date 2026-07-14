package tui

import (
	"context"
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/installation"
	"github.com/jcaltamar/alice-installer/internal/migration"
)

// MigrationHandoff owns the PM2 lease outside the TUI policy layer.
type MigrationHandoff interface {
	Begin(context.Context, migration.BackupRef) (*migration.PreInstallMigrationLease, error)
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
		lease, err := handoff.Begin(ctx, backup)
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
	if err != nil {
		return "pm2-recovery-unproven"
	}
	if recovery.Verified && recovery.Code == "pm2-recovery-verified" {
		return recovery.Code
	}
	return "pm2-recovery-unproven"
}

func (m Model) migrationQuiescenceView() string {
	return m.deps.Theme.TextMuted.Render("Verifying and stopping the confirmed legacy PM2 services before installation. Press Escape to cancel safely.\n")
}

func (m Model) migrationRecoveryView() string {
	return m.deps.Theme.TextMuted.Render("Completing bounded legacy PM2 recovery. Do not close the installer.\n")
}

func (m Model) migrationTerminalView() string {
	return m.deps.Theme.Danger.Render("Migration did not complete. The installer did not report installation success. Recovery status: " + m.migrationRecoveryCode + ".\n")
}
