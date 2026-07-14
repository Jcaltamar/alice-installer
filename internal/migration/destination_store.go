package migration

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

var (
	ErrDestinationUnsafe  = errors.New("backup destination is unsafe")
	ErrDestinationSpace   = errors.New("backup destination has insufficient free space")
	ErrDestinationLocked  = errors.New("backup destination is already in use")
	ErrDestinationFailure = errors.New("backup destination failed")
)

// SpaceChecker reports currently available bytes. This is only a conservative
// preflight check; writes still handle ENOSPC as a destination failure.
type SpaceChecker interface{ AvailableBytes(string) (uint64, error) }

// PrivilegeRunner is the repository command-runner seam used to request elevation.
type PrivilegeRunner interface {
	Run(context.Context, string, ...string) ([]byte, []byte, error)
}

type DestinationRequest struct {
	Directory   string
	SourceRoots []string
}

type DestinationPlan struct {
	directory string
	sources   []string
}

func (p DestinationPlan) Directory() string { return p.directory }

type DestinationStore interface {
	Preflight(context.Context, DestinationRequest) (DestinationPlan, error)
	Prepare(context.Context, DestinationPlan) (StagedArtifact, error)
}

type StagedArtifact interface {
	io.Writer
	Path() string
	Sync() error
	Close() error
	Cleanup() error
}

type OSDestinationStore struct {
	Space            SpaceChecker
	MinimumFreeBytes uint64
	Privilege        PrivilegeRunner
}

func (s OSDestinationStore) Preflight(ctx context.Context, request DestinationRequest) (DestinationPlan, error) {
	if ctx.Err() != nil || request.Directory == "" {
		return DestinationPlan{}, ErrDestinationUnsafe
	}
	directory, err := filepath.Abs(filepath.Clean(request.Directory))
	if err != nil || isSourcePath(directory, request.SourceRoots) || !safeExistingPath(directory) {
		return DestinationPlan{}, ErrDestinationUnsafe
	}
	if s.Space != nil {
		available, err := s.Space.AvailableBytes(nearestExisting(directory))
		if err != nil || available < s.MinimumFreeBytes {
			return DestinationPlan{}, ErrDestinationSpace
		}
	}
	return DestinationPlan{directory: directory, sources: append([]string(nil), request.SourceRoots...)}, nil
}

func (s OSDestinationStore) Prepare(ctx context.Context, plan DestinationPlan) (StagedArtifact, error) {
	if ctx.Err() != nil || plan.directory == "" || isSourcePath(plan.directory, plan.sources) || !safeExistingPath(plan.directory) {
		return nil, ErrDestinationUnsafe
	}
	if _, err := os.Lstat(plan.directory); os.IsNotExist(err) {
		if err := os.Mkdir(plan.directory, 0o700); err != nil {
			if s.Privilege == nil || s.elevatedPrepare(ctx, plan.directory) != nil {
				return nil, ErrDestinationFailure
			}
		}
		if err := os.Chmod(plan.directory, 0o700); err != nil {
			return nil, ErrDestinationFailure
		}
	} else if err != nil {
		return nil, ErrDestinationUnsafe
	}
	if !safeDirectory(plan.directory) {
		return nil, ErrDestinationUnsafe
	}
	if s.Space != nil {
		available, err := s.Space.AvailableBytes(plan.directory)
		if err != nil || available < s.MinimumFreeBytes {
			return nil, ErrDestinationSpace
		}
	}
	lock, err := os.OpenFile(filepath.Join(plan.directory, ".alice-installer-backup.lock"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		return nil, ErrDestinationLocked
	}
	if err != nil {
		return nil, ErrDestinationFailure
	}
	if err := lock.Close(); err != nil {
		_ = os.Remove(lock.Name())
		return nil, ErrDestinationFailure
	}
	for attempts := 0; attempts < 8; attempts++ {
		name, err := stagingName()
		if err != nil {
			break
		}
		path := filepath.Join(plan.directory, name)
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			_ = os.Remove(filepath.Join(plan.directory, ".alice-installer-backup.lock"))
			return nil, ErrDestinationFailure
		}
		if err := file.Chmod(0o600); err != nil {
			_ = file.Close()
			_ = os.Remove(path)
			_ = os.Remove(filepath.Join(plan.directory, ".alice-installer-backup.lock"))
			return nil, ErrDestinationFailure
		}
		return &osStagedArtifact{file: file, path: path, lockPath: filepath.Join(plan.directory, ".alice-installer-backup.lock")}, nil
	}
	_ = os.Remove(filepath.Join(plan.directory, ".alice-installer-backup.lock"))
	return nil, ErrDestinationFailure
}

