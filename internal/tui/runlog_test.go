package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/jcaltamar/alice-installer/internal/installation"
	"github.com/jcaltamar/alice-installer/internal/migration"
	"github.com/jcaltamar/alice-installer/internal/runlog"
)

type captureRunLog struct{ events []runlog.Event }

func (l *captureRunLog) Log(event runlog.Event) { l.events = append(l.events, event) }
func (*captureRunLog) Path() string             { return "/tmp/installer-run.jsonl" }
func (*captureRunLog) Warning() string          { return "" }
func (*captureRunLog) Close() error             { return nil }

func TestOriginalFailureIsRetainedBeforeRecoveryResult(t *testing.T) {
	log := &captureRunLog{}
	deps := buildTestDeps()
	deps.RunLog, deps.LogPath = log, log.Path()
	deps.MigrationHandoff = &fakeMigrationHandoff{recovery: installation.PM2Recovery{Attempted: 3, Recovered: 3, Verified: true, Code: "pm2-recovery-verified"}}
	m := NewModel(deps)
	m.state, m.migrationPending = StateVerify, true
	m.migrationLease = &migration.PreInstallMigrationLease{}

	updated, cmd := m.Update(InstallFailureMsg{Stage: "verify", Err: errors.New("postgres://canary-secret")})
	m = updated.(Model)
	if cmd == nil || m.state != StateMigrationRecovery {
		t.Fatalf("state/cmd = %v/%v", m.state, cmd)
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	view := m.View()
	for _, want := range []string{"Original failure: stage=verify code=verify-failed", "Recovery status: pm2-recovery-verified", "Log: /tmp/installer-run.jsonl"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q: %s", want, view)
		}
	}
	if len(log.events) < 2 || log.events[0].Event != "original-failure" {
		t.Fatalf("events = %#v", log.events)
	}
}
