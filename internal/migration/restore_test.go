package migration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jcaltamar/alice-installer/internal/workspace"
)

func TestTargetReplacementAdapterCrossesProductionBoundary(t *testing.T) {
	credential := replacementCredential(t)
	adapter := TargetReplacementAdapter{Executor: &replacementExecutor{outcome: ProcessSucceeded}, Prepare: func(context.Context, workspace.TargetDatabaseConfig) (CredentialFile, error) { return credential, nil }}
	evidence, mutated, err := adapter.Replace(context.Background(), workspace.TargetDatabaseConfig{Host: "127.0.0.1", Port: 5432, User: "alice", Database: "alice"}, ValidatedBackup{ref: backupRef("legacy")})
	if err == nil || !mutated || evidence != (DatabaseEvidence{}) {
		t.Fatalf("unobserved validation = evidence=%#v mutated=%t err=%v", evidence, mutated, err)
	}

	credential = replacementCredential(t)
	adapter.Executor = &replacementExecutor{outcome: ProcessSucceeded, output: "t\n2\nt\n"}
	evidence, mutated, err = adapter.Replace(context.Background(), workspace.TargetDatabaseConfig{Host: "127.0.0.1", Port: 5432, User: "alice", Database: "alice"}, ValidatedBackup{ref: backupRef("legacy")})
	if err != nil || !mutated || evidence != (DatabaseEvidence{RestoreExitOK: true, ConnectionOK: true, ApplicationTables: 2, PostgreSQLReachable: true}) {
		t.Fatalf("observed validation = evidence=%#v mutated=%t err=%v", evidence, mutated, err)
	}

	credential = replacementCredential(t)
	adapter.Executor = &replacementExecutor{fail: true, failAt: int(ReplacementDropDatabase)}
	evidence, mutated, err = adapter.Replace(context.Background(), workspace.TargetDatabaseConfig{Host: "127.0.0.1", Port: 5432, User: "alice", Database: "alice"}, ValidatedBackup{ref: backupRef("legacy")})
	if err == nil || !mutated || evidence != (DatabaseEvidence{}) {
		t.Fatalf("drop failure = evidence=%#v mutated=%t err=%v", evidence, mutated, err)
	}
}

func TestTargetReplacementAdapterRejectsMalformedOrZeroValidationEvidence(t *testing.T) {
	for _, output := range []string{"t\n0\nt\n", "t\nnot-a-count\nt\n", "t\n2\nf\n"} {
		t.Run(output, func(t *testing.T) {
			credential := replacementCredential(t)
			adapter := TargetReplacementAdapter{Executor: &replacementExecutor{outcome: ProcessSucceeded, output: output}, Prepare: func(context.Context, workspace.TargetDatabaseConfig) (CredentialFile, error) { return credential, nil }}
			evidence, mutated, err := adapter.Replace(context.Background(), workspace.TargetDatabaseConfig{Host: "127.0.0.1", Port: 5432, User: "alice", Database: "alice"}, ValidatedBackup{ref: backupRef("legacy")})
			if err == nil || !mutated || evidence != (DatabaseEvidence{}) {
				t.Fatalf("output %q accepted: evidence=%#v mutated=%t err=%v", output, evidence, mutated, err)
			}
		})
	}
}

func TestRestoreCoordinatorStopsBackendBeforeRollbackAndAfterRecoveryFailures(t *testing.T) {
	for _, failure := range []string{"health", "primary-rollback", "rollback-cancel"} {
		t.Run(failure, func(t *testing.T) {
			f := newCoordinatorFakes()
			f.fail = failure
			coordinator := RestoreCoordinator{Waiter: f, EnvReader: f, Legacy: f, Target: f, Services: f, PostgreSQL: f, PostgreSQLIdentity: f, Replacement: f, Health: f, BackendStopped: f, RollbackContext: f.rollbackContext}
			result := coordinator.Run(context.Background(), supportedRequest())
			if result.Outcome != RestorePartialCutover || result.Rollback == RollbackNotRequired || f.stops < 2 || f.backendRunning {
				t.Fatalf("result = %#v, stops = %d, running = %t, calls = %v", result, f.stops, f.backendRunning, f.calls)
			}
		})
	}
}

