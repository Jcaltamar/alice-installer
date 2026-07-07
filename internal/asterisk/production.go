package asterisk

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/jcaltamar/alice-installer/internal/platform"
)

type CommandPackageManager struct {
	Kind   PackageManagerKind
	Runner platform.CommandRunner
}

func NewCommandPackageManager(kind PackageManagerKind, runner platform.CommandRunner) CommandPackageManager {
	if runner == nil {
		runner = &platform.OSCommandRunner{}
	}
	return CommandPackageManager{Kind: kind, Runner: runner}
}

func (c CommandPackageManager) IsInstalled(ctx context.Context, name string) (bool, error) {
	cmd, args, err := c.queryCommand(name)
	if err != nil {
		return false, err
	}
	_, _, runErr := c.Runner.Run(ctx, cmd, args...)
	return runErr == nil, nil
}

func (c CommandPackageManager) Install(ctx context.Context, name string) error {
	cmd, args, err := c.installCommand(name)
	if err != nil {
		return err
	}
	_, stderr, runErr := c.Runner.Run(ctx, cmd, args...)
	return commandError("install "+name, stderr, runErr)
}

func (c CommandPackageManager) Remove(ctx context.Context, name string) error {
	cmd, args, err := c.removeCommand(name)
	if err != nil {
		return err
	}
	_, stderr, runErr := c.Runner.Run(ctx, cmd, args...)
	return commandError("remove "+name, stderr, runErr)
}

func (c CommandPackageManager) queryCommand(name string) (string, []string, error) {
	switch c.Kind {
	case PackageManagerAPT:
		return "dpkg", []string{"-s", name}, nil
	case PackageManagerDNF, PackageManagerYUM:
		return "rpm", []string{"-q", name}, nil
	case PackageManagerPacman:
		return "pacman", []string{"-Q", name}, nil
	default:
		return "", nil, UnsupportedHostError{Reason: "unsupported Linux host: install apt, dnf, yum, or pacman before selecting Asterisk"}
	}
}

func (c CommandPackageManager) installCommand(name string) (string, []string, error) {
	switch c.Kind {
	case PackageManagerAPT:
		return "sudo", []string{"apt-get", "install", "-y", name}, nil
	case PackageManagerDNF:
		return "sudo", []string{"dnf", "install", "-y", name}, nil
	case PackageManagerYUM:
		return "sudo", []string{"yum", "install", "-y", name}, nil
	case PackageManagerPacman:
		return "sudo", []string{"pacman", "-S", "--noconfirm", name}, nil
	default:
		return "", nil, UnsupportedHostError{Reason: "unsupported Linux host: install apt, dnf, yum, or pacman before selecting Asterisk"}
	}
}

func (c CommandPackageManager) removeCommand(name string) (string, []string, error) {
	switch c.Kind {
	case PackageManagerAPT:
		return "sudo", []string{"apt-get", "remove", "-y", name}, nil
	case PackageManagerDNF:
		return "sudo", []string{"dnf", "remove", "-y", name}, nil
	case PackageManagerYUM:
		return "sudo", []string{"yum", "remove", "-y", name}, nil
	case PackageManagerPacman:
		return "sudo", []string{"pacman", "-Rns", "--noconfirm", name}, nil
	default:
		return "", nil, UnsupportedHostError{Reason: "unsupported Linux host: install apt, dnf, yum, or pacman before selecting Asterisk"}
	}
}

type SystemdServiceManager struct {
	Runner platform.CommandRunner
}

func NewSystemdServiceManager(runner platform.CommandRunner) SystemdServiceManager {
	if runner == nil {
		runner = &platform.OSCommandRunner{}
	}
	return SystemdServiceManager{Runner: runner}
}

func (s SystemdServiceManager) IsEnabled(ctx context.Context, name string) (bool, error) {
	_, _, err := s.Runner.Run(ctx, "systemctl", "is-enabled", "--quiet", name)
	return err == nil, nil
}

func (s SystemdServiceManager) IsActive(ctx context.Context, name string) (bool, error) {
	_, _, err := s.Runner.Run(ctx, "systemctl", "is-active", "--quiet", name)
	return err == nil, nil
}

func (s SystemdServiceManager) Enable(ctx context.Context, name string) error {
	_, stderr, err := s.Runner.Run(ctx, "sudo", "systemctl", "enable", name)
	return commandError("enable "+name, stderr, err)
}

func (s SystemdServiceManager) Restart(ctx context.Context, name string) error {
	_, stderr, err := s.Runner.Run(ctx, "sudo", "systemctl", "restart", name)
	return commandError("restart "+name, stderr, err)
}

func (s SystemdServiceManager) Disable(ctx context.Context, name string) error {
	_, stderr, err := s.Runner.Run(ctx, "sudo", "systemctl", "disable", name)
	return commandError("disable "+name, stderr, err)
}

func (s SystemdServiceManager) Stop(ctx context.Context, name string) error {
	_, stderr, err := s.Runner.Run(ctx, "sudo", "systemctl", "stop", name)
	return commandError("stop "+name, stderr, err)
}

type TCPAMIProbe struct {
	Timeout time.Duration
}

func (p TCPAMIProbe) VerifyAMI(ctx context.Context, host string, port int, username string, password string) error {
	timeout := p.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return err
	}
	defer conn.Close()

	deadline := time.Now().Add(timeout)
	_ = conn.SetDeadline(deadline)
	reader := bufio.NewReader(conn)
	if _, err := reader.ReadString('\n'); err != nil {
		return fmt.Errorf("read AMI greeting: %w", err)
	}
	if _, err := fmt.Fprintf(conn, "Action: Login\r\nUsername: %s\r\nSecret: %s\r\nEvents: off\r\n\r\n", username, password); err != nil {
		return fmt.Errorf("send AMI login: %w", err)
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read AMI login response: %w", err)
		}
		line = strings.TrimSpace(line)
		if strings.EqualFold(line, "Response: Success") {
			return nil
		}
		if strings.EqualFold(line, "Response: Error") {
			return fmt.Errorf("AMI login rejected")
		}
		if line == "" {
			return fmt.Errorf("AMI login did not return success")
		}
	}
}

func NewProductionDependencies(detector HostDetector, packageManager PackageManagerKind, runner platform.CommandRunner) Dependencies {
	return Dependencies{
		Detector:  detector,
		Packages:  NewCommandPackageManager(packageManager, runner),
		Services:  NewSystemdServiceManager(runner),
		Configs:   FileConfigStore{},
		Resources: FileResourceStore{},
		Probe:     TCPAMIProbe{},
	}
}

func commandError(action string, stderr []byte, err error) error {
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(string(stderr))
	if detail == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: %w: %s", action, err, detail)
}
