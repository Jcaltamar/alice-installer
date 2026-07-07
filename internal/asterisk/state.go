package asterisk

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
)

type HostState struct {
	OS               string
	PackageManager   PackageManagerKind
	SystemdAvailable bool
}

type installSnapshot struct {
	packagePreInstalled bool
	servicePreEnabled   bool
	servicePreActive    bool
	configs             map[string]configSnapshot
	resources           ResourceBundleSnapshot
}

type configSnapshot struct {
	content string
	exists  bool
}

type ResourceBundleSnapshot struct {
	root    string
	exists  bool
	entries []resourceSnapshotEntry
}

type resourceSnapshotEntry struct {
	path       string
	mode       uint32
	isDir      bool
	isSymlink  bool
	linkTarget string
	content    []byte
}

type UnsupportedHostError struct {
	Reason string
}

func (e UnsupportedHostError) Error() string {
	return e.Reason
}

type HostEnvironmentDetector struct {
	GOOS     string
	LookPath func(string) (string, error)
}

func (d HostEnvironmentDetector) Detect(context.Context) (HostState, error) {
	goos := d.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	if goos != "linux" {
		return HostState{OS: goos, PackageManager: PackageManagerUnknown}, UnsupportedHostError{Reason: "Asterisk setup is supported only on Linux hosts"}
	}

	lookPath := d.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}

	pm := PackageManagerUnknown
	for _, candidate := range []struct {
		binary string
		kind   PackageManagerKind
	}{
		{binary: "apt-get", kind: PackageManagerAPT},
		{binary: "dnf", kind: PackageManagerDNF},
		{binary: "yum", kind: PackageManagerYUM},
		{binary: "pacman", kind: PackageManagerPacman},
	} {
		if _, err := lookPath(candidate.binary); err == nil {
			pm = candidate.kind
			break
		}
	}
	if pm == PackageManagerUnknown {
		return HostState{OS: goos, PackageManager: pm}, UnsupportedHostError{Reason: "unsupported Linux host: install apt, dnf, yum, or pacman before selecting Asterisk"}
	}

	if _, err := lookPath("systemctl"); err != nil {
		return HostState{OS: goos, PackageManager: pm}, UnsupportedHostError{Reason: "unsupported Linux host: systemd is required to manage the asterisk service"}
	}

	return HostState{OS: goos, PackageManager: pm, SystemdAvailable: true}, nil
}

func SupportedHost(pm PackageManagerKind) HostState {
	return HostState{OS: "linux", PackageManager: pm, SystemdAvailable: true}
}

func (s HostState) Validate() error {
	if s.OS != "linux" {
		return UnsupportedHostError{Reason: "Asterisk setup is supported only on Linux hosts"}
	}
	if s.PackageManager == PackageManagerUnknown || s.PackageManager == "" {
		return UnsupportedHostError{Reason: "unsupported Linux host: install apt, dnf, yum, or pacman before selecting Asterisk"}
	}
	if !s.SystemdAvailable {
		return UnsupportedHostError{Reason: "unsupported Linux host: systemd is required to manage the asterisk service"}
	}
	return nil
}

func optionalSetupError(stage string, err error) error {
	return fmt.Errorf("asterisk optional setup failed during %s: %w", stage, err)
}
