package installation

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceProbe(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)
		want  Presence
		kind  EvidenceKind
	}{
		{"complete artifacts", func(t *testing.T, dir string) {
			writeArtifact(t, dir, ".env")
			writeArtifact(t, dir, "docker-compose.yml")
		}, PresencePresent, EvidenceWorkspaceComplete},
		{"empty workspace", func(*testing.T, string) {}, PresenceAbsent, EvidenceWorkspaceAbsent},
		{"partial workspace", func(t *testing.T, dir string) { writeArtifact(t, dir, ".env") }, PresenceUncertain, EvidenceWorkspacePartial},
		{"directory is invalid", func(t *testing.T, dir string) {
			writeArtifact(t, dir, ".env")
			if err := os.Mkdir(filepath.Join(dir, "docker-compose.yml"), 0o755); err != nil {
				t.Fatal(err)
			}
		}, PresenceUncertain, EvidenceWorkspaceInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)
			got := WorkspaceProbe{WorkspaceDir: dir}.Probe(context.Background())
			if got.Presence != tt.want || len(got.Evidence) == 0 || got.Evidence[0].Kind != tt.kind {
				t.Fatalf("Probe() = %#v, want presence %v and evidence %v", got, tt.want, tt.kind)
			}
			for _, evidence := range got.Evidence {
				if evidence.Path != "" && filepath.Dir(evidence.Path) != dir {
					t.Fatalf("evidence path %q does not use override %q", evidence.Path, dir)
				}
			}
		})
	}
}

type errorFS struct{}

func (errorFS) Stat(string) (fs.FileInfo, error) { return nil, errors.New("permission denied") }

func TestWorkspaceProbe_StatFailureIsUncertain(t *testing.T) {
	got := WorkspaceProbe{WorkspaceDir: "/safe/workspace", FS: errorFS{}}.Probe(context.Background())
	if got.Presence != PresenceUncertain || got.Evidence[0].Kind != EvidenceWorkspaceUnreadable {
		t.Fatalf("Probe() = %#v, want unreadable uncertainty", got)
	}
}

func writeArtifact(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
}
