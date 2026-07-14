package migration

import (
	"context"
	"errors"
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
	assertArgsContainInOrder(t, spec.Args,
		"run", "--rm", "--pull=never", "--name", spec.Args[4],
		"--mount", "type=bind,src="+dump+",dst="+ContainerDumpPath+",readonly",
		string(PostgreSQL11Image), "pg_restore", "--list", ContainerDumpPath,
	)
	if strings.Contains(strings.Join(spec.Args, " "), "--host") || strings.Contains(strings.Join(spec.Args, " "), "--dbname") || strings.Contains(strings.Join(spec.Args, " "), "PGPASS") {
		t.Fatalf("validator may not construct a database connection: %#v", spec.Args)
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
