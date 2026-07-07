package asterisk

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallerSelectedPathInstallsConfiguresServiceAndVerifiesAMI(t *testing.T) {
	t.Parallel()

	fake := NewFakeEnvironment()
	fake.Detector.State = SupportedHost(PackageManagerAPT)
	fake.Configs.Files[ManagerConfigPath] = "[operator]\nkeep=yes\n"
	installer := NewInstaller(fake.Dependencies())

	result, err := installer.Install(context.Background(), Options{Enabled: true, ConfigRoot: t.TempDir(), AMI: AMIContract{Username: "guardian", Password: "secret"}})
	if err != nil {
		t.Fatalf("Install() returned unexpected error: %v", err)
	}
	if !result.Installed || result.AMIEndpoint != "127.0.0.1:5038" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if got := strings.Join(fake.Packages.InstalledPackages, ","); got != "asterisk" {
		t.Fatalf("expected asterisk package install, got %q", got)
	}
	if fake.Services.EnableCalls != 1 || fake.Services.RestartCalls != 1 {
		t.Fatalf("expected enable and restart once, got enable=%d restart=%d", fake.Services.EnableCalls, fake.Services.RestartCalls)
	}
	if fake.Probe.Host != "127.0.0.1" || fake.Probe.Port != 5038 || fake.Probe.Username != "guardian" || fake.Probe.Password != "secret" {
		t.Fatalf("AMI probe did not use localhost shared credentials: %+v", fake.Probe)
	}
	manager := fake.Configs.Files[ManagerConfigPath]
	for _, want := range []string{"[operator]\nkeep=yes", "bindaddr=127.0.0.1", "port=5038", "username=guardian"} {
		if !strings.Contains(manager, want) {
			t.Fatalf("manager config missing %q:\n%s", want, manager)
		}
	}
}

func TestInstallerReportsFailureWhenAMILocalhostVerificationFails(t *testing.T) {
	t.Parallel()

	fake := NewFakeEnvironment()
	fake.Detector.State = SupportedHost(PackageManagerAPT)
	fake.Probe.Err = errors.New("connection refused")
	installer := NewInstaller(fake.Dependencies())

	_, err := installer.Install(context.Background(), Options{Enabled: true, ConfigRoot: t.TempDir(), AMI: AMIContract{Username: "guardian", Password: "secret"}})
	if err == nil {
		t.Fatal("expected AMI verification failure")
	}
	if !strings.Contains(err.Error(), "asterisk optional setup failed during ami verification") {
		t.Fatalf("expected optional setup failure stage, got %q", err.Error())
	}
	if fake.Probe.Host != "127.0.0.1" || fake.Probe.Port != 5038 {
		t.Fatalf("AMI verification must target localhost:5038, got %+v", fake.Probe)
	}
}

func TestRollbackRemovesInstallerCreatedResourceBundleWhenLaterFailureOccurs(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "asterisk")
	fake := NewFakeEnvironment()
	fake.Resources = nil
	fake.Probe.Err = errors.New("verification failed after bundle creation")
	deps := fake.Dependencies()
	deps.Resources = FileResourceStore{}
	installer := NewInstaller(deps)

	_, err := installer.Install(context.Background(), Options{Enabled: true, ConfigRoot: root, AMI: AMIContract{Username: "guardian", Password: "secret"}})
	if err == nil {
		t.Fatal("expected failure after resource bundle creation")
	}
	if _, statErr := os.Stat(filepath.Join(root, "integration.env")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("rollback must remove installer-created integration.env, stat err=%v", statErr)
	}
	if _, statErr := os.Stat(root); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("rollback must remove installer-created resource root, stat err=%v", statErr)
	}
}

func TestRollbackRestoresPreExistingResourceBundleWhenLaterFailureOccurs(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "asterisk")
	if err := os.MkdirAll(filepath.Join(root, "templates"), 0o750); err != nil {
		t.Fatalf("seed pre-existing templates dir: %v", err)
	}
	integrationPath := filepath.Join(root, "integration.env")
	originalIntegration := "ASTERISK_AMI_PASSWORD=operator-secret\n"
	if err := os.WriteFile(integrationPath, []byte(originalIntegration), 0o600); err != nil {
		t.Fatalf("seed pre-existing integration.env: %v", err)
	}
	operatorTemplate := filepath.Join(root, "templates", "operator.conf")
	if err := os.WriteFile(operatorTemplate, []byte("operator-owned=true\n"), 0o640); err != nil {
		t.Fatalf("seed pre-existing operator template: %v", err)
	}

	fake := NewFakeEnvironment()
	fake.Resources = nil
	fake.Probe.Err = errors.New("verification failed after bundle creation")
	deps := fake.Dependencies()
	deps.Resources = FileResourceStore{}
	installer := NewInstaller(deps)

	_, err := installer.Install(context.Background(), Options{Enabled: true, ConfigRoot: root, AMI: AMIContract{Username: "guardian", Password: "installer-secret"}})
	if err == nil {
		t.Fatal("expected failure after resource bundle creation")
	}
	content, readErr := os.ReadFile(integrationPath)
	if readErr != nil {
		t.Fatalf("rollback must restore pre-existing integration.env: %v", readErr)
	}
	if string(content) != originalIntegration {
		t.Fatalf("rollback should restore original integration.env, got:\n%s", content)
	}
	templateContent, readErr := os.ReadFile(operatorTemplate)
	if readErr != nil {
		t.Fatalf("rollback must keep operator resource file: %v", readErr)
	}
	if string(templateContent) != "operator-owned=true\n" {
		t.Fatalf("rollback should restore operator resource file, got:\n%s", templateContent)
	}
}