func TestRestoreCoordinatorUsesBoundedRecoveryContextAndReportsRecoveryErrors(t *testing.T) {
	f := newCoordinatorFakes()
	f.fail = "env-start"
	coordinator := RestoreCoordinator{Waiter: f, EnvReader: f, Legacy: f, Target: f, Services: f, PostgreSQL: f, PostgreSQLIdentity: f, Replacement: f, Health: f, BackendStopped: f, RollbackContext: f.rollbackContext}
	result := coordinator.Run(context.Background(), supportedRequest())
	if result.Outcome != RestoreFailedBeforeCutover || result.Code != "restore-backend-recovery" || f.rollbackContexts != 1 {
		t.Fatalf("result = %#v, rollback contexts = %d", result, f.rollbackContexts)
	}
}

func TestRestoreCoordinatorFailureMatrixKeepsBackendStopped(t *testing.T) {
	for _, tc := range []struct {
		fail    string
		outcome RestoreOutcome
		stage   RestoreStage
		stopped bool
	}{
		{"wait", RestoreFailedBeforeCutover, StageWait, false},
		{"stop", RestoreFailedBeforeCutover, StageBackendStop, false},
		{"start", RestorePartialCutover, StageBackendStart, true},
		{"primary-rollback-revalidate", RestorePartialCutover, StageTargetReplacement, true},
		{"restore-exit", RestorePartialCutover, StageTargetReplacement, false},
		{"connection", RestorePartialCutover, StageTargetReplacement, false},
		{"postgres-evidence", RestorePartialCutover, StageTargetReplacement, false},
		{"zero-tables", RestorePartialCutover, StageTargetReplacement, false},
	} {
		t.Run(tc.fail, func(t *testing.T) {
			f := newCoordinatorFakes()
			f.fail = tc.fail
			result := RestoreCoordinator{Waiter: f, EnvReader: f, Legacy: f, Target: f, Services: f, PostgreSQL: f, PostgreSQLIdentity: f, Replacement: f, Health: f, BackendStopped: f, RollbackContext: f.rollbackContext}.Run(context.Background(), supportedRequest())
			if result.Outcome != tc.outcome || result.FailedStage != tc.stage || (tc.stopped && f.backendRunning) {
				t.Fatalf("result = %#v, running = %t", result, f.backendRunning)
			}
		})
	}
}

func TestRestoreCoordinatorSuccessfulCutoverUsesExactWaitAndBackendOnly(t *testing.T) {
	f := newCoordinatorFakes()
	coordinator := RestoreCoordinator{Waiter: f, EnvReader: f, Legacy: f, Target: f, Services: f, PostgreSQL: f, PostgreSQLIdentity: f, Replacement: f, Health: f, BackendStopped: f}
	result := coordinator.Run(context.Background(), supportedRequest())
	if result.Outcome != RestoreSucceeded || !result.Mutated || result.Rollback != RollbackNotRequired || !result.BackendHealthy || result.Code != "restore-succeeded" {
		t.Fatalf("result = %#v", result)
	}
	if got, want := f.calls, []string{"wait", "postgres-identity", "stop:backend", "env", "legacy", "postgres-identity", "postgres", "target", "replace:legacy", "postgres-identity", "start:backend", "health"}; !sameStrings(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	if len(f.waits) != 1 || f.waits[0] != 60*time.Second {
		t.Fatalf("waits = %v", f.waits)
	}
}

func TestRestoreCoordinatorPreMutationFailuresDoNotReplaceAndRecoverBackend(t *testing.T) {
	for _, tc := range []struct {
		name, fail, code string
	}{
		{"credentials", "env", "restore-credentials"},
		{"legacy", "legacy", "restore-legacy"},
		{"postgres", "postgres", "restore-postgres"},
		{"target", "target", "restore-target-backup"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCoordinatorFakes()
			f.fail = tc.fail
			result := RestoreCoordinator{Waiter: f, EnvReader: f, Legacy: f, Target: f, Services: f, PostgreSQL: f, PostgreSQLIdentity: f, Replacement: f, Health: f, BackendStopped: f}.Run(context.Background(), supportedRequest())
			if result.Outcome != RestoreFailedBeforeCutover || result.Mutated || result.Code != tc.code || f.replacements != 0 {
				t.Fatalf("result = %#v, replacements = %d", result, f.replacements)
			}
			if !coordinatorContains(f.calls, "start:backend") || !coordinatorContains(f.calls, "health") {
				t.Fatalf("unchanged target did not recover backend: %v", f.calls)
			}
		})
	}
}

