package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

// buildVerifyModel returns a VerifyModel with FakeComposeRunner.
func buildVerifyModel(runner *compose.FakeComposeRunner, timeout time.Duration) VerifyModel {
	m := NewVerifyModel(
		theme.Default(),
		runner,
		[]string{"docker-compose.yml"},
		"/tmp/.env",
	)
	m.timeout = timeout
	return m
}

// TestVerifyModelInitReturnsCmd verifies Init() returns a Cmd.
func TestVerifyModelInitReturnsCmd(t *testing.T) {
	runner := &compose.FakeComposeRunner{
		Healths: []compose.ServiceHealth{
			{Service: "backend", Status: "starting"},
		},
	}
	m := buildVerifyModel(runner, 3*time.Minute)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("VerifyModel.Init() should return a non-nil Cmd")
	}
}

// TestVerifyModelFirstTickPopulatesServicesNotDone verifies first tick populates services
// but does not set done when not all healthy.
func TestVerifyModelFirstTickPopulatesServicesNotDone(t *testing.T) {
	runner := &compose.FakeComposeRunner{
		Healths: []compose.ServiceHealth{
			{Service: "backend", Status: "starting", State: "running"},
			{Service: "web", Status: "healthy", State: "running"},
		},
	}
	m := buildVerifyModel(runner, 3*time.Minute)
	// Send a HealthTickMsg to trigger a poll.
	m, cmd := m.Update(HealthTickMsg{})
	_ = cmd
	if len(m.services) == 0 {
		t.Error("services should be populated after first tick")
	}
	if m.done {
		t.Error("should not be done if not all services are healthy")
	}
}

// TestVerifyModelAllHealthyEmitsInstallSuccessMsg verifies that when all services are
// healthy, InstallSuccessMsg is emitted.
func TestVerifyModelAllHealthyEmitsInstallSuccessMsg(t *testing.T) {
	runner := &compose.FakeComposeRunner{
		Healths: []compose.ServiceHealth{
			{Service: "backend", Status: "healthy", State: "running"},
			{Service: "web", Status: "healthy", State: "running"},
		},
	}
	m := buildVerifyModel(runner, 3*time.Minute)
	m, cmd := m.Update(HealthTickMsg{})
	if !m.done {
		t.Error("all healthy → model should be done")
	}
	if cmd == nil {
		t.Fatal("should emit a Cmd on success")
	}
	msg := cmd()
	if _, ok := msg.(InstallSuccessMsg); !ok {
		t.Errorf("all healthy → Cmd should produce InstallSuccessMsg, got %T", msg)
	}
}

// TestVerifyModelTimeoutEmitsInstallFailureMsg verifies timeout path.
func TestVerifyModelTimeoutEmitsInstallFailureMsg(t *testing.T) {
	runner := &compose.FakeComposeRunner{
		Healths: []compose.ServiceHealth{
			{Service: "backend", Status: "starting"},
		},
	}
	m := buildVerifyModel(runner, 50*time.Millisecond)
	// Set elapsed past the timeout.
	m.elapsed = 100 * time.Millisecond
	m, cmd := m.Update(HealthTickMsg{})
	if cmd == nil {
		t.Fatal("timeout should emit a Cmd")
	}
	msg := cmd()
	fail, ok := msg.(InstallFailureMsg)
	if !ok {
		t.Fatalf("timeout → Cmd should produce InstallFailureMsg, got %T", msg)
	}
	if fail.Stage != "verify" {
		t.Errorf("InstallFailureMsg.Stage = %q, want verify", fail.Stage)
	}
}

// TestVerifyModelRKeyTriggersImmediateTick verifies 'r' key causes a tick.
func TestVerifyModelRKeyTriggersImmediateTick(t *testing.T) {
	runner := &compose.FakeComposeRunner{
		Healths: []compose.ServiceHealth{
			{Service: "backend", Status: "healthy", State: "running"},
		},
	}
	m := buildVerifyModel(runner, 3*time.Minute)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	_ = m
	if cmd == nil {
		t.Fatal("'r' key should return a non-nil Cmd (immediate tick)")
	}
}

// TestVerifyModelViewContainsTitle verifies the View renders a title.
func TestVerifyModelViewContainsTitle(t *testing.T) {
	runner := &compose.FakeComposeRunner{}
	m := buildVerifyModel(runner, 3*time.Minute)
	view := m.View()
	if view == "" {
		t.Error("View() should not be empty")
	}
}

// TestVerifyModel_IsReadyRule covers the State-aware acceptance rule in poll().
func TestVerifyModel_IsReadyRule(t *testing.T) {
	tests := []struct {
		name        string
		healths     []compose.ServiceHealth
		wantSuccess bool // true → InstallSuccessMsg expected; false → HealthReportMsg expected
	}{
		{
			name: "no-healthcheck+running → InstallSuccessMsg (new rule)",
			healths: []compose.ServiceHealth{
				{Service: "rtsp", Status: "", State: "running"},
				{Service: "web", Status: "none", State: "running"},
			},
			wantSuccess: true,
		},
		{
			name: "crash-loop restarting → HealthReportMsg (not success)",
			healths: []compose.ServiceHealth{
				{Service: "websocket", Status: "", State: "restarting"},
			},
			wantSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &compose.FakeComposeRunner{Healths: tt.healths}
			m := buildVerifyModel(runner, 3*time.Minute)
			m, cmd := m.Update(HealthTickMsg{})
			_ = m
			if tt.wantSuccess {
				if cmd == nil {
					t.Fatal("expected a Cmd on success, got nil")
				}
				msg := cmd()
				if _, ok := msg.(InstallSuccessMsg); !ok {
					t.Errorf("expected InstallSuccessMsg, got %T", msg)
				}
			} else {
				// Not all ready → HealthReportMsg (next tick scheduled), done==false.
				if m.done {
					t.Error("should not be done when not all services are ready")
				}
			}
		})
	}
}
