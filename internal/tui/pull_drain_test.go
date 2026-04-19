package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

// TestPullDrainLoopReadsAllMessages is a regression test for the bug where
// drainPullCh only read ONE progress message and then silently stopped, so
// PullModel.View() was stuck on the generic "Pulling images…" spinner for
// the entire pull duration. The fix: store the channel on PullModel and
// reschedule drainNext() on every progress message until the channel closes.
//
// This test drives the real channel-backed flow (NOT directly feeding Update
// with msgs) so it actually exercises the drain path.
func TestPullDrainLoopReadsAllMessages(t *testing.T) {
	runner := &compose.FakeComposeRunner{
		PullProgressMsgs: []compose.PullProgressMsg{
			{Service: "backend", Status: "Pulling"},
			{Service: "web", Status: "Pulling"},
			{Service: "websocket", Status: "Pulling"},
			{Service: "backend", Status: "Pulled"},
			{Service: "web", Status: "Pulled"},
			{Service: "websocket", Status: "Pulled"},
		},
	}
	m := NewPullModel(theme.Default(), runner, []string{"docker-compose.yml"}, ".env")

	// Kick off Init. tea.Batch returns a Cmd that produces a batchMsg when
	// invoked; we can't unwrap that here so instead we drive each cmd
	// separately. Init spawns two cmds in a batch: runCmd (Pull + close) and
	// drainNext() (first drain read). Pull is synchronous in the fake, so we
	// can run the run cmd first to populate the channel, then drain.
	runCmd := func() tea.Msg {
		err := runner.Pull(nil, nil, "", m.progressCh)
		close(m.progressCh)
		if err != nil {
			return InstallFailureMsg{Err: err, Stage: "pull"}
		}
		return PullCompleteMsg{}
	}
	// Run the pull synchronously — fills the buffered channel.
	_ = runCmd()

	// Now repeatedly drain until the cmd returns nil (channel closed).
	drainReads := 0
	for {
		cmd := m.drainNext()
		msg := cmd()
		if msg == nil {
			break
		}
		drainReads++
		progress, ok := msg.(compose.PullProgressMsg)
		if !ok {
			t.Fatalf("drainNext produced non-progress msg: %T", msg)
		}
		var newCmd tea.Cmd
		m, newCmd = m.Update(progress)
		// Update is expected to reschedule another drain — assert it.
		if newCmd == nil {
			t.Fatalf("Update on PullProgressMsg #%d should return drain cmd, got nil", drainReads)
		}
	}

	if drainReads != len(runner.PullProgressMsgs) {
		t.Errorf("drained %d messages, want %d", drainReads, len(runner.PullProgressMsgs))
	}
	if len(m.services) != 3 {
		t.Errorf("services map size = %d, want 3 (backend, web, websocket)", len(m.services))
	}
	for _, svc := range []string{"backend", "web", "websocket"} {
		if m.services[svc] != "Pulled" {
			t.Errorf("services[%q] = %q, want 'Pulled' (final status)", svc, m.services[svc])
		}
	}
}

// TestPullViewShowsPerServiceStatus verifies the View renders each service
// individually when progress messages arrive, instead of the generic counter.
func TestPullViewShowsPerServiceStatus(t *testing.T) {
	runner := &compose.FakeComposeRunner{}
	m := NewPullModel(theme.Default(), runner, []string{"docker-compose.yml"}, ".env")
	m.services["backend"] = "Pulling"
	m.services["web"] = "Pulled"

	view := m.View()
	for _, svc := range []string{"backend", "web"} {
		if !contains(view, svc) {
			t.Errorf("view should contain service name %q, got:\n%s", svc, view)
		}
	}
	if !contains(view, "Pulling") || !contains(view, "Pulled") {
		t.Errorf("view should contain per-service status strings, got:\n%s", view)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
