package installation

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
)

// RootPM2Boundary is the migration-only privileged observation and mutation boundary.
type RootPM2Boundary struct{ Runner CommandRunner }

func (b RootPM2Boundary) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	if b.Runner == nil || !allowedRootObservation(name, args) {
		return nil, nil, errors.New("root PM2 command rejected")
	}
	return b.Runner.Run(ctx, "sudo", append([]string{"-n", name}, args...)...)
}

func allowedRootObservation(name string, args []string) bool {
	return name == "pm2" && len(args) == 1 && args[0] == "jlist" ||
		name == "ss" && len(args) == 2 && args[0] == "-H" && args[1] == "-ltnp"
}

func (b RootPM2Boundary) Read(ctx context.Context, pid int) (ProcIdentity, error) {
	if pid <= 0 || b.Runner == nil {
		return ProcIdentity{}, errors.New("root proc read rejected")
	}
	base := "/proc/" + strconv.Itoa(pid) + "/"
	cwd, err := b.read(ctx, "readlink", base+"cwd")
	if err != nil {
		return ProcIdentity{}, errors.New("proc cwd is unavailable")
	}
	exe, err := b.read(ctx, "readlink", base+"exe")
	if err != nil {
		return ProcIdentity{}, errors.New("proc executable is unavailable")
	}
	stat, err := b.read(ctx, "cat", base+"stat")
	if err != nil || len(stat) > defaultProcStatLimit {
		return ProcIdentity{}, errors.New("proc stat is unavailable")
	}
	ticks, err := ParseProcStartTicks(stat)
	if err != nil {
		return ProcIdentity{}, err
	}
	cwdPath, exePath := string(cwd), string(exe)
	if !filepath.IsAbs(cwdPath) || !filepath.IsAbs(exePath) {
		return ProcIdentity{}, errors.New("proc link is unavailable")
	}
	return ProcIdentity{CWD: filepath.Clean(cwdPath), ExecPath: filepath.Clean(exePath), StartTicks: ticks}, nil
}

func (b RootPM2Boundary) read(ctx context.Context, executable, path string) ([]byte, error) {
	if !validProcPath(path) || executable != "readlink" && executable != "cat" || executable == "cat" && !strings.HasSuffix(path, "/stat") {
		return nil, errors.New("root proc read rejected")
	}
	out, _, err := b.Runner.Run(ctx, "sudo", "-n", executable, path)
	return []byte(strings.TrimSpace(string(out))), err
}

func validProcPath(path string) bool {
	parts := strings.Split(path, "/")
	if len(parts) != 4 || parts[0] != "" || parts[1] != "proc" || parts[2] == "" {
		return false
	}
	n, err := strconv.Atoi(parts[2])
	return err == nil && n > 0 && strconv.Itoa(n) == parts[2] && (parts[3] == "cwd" || parts[3] == "exe" || parts[3] == "stat")
}

func (b RootPM2Boundary) mutate(ctx context.Context, action string, identity PM2ProcessIdentity) error {
	if b.Runner == nil || action != "stop" && action != "start" || identity.PMID < 0 || identity.PID <= 0 || identity.Port == 0 || identity.StartTicks == 0 || identity.CWD == "" || identity.ExecPath == "" {
		return errors.New("root PM2 mutation rejected")
	}
	id := strconv.FormatInt(identity.PMID, 10)
	_, _, err := b.Runner.Run(ctx, "sudo", "-n", "pm2", action, id)
	return err
}
