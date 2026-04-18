// Package theme defines the visual design system for the alice-installer TUI.
// It wraps Lipgloss styles with semantic tokens so callers never hard-code hex
// values directly — they use Theme fields or ColorToken constants instead.
package theme

import "github.com/charmbracelet/lipgloss"

// ColorToken is a hex colour constant (e.g. "#0f172a").
// It is defined as a named string type so it is distinguishable from arbitrary
// strings and can be passed directly to lipgloss.Color().
type ColorToken string

// Design palette — single source of truth for all hex values.
// Every token matches the spec defined in design.md (Theme section).
const (
	ColorBackground ColorToken = "#0f172a"
	ColorSurface    ColorToken = "#1e293b"
	ColorPrimary    ColorToken = "#22d3ee" // cyan
	ColorAccent     ColorToken = "#4fd1c5" // teal
	ColorSuccess    ColorToken = "#22c55e"
	ColorWarning    ColorToken = "#f59e0b"
	ColorDanger     ColorToken = "#ef4444"
	ColorTextPrimary ColorToken = "#f1f5f9"
	ColorTextMuted  ColorToken = "#64748b"
	ColorBorder     ColorToken = "#334155"
)

// color returns a lipgloss.Color for the given token.
func color(t ColorToken) lipgloss.Color {
	return lipgloss.Color(string(t))
}

// Theme holds a Lipgloss Style for each semantic token.
// Use these styles when rendering TUI elements; use the ColorToken constants
// when you need a raw hex value (e.g. for progress-bar colours).
type Theme struct {
	Background  lipgloss.Style
	Surface     lipgloss.Style
	Primary     lipgloss.Style
	Accent      lipgloss.Style
	Success     lipgloss.Style
	Warning     lipgloss.Style
	Danger      lipgloss.Style
	TextPrimary lipgloss.Style
	TextMuted   lipgloss.Style
	Border      lipgloss.Style
}

// Default returns the Theme populated with the design-spec colour palette.
// All styles set only the foreground colour; callers may chain additional
// Lipgloss modifiers (Bold, Italic, etc.) without mutating the base theme.
func Default() Theme {
	return Theme{
		Background:  lipgloss.NewStyle().Foreground(color(ColorBackground)),
		Surface:     lipgloss.NewStyle().Foreground(color(ColorSurface)),
		Primary:     lipgloss.NewStyle().Foreground(color(ColorPrimary)),
		Accent:      lipgloss.NewStyle().Foreground(color(ColorAccent)),
		Success:     lipgloss.NewStyle().Foreground(color(ColorSuccess)),
		Warning:     lipgloss.NewStyle().Foreground(color(ColorWarning)),
		Danger:      lipgloss.NewStyle().Foreground(color(ColorDanger)),
		TextPrimary: lipgloss.NewStyle().Foreground(color(ColorTextPrimary)),
		TextMuted:   lipgloss.NewStyle().Foreground(color(ColorTextMuted)),
		Border:      lipgloss.NewStyle().Foreground(color(ColorBorder)),
	}
}
