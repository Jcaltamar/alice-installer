package migration

import (
	"context"
	"testing"

	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/platform"
	"github.com/jcaltamar/alice-installer/internal/workspace"
)

func TestNewProductionRestoreCoordinatorComposesConcreteFailClosedAdapters(t *testing.T) {
	if _, err := NewProductionRestoreCoordinator(ProductionRestoreDependencies{}); err == nil {
		t.Fatal("missing production dependencies were accepted")
	}
	coordinator, err := NewProductionRestoreCoordinator(ProductionRestoreDependencies{
		Compose:        compose.NewCLICompose(nil, nil),
		OperationID:    func() (string, error) { return "operation", nil },
		DockerExecutor: OSBinaryExecutor{},
	})
	if err != nil {
		t.Fatalf("NewProductionRestoreCoordinator() error = %v", err)
	}
	if _, ok := coordinator.EnvReader.(workspace.TargetEnvFileReader); !ok {
		t.Fatalf("EnvReader = %T, want concrete generated-env reader", coordinator.EnvReader)
	}
	if _, ok := coordinator.Legacy.(BackupGate); !ok {
		t.Fatalf("Legacy = %T, want BackupGate", coordinator.Legacy)
	}
	if _, ok := coordinator.Target.(TargetRollbackBackupAdapter); !ok {
		t.Fatalf("Target = %T, want TargetRollbackBackupAdapter", coordinator.Target)
	}
	if _, ok := coordinator.Replacement.(TargetReplacementAdapter); !ok {
		t.Fatalf("Replacement = %T, want TargetReplacementAdapter", coordinator.Replacement)
	}
	if _, ok := coordinator.PostgreSQL.(PostgreSQLReachabilityAdapter); !ok {
		t.Fatalf("PostgreSQL = %T, want PostgreSQLReachabilityAdapter", coordinator.PostgreSQL)
	}
	if _, ok := coordinator.PostgreSQLIdentity.(ComposePostgreSQLIdentityProbe); !ok {
		t.Fatalf("PostgreSQLIdentity = %T, want ComposePostgreSQLIdentityProbe", coordinator.PostgreSQLIdentity)
	}
	if _, ok := coordinator.Health.(ComposeBackendProbe); !ok {
		t.Fatalf("Health = %T, want ComposeBackendProbe", coordinator.Health)
	}
	if _, ok := coordinator.BackendStopped.(ComposeBackendProbe); !ok {
		t.Fatalf("BackendStopped = %T, want ComposeBackendProbe", coordinator.BackendStopped)
	}
}

func TestRestoreCoordinatorRejectsIdentityBeforeBackendStop(t *testing.T) {
	f := newCoordinatorFakes()
	f.fail = "identity"
	result := RestoreCoordinator{Waiter: f, EnvReader: f, Legacy: f, Target: f, Services: f, PostgreSQL: f, PostgreSQLIdentity: f, Replacement: f, Health: f, BackendStopped: f}.Run(context.Background(), supportedRequest())
	if result.Outcome != RestoreFailedBeforeCutover || result.Code != "restore-postgres-identity" || coordinatorContains(f.calls, "stop:backend") || f.replacements != 0 {
		t.Fatalf("result = %#v, calls = %v, replacements = %d", result, f.calls, f.replacements)
	}
}

func TestRestoreCoordinatorRejectsMalformedProductionIdentityWithoutMutation(t *testing.T) {
	f := newCoordinatorFakes()
	runner := &platform.FakeCommandRunner{Outputs: map[string]platform.FakeCmdOutput{
		"docker": {Stdout: []byte(`{"Service":"postgresql-master","Name":"alice_postgresql-master","State":"running"}
not-json
`)},
	}}
	identity := ComposePostgreSQLIdentityProbe{Compose: compose.NewCLICompose(runner, nil)}
	result := RestoreCoordinator{Waiter: f, EnvReader: f, Legacy: f, Target: f, Services: f, PostgreSQL: f, PostgreSQLIdentity: identity, Replacement: f, Health: f, BackendStopped: f}.Run(context.Background(), supportedRequest())
	if result.Code != "restore-postgres-identity" || coordinatorContains(f.calls, "stop:backend") || f.replacements != 0 {
		t.Fatalf("result = %#v, calls = %v, replacements = %d", result, f.calls, f.replacements)
	}
}

