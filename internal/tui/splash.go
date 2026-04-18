package tui

import (
	"bytes"
	"image/color"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/eliukblau/pixterm/pkg/ansimage"

	"github.com/jcaltamar/alice-installer/internal/assets"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

// splashIconRows / splashIconCols size the rendered shield icon. Half-block
// rendering packs 2 image pixels per terminal row, so the image is sampled
// at splashIconCols × splashIconRows*2 pixels. 30×15 gives a crisp glyph on
// any terminal ≥ 80×24.
const (
	splashIconRows = 15
	splashIconCols = 30
)

// SplashModel renders the initial branding screen.
//
// Layout (top to bottom):
//  1. Alice Security shield icon (pixterm half-block ANSI, cached)
//  2. ALICE GUARDIAN plain-text wordmark in Primary colour
//  3. Installer subtitle in TextMuted colour
//
// Controls:
//   - Enter → emits PreflightStartedMsg to advance to the preflight state.
//   - Any other key → no-op.
type SplashModel struct {
	theme theme.Theme
	icon  string
}

// NewSplashModel constructs a SplashModel with the given theme.
func NewSplashModel(th theme.Theme) SplashModel {
	return SplashModel{theme: th, icon: renderSplashIcon()}
}

// Init implements tea.Model.
// Returns nil — the splash screen waits for the user to press Enter.
func (s SplashModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
// Enter key returns a command that emits PreflightStartedMsg.
func (s SplashModel) Update(msg tea.Msg) (SplashModel, tea.Cmd) {
	switch m := msg.(type) {
	case tea.KeyMsg:
		switch m.Type {
		case tea.KeyEnter:
			return s, func() tea.Msg { return PreflightStartedMsg{} }
		}
	}
	return s, nil
}

// View implements tea.Model.
// Renders the icon, the ALICE GUARDIAN wordmark, and the subtitle.
func (s SplashModel) View() string {
	wordmark := s.theme.Primary.Bold(true).Render("  ALICE GUARDIAN")
	subtitle := s.theme.TextMuted.Render("  Installer v0.1.0  —  press Enter to start")
	return s.icon + "\n" + wordmark + "\n\n" + subtitle + "\n"
}

// renderSplashIcon decodes the embedded shield PNG and returns a half-block
// ANSI string. Falls back to an empty string if decoding fails — the ASCII
// wordmark below still carries the branding, so the splash remains usable.
func renderSplashIcon() string {
	img, err := ansimage.NewScaledFromReader(
		bytes.NewReader(assets.LogoAliceSecurity),
		splashIconRows*2, // rows are doubled because half-blocks pack 2 image rows per terminal row
		splashIconCols,
		color.Transparent,
		ansimage.ScaleModeResize,
		ansimage.NoDithering,
	)
	if err != nil {
		return ""
	}
	return indent(img.Render(), "  ")
}

// indent prefixes every line of s with prefix.
func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
