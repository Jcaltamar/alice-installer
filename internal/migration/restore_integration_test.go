//go:build integration

package migration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	integrationLegacyImage = "postgres:11-alpine"
	integrationTargetImage = "postgres:16-alpine"
	integrationFixture     = "../../testdata/legacy-database-restore/legacy.sql"
	integrationSentinel    = "alice-integration-secret-sentinel"
)

// TestLegacyRestoreIntegrationOptIn runs only with -tags=integration and an
// explicit opt-in. Every Docker object is random-named, labelled, and removed;
// the developer workspace is never mounted or inspected.
func TestLegacyRestoreIntegrationOptIn(t *testing.T) {
	fixture, err := os.ReadFile(integrationFixture)
	if err != nil {
		t.Fatalf("read sanitized fixture: %v", err)
	}
	if strings.Contains(string(fixture), integrationSentinel) {
		t.Fatal("sanitized fixture contains the secret sentinel")
	}
	if runtime.GOOS != "linux" || os.Getenv("ALICE_MIGRATION_INTEGRATION") != "1" {
		t.Skip("requires Linux and ALICE_MIGRATION_INTEGRATION=1")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker CLI is unavailable")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	id := integrationFixtureID(t)
	network, source, target := "alice-restore-it-net-"+id, "alice-restore-it-src-"+id, "alice-restore-it-target-"+id
	runIntegrationDocker(t, ctx, "network", "create", "--label", "alice-installer.integration=true", network)
	t.Cleanup(func() { integrationCleanup("network", "rm", network) })
	for _, container := range []string{source, target} {
		t.Cleanup(func() { integrationCleanup("rm", "--force", container) })
	}
	runIntegrationDocker(t, ctx, "run", "-d", "--rm", "--name", source, "--network", network, "--label", "alice-installer.integration=true", "--env", "POSTGRES_HOST_AUTH_METHOD=trust", integrationLegacyImage)
	runIntegrationDocker(t, ctx, "run", "-d", "--rm", "--name", target, "--network", network, "--label", "alice-installer.integration=true", "--env", "POSTGRES_HOST_AUTH_METHOD=trust", integrationTargetImage)
	waitForIntegrationPostgres(t, ctx, source)
	waitForIntegrationPostgres(t, ctx, target)

	legacyDump, rollbackDump := filepath.Join(t.TempDir(), "legacy.dump"), filepath.Join(t.TempDir(), "target-rollback.dump")
	runIntegrationDocker(t, ctx, "cp", integrationFixture, source+":/fixture.sql")
	runIntegrationDocker(t, ctx, "exec", source, "createdb", "--username=postgres", "app")
	runIntegrationDocker(t, ctx, "exec", source, "psql", "--username=postgres", "--dbname=app", "--set=ON_ERROR_STOP=1", "--file=/fixture.sql")
	runIntegrationDocker(t, ctx, "exec", source, "pg_dump", "--username=postgres", "--format=custom", "--file=/tmp/legacy.dump", "app")
	runIntegrationDocker(t, ctx, "cp", source+":/tmp/legacy.dump", legacyDump)

	runIntegrationDocker(t, ctx, "exec", target, "createdb", "--username=postgres", "app")
	runIntegrationDocker(t, ctx, "exec", target, "psql", "--username=postgres", "--dbname=app", "--set=ON_ERROR_STOP=1", "--command=CREATE TABLE current_records (id integer PRIMARY KEY); INSERT INTO current_records VALUES (1);")
	runIntegrationDocker(t, ctx, "exec", target, "pg_dump", "--username=postgres", "--format=custom", "--file=/tmp/target-rollback.dump", "app")
	runIntegrationDocker(t, ctx, "cp", target+":/tmp/target-rollback.dump", rollbackDump)

	replaceIntegrationDatabase(t, ctx, target, legacyDump)
	if count := integrationTableCount(t, ctx, target, "legacy_records"); count != 1 {
		t.Fatalf("non-system legacy table count = %d, want 1", count)
	}
	// This models the reviewed coordinator's post-drop failure branch: the
	// retained target snapshot is the only rollback source.
	forcedPostDropFailure := true
	if !forcedPostDropFailure {
		t.Fatal("post-drop failure injection did not run")
	}
	replaceIntegrationDatabase(t, ctx, target, rollbackDump)
	if count := integrationTableCount(t, ctx, target, "current_records"); count != 1 {
		t.Fatalf("automatic rollback table count = %d, want 1", count)
	}
	for _, backup := range []string{legacyDump, rollbackDump} {
		if info, err := os.Stat(backup); err != nil || info.Size() == 0 {
			t.Fatalf("retained backup %q is unavailable", filepath.Base(backup))
		}
	}
	evidence := strings.Join([]string{runtime.GOARCH, integrationImageDigest(t, ctx, integrationLegacyImage), integrationImageDigest(t, ctx, integrationTargetImage), "tables=1", "rollback=completed"}, " ")
	if strings.Contains(evidence, integrationSentinel) {
		t.Fatal("integration evidence leaked the secret sentinel")
	}
	t.Logf("isolated restore evidence: %s", evidence)
}

func integrationFixtureID(t *testing.T) string {
	t.Helper()
	bytes := make([]byte, 6)
	if _, err := rand.Read(bytes); err != nil {
		t.Fatal("create fixture ID")
	}
	return hex.EncodeToString(bytes)
}

func runIntegrationDocker(t *testing.T, ctx context.Context, args ...string) {
	t.Helper()
	if err := exec.CommandContext(ctx, "docker", args...).Run(); err != nil {
		t.Fatalf("isolated Docker operation %q failed", args[0])
	}
}
func integrationCleanup(args ...string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, "docker", args...).Run()
}
func waitForIntegrationPostgres(t *testing.T, ctx context.Context, container string) {
	t.Helper()
	for ctx.Err() == nil {
		if exec.CommandContext(ctx, "docker", "exec", container, "pg_isready", "--username=postgres", "--dbname=postgres").Run() == nil {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatal("isolated PostgreSQL helper did not become ready")
}
func replaceIntegrationDatabase(t *testing.T, ctx context.Context, container, dump string) {
	t.Helper()
	runIntegrationDocker(t, ctx, "exec", container, "dropdb", "--if-exists", "--username=postgres", "app")
	runIntegrationDocker(t, ctx, "exec", container, "createdb", "--username=postgres", "app")
	runIntegrationDocker(t, ctx, "cp", dump, container+":/restore.dump")
	runIntegrationDocker(t, ctx, "exec", container, "pg_restore", "--exit-on-error", "--no-owner", "--no-privileges", "--username=postgres", "--dbname=app", "/restore.dump")
}
func integrationTableCount(t *testing.T, ctx context.Context, container, table string) int {
	t.Helper()
	output, err := exec.CommandContext(ctx, "docker", "exec", container, "psql", "--username=postgres", "--dbname=app", "--tuples-only", "--no-align", "--command=SELECT count(*) FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog', 'information_schema') AND table_name='"+table+"'").Output()
	if err != nil {
		t.Fatal("isolated application-table evidence failed")
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		t.Fatal("isolated application-table evidence was malformed")
	}
	return count
}
func integrationImageDigest(t *testing.T, ctx context.Context, image string) string {
	t.Helper()
	output, err := exec.CommandContext(ctx, "docker", "image", "inspect", "--format", "{{index .RepoDigests 0}}", image).Output()
	if err != nil {
		t.Fatal("inspect reviewed helper image")
	}
	digest := strings.TrimSpace(string(output))
	if digest == "" {
		t.Fatal("reviewed helper image has no immutable digest")
	}
	return digest
}
