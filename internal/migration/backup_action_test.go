package migration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBackupActionRequiresConfirmationBeforeCreatingDestination(t *testing.T) {
	destination := filepath.Join(secureTempDir(t), "backups")
	action := testBackupAction(t, []byte("custom\x00dump"))
	plan, err := action.Preflight(context.Background(), BackupRequest{Destination: destination, SourceRoots: []string{secureTempDir(t)}})
	if err != nil {
		t.Fatalf("Preflight() error = %v", err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination exists before confirmation: %v", err)
	}

	result := action.Run(context.Background(), plan)
	if result.Outcome != BackupValidated {
		t.Fatalf("Run() outcome = %#v", result)
	}
	if len(result.Stages) != int(BackupStagePublication)+1 {
		t.Fatalf("validated stages = %#v", result.Stages)
	}
	for _, stage := range result.Stages {
		if stage.Status != BackupStagePassed {
			t.Fatalf("validated stage = %#v, want passed", stage)
		}
	}
	for _, path := range []string{result.DumpPath, result.ManifestPath} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("published mode = %v, err = %v", info.Mode(), err)
		}
		if !strings.HasPrefix(path, destination+string(os.PathSeparator)) {
			t.Fatalf("published path = %q, destination = %q", path, destination)
		}
	}
	if _, err := os.Stat(filepath.Join(destination, ".alice-installer-backup.lock")); !os.IsNotExist(err) {
		t.Fatalf("operation lock remains after streaming completes: %v", err)
	}
	if strings.Contains(result.String(), processSecretSentinel) {
		t.Fatal("secret escaped through backup result")
	}
}

func TestBackupActionPublishesOnlyValidatedOutcome(t *testing.T) {
	destination := filepath.Join(secureTempDir(t), "backups")
	action := testBackupAction(t, []byte("custom\x00dump"))
	action.Validator = archiveValidatorFunc(func(context.Context, string) error { return nil })
	plan, err := action.Preflight(context.Background(), BackupRequest{Destination: destination, SourceRoots: []string{secureTempDir(t)}})
	if err != nil {
		t.Fatal(err)
	}
	result := action.Run(context.Background(), plan)
	if result.Outcome != BackupValidated || result.DumpPath == "" || result.ManifestPath == "" || result.SHA256 == "" || result.Size == 0 {
		t.Fatalf("validated result = %#v", result)
	}
	if strings.Contains(result.String(), processSecretSentinel) {
		t.Fatal("secret escaped through validated result")
	}
	for _, path := range []string{result.DumpPath, result.ManifestPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("published path %q unavailable: %v", path, err)
		}
	}
}

func TestBackupActionValidationFailureRemovesStaging(t *testing.T) {
	destination := filepath.Join(secureTempDir(t), "backups")
	action := testBackupAction(t, []byte("custom\x00dump"))
	action.Validator = archiveValidatorFunc(func(context.Context, string) error { return ErrArchiveValidation })
	plan, err := action.Preflight(context.Background(), BackupRequest{Destination: destination, SourceRoots: []string{secureTempDir(t)}})
	if err != nil {
		t.Fatal(err)
	}
	result := action.Run(context.Background(), plan)
	if result.Outcome != BackupValidationFailed || result.DumpPath != "" || result.ManifestPath != "" || result.SHA256 != "" || result.Size != 0 {
		t.Fatalf("validation failure result = %#v", result)
	}
	if result.FailureCode != BackupFailureArchiveValidation || result.Remediation != BackupRemediationArchive {
		t.Fatalf("validation diagnostics = %#v", result)
	}
	for _, stage := range result.Stages {
		want := BackupStagePassed
		if stage.Stage == BackupStageArchiveValidation {
			want = BackupStageFailed
		} else if stage.Stage > BackupStageArchiveValidation {
			want = BackupStageNotRun
		}
		if stage.Status != want {
			t.Fatalf("validation stage = %#v, want status %v", stage, want)
		}
	}
	entries, err := os.ReadDir(destination)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("validation failure retained artifacts: %#v", entries)
	}
}

func TestDestinationStoreAllowsSecureRootOwnedParentsForDefaultDestination(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux ownership policy")
	}
	const destination = "/opt/alice/backups/"
	parent := nearestExisting(destination)
	parentInfo, err := os.Lstat(parent)
	if err != nil {
		t.Fatalf("default destination parent stat error = %v", err)
	}
	if !safeDirectory(parent) && !safeRootOwnedAncestor(parentInfo) {
		t.Skipf("host %s is not a secure root-owned ancestor", parent)
	}
	store := OSDestinationStore{}
	plan, err := store.Preflight(context.Background(), DestinationRequest{Directory: destination})
	if err != nil {
		t.Fatalf("default destination preflight error = %v", err)
	}
	if plan.Directory() != "/opt/alice/backups" {
		t.Fatalf("default destination = %q", plan.Directory())
	}
}

