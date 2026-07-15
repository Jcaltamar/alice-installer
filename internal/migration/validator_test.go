package migration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const validationSecretSentinel = "synthetic-secret-validation-boundary"

func TestPG11ArchiveValidatorUsesPinnedRestoreListWithoutDatabaseConnection(t *testing.T) {
	dump := filepath.Join(t.TempDir(), "staged.dump.part")
	if err := os.WriteFile(dump, []byte("custom dump"), 0o600); err != nil {
		t.Fatal(err)
	}
	executor := &validationFakeExecutor{listing: "; Archive created at 2026-07-11\n1; 0 0 TABLE public alice\n"}
	validator := PG11ArchiveValidator{Executor: executor, Timeout: time.Second}

	if err := validator.Validate(context.Background(), dump); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if len(executor.specs) != 2 {
		t.Fatalf("executor calls = %#v, want validation plus named cleanup", executor.specs)
	}
	spec := executor.specs[0]
	if spec.Name != "docker" || containsShell(spec.Args) || strings.Contains(strings.Join(spec.Args, " "), validationSecretSentinel) {
		t.Fatalf("unsafe validator spec: %#v", spec)
	}
	info, err := os.Lstat(dump)
	if err != nil {
		t.Fatal(err)
	}
	uid, gid, ok := numericOwner(info)
	if !ok {
		t.Fatal("numeric owner unavailable")
	}
	wantArgs := []string{
		"run", "--rm", "--pull=never", "--name", spec.Args[4],
		"--mount", "type=bind,src=" + dump + ",dst=" + ContainerDumpPath + ",readonly",
		"--user", fmt.Sprintf("%d:%d", uid, gid),
		string(PostgreSQL11Image), "pg_restore", "--list", ContainerDumpPath,
	}
	if strings.Join(spec.Args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("validator argv = %#v, want %#v", spec.Args, wantArgs)
	}
	if strings.Contains(strings.Join(spec.Args, " "), "--host") || strings.Contains(strings.Join(spec.Args, " "), "--dbname") || strings.Contains(strings.Join(spec.Args, " "), "PGPASS") {
		t.Fatalf("validator may not construct a database connection: %#v", spec.Args)
	}
}

func TestSafeStagedDumpRejectsMismatchedOwnership(t *testing.T) {
	dump := filepath.Join(t.TempDir(), "staged.dump.part")
	if err := os.WriteFile(dump, []byte("custom dump"), 0o600); err != nil {
		t.Fatal(err)
	}
	uid, gid, ok := stagedDumpOwner(dump)
	if !ok {
		t.Fatal("staged owner unavailable")
	}
	if safeOwnedStagedDump(dump, uid+1, gid) || safeOwnedStagedDump(dump, uid, gid+1) {
		t.Fatal("staged dump accepted mismatched ownership")
	}
}

func TestPG11ArchiveValidatorRejectsUnsafePathAndMode(t *testing.T) {
	dir := t.TempDir()
	dump := filepath.Join(dir, "staged.dump.part")
	if err := os.WriteFile(dump, []byte("custom dump"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(dir, "staged-link")
	if err := os.Symlink(dump, symlink); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		path string
		mode os.FileMode
	}{
		{name: "relative path", path: filepath.Base(dump), mode: 0o600},
		{name: "symlink", path: symlink, mode: 0o600},
		{name: "group readable", path: dump, mode: 0o640},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.path == dump {
				if err := os.Chmod(dump, tt.mode); err != nil {
					t.Fatal(err)
				}
			}
			executor := &validationFakeExecutor{}
			if err := (PG11ArchiveValidator{Executor: executor}).Validate(context.Background(), tt.path); !errors.Is(err, ErrArchiveValidation) {
				t.Fatalf("Validate() error = %v, want ErrArchiveValidation", err)
			}
			if len(executor.specs) != 0 {
				t.Fatalf("unsafe staged dump executed Docker: %#v", executor.specs)
			}
		})
	}
}

