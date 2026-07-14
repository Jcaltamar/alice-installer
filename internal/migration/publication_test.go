package migration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPublishBackupPairAtomicallyPublishesProtectedSecretFreeManifest(t *testing.T) {
	dir := secureTempDir(t)
	dump := filepath.Join(dir, ".alice-backup.dump.part")
	content := []byte("custom\x00archive")
	if err := os.WriteFile(dump, content, 0o600); err != nil {
		t.Fatal(err)
	}
	publication, err := publishOwnedBackupPair(context.Background(), PublicationRequest{
		Directory: dir,
		DumpPath:  dump,
		Container: ContainerIdentity{ID: strings.Repeat("a", 64), Image: PostgreSQL11Image, Digest: "sha256:safe"},
		Config:    ResolvedConfig{Environment: EnvironmentProduction, Dialect: DialectPostgreSQL, Database: "alice", Username: "guardian", Host: "127.0.0.1", Port: 5432, password: newSecret(validationSecretSentinel)},
		Now:       time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("PublishBackupPair() error = %v", err)
	}
	if publication.Size != int64(len(content)) || publication.SHA256 != "63ef699bc22762f48d4eb22e0a4fb70afcf768a98f3736b7a15f7cae2b6995cd" {
		t.Fatalf("publication checksum/size = %#v", publication)
	}
	for _, path := range []string{publication.DumpPath, publication.ManifestPath} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("published artifact %q mode=%v err=%v", path, info.Mode(), err)
		}
	}
	manifest, err := os.ReadFile(publication.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{validationSecretSentinel, "password", "pgpass", "config.js", "stderr"} {
		if strings.Contains(strings.ToLower(string(manifest)), strings.ToLower(forbidden)) {
			t.Fatalf("manifest leaked %q: %s", forbidden, manifest)
		}
	}
	for _, required := range []string{"\"schema_version\":1", "\"format\":\"postgresql-custom\"", "\"validation\":\"validated\"", publication.SHA256} {
		if !strings.Contains(string(manifest), required) {
			t.Fatalf("manifest missing %q: %s", required, manifest)
		}
	}
}

func TestPublishBackupPairNeverReplacesExistingFinalArtifact(t *testing.T) {
	dir := secureTempDir(t)
	content := []byte("custom archive")
	dump := filepath.Join(dir, ".alice-backup.dump.part")
	if err := os.WriteFile(dump, content, 0o600); err != nil {
		t.Fatal(err)
	}
	checksum, _, err := hashFile(dump)
	if err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(dir, "alice-backup-"+checksum+".dump")
	if err := os.WriteFile(existing, []byte("operator-owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = publishOwnedBackupPair(context.Background(), PublicationRequest{Directory: dir, DumpPath: dump, Container: ContainerIdentity{ID: strings.Repeat("a", 64), Image: PostgreSQL11Image}, Config: testProcessConfig(validationSecretSentinel)})
	if err == nil {
		t.Fatal("PublishBackupPair() overwrote an existing final artifact")
	}
	if got, readErr := os.ReadFile(existing); readErr != nil || string(got) != "operator-owned" {
		t.Fatalf("existing artifact changed: %q, %v", got, readErr)
	}
}

func TestPublishBackupPairPreservesPathsWithoutOperationOwnership(t *testing.T) {
	dir := secureTempDir(t)
	dump := filepath.Join(dir, ".caller-owned.dump.part")
	stagedManifest := filepath.Join(dir, ".alice-backup-ignored.manifest.part")
	for path, content := range map[string]string{dump: "caller-owned", stagedManifest: "operator-owned"} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	_, err := publishBackupPair(context.Background(), PublicationRequest{
		Directory: dir,
		DumpPath:  dump,
		Container: ContainerIdentity{ID: strings.Repeat("a", 64), Image: PostgreSQL11Image},
		Config:    testProcessConfig(validationSecretSentinel),
	})
	if err == nil {
		t.Fatal("publishBackupPair() accepted a dump without an operation-created capability")
	}
	for path, want := range map[string]string{dump: "caller-owned", stagedManifest: "operator-owned"} {
		got, readErr := os.ReadFile(path)
		if readErr != nil || string(got) != want {
			t.Fatalf("unowned path %q = %q, %v; want preserved %q", path, got, readErr, want)
		}
	}
}

func TestPublishBackupPairCleansOnlyOperationCreatedHalfPairs(t *testing.T) {
	for _, tt := range []struct {
		name   string
		cancel bool
		fail   string
	}{
		{name: "cancelled", cancel: true},
		{name: "dump rename failure", fail: "dump-rename"},
		{name: "manifest rename failure", fail: "manifest-rename"},
		{name: "directory sync failure", fail: "directory-sync"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := secureTempDir(t)
			dump := filepath.Join(dir, ".alice-backup.dump.part")
			if err := os.WriteFile(dump, []byte("custom archive"), 0o600); err != nil {
				t.Fatal(err)
			}
			operatorOwned := filepath.Join(dir, "operator-owned.dump")
			if err := os.WriteFile(operatorOwned, []byte("preserve"), 0o600); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if tt.cancel {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			_, err := publishOwnedBackupPair(ctx, PublicationRequest{Directory: dir, DumpPath: dump, Container: ContainerIdentity{ID: strings.Repeat("a", 64), Image: PostgreSQL11Image}, Config: testProcessConfig(validationSecretSentinel), fault: tt.fail})
			if err == nil {
				t.Fatal("PublishBackupPair() unexpectedly succeeded")
			}
			if got, readErr := os.ReadFile(operatorOwned); readErr != nil || string(got) != "preserve" {
				t.Fatalf("operator-owned artifact changed: %q, %v", got, readErr)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].Name() != "operator-owned.dump" {
				t.Fatalf("operation artifacts remain: %#v", entries)
			}
		})
	}
}

func TestPublishBackupPairPreservesPreexistingManifestStaging(t *testing.T) {
	dir := secureTempDir(t)
	dump := filepath.Join(dir, ".alice-backup.dump.part")
	if err := os.WriteFile(dump, []byte("custom archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	sum, _, err := hashFile(dump)
	if err != nil {
		t.Fatal(err)
	}
	manifestStaged := filepath.Join(dir, ".alice-backup-"+sum+".manifest.part")
	if err := os.WriteFile(manifestStaged, []byte("operator-owned"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = publishOwnedBackupPair(context.Background(), PublicationRequest{Directory: dir, DumpPath: dump, Container: ContainerIdentity{ID: strings.Repeat("a", 64), Image: PostgreSQL11Image}, Config: testProcessConfig(validationSecretSentinel)})
	if err == nil {
		t.Fatal("publishOwnedBackupPair() accepted a pre-existing manifest staging path")
	}
	if got, readErr := os.ReadFile(manifestStaged); readErr != nil || string(got) != "operator-owned" {
		t.Fatalf("pre-existing manifest staging = %q, %v; want preserved", got, readErr)
	}
	if _, statErr := os.Stat(dump); !os.IsNotExist(statErr) {
		t.Fatalf("operation-created dump remained after failed publication: %v", statErr)
	}
}

func publishOwnedBackupPair(ctx context.Context, request PublicationRequest) (BackupPublication, error) {
	return PublishBackupPair(ctx, ownedPublicationRequest(request, request.DumpPath))
}
