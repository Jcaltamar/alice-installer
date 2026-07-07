package asterisk

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSupportDetectorAcceptsOnlySupportedLinuxPackageManagerAndSystemd(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		goos    string
		bins    map[string]string
		wantPM  PackageManagerKind
		wantErr string
	}{
		{
			name:   "linux apt with systemd is supported",
			goos:   "linux",
			bins:   map[string]string{"apt-get": "/usr/bin/apt-get", "systemctl": "/bin/systemctl"},
			wantPM: PackageManagerAPT,
		},
		{
			name:   "linux dnf with systemd is supported",
			goos:   "linux",
			bins:   map[string]string{"dnf": "/usr/bin/dnf", "systemctl": "/bin/systemctl"},
			wantPM: PackageManagerDNF,
		},
		{
			name:    "non linux is unsupported",
			goos:    "darwin",
			bins:    map[string]string{"apt-get": "/usr/bin/apt-get", "systemctl": "/bin/systemctl"},
			wantErr: "Asterisk setup is supported only on Linux hosts",
		},
		{
			name:    "linux without supported package manager is unsupported",
			goos:    "linux",
			bins:    map[string]string{"systemctl": "/bin/systemctl"},
			wantErr: "install apt, dnf, yum, or pacman",
		},
		{
			name:    "linux without systemd is unsupported",
			goos:    "linux",
			bins:    map[string]string{"apt-get": "/usr/bin/apt-get"},
			wantErr: "systemd is required to manage the asterisk service",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			detector := HostEnvironmentDetector{
				GOOS: tt.goos,
				LookPath: func(name string) (string, error) {
					if path, ok := tt.bins[name]; ok {
						return path, nil
					}
					return "", errors.New("not found")
				},
			}

			state, err := detector.Detect(context.Background())
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected unsupported host error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("Detect() returned unexpected error: %v", err)
			}
			if state.OS != "linux" || state.PackageManager != tt.wantPM || !state.SystemdAvailable {
				t.Fatalf("unexpected support state: %+v", state)
			}
		})
	}
}

func TestInstallerFailsFastWhenHostDetectionIsUnsupported(t *testing.T) {
	t.Parallel()

	fake := NewFakeEnvironment()
	fake.Detector.State = HostState{OS: "linux", PackageManager: PackageManagerUnknown, SystemdAvailable: false}
	fake.Detector.Err = UnsupportedHostError{Reason: "install apt, dnf, yum, or pacman before selecting Asterisk"}
	installer := NewInstaller(fake.Dependencies())

	_, err := installer.Install(context.Background(), Options{Enabled: true, ConfigRoot: t.TempDir(), AMI: AMIContract{Username: "alice", Password: "secret"}})
	if err == nil {
		t.Fatal("expected install to fail fast on unsupported host")
	}
	if !strings.Contains(err.Error(), "install apt, dnf, yum, or pacman") {
		t.Fatalf("expected actionable unsupported-host error, got %q", err.Error())
	}
	if len(fake.Packages.InstalledPackages) != 0 {
		t.Fatalf("package install should not run for unsupported host, got %v", fake.Packages.InstalledPackages)
	}
}
