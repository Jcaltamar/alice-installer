package migration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jcaltamar/alice-installer/internal/workspace"
)

type recordingArchiveValidator struct {
	paths []string
	err   error
}

func (v *recordingArchiveValidator) Validate(_ context.Context, path string) error {
	v.paths = append(v.paths, path)
	return v.err
}

func TestBackupGateRevalidatesMatchingLegacyPair(t *testing.T) {
	dir := t.TempDir()
	dump := filepath.Join(dir, "legacy.dump")
	writeBackupPair(t, dump, "legacy archive")
	validator := &recordingArchiveValidator{}
	backup, err := revalidateBackupInRoot(context.Background(), validator, backupRefFor(t, dump), dir)
	if err != nil {
		t.Fatalf("Revalidate() error = %v", err)
	}
	if backup.ref.DumpPath != dump || backup.targetRollback {
		t.Fatalf("Revalidate() = %+v, want validated legacy backup", backup)
	}
	if len(validator.paths) != 1 || validator.paths[0] != dump {
		t.Fatalf("validator paths = %v, want [%q]", validator.paths, dump)
	}
}

func TestBackupGateRejectsInvalidLegacyBeforeArchiveValidation(t *testing.T) {
	dir := t.TempDir()
	dump := filepath.Join(dir, "legacy.dump")
	writeBackupPair(t, dump, "legacy archive")
	validator := &recordingArchiveValidator{}
	ref := backupRefFor(t, dump)
	ref.SHA256 = "wrong"

	if _, err := revalidateBackupInRoot(context.Background(), validator, ref, dir); !errors.Is(err, ErrRestoreBackupGate) {
		t.Fatalf("Revalidate() error = %v, want backup gate error", err)
	}
	if len(validator.paths) != 0 {
		t.Fatalf("validator called for invalid pair: %v", validator.paths)
	}
}

func TestTargetRollbackCreatorPublishesDistinctProtectedPair(t *testing.T) {
	dir := t.TempDir()
	validator := &recordingArchiveValidator{}
	var stagedOperationID string
	creator := TargetRollbackBackupCreator{
		Validator:   validator,
		OperationID: func() (string, error) { return "operation-1", nil },
		Stage: func(_ context.Context, _ workspace.TargetDatabaseConfig, destination, operationID string) (string, error) {
			stagedOperationID = operationID
			path := filepath.Join(destination, ".target-rollback-operation-1.part")
			return path, os.WriteFile(path, []byte("target archive"), 0o600)
		},
	}

	backup, err := creator.CreateValidated(context.Background(), workspace.TargetDatabaseConfig{}, dir)
	if err != nil {
		t.Fatalf("CreateValidated() error = %v", err)
	}
	if stagedOperationID != "operation-1" {
		t.Fatalf("stage operation ID = %q", stagedOperationID)
	}
	if !backup.targetRollback || backup.ref.DumpPath == "" || backup.ref.ManifestPath == "" {
		t.Fatalf("CreateValidated() = %+v, want published target rollback backup", backup)
	}
	for _, path := range []string{backup.ref.DumpPath, backup.ref.ManifestPath} {
		info, statErr := os.Stat(path)
		if statErr != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("published %q mode/error = %v/%v, want 0600 and nil", path, info, statErr)
		}
	}
	if len(validator.paths) != 1 || validator.paths[0] == backup.ref.DumpPath {
		t.Fatalf("validator paths = %v, want staging path only", validator.paths)
	}
}

func TestReplacementAllowedRequiresBothDistinctValidatedArtifacts(t *testing.T) {
	dir := t.TempDir()
	legacyDump := filepath.Join(dir, "legacy.dump")
	writeBackupPair(t, legacyDump, "legacy archive")
	legacy, err := revalidateBackupInRoot(context.Background(), &recordingArchiveValidator{}, backupRefFor(t, legacyDump), dir)
	if err != nil {
		t.Fatal(err)
	}
	target := ValidatedBackup{targetRollback: true}
	if ReplacementAllowed(legacy, target) {
		t.Fatal("replacement allowed without validated target rollback backup")
	}
	target.ref = backupRefFor(t, legacyDump)
	if ReplacementAllowed(legacy, target) {
		t.Fatal("replacement allowed when target aliases legacy artifact")
	}
	creator := TargetRollbackBackupCreator{Validator: &recordingArchiveValidator{}, OperationID: func() (string, error) { return "gate", nil }, Stage: func(_ context.Context, _ workspace.TargetDatabaseConfig, d, _ string) (string, error) {
		path := filepath.Join(d, ".target-rollback-gate.part")
		return path, os.WriteFile(path, []byte("target archive"), 0o600)
	}}
	target, err = creator.CreateValidated(context.Background(), workspace.TargetDatabaseConfig{}, dir)
	if err != nil || !ReplacementAllowed(legacy, target) {
		t.Fatalf("validated pair did not open gate: %v", err)
	}
}

