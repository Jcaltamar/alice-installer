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
			return nil, wrapObservationUnavailable(contextErr.Error(), err)
		}
		return nil, wrapObservationUnavailable("socket snapshot command failed", err)
	}
	if contextErr := acquisitionContextError(commandCtx, "socket snapshot"); contextErr != nil {
		return nil, contextErr
	}
	if len(stdout) > acquisitionOutputLimit(s.MaxOutput) {
		return nil, wrapObservationUnavailable("socket snapshot output exceeded limit", observationOutputError("socket-listeners", "sudo -n ss -H -ltnp", "output-too-large"))
	}
	owners, err := ParseSocketSnapshot(stdout)
	if err != nil {
		return nil, wrapObservationUnavailable("socket snapshot output is invalid", observationOutputError("socket-listeners", "sudo -n ss -H -ltnp", "output-invalid"))
	}
	return owners, nil
}

var socketPID = regexp.MustCompile(`pid=([0-9]+),`)

var migrationPorts = map[uint16]bool{4550: true, 8080: true, 9090: true}

func ParseSocketSnapshot(data []byte) ([]SocketOwner, error) {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return nil, errors.New("socket snapshot is empty")
	}
	ports := map[uint16]int{}
	seen := map[SocketOwner]bool{}
	owners := make([]SocketOwner, 0, len(migrationPorts))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			if mentionsMigrationPort(line) {
				return nil, errors.New("socket snapshot is invalid")
			}
			continue
		}
		separator := strings.LastIndexByte(fields[3], ':')
		if separator < 0 {
			if mentionsMigrationPort(line) {
				return nil, errors.New("socket snapshot is invalid")
			}
			continue
		}
		portText := fields[3][separator+1:]
		port, err := strconv.ParseUint(portText, 10, 16)
		if err != nil {
			if mentionsMigrationPort(fields[3]) {
				return nil, errors.New("socket snapshot is invalid")
			}
			continue
		}
		if !migrationPorts[uint16(port)] {
			continue
		}
		if fields[0] != "LISTEN" || port == 0 || strconv.FormatUint(port, 10) != portText || strings.Count(line, "pid=") != 1 {
			return nil, errors.New("socket ownership is ambiguous")
		}
		match := socketPID.FindStringSubmatch(line)
		if len(match) != 2 {
			return nil, errors.New("socket snapshot is invalid")
		}
		pid, err := strconv.Atoi(match[1])
		if err != nil || pid <= 0 || strconv.Itoa(pid) != match[1] {
			return nil, errors.New("socket owner is invalid")
		}
		owner := SocketOwner{PID: pid, Port: uint16(port)}
		if existing, ok := ports[owner.Port]; ok && existing != owner.PID {
			return nil, errors.New("socket ownership is ambiguous")
		}
		ports[owner.Port] = owner.PID
		if !seen[owner] {
			seen[owner] = true
			owners = append(owners, owner)
		}
	}
	return owners, nil
}

func mentionsMigrationPort(value string) bool {
	for _, field := range strings.Fields(value) {
		separator := strings.LastIndexByte(field, ':')
		for port := range migrationPorts {
			approved := strconv.FormatUint(uint64(port), 10)
			candidate := field[separator+1:]
			if strings.HasPrefix(candidate, approved) && (len(candidate) == len(approved) || candidate[len(approved)] < '0' || candidate[len(approved)] > '9') {
				return true
			}
		}
	}
	return false
}
