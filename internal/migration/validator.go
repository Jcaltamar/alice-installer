package migration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const ContainerDumpPath = "/run/alice-installer/dump"

var ErrArchiveValidation = errors.New("backup archive validation failed")

// ArchiveValidator validates a staged custom archive without permitting a database connection.
type ArchiveValidator interface {
	Validate(context.Context, string) error
}

// PG11ArchiveValidator runs the pinned PostgreSQL 11 pg_restore client in an
// ephemeral helper container. It mounts only the staged dump read-only.
type PG11ArchiveValidator struct {
	Executor BinaryExecutor
	Timeout  time.Duration
}

func (v PG11ArchiveValidator) Validate(ctx context.Context, stagedPath string) error {
	uid, gid, ok := stagedDumpOwner(stagedPath)
	if ctx.Err() != nil || v.Executor == nil || !ok || !safeOwnedStagedDump(stagedPath, uid, gid) {
		return ErrArchiveValidation
	}
	name, err := randomToken()
	if err != nil {
		return ErrArchiveValidation
	}
	timeout := v.Timeout
	if timeout <= 0 {
		timeout = defaultProcessTimeout
	}
	run := HelperRun{
		Name: "alice-pg11-validate-" + name,
		Spec: ProcessSpec{Name: "docker", Args: []string{
			"run", "--rm", "--pull=never", "--name", "alice-pg11-validate-" + name,
			"--mount", "type=bind,src=" + stagedPath + ",dst=" + ContainerDumpPath + ",readonly",
			"--user", fmt.Sprintf("%d:%d", uid, gid),
			string(PostgreSQL11Image), "pg_restore", "--list", ContainerDumpPath,
		}, Timeout: timeout},
	}
	var listing boundedListing
	result := v.Executor.Run(ctx, run.Spec, &listing)
	cleanupCtx, cancel := context.WithTimeout(context.Background(), run.CleanupSpec().Timeout)
	defer cancel()
	if CleanupHelper(cleanupCtx, v.Executor, run) != nil || result.Outcome != ProcessSucceeded || !listing.valid() || ctx.Err() != nil {
		return ErrArchiveValidation
	}
	return nil
}

func stagedDumpOwner(path string) (int, int, bool) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return 0, 0, false
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 || info.Size() <= 0 {
		return 0, 0, false
	}
	return numericOwner(info)
}

func safeOwnedStagedDump(path string, uid, gid int) bool {
	if uid < 0 || gid < 0 || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 && info.Mode().Perm() == 0o600 && info.Size() > 0 && ownedBy(info, uid, gid)
}

func safeStagedDump(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 && info.Size() > 0
}

type boundedListing struct{ bytes.Buffer }

func (b *boundedListing) Write(p []byte) (int, error) {
	const limit = 64 << 10
	if b.Len()+len(p) > limit {
		return 0, io.ErrShortWrite
	}
	return b.Buffer.Write(p)
}

func (b *boundedListing) valid() bool {
	lines := strings.Split(b.String(), "\n")
	header, entry := false, false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "; Archive") {
			header = true
		}
		if len(line) > 2 && line[0] >= '0' && line[0] <= '9' && strings.Contains(line, ";") {
			entry = true
		}
	}
	return header && entry
}
