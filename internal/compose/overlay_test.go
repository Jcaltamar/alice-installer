package compose_test

import (
	"testing"

	"github.com/jcaltamar/alice-installer/internal/compose"
)

func TestComposeFiles(t *testing.T) {
	tests := []struct {
		name        string
		gpuDetected bool
		baseline    string
		overlay     string
		wantFiles   []string
	}{
		{
			name:        "gpu=false returns only baseline",
			gpuDetected: false,
			baseline:    "docker-compose.yml",
			overlay:     "docker-compose.gpu.yml",
			wantFiles:   []string{"docker-compose.yml"},
		},
		{
			name:        "gpu=true returns baseline then overlay",
			gpuDetected: true,
			baseline:    "docker-compose.yml",
			overlay:     "docker-compose.gpu.yml",
			wantFiles:   []string{"docker-compose.yml", "docker-compose.gpu.yml"},
		},
		{
			name:        "gpu=false with custom paths returns only baseline",
			gpuDetected: false,
			baseline:    "/opt/ag/base.yml",
			overlay:     "/opt/ag/gpu.yml",
			wantFiles:   []string{"/opt/ag/base.yml"},
		},
		{
			name:        "gpu=true with custom paths returns both",
			gpuDetected: true,
			baseline:    "/opt/ag/base.yml",
			overlay:     "/opt/ag/gpu.yml",
			wantFiles:   []string{"/opt/ag/base.yml", "/opt/ag/gpu.yml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compose.ComposeFiles(tt.gpuDetected, tt.baseline, tt.overlay)
			if len(got) != len(tt.wantFiles) {
				t.Fatalf("ComposeFiles() = %v (len=%d), want %v (len=%d)",
					got, len(got), tt.wantFiles, len(tt.wantFiles))
			}
			for i, want := range tt.wantFiles {
				if got[i] != want {
					t.Errorf("ComposeFiles()[%d] = %q, want %q", i, got[i], want)
				}
			}
		})
	}
}
