package migration

import (
	"bytes"
	"context"
	"errors"
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
	if ctx.Err() != nil || v.Executor == nil || !safeStagedDump(stagedPath) {
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
