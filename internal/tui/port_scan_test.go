package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/ports"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

// defaultPorts is the set of required ports used in tests.
var defaultPorts = map[string]int{
	"POSTGRES_PORT": 5432,
	"REDIS_PORT":    6379,
}

func newTestPortScan(occupied []int) PortScanModel {
	return NewPortScanModel(
		theme.Default(),
		&ports.FakePortScanner{OccupiedPorts: occupied},
		defaultPorts,
		nil, // no UDP ports in these tests
	)
}

// TestPortScanInitReturnsCmd verifies Init() returns a non-nil Cmd.
func TestPortScanInitReturnsCmd(t *testing.T) {
	m := newTestPortScan(nil)
	if m.Init() == nil {
		t.Fatal("PortScanModel.Init should return a non-nil command")
	}
}

// TestPortScanNoConflictsEmitsPortsConfirmedMsg verifies that when all ports
// are free the scan immediately emits PortsConfirmedMsg.
func TestPortScanNoConflictsEmitsPortsConfirmedMsg(t *testing.T) {
	m := newTestPortScan(nil) // no occupied ports
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init should return a cmd")
	}
	msg := cmd()
	scanResult, ok := msg.(PortScanResultMsg)
	if !ok {
		t.Fatalf("Init cmd should produce PortScanResultMsg, got %T", msg)
	}
	if len(scanResult.Conflicts) != 0 {
		t.Errorf("no occupied ports → Conflicts should be empty, got %v", scanResult.Conflicts)
	}
}

// TestPortScanOneConflictBuildsConflictList verifies that one occupied port
// shows up in the Conflicts list.
func TestPortScanOneConflictBuildsConflictList(t *testing.T) {
	m := newTestPortScan([]int{5432}) // POSTGRES_PORT occupied
	cmd := m.Init()
	msg := cmd()
	scanResult, ok := msg.(PortScanResultMsg)
	if !ok {
		t.Fatalf("Init cmd should produce PortScanResultMsg, got %T", msg)
	}
	if len(scanResult.Conflicts) != 1 {
		t.Errorf("one occupied port → 1 conflict, got %d", len(scanResult.Conflicts))
	}
	if scanResult.Conflicts[0].Key != "POSTGRES_PORT" {
		t.Errorf("conflict key = %q, want %q", scanResult.Conflicts[0].Key, "POSTGRES_PORT")
	}
}

// TestPortScanResolvingConflictWithFreePort verifies that entering an alternate
// free port resolves the conflict.
func TestPortScanResolvingConflictWithFreePort(t *testing.T) {
	m := newTestPortScan([]int{5432}) // POSTGRES_PORT occupied; 5433 is free

	// Deliver the scan result to put the model in resolving state.
	m, _ = m.Update(PortScanResultMsg{
		Conflicts: []PortConflict{{Key: "POSTGRES_PORT", Requested: 5432, Reason: "occupied"}},
		FreePlan:  map[string]int{"REDIS_PORT": 6379},
	})

	if !m.resolving {
		t.Fatal("model should be in resolving state after conflict")
	}

	// Type "5433" as the alternate port.
	for _, r := range "5433" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Press Enter to confirm.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on valid alternate port should return a command")
	}
	msg := cmd()
	if _, ok := msg.(PortsConfirmedMsg); !ok {
		t.Errorf("resolving last conflict should emit PortsConfirmedMsg, got %T", msg)
	}
}

// TestPortScanResolvingWithOccupiedAlternateSetsError verifies that entering an
// also-occupied port shows an error.
func TestPortScanResolvingWithOccupiedAlternateSetsError(t *testing.T) {
	// Both 5432 and 5433 are occupied.
	m := NewPortScanModel(
		theme.Default(),
		&ports.FakePortScanner{OccupiedPorts: []int{5432, 5433}},
		defaultPorts,
		nil,
	)

	// Put model in resolving state.
	m, _ = m.Update(PortScanResultMsg{
		Conflicts: []PortConflict{{Key: "POSTGRES_PORT", Requested: 5432, Reason: "occupied"}},
		FreePlan:  map[string]int{"REDIS_PORT": 6379},
	})

	// Type the also-occupied port.
	for _, r := range "5433" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.err == "" {
		t.Error("submitting an occupied alternate port should set an error")
	}
}

// TestPortScanResolvingWithInvalidInputSetsError verifies that non-numeric
// input sets an error.
func TestPortScanResolvingWithInvalidInputSetsError(t *testing.T) {
	m := newTestPortScan([]int{5432})
	m, _ = m.Update(PortScanResultMsg{
		Conflicts: []PortConflict{{Key: "POSTGRES_PORT", Requested: 5432, Reason: "occupied"}},
		FreePlan:  map[string]int{"REDIS_PORT": 6379},
	})

	for _, r := range "abc" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.err == "" {
		t.Error("non-numeric alternate port should set an error")
	}
}

// TestPortScanResolvingWithOutOfRangePortSetsError verifies that port 0 and
// port 65536 are rejected.
func TestPortScanResolvingWithOutOfRangePortSetsError(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"zero", "0"},
		{"above max", "65536"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestPortScan([]int{5432})
			m, _ = m.Update(PortScanResultMsg{
				Conflicts: []PortConflict{{Key: "POSTGRES_PORT", Requested: 5432, Reason: "occupied"}},
				FreePlan:  map[string]int{"REDIS_PORT": 6379},
			})

			for _, r := range tt.input {
				m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			}

			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if updated.err == "" {
				t.Errorf("port %q should set a range error", tt.input)
			}
		})
	}
}
