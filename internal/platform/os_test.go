package platform_test

import (
	"testing"

	"github.com/jcaltamar/alice-installer/internal/platform"
)

func TestRuntimeOSGuard_IsLinux(t *testing.T) {
	tests := []struct {
		name      string
		goos      string
		wantLinux bool
		wantName  string
	}{
		{"linux → IsLinux=true", "linux", true, "linux"},
		{"darwin → IsLinux=false", "darwin", false, "darwin"},
		{"windows → IsLinux=false", "windows", false, "windows"},
		{"freebsd → IsLinux=false", "freebsd", false, "freebsd"},
		{"openbsd → IsLinux=false", "openbsd", false, "openbsd"},
		{"empty → IsLinux=false", "", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := platform.NewRuntimeOSGuard(func() string { return tt.goos })
			if got := g.IsLinux(); got != tt.wantLinux {
				t.Errorf("IsLinux() = %v, want %v", got, tt.wantLinux)
			}
			if got := g.OSName(); got != tt.wantName {
				t.Errorf("OSName() = %q, want %q", got, tt.wantName)
			}
		})
	}
}

func TestOSGuard_Interface(t *testing.T) {
	var _ platform.OSGuard = platform.NewRuntimeOSGuard(nil)
	t.Log("interface satisfied")
}
