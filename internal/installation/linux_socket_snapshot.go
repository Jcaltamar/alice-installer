package installation

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type SocketOwner struct {
	PID  int
	Port uint16
}

// LinuxSocketSnapshot acquires listening TCP ownership through the one reviewed
// command shape required by PM2 correlation.
type LinuxSocketSnapshot struct {
	Runner    CommandRunner
	Timeout   time.Duration
	MaxOutput int
}

func (s LinuxSocketSnapshot) Snapshot(ctx context.Context) ([]SocketOwner, error) {
	if s.Runner == nil {
		return nil, errors.New("socket snapshot unavailable")
	}
	if err := acquisitionContextError(ctx, "socket snapshot"); err != nil {
		return nil, err
	}
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = defaultPM2AcquisitionTimeout
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stdout, _, err := s.Runner.Run(commandCtx, "ss", "-H", "-ltnp")
	if err != nil {
		if contextErr := acquisitionContextError(commandCtx, "socket snapshot"); contextErr != nil {
			return nil, contextErr
		}
		return nil, errors.New("socket snapshot command failed")
	}
	if contextErr := acquisitionContextError(commandCtx, "socket snapshot"); contextErr != nil {
		return nil, contextErr
	}
	if len(stdout) > acquisitionOutputLimit(s.MaxOutput) {
		return nil, errors.New("socket snapshot output exceeded limit")
	}
	owners, err := ParseSocketSnapshot(stdout)
	if err != nil {
		return nil, errors.New("socket snapshot output is invalid")
	}
	return owners, nil
}

var socketLine = regexp.MustCompile(`^LISTEN\s+.*:([0-9]+)\s+.*pid=([0-9]+),`)

func ParseSocketSnapshot(data []byte) ([]SocketOwner, error) {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return nil, errors.New("socket snapshot is empty")
	}
	ports, pids := map[uint16]bool{}, map[int]bool{}
	owners := make([]SocketOwner, 0, len(lines))
	for _, line := range lines {
		if strings.Count(line, "pid=") != 1 {
			return nil, errors.New("socket ownership is ambiguous")
		}
		match := socketLine.FindStringSubmatch(line)
		if len(match) != 3 {
			return nil, errors.New("socket snapshot is invalid")
		}
		port, err := strconv.ParseUint(match[1], 10, 16)
		if err != nil || port == 0 || ports[uint16(port)] {
			return nil, errors.New("socket ownership is ambiguous")
		}
		pid, err := strconv.Atoi(match[2])
		if err != nil || pid <= 0 || pids[pid] {
			return nil, errors.New("socket owner is invalid")
		}
		ports[uint16(port)], pids[pid] = true, true
		owners = append(owners, SocketOwner{PID: pid, Port: uint16(port)})
	}
	return owners, nil
}
