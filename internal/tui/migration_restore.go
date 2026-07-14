package tui

import (
	"github.com/jcaltamar/alice-installer/internal/migration"
)

// RestoreCompletedMsg carries only the typed, redacted coordinator result.
type RestoreCompletedMsg struct{ Result migration.RestoreResult }

func (m Model) migrationRestoreView() string {
	return m.deps.Theme.TextMuted.Render("Restoring validated legacy database. Press Escape to cancel safely.\n")
}

func (m Model) migrationResultView() string {
	return m.deps.Theme.Danger.Render("Migration restore did not complete. The installer did not report installation success. Review the bounded recovery guidance for the reported stage.\n")
}