func TestDestinationStoreExplicitlyElevatesWhenDestinationCreationNeedsPrivileges(t *testing.T) {
	root := secureTempDir(t)
	destination := filepath.Join(root, "alice", "backups")
	runner := &recordingPrivilegeRunner{run: func(name string, args ...string) error {
		if name != "sudo" {
			t.Fatalf("privilege command = %q, want sudo", name)
		}
		if len(args) == 3 && args[0] == "mkdir" && args[1] == "-p" {
			return os.MkdirAll(args[2], 0o700)
		}
		return nil
	}}
	store := OSDestinationStore{Privilege: runner}
	plan, err := store.Preflight(context.Background(), DestinationRequest{Directory: destination})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := store.Prepare(context.Background(), plan)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	defer artifact.Cleanup()
	if len(runner.calls) == 0 || runner.calls[0] != "sudo mkdir -p "+destination {
		t.Fatalf("elevation calls = %v", runner.calls)
	}
	info, err := os.Stat(destination)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("destination info = %v, err = %v", info, err)
	}
}

type recordingPrivilegeRunner struct {
	calls []string
	run   func(string, ...string) error
}

func (r *recordingPrivilegeRunner) Run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, strings.Join(append([]string{name}, args...), " "))
	if r.run != nil {
		return nil, nil, r.run(name, args...)
	}
	return nil, nil, nil
}

