//go:build integration

package migration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const integrationPostgreSQLImage = "postgres:11-alpine"

// TestBackupIntegrationOptIn proves the Docker/PostgreSQL fixture contract only
// when an operator explicitly opts in. It creates a private network and a
// disposable PostgreSQL container; it never discovers or accesses operator data.
func TestBackupIntegrationOptIn(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping migration integration test in -short mode")
	}
	docker := os.Getenv("ALICE_MIGRATION_INTEGRATION_DOCKER")
	if docker == "" {
		t.Skip("set ALICE_MIGRATION_INTEGRATION_DOCKER to opt into the isolated Docker/PostgreSQL fixture")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	resourceID := isolatedFixtureID(t)
	network := "alice-migration-it-net-" + resourceID
	container := "alice-migration-it-pg-" + resourceID
	label := "alice-installer.integration=true"

	runDocker(t, ctx, docker, "network", "create", "--label", label, network)
	t.Cleanup(func() { runDockerCleanup(docker, "network", "rm", network) })
	runDocker(t, ctx, docker,
		"run", "--detach", "--rm", "--name", container,
		"--label", label,
		"--network", network,
		"--env", "POSTGRES_HOST_AUTH_METHOD=trust",
		integrationPostgreSQLImage,
	)
	t.Cleanup(func() { runDockerCleanup(docker, "rm", "--force", container) })

	for {
		if ctx.Err() != nil {
			t.Fatal("isolated PostgreSQL fixture did not become ready")
		}
		cmd := exec.CommandContext(ctx, docker, "exec", container, "pg_isready", "--username=postgres", "--dbname=postgres")
		if err := cmd.Run(); err == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	runDocker(t, ctx, docker, "exec", container, "psql", "--username=postgres", "--dbname=postgres", "--no-psqlrc", "--tuples-only", "--command", "SELECT 1")
}

func isolatedFixtureID(t *testing.T) string {
	t.Helper()
	bytes := make([]byte, 6)
	if _, err := rand.Read(bytes); err != nil {
		t.Fatal("could not create isolated fixture ID")
	}
	return hex.EncodeToString(bytes)
}

func runDocker(t *testing.T, ctx context.Context, docker string, args ...string) {
	t.Helper()
	output, err := exec.CommandContext(ctx, docker, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("isolated fixture Docker command failed (%s): %s", args[0], boundedIntegrationOutput(output))
	}
}

func runDockerCleanup(docker string, args ...string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, docker, args...).Run()
}

func boundedIntegrationOutput(output []byte) string {
	const limit = 512
	text := strings.TrimSpace(string(output))
	if len(text) > limit {
		return text[:limit] + "…"
	}
	return text
}