func TestRestoreCoordinatorUsesReplacementMutationEvidenceAtDropBoundary(t *testing.T) {
	f := newCoordinatorFakes()
	f.fail = "pre-drop"
	result := RestoreCoordinator{Waiter: f, EnvReader: f, Legacy: f, Target: f, Services: f, PostgreSQL: f, PostgreSQLIdentity: f, Replacement: f, Health: f, BackendStopped: f}.Run(context.Background(), supportedRequest())
	if result.Outcome != RestoreFailedBeforeCutover || result.Mutated || result.Rollback != RollbackNotRequired || f.replacements != 1 || !coordinatorContains(f.calls, "start:backend") {
		t.Fatalf("result = %#v, calls = %v", result, f.calls)
	}
}

func TestRestoreCoordinatorPostMutationFailureRollsBackOnlyValidatedTarget(t *testing.T) {
	for _, failure := range []string{"primary", "primary-rollback-unmutated"} {
		t.Run(failure, func(t *testing.T) {
			f := newCoordinatorFakes()
			f.fail = failure
			result := RestoreCoordinator{Waiter: f, EnvReader: f, Legacy: f, Target: f, Services: f, PostgreSQL: f, PostgreSQLIdentity: f, Replacement: f, Health: f, BackendStopped: f}.Run(context.Background(), supportedRequest())
			if result.Outcome != RestorePartialCutover || !result.Mutated || result.Rollback != RollbackSucceeded || result.Code != "restore-primary-failed" || !result.BackendHealthy {
				t.Fatalf("result = %#v", result)
			}
			if got, want := f.sources, []bool{false, true}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
				t.Fatalf("replacement sources targetRollback = %v, want %v", got, want)
			}
		})
	}
}

func TestRestoreCoordinatorRollbackProofsPrecedeBackendStopAndReplacement(t *testing.T) {
	for _, failure := range []string{"rollback-identity", "rollback-postgres"} {
		t.Run(failure, func(t *testing.T) {
			f := newCoordinatorFakes()
			f.fail = failure
			result := RestoreCoordinator{Waiter: f, EnvReader: f, Legacy: f, Target: f, Services: f, PostgreSQL: f, PostgreSQLIdentity: f, Replacement: f, Health: f, BackendStopped: f}.Run(context.Background(), supportedRequest())
			if result.Outcome != RestorePartialCutover || result.Rollback != RollbackFailed || result.Code != "restore-rollback-postgres" || f.replacements != 1 || f.stops != 1 || f.backendRunning {
				t.Fatalf("result = %#v, replacements = %d, stops = %d, running = %t, calls = %v", result, f.replacements, f.stops, f.backendRunning, f.calls)
			}
			want := []string{"wait", "postgres-identity", "stop:backend", "env", "legacy", "postgres-identity", "postgres", "target", "replace:legacy", "postgres-identity"}
			if failure == "rollback-postgres" {
				want = append(want, "postgres")
			}
			if got := f.calls; !sameStrings(got, want) {
				t.Fatalf("calls = %v, want %v", got, want)
			}
		})
	}
}

func TestRestoreCoordinatorRollbackCallsInOrder(t *testing.T) {
	f := newCoordinatorFakes()
	f.fail = "primary"
	RestoreCoordinator{Waiter: f, EnvReader: f, Legacy: f, Target: f, Services: f, PostgreSQL: f, PostgreSQLIdentity: f, Replacement: f, Health: f, BackendStopped: f}.Run(context.Background(), supportedRequest())
	want := []string{"wait", "postgres-identity", "stop:backend", "env", "legacy", "postgres-identity", "postgres", "target", "replace:legacy", "postgres-identity", "postgres", "stop:backend", "backend-stopped", "legacy", "postgres-identity", "postgres", "replace:target", "start:backend", "health"}
	if f.stops != 2 || f.replacements != 2 || !sameStrings(f.calls, want) {
		t.Fatalf("stops = %d, replacements = %d, calls = %v, want %v", f.stops, f.replacements, f.calls, want)
	}
}

