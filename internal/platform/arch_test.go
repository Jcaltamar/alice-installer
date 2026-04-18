package platform_test

import (
	"testing"

	"github.com/jcaltamar/alice-installer/internal/platform"
)

func TestRuntimeArchDetector_Detect(t *testing.T) {
	tests := []struct {
		name     string
		goarch   string
		wantArch platform.Arch
	}{
		{"amd64 maps to ArchAMD64", "amd64", platform.ArchAMD64},
		{"arm64 maps to ArchARM64", "arm64", platform.ArchARM64},
		{"386 maps to ArchUnknown", "386", platform.ArchUnknown},
		{"arm (32-bit) maps to ArchUnknown", "arm", platform.ArchUnknown},
		{"riscv64 maps to ArchUnknown", "riscv64", platform.ArchUnknown},
		{"empty string maps to ArchUnknown", "", platform.ArchUnknown},
		{"darwin (wrong type) maps to ArchUnknown", "darwin", platform.ArchUnknown},
		{"s390x maps to ArchUnknown", "s390x", platform.ArchUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := platform.NewRuntimeArchDetector(func() string { return tt.goarch })
			got := d.Detect()
			if got != tt.wantArch {
				t.Errorf("Detect() = %q, want %q", got, tt.wantArch)
			}
		})
	}
}

func TestArchDetector_Interface(t *testing.T) {
	// Compile-time check: RuntimeArchDetector implements ArchDetector
	var _ platform.ArchDetector = platform.NewRuntimeArchDetector(nil)
	t.Log("interface satisfied")
}
