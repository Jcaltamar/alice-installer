package tui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jcaltamar/alice-installer/internal/ports"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

// PortScanModel renders the port-conflict resolution screen.
//
// Behaviour:
//   - Init() scans all required ports and emits PortScanResultMsg.
//   - If no conflicts → immediately emits PortsConfirmedMsg.
//   - If conflicts → resolving=true, focuses on first conflict.
//   - User enters an alternate port; on Enter: parse, range-check, scan, update FreePlan.
//   - After all conflicts resolved → emits PortsConfirmedMsg.
//   - Ctrl+R → rescan all ports.
type PortScanModel struct {
	theme    theme.Theme
	scanner  ports.PortScanner
	required map[string]int // env key → default port (TCP)
	udp      map[string]int // env key → default port (UDP)
	result   *PortScanResultMsg

	// resolution state
	resolving    bool
	conflicts    []PortConflict // remaining unresolved conflicts
	freePlan     map[string]int // resolved ports so far
	currentKey   string
	currentPort  int
	input        textinput.Model
	err          string
}

// NewPortScanModel constructs a PortScanModel.
// required maps env-var names to their default TCP ports.
// udp maps env-var names to their default UDP ports (may be nil).
func NewPortScanModel(
	th theme.Theme,
	scanner ports.PortScanner,
	required map[string]int,
	udp map[string]int,
) PortScanModel {
	ti := textinput.New()
	ti.Placeholder = "e.g. 5433"
	ti.CharLimit = 6
	ti.Width = 20
	ti.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(string(theme.ColorTextPrimary)))
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(string(theme.ColorTextMuted)))

	return PortScanModel{
		theme:    th,
		scanner:  scanner,
		required: required,
		udp:      udp,
		input:    ti,
	}
}

// Init implements tea.Model.
// Returns a Cmd that scans all required ports and emits PortScanResultMsg.
func (p PortScanModel) Init() tea.Cmd {
	return func() tea.Msg {
		return p.scanAll()
	}
}

// scanAll scans all required ports and builds a PortScanResultMsg.
func (p PortScanModel) scanAll() PortScanResultMsg {
	ctx := context.Background()
	var conflicts []PortConflict
	freePlan := make(map[string]int)

	// Sort keys for deterministic order.
	keys := make([]string, 0, len(p.required))
	for k := range p.required {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		port := p.required[k]
		if p.scanner.IsAvailable(ctx, port) {
			freePlan[k] = port
		} else {
			conflicts = append(conflicts, PortConflict{Key: k, Requested: port, Reason: "occupied"})
		}
	}

	// UDP ports.
	udpKeys := make([]string, 0, len(p.udp))
	for k := range p.udp {
		udpKeys = append(udpKeys, k)
	}
	sort.Strings(udpKeys)

	for _, k := range udpKeys {
		port := p.udp[k]
		if p.scanner.IsUDPAvailable(ctx, port) {
			freePlan[k] = port
		} else {
			conflicts = append(conflicts, PortConflict{Key: k, Requested: port, Reason: "occupied"})
		}
	}

	return PortScanResultMsg{Conflicts: conflicts, FreePlan: freePlan}
}

// Update implements tea.Model.
func (p PortScanModel) Update(msg tea.Msg) (PortScanModel, tea.Cmd) {
	var cmd tea.Cmd

	switch m := msg.(type) {
	case PortScanResultMsg:
		p.result = &m
		p.freePlan = m.FreePlan
		if p.freePlan == nil {
			p.freePlan = make(map[string]int)
		}

		if len(m.Conflicts) == 0 {
			// No conflicts — confirm immediately.
			finalPorts := copyMap(p.freePlan)
			return p, func() tea.Msg { return PortsConfirmedMsg{FinalPorts: finalPorts} }
		}

		// Enter resolving mode.
		p.conflicts = append([]PortConflict{}, m.Conflicts...)
		p.resolving = true
		p.currentKey = p.conflicts[0].Key
		p.currentPort = p.conflicts[0].Requested
		p.input.Focus()
		p.input.SetValue("")
		return p, textinput.Blink

	case tea.KeyMsg:
		switch {
		case m.Type == tea.KeyCtrlR:
			// Rescan.
			return p, func() tea.Msg { return p.scanAll() }

		case m.Type == tea.KeyEnter && p.resolving:
			return p.handleAlternatePort()
		}
	}

	if p.resolving {
		p.input, cmd = p.input.Update(msg)
	}
	return p, cmd
}

// handleAlternatePort parses and validates the typed alternate port.
func (p PortScanModel) handleAlternatePort() (PortScanModel, tea.Cmd) {
	raw := strings.TrimSpace(p.input.Value())
	port, err := strconv.Atoi(raw)
	if err != nil {
		p.err = fmt.Sprintf("Enter a numeric port number (got %q)", raw)
		return p, nil
	}
	if port <= 0 || port > 65535 {
		p.err = fmt.Sprintf("Port must be between 1 and 65535 (got %d)", port)
		return p, nil
	}

	// Check availability.
	ctx := context.Background()
	if !p.scanner.IsAvailable(ctx, port) {
		p.err = fmt.Sprintf("Port %d is also occupied. Try a different port.", port)
		return p, nil
	}

	// Port is free — accept it.
	p.err = ""
	p.freePlan[p.currentKey] = port

	// Remove the resolved conflict from the list.
	remaining := p.conflicts[1:]
	p.conflicts = remaining

	if len(remaining) == 0 {
		// All resolved.
		p.resolving = false
		finalPorts := copyMap(p.freePlan)
		return p, func() tea.Msg { return PortsConfirmedMsg{FinalPorts: finalPorts} }
	}

	// Advance to next conflict.
	p.currentKey = remaining[0].Key
	p.currentPort = remaining[0].Requested
	p.input.SetValue("")
	return p, nil
}

// View implements tea.Model.
func (p PortScanModel) View() string {
	title := p.theme.Primary.Bold(true).Render("Port Availability")

	if p.result == nil {
		// Scanning.
		return title + "\n\n" + p.theme.TextMuted.Render("Scanning required ports…") + "\n"
	}

	if !p.resolving {
		return title + "\n\n" + p.theme.Success.Render("✓  All required ports are available.") + "\n"
	}

	// Resolving a conflict.
	var sb strings.Builder
	sb.WriteString(title)
	sb.WriteString("\n\n")
	sb.WriteString(p.theme.Warning.Render(fmt.Sprintf(
		"Port %d (%s) is occupied.",
		p.currentPort, p.currentKey,
	)))
	sb.WriteString("\n\n")
	sb.WriteString(p.theme.TextPrimary.Render("Enter a new port:"))
	sb.WriteString("\n  ")
	sb.WriteString(p.input.View())
	sb.WriteString("\n")

	if p.err != "" {
		sb.WriteString("\n  ")
		sb.WriteString(p.theme.Danger.Render("✗  " + p.err))
		sb.WriteString("\n")
	}

	remaining := len(p.conflicts)
	if remaining > 1 {
		sb.WriteString(p.theme.TextMuted.Render(fmt.Sprintf("\n  %d more conflict(s) to resolve.", remaining-1)))
	}

	return sb.String()
}

// copyMap returns a shallow copy of an int map.
func copyMap(m map[string]int) map[string]int {
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