func TestComposePostgreSQLIdentityProbeRequiresExactRunningPair(t *testing.T) {
	for _, tc := range []struct {
		name string
		rows []compose.ServiceHealth
		ok   bool
	}{
		{"exact", []compose.ServiceHealth{{Service: compose.PostgreSQLService, Container: compose.PostgreSQLContainer, State: "running"}}, true},
		{"stopped", []compose.ServiceHealth{{Service: compose.PostgreSQLService, Container: compose.PostgreSQLContainer, State: "exited"}}, false},
		{"renamed", []compose.ServiceHealth{{Service: compose.PostgreSQLService, Container: "other", State: "running"}}, false},
		{"ambiguous", []compose.ServiceHealth{{Service: compose.PostgreSQLService, Container: compose.PostgreSQLContainer, State: "running"}, {Service: compose.PostgreSQLService, Container: compose.PostgreSQLContainer, State: "running"}}, false},
		{"missing", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probe := ComposePostgreSQLIdentityProbe{Compose: &compose.FakeComposeRunner{Healths: tc.rows}}
			err := probe.Prove(context.Background(), []string{"compose.yml"}, ".env")
			if (err == nil) != tc.ok {
				t.Fatalf("Prove() error = %v", err)
			}
		})
	}
}

func TestComposePostgreSQLIdentityProbeRejectsCancelledAndFailedEvidence(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	probe := ComposePostgreSQLIdentityProbe{Compose: &compose.FakeComposeRunner{Healths: []compose.ServiceHealth{{Service: compose.PostgreSQLService, Container: compose.PostgreSQLContainer, State: "running"}}}}
	if err := probe.Prove(ctx, nil, ""); err == nil {
		t.Fatal("cancelled identity evidence was accepted")
	}
	probe = ComposePostgreSQLIdentityProbe{Compose: &compose.FakeComposeRunner{HealthErr: context.DeadlineExceeded}}
	if err := probe.Prove(context.Background(), nil, ""); err == nil {
		t.Fatal("failed identity evidence was accepted")
	}
}

func TestComposeBackendProbeFailsClosedForCancelledAndFailedEvidence(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	probe := ComposeBackendProbe{Compose: &compose.FakeComposeRunner{Healths: []compose.ServiceHealth{{Service: compose.BackendService, State: "exited"}}}}
	if stopped, err := probe.BackendStopped(ctx, []string{"compose.yml"}, ".env"); err == nil || stopped {
		t.Fatalf("cancelled stopped evidence = %t, %v", stopped, err)
	}
	probe = ComposeBackendProbe{Compose: &compose.FakeComposeRunner{HealthErr: context.DeadlineExceeded}}
	if err := probe.WaitHealthy(context.Background(), []string{"compose.yml"}, ".env"); err == nil {
		t.Fatal("failed health evidence was accepted")
	}
}

func TestComposeBackendProbeRequiresPositiveUnambiguousEvidence(t *testing.T) {
	for _, tc := range []struct {
		name    string
		healths []compose.ServiceHealth
		stopped bool
		healthy bool
	}{
		{"stopped", []compose.ServiceHealth{{Service: compose.BackendService, State: "exited"}}, true, false},
		{"healthy", []compose.ServiceHealth{{Service: compose.BackendService, State: "running", Status: "healthy"}}, false, true},
		{"ambiguous", []compose.ServiceHealth{{Service: compose.BackendService, State: "exited"}, {Service: compose.BackendService, State: "exited"}}, false, false},
		{"missing", nil, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probe := ComposeBackendProbe{Compose: &compose.FakeComposeRunner{Healths: tc.healths}}
			stopped, stopErr := probe.BackendStopped(context.Background(), []string{"compose.yml"}, ".env")
			if stopped != tc.stopped || (tc.name == "ambiguous" || tc.name == "missing") != (stopErr != nil) {
				t.Fatalf("stopped = %t, %v", stopped, stopErr)
			}
			healthErr := probe.WaitHealthy(context.Background(), []string{"compose.yml"}, ".env")
			if (healthErr == nil) != tc.healthy {
				t.Fatalf("health = %v", healthErr)
			}
		})
	}
}