type osStagedArtifact struct {
	file     *os.File
	path     string
	lockPath string
	closed   bool
}

func (a *osStagedArtifact) Write(p []byte) (int, error) { return a.file.Write(p) }
func (a *osStagedArtifact) Path() string                { return a.path }
func (a *osStagedArtifact) Sync() error                 { return a.file.Sync() }
func (a *osStagedArtifact) Close() error {
	if a.closed {
		return nil
	}
	a.closed = true
	if err := a.file.Close(); err != nil {
		return err
	}
	if err := os.Remove(a.lockPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
func (a *osStagedArtifact) Cleanup() error {
	_ = a.Close()
	err := os.Remove(a.path)
	if os.IsNotExist(err) {
		err = nil
	}
	lockErr := os.Remove(a.lockPath)
	if os.IsNotExist(lockErr) {
		lockErr = nil
	}
	if err != nil {
		return err
	}
	return lockErr
}

func (s OSDestinationStore) elevatedPrepare(ctx context.Context, directory string) error {
	uid, gid := fmt.Sprintf("%d", os.Geteuid()), fmt.Sprintf("%d", os.Getegid())
	commands := [][]string{
		{"mkdir", "-p", directory},
		{"chown", "-R", uid + ":" + gid, directory},
		{"chmod", "700", directory},
	}
	for _, args := range commands {
		if _, _, err := s.Privilege.Run(ctx, "sudo", args...); err != nil {
			return err
		}
	}
	return nil
}

func stagingName() (string, error) {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return fmt.Sprintf(".alice-backup-%x.dump.part", bytes), nil
}

func isSourcePath(destination string, roots []string) bool {
	for _, root := range roots {
		if root == "" {
			continue
		}
		absolute, err := filepath.Abs(filepath.Clean(root))
		if err != nil {
			return true
		}
		rel, err := filepath.Rel(absolute, destination)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func safeExistingPath(path string) bool {
	root := filepath.VolumeName(path) + string(os.PathSeparator)
	current := root
	for _, component := range strings.Split(strings.TrimPrefix(path, root), string(os.PathSeparator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			parent := filepath.Dir(current)
			if safeDirectory(parent) {
				return true
			}
			parentInfo, parentErr := os.Lstat(parent)
			return parentErr == nil && safeRootOwnedAncestor(parentInfo)
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return false
		}
		// The system temporary root may be sticky and shared; every operator-selected
		// component below it must still be private and owned by the invoking user.
		if filepath.Clean(current) == filepath.Clean(os.TempDir()) && info.Mode()&os.ModeSticky != 0 {
			continue
		}
		if !safeDirectory(current) && !safeRootOwnedAncestor(info) {
			return false
		}
	}
	return true
}

func safeRootOwnedAncestor(info os.FileInfo) bool {
	if runtime.GOOS != "linux" || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o022 != 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == 0
}

func safeDirectory(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return false
	}
	if runtime.GOOS == "linux" {
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || int(stat.Uid) != os.Geteuid() {
			return false
		}
	}
	return true
}

func nearestExisting(path string) string {
	for {
		if _, err := os.Lstat(path); err == nil {
			return path
		}
		parent := filepath.Dir(path)
		if parent == path {
			return path
		}
		path = parent
	}
}
