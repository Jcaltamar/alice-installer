package installation

import (
	"context"
	"os"
	"path/filepath"
)

const KnownLegacyAliceDirectory = "/opt/backend_alice_guardian/node"

// KnownLegacyDirectoryProbe checks the policy-owned legacy Alice directory.
type KnownLegacyDirectoryProbe struct {
	FS       FileSystem
	Platform Platform
}

func (p KnownLegacyDirectoryProbe) Probe(context.Context) ProbeResult {
	if p.Platform.GOOS != "linux" || (p.Platform.GOARCH != "amd64" && p.Platform.GOARCH != "arm64") {
		return legacyDirectoryResult(PresenceUnsupported, EvidenceLegacyDirectoryUnsupported, "legacy directory probing is unsupported on this platform", "")
	}
	filesystem := p.FS
	if filesystem == nil {
		filesystem = osFS{}
	}
	info, err := filesystem.Stat(KnownLegacyAliceDirectory)
	if os.IsNotExist(err) {
		return legacyDirectoryResult(PresenceAbsent, EvidenceLegacyDirectoryAbsent, "known legacy Alice directory is absent", KnownLegacyAliceDirectory)
	}
	if err != nil {
		return legacyDirectoryResult(PresenceUncertain, EvidenceLegacyDirectoryUnreadable, "known legacy Alice directory cannot be inspected", KnownLegacyAliceDirectory)
	}
	if !info.IsDir() {
		return legacyDirectoryResult(PresenceUncertain, EvidenceLegacyDirectoryInvalid, "known legacy Alice path is not a directory", KnownLegacyAliceDirectory)
	}
	return legacyDirectoryResult(PresencePresent, EvidenceLegacyDirectory, "known legacy Alice directory found", KnownLegacyAliceDirectory)
}

// LegacyFallbackProbe accepts exact directory evidence without invoking PM2 and
// otherwise retains PM2 as optional fallback evidence.
type LegacyFallbackProbe struct {
	Directory Probe
	PM2       Probe
}

func (p LegacyFallbackProbe) Probe(ctx context.Context) ProbeResult {
	directory := p.Directory.Probe(ctx)
	if directory.Presence == PresencePresent || directory.Presence == PresenceUncertain {
		return directory
	}
	pm2 := p.PM2.Probe(ctx)
	pm2.Evidence = append(directory.Evidence, pm2.Evidence...)
	return pm2
}

func legacyDirectoryResult(presence Presence, kind EvidenceKind, detail, path string) ProbeResult {
	if path != "" {
		path = filepath.Clean(path)
	}
	return ProbeResult{Presence: presence, Evidence: []Evidence{{Kind: kind, Source: "legacy-directory", Detail: detail, Path: path}}}
}
