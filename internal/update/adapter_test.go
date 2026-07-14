package update

import (
	"context"
	"testing"

	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/platform"
)

func TestActionPreservesUpdateContract(t *testing.T) {
	workspace := t.TempDir()
	mustWrite(t, workspace+"/.env", "WORKSPACE=test\n")
	mustWrite(t, workspace+"/docker-compose.yml", "services: {}\n")
	composeFake := &compose.FakeComposeRunner{}
	action := Action{Config: Config{WorkspaceDir: workspace}, Dependencies: Dependencies{Compose: composeFake, GPU: &platform.FakeGPUDetector{}}}
	if err := action.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := composeFake.CallOrder; len(got) != 2 || got[0] != "pull" || got[1] != "up" {
		t.Fatalf("CallOrder = %v, want [pull up]", got)
	}
}