func TestRestoreCoordinatorRollbackFailureAndPostMutationCancellationRemainPartial(t *testing.T) {
	for _, tc := range []struct{ name, fail, code string }{{"rollback", "rollback", "restore-rollback-failed"}, {"health", "health", "restore-rollback-recovery"}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCoordinatorFakes()
			if tc.fail == "health" {
				f.fail = "health"
			} else {
				f.fail = "primary-rollback"
			}
			result := RestoreCoordinator{Waiter: f, EnvReader: f, Legacy: f, Target: f, Services: f, PostgreSQL: f, PostgreSQLIdentity: f, Replacement: f, Health: f, BackendStopped: f}.Run(context.Background(), supportedRequest())
			if result.Outcome != RestorePartialCutover || result.Rollback != RollbackFailed || result.Code != tc.code || result.BackendHealthy {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestRestoreCoordinatorCancellationAndUnsupportedDoNotMutate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := newCoordinatorFakes()
	coordinator := RestoreCoordinator{Waiter: f, EnvReader: f, Legacy: f, Target: f, Services: f, PostgreSQL: f, PostgreSQLIdentity: f, Replacement: f, Health: f, BackendStopped: f}
	cancelled := coordinator.Run(ctx, supportedRequest())
	if cancelled.Outcome != RestoreCancelledBeforeCutover || cancelled.FailedStage != StageWait || cancelled.Mutated || cancelled.Code != "restore-cancelled" {
		t.Fatalf("cancelled = %#v", cancelled)
	}
	unsupported := coordinator.Run(context.Background(), RestoreRequest{GOOS: "windows", GOARCH: "amd64"})
	if unsupported.Outcome != RestoreUnsupported || unsupported.Code != "restore-unsupported" || len(f.calls) != 1 {
		t.Fatalf("unsupported = %#v, calls = %v", unsupported, f.calls)
	}
}

func TestRestoreCoordinatorRollbackRequiresProvenBackendStop(t *testing.T) {
	for _, tc := range []struct {
		failure  string
		rollback RollbackStatus
	}{{"rollback-stop-error", RollbackFailed}, {"rollback-stop-cancel", RollbackFailed}, {"rollback-stop-ambiguous", RollbackFailed}} {
		t.Run(tc.failure, func(t *testing.T) {
			f := newCoordinatorFakes()
			f.fail = tc.failure
			result := RestoreCoordinator{Waiter: f, EnvReader: f, Legacy: f, Target: f, Services: f, PostgreSQL: f, PostgreSQLIdentity: f, Replacement: f, Health: f, BackendStopped: f, RollbackContext: f.rollbackContext}.Run(context.Background(), supportedRequest())
			if result.Outcome != RestorePartialCutover || result.Rollback != tc.rollback || result.Code != "restore-rollback-stop" || f.replacements != 1 || !f.backendRunning {
				t.Fatalf("result = %#v, replacements = %d, running = %t", result, f.replacements, f.backendRunning)
			}
		})
	}
}

type coordinatorFakes struct {
	calls            []string
	waits            []time.Duration
	fail             string
	replacements     int
	sources          []bool
	stops            int
	backendRunning   bool
	rollbackContexts int
	revalidations    int
}

func newCoordinatorFakes() *coordinatorFakes { return &coordinatorFakes{} }
func (f *coordinatorFakes) Wait(ctx context.Context, d time.Duration) error {
	f.calls = append(f.calls, "wait")
	f.waits = append(f.waits, d)
	if f.fail == "wait" {
		return errors.New("wait")
	}
	return ctx.Err()
}
func (f *coordinatorFakes) ReadTargetDatabase(_ context.Context, _ string) (workspace.TargetDatabaseConfig, error) {
	f.calls = append(f.calls, "env")
	if f.fail == "env" || f.fail == "env-start" {
		return workspace.TargetDatabaseConfig{}, errors.New("env")
	}
	return workspace.TargetDatabaseConfig{Host: "127.0.0.1", Port: 5432, User: "alice", Database: "alice"}, nil
}
func (f *coordinatorFakes) Revalidate(_ context.Context, _ BackupRef) (ValidatedBackup, error) {
	f.calls = append(f.calls, "legacy")
	f.revalidations++
	if f.fail == "legacy" || (f.fail == "primary-rollback-revalidate" && f.revalidations == 2) {
		return ValidatedBackup{}, errors.New("legacy")
	}
	return ValidatedBackup{ref: backupRef("legacy")}, nil
}
func (f *coordinatorFakes) CreateValidated(_ context.Context, _ workspace.TargetDatabaseConfig, _ string) (ValidatedBackup, error) {
	f.calls = append(f.calls, "target")
	if f.fail == "target" {
		return ValidatedBackup{}, errors.New("target")
	}
	return ValidatedBackup{ref: backupRef("target"), targetRollback: true}, nil
}
func (f *coordinatorFakes) StopService(ctx context.Context, _ []string, _ string, service string) error {
	f.calls = append(f.calls, "stop:"+service)
	f.stops++
	if err := ctx.Err(); err != nil {
		return err
	}
	if f.fail == "stop" || (f.stops > 1 && (f.fail == "rollback-stop-error" || f.fail == "rollback-stop-ambiguous")) {
		return errors.New("stop")
	}
	f.backendRunning = false
	return nil
}
func (f *coordinatorFakes) BackendStopped(_ context.Context, _ []string, _ string) (bool, error) {
	f.calls = append(f.calls, "backend-stopped")
	if f.fail == "rollback-stop-ambiguous" {
		return false, errors.New("ambiguous")
	}
	return !f.backendRunning, nil
}
func (f *coordinatorFakes) StartService(_ context.Context, _ []string, _ string, service string) error {
	f.calls = append(f.calls, "start:"+service)
	if f.fail == "start" || f.fail == "env-start" {
		return errors.New("start")
	}
	f.backendRunning = true
	return nil
}
func (f *coordinatorFakes) rollbackContext() (context.Context, context.CancelFunc) {
	f.rollbackContexts++
	if f.fail == "rollback-cancel" || f.fail == "rollback-stop-cancel" {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx, func() {}
	}
	return context.WithTimeout(context.Background(), time.Second)
}
func (f *coordinatorFakes) Prove(_ context.Context, _ []string, _ string) error {
	f.calls = append(f.calls, "postgres-identity")
	if f.fail == "identity" || (f.fail == "rollback-identity" && f.replacements == 1) {
		return errors.New("identity")
	}
	return nil
}
func (f *coordinatorFakes) Reachable(_ context.Context, _ workspace.TargetDatabaseConfig) error {
	f.calls = append(f.calls, "postgres")
	if f.fail == "postgres" || (f.fail == "rollback-postgres" && f.replacements == 1) {
		return errors.New("postgres")
	}
	return nil
}
func (f *coordinatorFakes) Replace(_ context.Context, _ workspace.TargetDatabaseConfig, source ValidatedBackup) (DatabaseEvidence, bool, error) {
	f.replacements++
	f.sources = append(f.sources, source.targetRollback)
	f.calls = append(f.calls, map[bool]string{false: "replace:legacy", true: "replace:target"}[source.targetRollback])
	if f.fail == "pre-drop" && !source.targetRollback {
		return DatabaseEvidence{}, false, errors.New("replace")
	}
	if ((f.fail == "primary" || f.fail == "primary-rollback" || f.fail == "rollback-cancel" || f.fail == "primary-rollback-revalidate" || f.fail == "primary-rollback-unmutated" || f.fail == "rollback-identity" || f.fail == "rollback-postgres") && !source.targetRollback) || ((f.fail == "rollback" || f.fail == "primary-rollback") && source.targetRollback) {
		return DatabaseEvidence{}, true, errors.New("replace")
	}
	if !source.targetRollback {
		switch f.fail {
		case "restore-exit":
			return DatabaseEvidence{ConnectionOK: true, ApplicationTables: 1, PostgreSQLReachable: true}, true, nil
		case "connection":
			return DatabaseEvidence{RestoreExitOK: true, ApplicationTables: 1, PostgreSQLReachable: true}, true, nil
		case "postgres-evidence":
			return DatabaseEvidence{RestoreExitOK: true, ConnectionOK: true, ApplicationTables: 1}, true, nil
		case "zero-tables":
			return DatabaseEvidence{RestoreExitOK: true, ConnectionOK: true, PostgreSQLReachable: true}, true, nil
		}
	}
	return DatabaseEvidence{RestoreExitOK: true, ConnectionOK: true, ApplicationTables: 1, PostgreSQLReachable: true}, !(f.fail == "primary-rollback-unmutated" && source.targetRollback), nil
}
func (f *coordinatorFakes) WaitHealthy(_ context.Context, _ []string, _ string) error {
	f.calls = append(f.calls, "health")
	if f.fail == "health" || f.fail == "rollback-stop-error" || f.fail == "rollback-stop-cancel" || f.fail == "rollback-stop-ambiguous" {
		return errors.New("health")
	}
	return nil
}
func supportedRequest() RestoreRequest {
	return RestoreRequest{GOOS: "linux", GOARCH: "amd64", ComposeFiles: []string{"compose.yml"}, EnvFile: ".env", BackupDestination: "/backups", Legacy: backupRef("legacy")}
}
func backupRef(name string) BackupRef {
	return BackupRef{DumpPath: "/backups/" + name + ".dump", ManifestPath: "/backups/" + name + ".manifest", SHA256: name, Size: 1}
}
func coordinatorContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
