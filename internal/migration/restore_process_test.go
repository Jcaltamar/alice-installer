package migration

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jcaltamar/alice-installer/internal/workspace"
)

const replacementSecret = "replace-secret-sentinel"

func TestBuildTargetReplacementUsesDirectSecretFreeArgv(t *testing.T) {
	credential := replacementCredential(t)
	defer credential.Cleanup()
	cfg := workspace.TargetDatabaseConfig{Host: "127.0.0.1", Port: 5432, User: "alice", Database: "alice"}
	runs, err := BuildTargetReplacement(TargetReplacementRequest{Config: cfg, Credential: credential, DumpPath: "/safe/legacy.dump", Timeout: time.Minute})
	if err != nil {
		t.Fatalf("BuildTargetReplacement() error = %v", err)
	}
	if len(runs.Specs()) != 5 {
		t.Fatalf("runs = %d, want 5", len(runs.Specs()))
	}
	for _, run := range runs.Specs() {
		joined := strings.Join(append([]string{run.Name}, run.Args...), " ")
		if run.Name != "docker" || strings.Contains(joined, replacementSecret) || strings.Contains(joined, "PGPASSWORD") || strings.Contains(joined, "sh -c") || strings.Contains(joined, "bash -c") {
			t.Fatalf("unsafe process spec: %q", joined)
		}
		if !strings.Contains(joined, "--pull=never") || !strings.Contains(joined, "--network host") || !strings.Contains(joined, "PGPASSFILE="+ContainerPGPassPath) {
			t.Fatalf("missing protected execution boundary: %q", joined)
		}
	}
	common := []string{"run", "--rm", "--pull=never", "--network", "host", "--mount", "type=bind,src=" + credential.hostPath + ",dst=" + ContainerPGPassPath + ",readonly", "--mount", "type=bind,src=" + credential.root + "/terminate.sql,dst=/run/alice-installer/terminate.sql,readonly", "--mount", "type=bind,src=" + credential.root + "/validate.sql,dst=/run/alice-installer/validate.sql,readonly", "--mount", "type=bind,src=/safe/legacy.dump,dst=/run/alice-installer/legacy.dump,readonly", "--env", "PGPASSFILE=" + ContainerPGPassPath, string(PostgreSQLClientImage)}
	maintenance := []string{"--host=127.0.0.1", "--port=5432", "--username=alice", "--no-password"}
	want := [][]string{
		append(append(append([]string{}, common...), "psql", "--dbname=postgres", "--set=ON_ERROR_STOP=1", "-v", "target_db=alice", "-f", "/run/alice-installer/terminate.sql"), maintenance...),
		append(append(append([]string{}, common...), "dropdb", "--if-exists", "--force", "--maintenance-db=postgres"), append(maintenance, "alice")...),
		append(append(append([]string{}, common...), "createdb", "--maintenance-db=postgres"), append(maintenance, "alice")...),
		append(append(append([]string{}, common...), "pg_restore", "--exit-on-error", "--no-owner", "--no-privileges"), append(maintenance, "--dbname=alice", "/run/alice-installer/legacy.dump")...),
		append(append(append([]string{}, common...), "psql", "--dbname=alice", "--set=ON_ERROR_STOP=1", "--tuples-only", "--no-align", "-v", "target_db=alice", "-f", "/run/alice-installer/validate.sql"), maintenance...),
	}
	for i := range want {
		if fmt.Sprintf("%q", runs.Specs()[i].Args) != fmt.Sprintf("%q", want[i]) {
			t.Fatalf("argv[%d] = %#v, want %#v", i, runs.Specs()[i].Args, want[i])
		}
	}
}

func TestBuildTargetReplacementRejectsUnsafeInputs(t *testing.T) {
	credential := replacementCredential(t)
	defer credential.Cleanup()
	valid := workspace.TargetDatabaseConfig{Host: "127.0.0.1", Port: 5432, User: "alice", Database: "alice"}
	for _, request := range []TargetReplacementRequest{{Config: valid, Credential: credential}, {Config: workspace.TargetDatabaseConfig{Host: "127.0.0.1", Port: 5432, User: "alice", Database: "bad-name"}, Credential: credential, DumpPath: "/safe/a.dump"}, {Config: valid, DumpPath: "/safe/a.dump"}} {
		if _, err := BuildTargetReplacement(request); err == nil {
			t.Fatal("BuildTargetReplacement() accepted unsafe request")
		}
	}
}

