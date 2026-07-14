package installation

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/jcaltamar/alice-installer/internal/workspace"
)

type FileSystem interface {
	Stat(string) (fs.FileInfo, error)
}

type osFS struct{}

func (osFS) Stat(path string) (fs.FileInfo, error) { return os.Stat(path) }

// WorkspaceProbe validates persisted workspace files without reading or changing them.
type WorkspaceProbe struct {
	WorkspaceDir string
	FS           FileSystem
}

func (p WorkspaceProbe) Probe(context.Context) ProbeResult {
	filesystem := p.FS
	if filesystem == nil {
		filesystem = osFS{}
	}
	paths := workspace.RequiredArtifactPaths(p.WorkspaceDir)
	infos := []struct {
		path string
		info fs.FileInfo
		err  error
	}{
		{path: paths.EnvFile}, {path: paths.BaseFile},
	}
	missing := 0
	for i := range infos {
		infos[i].info, infos[i].err = filesystem.Stat(infos[i].path)
		if os.IsNotExist(infos[i].err) {
			missing++
		}
	}
	if missing == len(infos) {
		return workspaceResult(PresenceAbsent, EvidenceWorkspaceAbsent, "required workspace artifacts are absent", "")
	}
	for _, item := range infos {
		if item.err != nil {
			kind := EvidenceWorkspaceUnreadable
			if os.IsNotExist(item.err) {
				kind = EvidenceWorkspacePartial
			}
			return workspaceResult(PresenceUncertain, kind, "workspace artifacts cannot be validated", item.path)
		}
		if !item.info.Mode().IsRegular() {
			return workspaceResult(PresenceUncertain, EvidenceWorkspaceInvalid, "workspace artifact is not a regular file", item.path)
		}
	}
	return workspaceResult(PresencePresent, EvidenceWorkspaceComplete, "required workspace artifacts are complete", filepath.Clean(paths.BaseFile))
}

func workspaceResult(presence Presence, kind EvidenceKind, detail, path string) ProbeResult {
	return ProbeResult{Presence: presence, Evidence: []Evidence{{Kind: kind, Source: "workspace", Detail: detail, Path: path}}}
}
