package installation

import (
	"context"
	"errors"
	"io/fs"
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name            string
		current, legacy ProbeResult
		want            State
	}{
		{"absent probes", ProbeResult{Presence: PresenceAbsent}, ProbeResult{Presence: PresenceAbsent}, StateNotInstalled},
		{"unsupported legacy", ProbeResult{Presence: PresenceAbsent}, ProbeResult{Presence: PresenceUnsupported}, StateNotInstalled},
		{"unsupported current", ProbeResult{Presence: PresenceUnsupported}, ProbeResult{Presence: PresencePresent}, StateUnknown},
		{"current", ProbeResult{Presence: PresencePresent}, ProbeResult{Presence: PresenceUnsupported}, StateCurrent},
		{"legacy", ProbeResult{Presence: PresenceAbsent}, ProbeResult{Presence: PresencePresent}, StateLegacyPM2},
		{"conflict", ProbeResult{Presence: PresencePresent}, ProbeResult{Presence: PresencePresent}, StateConflict},
		{"current uncertainty wins", ProbeResult{Presence: PresenceUncertain}, ProbeResult{Presence: PresencePresent}, StateUnknown},
		{"legacy uncertainty wins", ProbeResult{Presence: PresenceAbsent}, ProbeResult{Presence: PresenceUncertain}, StateUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.current, tt.legacy).State; got != tt.want {
				t.Fatalf("Classify() = %v, want %v", got, tt.want)
			}
		})
	}
}

type fakeProbe struct{ result ProbeResult }

func (f fakeProbe) Probe(context.Context) ProbeResult { return f.result }

type legacyStatFS struct {
	info fs.FileInfo
	err  error
}

func (f legacyStatFS) Stat(string) (fs.FileInfo, error) { return f.info, f.err }

type legacyFileInfo struct{ directory bool }

func (f legacyFileInfo) Name() string { return "node" }
func (f legacyFileInfo) Size() int64  { return 0 }
func (f legacyFileInfo) Mode() fs.FileMode {
	if f.directory {
		return fs.ModeDir
	}
	return 0
}
func (f legacyFileInfo) ModTime() time.Time { return time.Time{} }
func (f legacyFileInfo) IsDir() bool        { return f.directory }
func (f legacyFileInfo) Sys() any           { return nil }

func TestKnownLegacyDirectoryProbe(t *testing.T) {
	tests := []struct {
		name string
		fs   FileSystem
		want Presence
		kind EvidenceKind
	}{
		{"known directory exists", legacyStatFS{info: legacyFileInfo{directory: true}}, PresencePresent, EvidenceLegacyDirectory},
		{"known path is missing", legacyStatFS{err: fs.ErrNotExist}, PresenceAbsent, EvidenceLegacyDirectoryAbsent},
		{"known path is a file", legacyStatFS{info: legacyFileInfo{}}, PresenceUncertain, EvidenceLegacyDirectoryInvalid},
		{"known path cannot be inspected", legacyStatFS{err: errors.New("permission denied")}, PresenceUncertain, EvidenceLegacyDirectoryUnreadable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (KnownLegacyDirectoryProbe{FS: tt.fs, Platform: Platform{GOOS: "linux", GOARCH: "amd64"}}).Probe(context.Background())
			if got.Presence != tt.want || len(got.Evidence) != 1 || got.Evidence[0].Kind != tt.kind {
				t.Fatalf("Probe() = %#v, want %v/%v", got, tt.want, tt.kind)
			}
		})
	}
}

func TestKnownLegacyDirectoryProbe_UnsupportedPlatformDoesNotStat(t *testing.T) {
	got := (KnownLegacyDirectoryProbe{FS: legacyStatFS{err: errors.New("must not stat")}, Platform: Platform{GOOS: "windows", GOARCH: "amd64"}}).Probe(context.Background())
	if got.Presence != PresenceUnsupported {
		t.Fatalf("Probe() = %#v, want unsupported", got)
	}
}

type countingProbe struct {
	result ProbeResult
	calls  int
}

func (p *countingProbe) Probe(context.Context) ProbeResult { p.calls++; return p.result }

func TestLegacyFallbackProbe(t *testing.T) {
	tests := []struct {
		name         string
		directory    Presence
		pm2          Presence
		want         Presence
		wantPM2Calls int
	}{
		{"directory evidence does not require PM2", PresencePresent, PresenceUncertain, PresencePresent, 0},
		{"missing directory falls back to PM2", PresenceAbsent, PresencePresent, PresencePresent, 1},
		{"directory stat error remains uncertain", PresenceUncertain, PresencePresent, PresenceUncertain, 0},
		{"unsupported directory retains PM2 platform result", PresenceUnsupported, PresenceUnsupported, PresenceUnsupported, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm2 := &countingProbe{result: ProbeResult{Presence: tt.pm2}}
			got := (LegacyFallbackProbe{Directory: fakeProbe{ProbeResult{Presence: tt.directory}}, PM2: pm2}).Probe(context.Background())
			if got.Presence != tt.want || pm2.calls != tt.wantPM2Calls {
				t.Fatalf("Probe() = %#v, PM2 calls = %d; want %v/%d", got, pm2.calls, tt.want, tt.wantPM2Calls)
			}
		})
	}
}

func TestCompositeDetector_CombinesAndSortsEvidence(t *testing.T) {
	detector := CompositeDetector{
		Current: fakeProbe{ProbeResult{Presence: PresencePresent, Evidence: []Evidence{{Kind: EvidenceWorkspaceComplete, Source: "workspace", Path: "/z"}}}},
		Legacy:  fakeProbe{ProbeResult{Presence: PresencePresent, Evidence: []Evidence{{Kind: EvidencePM2AliceProcess, Source: "pm2", Path: "/a"}}}},
	}
	got := detector.Detect(context.Background())
	if got.State != StateConflict {
		t.Fatalf("state = %v, want conflict", got.State)
	}
	if len(got.Evidence) != 2 || got.Evidence[0].Kind != EvidenceWorkspaceComplete || got.Evidence[1].Kind != EvidencePM2AliceProcess {
		t.Fatalf("evidence = %#v, want workspace then pm2", got.Evidence)
	}
}