func TestRunTargetReplacementCleansCredentialOnTerminalOutcome(t *testing.T) {
	for _, outcome := range []ProcessOutcome{ProcessSucceeded, ProcessFailed, ProcessCancelled, ProcessTimedOut} {
		t.Run(fmt.Sprint(outcome), func(t *testing.T) {
			credential := replacementCredential(t)
			runs, err := BuildTargetReplacement(TargetReplacementRequest{Config: workspace.TargetDatabaseConfig{Host: "127.0.0.1", Port: 5432, User: "alice", Database: "alice"}, Credential: credential, DumpPath: "/safe/a.dump"})
			if err != nil {
				t.Fatal(err)
			}
			result := RunTargetReplacement(context.Background(), &replacementExecutor{outcome: outcome}, runs, credential)
			if result.Process.Outcome != outcome {
				t.Fatalf("result = %#v", result)
			}
			if _, err := os.Stat(credential.HostPath()); !os.IsNotExist(err) {
				t.Fatalf("credential still exists after cleanup: %v", err)
			}
		})
	}
}

func TestRunTargetReplacementClassifiesCleanupFailure(t *testing.T) {
	credential := replacementCredential(t)
	defer credential.Cleanup()
	runs, err := BuildTargetReplacement(TargetReplacementRequest{Config: workspace.TargetDatabaseConfig{Host: "127.0.0.1", Port: 5432, User: "alice", Database: "alice"}, Credential: credential, DumpPath: "/safe/a.dump"})
	if err != nil {
		t.Fatal(err)
	}
	result := RunTargetReplacement(context.Background(), &replacementExecutor{outcome: ProcessSucceeded}, runs, CredentialFile{root: "\x00"})
	if result.Process != (ProcessResult{Outcome: ProcessCleanupFailed, StderrCode: "restore-cleanup-failed"}) {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunTargetReplacementCleansCredentialAfterPanic(t *testing.T) {
	credential := replacementCredential(t)
	runs, err := BuildTargetReplacement(TargetReplacementRequest{Config: workspace.TargetDatabaseConfig{Host: "127.0.0.1", Port: 5432, User: "alice", Database: "alice"}, Credential: credential, DumpPath: "/safe/a.dump"})
	if err != nil {
		t.Fatal(err)
	}
	result := RunTargetReplacement(context.Background(), &replacementExecutor{panic: true}, runs, credential)
	if result.Process != (ProcessResult{Outcome: ProcessCleanupFailed, StderrCode: "restore-cleanup-failed"}) {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Stat(credential.HostPath()); !os.IsNotExist(err) {
		t.Fatalf("credential still exists after panic: %v", err)
	}
}

type replacementExecutor struct {
	outcome ProcessOutcome
	panic   bool
	fail    bool
	failAt  int
	output  string
	calls   int
	specs   []ProcessSpec
}

func (e *replacementExecutor) Run(_ context.Context, spec ProcessSpec, stdout io.Writer) ProcessResult {
	e.specs = append(e.specs, spec)
	if e.panic {
		panic("fake")
	}
	if e.fail && e.calls == e.failAt {
		e.calls++
		return ProcessResult{Outcome: ProcessFailed}
	}
	if e.calls == int(ReplacementValidate) || e.output != "" {
		_, _ = io.WriteString(stdout, e.output)
	}
	e.calls++
	return ProcessResult{Outcome: e.outcome}
}

func replacementCredential(t *testing.T) CredentialFile {
	t.Helper()
	config := testProcessConfig(replacementSecret)
	credential, err := (CredentialTransport{TempRoot: t.TempDir()}).Prepare(config)
	if err != nil {
		t.Fatal(err)
	}
	return credential
}
func TestCredentialTransportPreparesTargetConfigWithoutLeakingPassword(t *testing.T) {
	cfg := targetRestoreConfig(t)
	credential, err := PrepareTargetCredential(CredentialTransport{TempRoot: t.TempDir()}, cfg)
	if err != nil {
		t.Fatalf("PrepareTarget() error = %v", err)
	}
	defer credential.Cleanup()
	data, err := os.ReadFile(credential.HostPath())
	if err != nil || !strings.Contains(string(data), replacementSecret) {
		t.Fatalf("credential contents/error = %q/%v", data, err)
	}
	plan, err := BuildTargetReplacement(TargetReplacementRequest{Config: cfg, Credential: credential, DumpPath: "/safe/legacy.dump"})
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range plan.Specs() {
		if strings.Contains(strings.Join(spec.Args, " "), replacementSecret) {
			t.Fatalf("secret leaked into argv: %q", spec.Args)
		}
	}
}

func targetRestoreConfig(t *testing.T) workspace.TargetDatabaseConfig {
	t.Helper()
	path := t.TempDir() + "/.env"
	if err := os.WriteFile(path, []byte("POSTGRES_HOST=127.0.0.1\nPOSTGRES_PORT=5432\nPOSTGRES_USER=alice\nPOSTGRES_PASSWORD="+replacementSecret+"\nPOSTGRES_DATABASE=alice\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := (workspace.TargetEnvFileReader{}).ReadTargetDatabase(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestPostgreSQLReachabilityAdapterRequiresExactSecretFreeEvidence(t *testing.T) {
	cfg := targetRestoreConfig(t)
	for _, tc := range []struct {
		name    string
		outcome ProcessOutcome
		output  string
		wantErr bool
	}{
		{name: "reachable", outcome: ProcessSucceeded, output: "1\n"},
		{name: "malformed", outcome: ProcessSucceeded, output: "true\n", wantErr: true},
		{name: "failed", outcome: ProcessFailed, wantErr: true},
		{name: "cancelled", outcome: ProcessCancelled, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			credential := replacementCredential(t)
			executor := &replacementExecutor{outcome: tc.outcome, output: tc.output}
			probe := PostgreSQLReachabilityAdapter{Executor: executor, Prepare: func(context.Context, workspace.TargetDatabaseConfig) (CredentialFile, error) { return credential, nil }}
			err := probe.Reachable(context.Background(), cfg)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Reachable() error = %v, wantErr %t", err, tc.wantErr)
			}
			if len(executor.specs) != 1 || strings.Contains(strings.Join(executor.specs[0].Args, " "), replacementSecret) || strings.Contains(strings.Join(executor.specs[0].Args, " "), "PGPASSWORD") {
				t.Fatalf("probe spec = %#v", executor.specs)
			}
			if _, statErr := os.Stat(credential.HostPath()); !os.IsNotExist(statErr) {
				t.Fatalf("credential remains after probe: %v", statErr)
			}
		})
	}
}

func TestReplacementPlanSpecsCannotChangeExecutedPlan(t *testing.T) {
	credential := replacementCredential(t)
	plan, err := BuildTargetReplacement(TargetReplacementRequest{Config: workspace.TargetDatabaseConfig{Host: "127.0.0.1", Port: 5432, User: "alice", Database: "alice"}, Credential: credential, DumpPath: "/safe/a.dump"})
	if err != nil {
		t.Fatal(err)
	}
	view := plan.Specs()
	view[0].Args[0] = "unsafe-reordered-command"
	executor := &replacementExecutor{outcome: ProcessSucceeded}
	if result := RunTargetReplacement(context.Background(), executor, plan, credential); result.Process.Outcome != ProcessSucceeded {
		t.Fatalf("result = %#v", result)
	}
	if got := executor.specs[0].Args[0]; got != "run" {
		t.Fatalf("executed argv[0] = %q, want immutable plan", got)
	}
}

func TestRunTargetReplacementRecordsBuiltSpecsAtEveryFailureBoundary(t *testing.T) {
	for step, wantMutation := range map[int]bool{0: false, 1: true, 2: true, 3: true, 4: true} {
		t.Run(fmt.Sprint(step), func(t *testing.T) {
			credential := replacementCredential(t)
			plan, err := BuildTargetReplacement(TargetReplacementRequest{Config: workspace.TargetDatabaseConfig{Host: "127.0.0.1", Port: 5432, User: "alice", Database: "alice"}, Credential: credential, DumpPath: "/safe/a.dump"})
			if err != nil {
				t.Fatal(err)
			}
			executor := &replacementExecutor{fail: true, failAt: step}
			result := RunTargetReplacement(context.Background(), executor, plan, credential)
			if result.FailedStep != ReplacementStep(step) || result.Mutated != wantMutation || result.Process.StderrCode != "restore-process-failed" {
				t.Fatalf("step %d result = %#v", step, result)
			}
			if got, want := fmt.Sprintf("%#v", executor.specs), fmt.Sprintf("%#v", plan.Specs()[:step+1]); got != want {
				t.Fatalf("executed specs = %s, want %s", got, want)
			}
		})
	}
}