func TestDestinationStoreFailsClosedForUnsafePathsAndPreventsOverwrite(t *testing.T) {
	root := secureTempDir(t)
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(source, link); err != nil {
		t.Fatal(err)
	}
	insecure := filepath.Join(root, "insecure")
	if err := os.Mkdir(insecure, 0o755); err != nil {
		t.Fatal(err)
	}
	store := OSDestinationStore{Space: fixedSpace{available: 1 << 30}, MinimumFreeBytes: 1}
	for _, destination := range []string{source, filepath.Join(source, "nested"), link, filepath.Join(insecure, "backup")} {
		if _, err := store.Preflight(context.Background(), DestinationRequest{Directory: destination, SourceRoots: []string{source}}); !errors.Is(err, ErrDestinationUnsafe) {
			t.Fatalf("Preflight(%q) error = %v, want ErrDestinationUnsafe", destination, err)
		}
	}

	destination := filepath.Join(root, "safe")
	plan, err := store.Preflight(context.Background(), DestinationRequest{Directory: destination, SourceRoots: []string{source}})
	if err != nil {
		t.Fatal(err)
	}
	staged, err := store.Prepare(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	first := staged.Path()
	if err := staged.Close(); err != nil {
		t.Fatal(err)
	}
	if err := staged.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, []byte("operator-owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := store.Prepare(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Cleanup()
	if second.Path() == first {
		t.Fatal("staging path overwrote an existing artifact")
	}
}

func TestBackupActionCleansPartialStagingAndLockOnFailureCancellationAndTimeout(t *testing.T) {
	for _, tt := range []struct {
		name     string
		executor *backupFakeExecutor
		cancel   bool
		outcome  BackupOutcome
	}{
		{"dump failure", &backupFakeExecutor{result: ProcessResult{Outcome: ProcessFailed, StderrCode: "safe"}}, false, BackupDumpFailed},
		{"timeout", &backupFakeExecutor{result: ProcessResult{Outcome: ProcessTimedOut, StderrCode: "safe"}}, false, BackupTimedOut},
		{"empty successful dump", &backupFakeExecutor{result: ProcessResult{Outcome: ProcessSucceeded}}, false, BackupDumpFailed},
		{"cancelled before run", &backupFakeExecutor{result: ProcessResult{Outcome: ProcessSucceeded}}, true, BackupCancelled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			destination := filepath.Join(secureTempDir(t), "backups")
			action := testBackupAction(t, []byte("partial"))
			action.Executor = tt.executor
			plan, err := action.Preflight(context.Background(), BackupRequest{Destination: destination, SourceRoots: []string{secureTempDir(t)}})
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if tt.cancel {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			result := action.Run(ctx, plan)
			if result.Outcome != tt.outcome {
				t.Fatalf("Run() = %#v, want %v", result, tt.outcome)
			}
			entries, err := os.ReadDir(destination)
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("partial artifacts remain: %#v", entries)
			}
		})
	}
}

func TestDestinationStoreChecksSpaceAndRejectsConcurrentLock(t *testing.T) {
	destination := filepath.Join(secureTempDir(t), "backups")
	store := OSDestinationStore{Space: fixedSpace{available: 0}, MinimumFreeBytes: 1}
	if _, err := store.Preflight(context.Background(), DestinationRequest{Directory: destination}); !errors.Is(err, ErrDestinationSpace) {
		t.Fatalf("space preflight error = %v", err)
	}

	store = OSDestinationStore{Space: fixedSpace{available: 1 << 30}, MinimumFreeBytes: 1}
	plan, err := store.Preflight(context.Background(), DestinationRequest{Directory: destination})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Prepare(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Cleanup()
	if _, err := store.Prepare(context.Background(), plan); !errors.Is(err, ErrDestinationLocked) {
		t.Fatalf("second prepare error = %v, want ErrDestinationLocked", err)
	}
}

type fixedSpace struct{ available uint64 }

func (s fixedSpace) AvailableBytes(string) (uint64, error) { return s.available, nil }

type backupFakeExecutor struct {
	result ProcessResult
	output []byte
}

func (e *backupFakeExecutor) Run(_ context.Context, spec ProcessSpec, stdout io.Writer) ProcessResult {
	if len(spec.Args) > 0 && spec.Args[0] == "rm" {
		return ProcessResult{Outcome: ProcessSucceeded}
	}
	_, _ = stdout.Write(e.output)
	return e.result
}

func secureTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(os.TempDir(), "alice-migration-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func testBackupAction(t *testing.T, output []byte) BackupAction {
	t.Helper()
	config := testProcessConfig(processSecretSentinel)
	id := strings.Repeat("a", 64)
	return BackupAction{
		Resolver:  staticBackupResolver{config: config},
		Inspector: staticBackupInspector{identity: ContainerIdentity{ID: id, Image: PostgreSQL11Image}},
		Store:     OSDestinationStore{Space: fixedSpace{available: 1 << 30}, MinimumFreeBytes: 1},
		Executor:  &backupFakeExecutor{result: ProcessResult{Outcome: ProcessSucceeded}, output: output},
		Transport: CredentialTransport{TempRoot: t.TempDir()},
		Validator: archiveValidatorFunc(func(context.Context, string) error { return nil }),
		GOOS:      "linux",
		Timeout:   time.Second,
	}
}

type archiveValidatorFunc func(context.Context, string) error

func (f archiveValidatorFunc) Validate(ctx context.Context, path string) error { return f(ctx, path) }

type staticBackupResolver struct{ config ResolvedConfig }

func (r staticBackupResolver) Resolve(context.Context, ConfigRequest) (ResolvedConfig, error) {
	return r.config, nil
}

type staticBackupInspector struct{ identity ContainerIdentity }

func (i staticBackupInspector) Candidates(context.Context, ImageIdentity) ([]ContainerSummary, error) {
	return nil, nil
}
func (i staticBackupInspector) Inspect(context.Context, string) (ContainerDetails, error) {
	return ContainerDetails{}, errors.New("not used")
}
func (i staticBackupInspector) Discover(context.Context, ResolvedConfig) (ContainerIdentity, error) {
	return i.identity, nil
}

func TestBackupPlanReviewIsRedactedAndImmutable(t *testing.T) {
	secretSentinel := "synthetic-secret-review-only"
	plan := BackupPlan{
		config:      ResolvedConfig{Environment: EnvironmentProduction, Host: "db.internal", Port: 5432, Database: "alice", Username: "operator", password: newSecret(secretSentinel)},
		container:   ContainerIdentity{ID: strings.Repeat("b", 64), Image: PostgreSQL11Image},
		destination: DestinationPlan{directory: "/safe/backups"},
	}
	review := plan.Review()
	if review.Environment != EnvironmentProduction || review.Endpoint != "db.internal:5432" || review.Database != "alice" || review.User != "operator" || review.ContainerID != strings.Repeat("b", 64) || review.Image != PostgreSQL11Image || review.Destination != "/safe/backups" {
		t.Fatalf("review = %#v", review)
	}
	if strings.Contains(fmt.Sprintf("%#v", review), secretSentinel) {
		t.Fatalf("review leaked secret: %#v", review)
	}
}
