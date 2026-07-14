package installation

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultProcStatLimit = 4 * 1024

type ProcIdentity struct {
	CWD, ExecPath string
	StartTicks    uint64
}

type LinuxProcIdentity struct {
	ProcRoot     string
	MaxStatBytes int
	ReadLink     func(string) (string, error)
}

func (p LinuxProcIdentity) Read(ctx context.Context, pid int) (ProcIdentity, error) {
	if pid <= 0 {
		return ProcIdentity{}, errors.New("proc pid is invalid")
	}
	if err := ctx.Err(); err != nil {
		return ProcIdentity{}, err
	}
	root := p.ProcRoot
	if root == "" {
		root = "/proc"
	}
	limit := p.MaxStatBytes
	if limit <= 0 {
		limit = defaultProcStatLimit
	}
	procDir := filepath.Join(filepath.Clean(root), strconv.Itoa(pid))
	cwd, err := canonicalProcLink(p.ReadLink, filepath.Join(procDir, "cwd"))
	if err != nil {
		return ProcIdentity{}, errors.New("proc cwd is unavailable")
	}
	executable, err := canonicalProcLink(p.ReadLink, filepath.Join(procDir, "exe"))
	if err != nil {
		return ProcIdentity{}, errors.New("proc executable is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return ProcIdentity{}, err
	}
	stat, err := readBoundedProcFile(filepath.Join(procDir, "stat"), limit)
	if err != nil {
		return ProcIdentity{}, errors.New("proc stat is unavailable")
	}
	ticks, err := ParseProcStartTicks(stat)
	if err != nil {
		return ProcIdentity{}, err
	}
	if err := ctx.Err(); err != nil {
		return ProcIdentity{}, err
	}
	return ProcIdentity{CWD: cwd, ExecPath: executable, StartTicks: ticks}, nil
}

func canonicalProcLink(readLink func(string) (string, error), path string) (string, error) {
	if readLink != nil {
		target, err := readLink(path)
		if err != nil || !filepath.IsAbs(target) {
			return "", errors.New("proc link is unavailable")
		}
		return filepath.Clean(target), nil
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || !filepath.IsAbs(resolved) {
		return "", errors.New("proc link is unavailable")
	}
	return filepath.Clean(resolved), nil
}

func readBoundedProcFile(path string, limit int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil || len(data) > limit {
		return nil, errors.New("proc file is invalid")
	}
	return data, nil
}

func ParseProcStartTicks(data []byte) (uint64, error) {
	value := string(data)
	end := strings.LastIndex(value, ")")
	if end < 0 {
		return 0, errors.New("proc stat is invalid")
	}
	fields := strings.Fields(value[end+1:])
	if len(fields) <= 19 || len(fields[0]) != 1 || !strings.ContainsRune("RSDZTWtXxKPI", rune(fields[0][0])) {
		return 0, errors.New("proc stat is incomplete")
	}
	ticks, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil || ticks == 0 {
		return 0, errors.New("proc start ticks are invalid")
	}
	return ticks, nil
}
