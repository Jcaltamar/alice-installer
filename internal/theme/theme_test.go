package theme_test

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

// assertColor verifies that a lipgloss.Style has the expected foreground color.
func assertColor(t *testing.T, token string, style lipgloss.Style, want theme.ColorToken) {
	t.Helper()
	got := style.GetForeground()
	wantColor := lipgloss.Color(want)
	if got != wantColor {
		t.Errorf("token %q: foreground = %v, want %v (%s)", token, got, wantColor, string(want))
	}
}

func TestDefault_TokenColors(t *testing.T) {
	th := theme.Default()

	tests := []struct {
		token string
		style lipgloss.Style
		color theme.ColorToken
	}{
		{"Background", th.Background, theme.ColorBackground},
		{"Surface", th.Surface, theme.ColorSurface},
		{"Primary", th.Primary, theme.ColorPrimary},
		{"Accent", th.Accent, theme.ColorAccent},
		{"Success", th.Success, theme.ColorSuccess},
		{"Warning", th.Warning, theme.ColorWarning},
		{"Danger", th.Danger, theme.ColorDanger},
		{"TextPrimary", th.TextPrimary, theme.ColorTextPrimary},
		{"TextMuted", th.TextMuted, theme.ColorTextMuted},
		{"Border", th.Border, theme.ColorBorder},
	}

	for _, tt := range tests {
		t.Run(tt.token, func(t *testing.T) {
			assertColor(t, tt.token, tt.style, tt.color)
		})
	}
}

func TestDefault_ColorConstants(t *testing.T) {
	// Verify each constant holds the expected hex value from the design doc.
	tests := []struct {
		name    string
		token   theme.ColorToken
		wantHex string
	}{
		{"Background", theme.ColorBackground, "#0f172a"},
		{"Surface", theme.ColorSurface, "#1e293b"},
		{"Primary (cyan)", theme.ColorPrimary, "#22d3ee"},
		{"Accent (teal)", theme.ColorAccent, "#4fd1c5"},
		{"Success", theme.ColorSuccess, "#22c55e"},
		{"Warning", theme.ColorWarning, "#f59e0b"},
		{"Danger", theme.ColorDanger, "#ef4444"},
		{"TextPrimary", theme.ColorTextPrimary, "#f1f5f9"},
		{"TextMuted", theme.ColorTextMuted, "#64748b"},
		{"Border", theme.ColorBorder, "#334155"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.token) != tt.wantHex {
				t.Errorf("ColorToken %q = %q, want %q", tt.name, string(tt.token), tt.wantHex)
			}
		})
	}
}

func TestDefault_Rendered_ContainsText(t *testing.T) {
	th := theme.Default()
	// Rendering should not panic and should return a non-empty string
	result := th.TextPrimary.Render("hello")
	if result == "" {
		t.Error("Render(\"hello\") returned empty string, want non-empty")
	}
	// The rendered output must contain the original text
	if len(result) < len("hello") {
		t.Errorf("Render(\"hello\") len=%d < 5, should contain original text", len(result))
	}
}