func TestPG11ArchiveValidatorAcceptsDockerAutoRemovalWithRealExecutor(t *testing.T) {
	dump := filepath.Join(t.TempDir(), "staged.dump.part")
	if err := os.WriteFile(dump, []byte("custom dump"), 0o600); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	docker := "#!/bin/sh\n" +
		"if [ \"$1\" = run ]; then printf '; Archive created\\n1; 0 0 TABLE public alice\\n'; exit 0; fi\n" +
		"if [ \"$1\" = rm ]; then echo 'Error response from daemon: No such container: auto-removed' >&2; exit 1; fi\n" +
		"exit 2\n"
	if err := os.WriteFile(dockerPath, []byte(docker), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	validator := PG11ArchiveValidator{Executor: OSBinaryExecutor{}, Timeout: time.Second}
	if err := validator.Validate(context.Background(), dump); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestPG11ArchiveValidatorRejectsEmptyMalformedAndFailedListings(t *testing.T) {
	dump := filepath.Join(t.TempDir(), "staged.dump.part")
	if err := os.WriteFile(dump, []byte("custom dump"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name    string
		result  ProcessResult
		listing string
	}{
		{name: "empty", result: ProcessResult{Outcome: ProcessSucceeded}},
		{name: "malformed", result: ProcessResult{Outcome: ProcessSucceeded}, listing: "not an archive listing\n"},
		{name: "failed", result: ProcessResult{Outcome: ProcessFailed, StderrCode: validationSecretSentinel}, listing: "; Archive created\n1; 0 0 TABLE public alice\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			validator := PG11ArchiveValidator{Executor: &validationFakeExecutor{result: tt.result, listing: tt.listing}}
			if err := validator.Validate(context.Background(), dump); !errors.Is(err, ErrArchiveValidation) {
				t.Fatalf("Validate() error = %v, want ErrArchiveValidation", err)
			}
		})
	}
}

type validationFakeExecutor struct {
	result  ProcessResult
	listing string
	cleanup ProcessResult
	specs   []ProcessSpec
}

func (e *validationFakeExecutor) Run(_ context.Context, spec ProcessSpec, stdout io.Writer) ProcessResult {
	e.specs = append(e.specs, spec)
	if len(spec.Args) > 0 && spec.Args[0] == "rm" {
		if e.cleanup.Outcome != ProcessSucceeded {
			return e.cleanup
		}
		return ProcessResult{Outcome: ProcessSucceeded}
	}
	_, _ = io.WriteString(stdout, e.listing)
	if e.result.Outcome == ProcessSucceeded && e.result.StderrCode == "" {
		return ProcessResult{Outcome: ProcessSucceeded}
	}
	return e.result
}

func TestPG11ArchiveValidatorFailsClosedOnCancellationOverflowAndCleanupFailure(t *testing.T) {
	dump := filepath.Join(t.TempDir(), "staged.dump.part")
	if err := os.WriteFile(dump, []byte("custom dump"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		ctx      context.Context
		executor *validationFakeExecutor
	}{
		{name: "cancelled", ctx: cancelledValidationContext(), executor: &validationFakeExecutor{}},
		{name: "oversized listing", ctx: context.Background(), executor: &validationFakeExecutor{listing: strings.Repeat("x", 64<<10+1)}},
		{name: "cleanup failure", ctx: context.Background(), executor: &validationFakeExecutor{listing: "; Archive created\n1; 0 0 TABLE public alice\n", cleanup: ProcessResult{Outcome: ProcessFailed}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := PG11ArchiveValidator{Executor: tt.executor}
			if err := validator.Validate(tt.ctx, dump); !errors.Is(err, ErrArchiveValidation) {
				t.Fatalf("Validate() error = %v, want ErrArchiveValidation", err)
			}
		})
	}
}

func cancelledValidationContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}