func TestTargetRollbackCreatorNeverOverwritesPublishedPair(t *testing.T) {
	dir := t.TempDir()
	creator := TargetRollbackBackupCreator{
		Validator:   &recordingArchiveValidator{},
		OperationID: func() (string, error) { return "operation-3", nil },
		Stage: func(_ context.Context, _ workspace.TargetDatabaseConfig, destination, _ string) (string, error) {
			path := filepath.Join(destination, ".target-rollback-operation-3.part")
			return path, os.WriteFile(path, []byte("target archive"), 0o600)
		},
	}
	first, err := creator.CreateValidated(context.Background(), workspace.TargetDatabaseConfig{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := creator.CreateValidated(context.Background(), workspace.TargetDatabaseConfig{}, dir); !errors.Is(err, ErrRestoreBackupGate) {
		t.Fatalf("second CreateValidated() error = %v, want backup gate error", err)
	}
	if _, err := os.Stat(first.ref.DumpPath); err != nil {
		t.Fatalf("first published dump was overwritten or removed: %v", err)
	}
}

func TestPublishTargetRollbackNeverOverwritesExistingManifest(t *testing.T) {
	dir := t.TempDir()
	staged := filepath.Join(dir, ".target-rollback-operation-4.part")
	if err := os.WriteFile(staged, []byte("target archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	sum, _, err := restoreHash(staged)
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "alice-target-rollback-operation-4-"+sum+".dump.manifest.json")
	if err := os.WriteFile(manifest, []byte("retained"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := publishTargetRollback(staged, dir, "operation-4"); !errors.Is(err, ErrRestoreBackupGate) {
		t.Fatalf("publish error = %v, want backup gate", err)
	}
	data, err := os.ReadFile(manifest)
	if err != nil || string(data) != "retained" {
		t.Fatalf("manifest overwritten: %q / %v", data, err)
	}
}

func TestTargetRollbackCreatorFailurePreservesExistingArtifacts(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "legacy.dump")
	writeBackupPair(t, legacy, "legacy archive")
	creator := TargetRollbackBackupCreator{
		Validator:   &recordingArchiveValidator{err: errors.New("invalid archive")},
		OperationID: func() (string, error) { return "operation-2", nil },
		Stage: func(_ context.Context, _ workspace.TargetDatabaseConfig, destination, _ string) (string, error) {
			path := filepath.Join(destination, ".target-rollback-operation-2.part")
			return path, os.WriteFile(path, []byte("bad archive"), 0o600)
		},
	}

	if _, err := creator.CreateValidated(context.Background(), workspace.TargetDatabaseConfig{}, dir); !errors.Is(err, ErrRestoreBackupGate) {
		t.Fatalf("CreateValidated() error = %v, want backup gate error", err)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("legacy dump was not retained: %v", err)
	}
	if _, err := os.Stat(legacy + ".manifest.json"); err != nil {
		t.Fatalf("legacy manifest was not retained: %v", err)
	}
}

func TestTargetRollbackBackupAdapterUsesProtectedOperationStaging(t *testing.T) {
	adapter := TargetRollbackBackupAdapter{
		Validator:   &recordingArchiveValidator{},
		Executor:    &backupExecutor{},
		Credentials: CredentialTransport{TempRoot: t.TempDir()},
		OperationID: func() (string, error) { return "operation-bridge-1", nil },
	}
	cfg := targetRestoreConfig(t)
	if _, err := adapter.CreateValidated(context.Background(), cfg, t.TempDir()); !errors.Is(err, ErrRestoreBackupGate) {
		t.Fatalf("caller-controlled root error = %v, want backup gate", err)
	}
}

type backupExecutor struct{ specs []ProcessSpec }

func (e *backupExecutor) Run(_ context.Context, spec ProcessSpec, stdout io.Writer) ProcessResult {
	e.specs = append(e.specs, spec)
	_, _ = io.WriteString(stdout, "target archive")
	return ProcessResult{Outcome: ProcessSucceeded}
}

func TestTargetRollbackBackupAdapterStagesSecretFreeCustomDump(t *testing.T) {
	executor := &backupExecutor{}
	adapter := TargetRollbackBackupAdapter{Executor: executor, Credentials: CredentialTransport{TempRoot: t.TempDir()}}
	staged, err := adapter.stage(context.Background(), targetRestoreConfig(t), t.TempDir(), "operation-bridge-2")
	if err != nil || filepath.Base(staged) != ".target-rollback-operation-bridge-2.part" {
		t.Fatalf("stage = %q, err = %v", staged, err)
	}
	info, err := os.Stat(staged)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("staged mode/error = %v/%v", info, err)
	}
	joined := strings.Join(executor.specs[0].Args, " ")
	if !strings.Contains(joined, "pg_dump --format=custom") || strings.Contains(joined, replacementSecret) || strings.Contains(joined, "PGPASSWORD") {
		t.Fatalf("unsafe backup argv = %q", joined)
	}
}

func TestBackupGateUsesImmutableProductionRoot(t *testing.T) {
	gate := BackupGate{Validator: &recordingArchiveValidator{}}
	if _, err := gate.Revalidate(context.Background(), BackupRef{}); !errors.Is(err, ErrRestoreBackupGate) {
		t.Fatalf("Revalidate() error = %v", err)
	}
	if authoritativeBackupRoot() != defaultBackupRoot {
		t.Fatalf("authoritative root = %q, want %q", authoritativeBackupRoot(), defaultBackupRoot)
	}
}

func TestRevalidateBackupInRootRejectsEscapesAndSwaps(t *testing.T) {
	root := filepath.Join(t.TempDir(), "backups")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	good := filepath.Join(root, "nested", "legacy.dump")
	if err := os.Mkdir(filepath.Dir(good), 0o700); err != nil {
		t.Fatal(err)
	}
	writeBackupPair(t, good, "legacy archive")
	outside := filepath.Join(filepath.Dir(root), "sibling.dump")
	writeBackupPair(t, outside, "outside archive")
	for name, ref := range map[string]BackupRef{
		"sibling":   backupRefFor(t, outside),
		"traversal": backupRefFor(t, filepath.Join(root, "..", "sibling.dump")),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := revalidateBackupInRoot(context.Background(), &recordingArchiveValidator{}, ref, root); !errors.Is(err, ErrRestoreBackupGate) {
				t.Fatalf("Revalidate() error = %v", err)
			}
		})
	}
	finalLink := filepath.Join(root, "linked.dump")
	if err := os.Symlink(outside, finalLink); err != nil {
		t.Fatal(err)
	}
	linked := backupRefFor(t, outside)
	linked.DumpPath, linked.ManifestPath = finalLink, finalLink+".manifest.json"
	if _, err := revalidateBackupInRoot(context.Background(), &recordingArchiveValidator{}, linked, root); !errors.Is(err, ErrRestoreBackupGate) {
		t.Fatalf("final symlink escape error = %v", err)
	}
	intermediateLink := filepath.Join(root, "linked-manifest")
	if err := os.Symlink(filepath.Dir(outside), intermediateLink); err != nil {
		t.Fatal(err)
	}
	linked.DumpPath, linked.ManifestPath = filepath.Join(intermediateLink, filepath.Base(outside)), filepath.Join(intermediateLink, filepath.Base(outside)+".manifest.json")
	if _, err := revalidateBackupInRoot(context.Background(), &recordingArchiveValidator{}, linked, root); !errors.Is(err, ErrRestoreBackupGate) {
		t.Fatalf("intermediate symlink escape error = %v", err)
	}
	validator := &swapArchiveValidator{swap: func() {
		_ = os.RemoveAll(filepath.Dir(good))
		_ = os.Symlink(filepath.Dir(outside), filepath.Dir(good))
	}}
	if _, err := revalidateBackupInRoot(context.Background(), validator, backupRefFor(t, good), root); !errors.Is(err, ErrRestoreBackupGate) {
		t.Fatalf("intermediate path swap error = %v", err)
	}
}

type swapArchiveValidator struct{ swap func() }

func (v *swapArchiveValidator) Validate(_ context.Context, _ string) error { v.swap(); return nil }

func writeBackupPair(t *testing.T, dump, content string) {
	t.Helper()
	if err := os.WriteFile(dump, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	ref := backupRefFor(t, dump)
	manifest := `{"format":"postgresql-custom","validation":"validated","sha256":"` + ref.SHA256 + `","byte_size":` + stringInt(ref.Size) + `}`
	if err := os.WriteFile(dump+".manifest.json", []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
}

func backupRefFor(t *testing.T, dump string) BackupRef {
	t.Helper()
	data, err := os.ReadFile(dump)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return BackupRef{DumpPath: dump, ManifestPath: dump + ".manifest.json", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(data))}
}

func TestTargetRollbackCreatorRejectsUnsafeOperationIDsBeforeStaging(t *testing.T) {
	for _, operationID := range []string{"", "../escape", "nested/id", `nested\\id`, "/absolute", "sibling..", strings.Repeat("a", 65), "unsafe.id"} {
		stageCalls := 0
		creator := TargetRollbackBackupCreator{Validator: &recordingArchiveValidator{}, OperationID: func() (string, error) { return operationID, nil }, Stage: func(context.Context, workspace.TargetDatabaseConfig, string, string) (string, error) {
			stageCalls++
			return "", nil
		}}
		if _, err := creator.CreateValidated(context.Background(), workspace.TargetDatabaseConfig{}, t.TempDir()); !errors.Is(err, ErrRestoreBackupGate) || stageCalls != 0 {
			t.Fatalf("operation ID %q error/calls = %v/%d, want gate error/0", operationID, err, stageCalls)
		}
	}
}

type faultStagingFile struct {
	*os.File
	writeErr, closeErr bool
	closeCalls         int
}

func (f *faultStagingFile) Write(data []byte) (int, error) {
	if f.writeErr {
		return 0, errors.New("injected write failure")
	}
	return f.File.Write(data)
}
func (f *faultStagingFile) Close() error {
	f.closeCalls++
	if err := f.File.Close(); err != nil {
		return err
	}
	if f.closeErr {
		return errors.New("injected close failure")
	}
	return nil
}

func TestTargetRollbackBackupAdapterStagingFaultsBlockValidationAndPublication(t *testing.T) {
	for _, test := range []struct {
		name         string
		write, close bool
	}{{"write", true, false}, {"close", false, true}} {
		destination, validator := t.TempDir(), &recordingArchiveValidator{}
		var staged *faultStagingFile
		adapter := TargetRollbackBackupAdapter{Executor: &backupExecutor{}, Credentials: CredentialTransport{TempRoot: t.TempDir()}, OpenStaging: func(path string, flag int, mode os.FileMode) (stagingFile, error) {
			file, err := os.OpenFile(path, flag, mode)
			if err != nil {
				return nil, err
			}
			staged = &faultStagingFile{File: file, writeErr: test.write, closeErr: test.close}
			return staged, nil
		}}
		creator := TargetRollbackBackupCreator{Validator: validator, OperationID: func() (string, error) { return test.name + "-failure", nil }, Stage: adapter.stage}
		if _, err := creator.CreateValidated(context.Background(), targetRestoreConfig(t), destination); !errors.Is(err, ErrRestoreBackupGate) || staged.closeCalls != 1 || len(validator.paths) != 0 {
			t.Fatalf("%s error/close/validation = %v/%d/%v, want gate error/1/none", test.name, err, staged.closeCalls, validator.paths)
		}
		if _, err := os.Stat(filepath.Join(destination, ".target-rollback-"+test.name+"-failure.part")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s staging stat = %v, want not exist", test.name, err)
		}
	}
}

func stringInt(value int64) string {
	return strconv.FormatInt(value, 10)
}
