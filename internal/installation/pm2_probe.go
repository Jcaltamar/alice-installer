package installation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"time"
)

type CommandRunner interface {
	Run(context.Context, string, ...string) (stdout, stderr []byte, err error)
}

type LegacyPolicy struct {
	ProcessNames    []string
	ScriptBasenames []string
	DeploymentRoots []string
	EcosystemFiles  []string
}

type Platform struct{ GOOS, GOARCH string }

type PM2Probe struct {
	Runner   CommandRunner
	Platform Platform
	Policy   LegacyPolicy
	Timeout  time.Duration
}

type pm2Record struct {
	Name        string `json:"name"`
	ExecPath    string `json:"pm_exec_path"`
	Environment struct {
		CWD      string `json:"cwd"`
		ExecPath string `json:"pm_exec_path"`
	} `json:"pm2_env"`
}

func (p PM2Probe) Probe(ctx context.Context) ProbeResult {
	if p.Platform.GOOS != "linux" || (p.Platform.GOARCH != "amd64" && p.Platform.GOARCH != "arm64") {
		return pm2Result(PresenceUnsupported, EvidencePM2Unsupported, "legacy PM2 probing is unsupported on this platform")
	}
	if p.Policy.empty() {
		return pm2Result(PresenceAbsent, EvidencePM2Absent, "legacy PM2 probing is disabled")
	}
	if p.Runner == nil {
		return pm2Result(PresenceUncertain, EvidencePM2Failed, "legacy PM2 probe is unavailable")
	}
	if !p.Policy.valid() {
		return pm2Result(PresenceUncertain, EvidencePM2Failed, "legacy PM2 policy is invalid")
	}
	timeout := p.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stdout, _, err := p.Runner.Run(ctx, "pm2", "jlist")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return pm2Result(PresenceAbsent, EvidencePM2Unavailable, "pm2 is not installed")
		}
		return pm2Result(PresenceUncertain, EvidencePM2Failed, "pm2 probe failed")
	}
	var records []pm2Record
	if bytes.Equal(bytes.TrimSpace(stdout), []byte("null")) {
		return pm2Result(PresenceUncertain, EvidencePM2Failed, "pm2 output could not be validated")
	}
	if err := json.Unmarshal(stdout, &records); err != nil {
		return pm2Result(PresenceUncertain, EvidencePM2Failed, "pm2 output could not be validated")
	}
	ambiguous := false
	for _, record := range records {
		matched, weak := p.Policy.matches(record)
		if matched {
			return pm2Result(PresencePresent, EvidencePM2AliceProcess, "configured Alice PM2 deployment found")
		}
		ambiguous = ambiguous || weak
	}
	if ambiguous {
		return pm2Result(PresenceUncertain, EvidencePM2Ambiguous, "PM2 evidence requires manual verification")
	}
	return pm2Result(PresenceAbsent, EvidencePM2Absent, "no configured Alice PM2 deployment found")
}

func (p LegacyPolicy) empty() bool {
	return len(p.ProcessNames) == 0 && len(p.ScriptBasenames) == 0 && len(p.DeploymentRoots) == 0 && len(p.EcosystemFiles) == 0
}

func (p LegacyPolicy) valid() bool {
	identities := len(p.ProcessNames) + len(p.ScriptBasenames) + len(p.EcosystemFiles)
	if identities == 0 {
		return false
	}
	for _, root := range p.DeploymentRoots {
		if root != "" {
			return true
		}
	}
	return false
}

func (p LegacyPolicy) matches(record pm2Record) (bool, bool) {
	nameMatch := containsExact(p.ProcessNames, record.Name)
	scriptMatch := containsBase(p.ScriptBasenames, record.ExecPath) || containsBase(p.ScriptBasenames, record.Environment.ExecPath)
	ecosystemMatch := containsBase(p.EcosystemFiles, record.ExecPath) || containsBase(p.EcosystemFiles, record.Environment.ExecPath)
	for _, root := range p.DeploymentRoots {
		cwdMatch := contained(root, record.Environment.CWD, true)
		execMatch := contained(root, record.ExecPath, false) || contained(root, record.Environment.ExecPath, false)
		if (nameMatch && (cwdMatch || execMatch)) || (scriptMatch && cwdMatch) || (ecosystemMatch && execMatch) {
			return true, false
		}
	}
	return false, nameMatch || scriptMatch || ecosystemMatch
}

func contained(root, path string, allowRoot bool) bool {
	if root == "" || path == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil || rel == ".." || filepath.IsAbs(rel) || (len(rel) >= 3 && rel[:3] == ".."+string(filepath.Separator)) {
		return false
	}
	return allowRoot || rel != "."
}
func containsExact(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
func containsBase(values []string, path string) bool {
	return containsExact(values, filepath.Base(filepath.Clean(path)))
}
func pm2Result(presence Presence, kind EvidenceKind, detail string) ProbeResult {
	return ProbeResult{Presence: presence, Evidence: []Evidence{{Kind: kind, Source: "pm2", Detail: detail}}}
}