func TestInstallerRejectsExistingManagerConfigWithExternalAMIBind(t *testing.T) {
	t.Parallel()

	fake := NewFakeEnvironment()
	fake.Detector.State = SupportedHost(PackageManagerAPT)
	fake.Configs.Files[ManagerConfigPath] = "[general]\nenabled=yes\nbindaddr=0.0.0.0\nport=5038\n"
	installer := NewInstaller(fake.Dependencies())

	_, err := installer.Install(context.Background(), Options{Enabled: true, ConfigRoot: t.TempDir(), AMI: AMIContract{Username: "guardian", Password: "secret"}})
	if err == nil {
		t.Fatal("expected external existing AMI bind to be rejected")
	}
	if !strings.Contains(err.Error(), "existing AMI bindaddr must be 127.0.0.1") {
		t.Fatalf("expected existing bindaddr validation error, got %q", err.Error())
	}
	if fake.Services.RestartCalls != 0 || fake.Probe.Host != "" {
		t.Fatalf("service/probe must not run for unsafe existing AMI config, restart=%d probe=%+v", fake.Services.RestartCalls, fake.Probe)
	}
}

func TestInstallerRejectsNonLocalhostAMIHost(t *testing.T) {
	t.Parallel()

	fake := NewFakeEnvironment()
	installer := NewInstaller(fake.Dependencies())

	_, err := installer.Install(context.Background(), Options{Enabled: true, ConfigRoot: t.TempDir(), AMI: AMIContract{Username: "guardian", Password: "secret", Host: "0.0.0.0"}})
	if err == nil {
		t.Fatal("expected non-localhost AMI host to be rejected")
	}
	if !strings.Contains(err.Error(), "AMI must bind to 127.0.0.1") {
		t.Fatalf("expected localhost validation error, got %q", err.Error())
	}
	if fake.Probe.Host != "" {
		t.Fatalf("AMI verification must not run for invalid host, probe=%+v", fake.Probe)
	}
}

func TestInstallerRejectsNonDefaultAMIPort(t *testing.T) {
	t.Parallel()

	fake := NewFakeEnvironment()
	installer := NewInstaller(fake.Dependencies())

	_, err := installer.Install(context.Background(), Options{Enabled: true, ConfigRoot: t.TempDir(), AMI: AMIContract{Username: "guardian", Password: "secret", Port: 15038}})
	if err == nil {
		t.Fatal("expected non-default AMI port to be rejected")
	}
	if !strings.Contains(err.Error(), "AMI must listen on port 5038") {
		t.Fatalf("expected fixed-port validation error, got %q", err.Error())
	}
	if fake.Probe.Port != 0 {
		t.Fatalf("AMI verification must not run for invalid port, probe=%+v", fake.Probe)
	}
}

func TestRollbackPreservesPreExistingOperatorManagedAsterisk(t *testing.T) {
	t.Parallel()

	originalManager := "[operator]\nkeep=yes\n"
	fake := NewFakeEnvironment()
	fake.Detector.State = SupportedHost(PackageManagerAPT)
	fake.Packages.PreInstalled = true
	fake.Services.PreEnabled = true
	fake.Services.PreActive = true
	fake.Configs.Files[ManagerConfigPath] = originalManager
	fake.Probe.Err = errors.New("verification failed")
	installer := NewInstaller(fake.Dependencies())

	_, err := installer.Install(context.Background(), Options{Enabled: true, ConfigRoot: t.TempDir(), AMI: AMIContract{Username: "guardian", Password: "secret"}})
	if err == nil {
		t.Fatal("expected failure to trigger rollback")
	}
	if fake.Packages.RemoveCalls != 0 {
		t.Fatalf("rollback must not uninstall pre-existing Asterisk, remove calls=%d", fake.Packages.RemoveCalls)
	}
	if fake.Services.DisableCalls != 0 || fake.Services.StopCalls != 0 {
		t.Fatalf("rollback must not disable/stop pre-existing service, disable=%d stop=%d", fake.Services.DisableCalls, fake.Services.StopCalls)
	}
	if got := fake.Configs.Files[ManagerConfigPath]; got != originalManager {
		t.Fatalf("rollback should restore original manager config, got:\n%s", got)
	}
}
