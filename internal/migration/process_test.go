package migration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

const processSecretSentinel = "synthetic-secret-process-boundary"

func TestCredentialTransportAndHelperDumpSpecKeepSecretOutsideObservableBoundary(t *testing.T) {
	config := testProcessConfig(processSecretSentinel)
	transport := CredentialTransport{TempRoot: t.TempDir()}
	credential, err := transport.Prepare(config)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	t.Cleanup(func() { _ = credential.Cleanup() })

	rootInfo, err := os.Stat(filepath.Dir(credential.HostPath()))
	if err != nil || rootInfo.Mode().Perm() != 0o700 {
		t.Fatalf("credential directory mode = %v, err = %v", rootInfo.Mode(), err)
	}
	fileInfo, err := os.Stat(credential.HostPath())
	if err != nil || fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("credential file mode = %v, err = %v", fileInfo.Mode(), err)
	}
	if got := string(mustReadFile(t, credential.HostPath())); !strings.Contains(got, processSecretSentinel) {
		t.Fatal("credential file did not receive the test password")
	}

	run, err := BuildHelperDump(HelperDumpRequest{
		GOOS:       "linux",
		Container:  ContainerIdentity{ID: strings.Repeat("a", 64), Image: PostgreSQL11Image},
		Config:     config,
		Credential: credential,
		Timeout:    30 * time.Second,
	})
	if err != nil {
		t.Fatalf("BuildHelperDump() error = %v", err)
	}
	if run.Image != PostgreSQL11Image || run.Name == "" || run.OperationID == "" {
		t.Fatalf("run identity = %#v", run)
	}
	if run.Spec.Name != "docker" || containsShell(run.Spec.Args) || containsSecret(fmt.Sprint(run, run.Spec)) {
		t.Fatalf("unsafe helper spec: %#v", run)
	}
	assertArgsContainInOrder(t, run.Spec.Args,
		"run", "--rm", "--pull=never", "--name", run.Name, "--label", HelperCleanupLabel+"=true", "--label", HelperOperationLabel+"="+run.OperationID,
		"--network", "host", "--mount", "type=bind,src="+credential.HostPath()+",dst="+ContainerPGPassPath+",readonly",
		"--env", "PGPASSFILE="+ContainerPGPassPath, string(PostgreSQL11Image),
		"pg_dump", "--format=custom", "--file=-", "--no-password", "--host="+config.Host,
		"--port=5432", "--username="+config.Username, "--dbname="+config.Database,
	)
	if got := run.CleanupSpec(); got.Name != "docker" || strings.Contains(fmt.Sprint(got), processSecretSentinel) || !equalStrings(got.Args, []string{"rm", "--force", run.Name}) {
		t.Fatalf("cleanup spec = %#v", got)
	}
	assertNoProcessLeak(t, run, run.Spec, run.CleanupSpec())
}

func TestHelperDumpRejectsUnpinnedImageUnsafeMountAndInvalidInputs(t *testing.T) {
	config := testProcessConfig(processSecretSentinel)
	transport := CredentialTransport{TempRoot: t.TempDir()}
	credential, err := transport.Prepare(config)
	if err != nil {
		t.Fatal(err)
	}
	defer credential.Cleanup()

	cases := []struct {
		name    string
		request HelperDumpRequest
	}{
		{"image must use approved exact pin", HelperDumpRequest{GOOS: "linux", Container: ContainerIdentity{ID: strings.Repeat("a", 64), Image: "postgres:11"}, Config: config, Credential: credential}},
		{"invalid container id", HelperDumpRequest{GOOS: "linux", Container: ContainerIdentity{ID: "short", Image: PostgreSQL11Image}, Config: config, Credential: credential}},
		{"credential mount must be protected", HelperDumpRequest{GOOS: "linux", Container: ContainerIdentity{ID: strings.Repeat("a", 64), Image: PostgreSQL11Image}, Config: config, Credential: CredentialFile{hostPath: "/tmp/unsafe"}}},
		{"invalid timeout", HelperDumpRequest{GOOS: "linux", Container: ContainerIdentity{ID: strings.Repeat("a", 64), Image: PostgreSQL11Image}, Config: config, Credential: credential, Timeout: -time.Second}},
		{"host networking is Linux-only", HelperDumpRequest{GOOS: "windows", Container: ContainerIdentity{ID: strings.Repeat("a", 64), Image: PostgreSQL11Image}, Config: config, Credential: credential}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := BuildHelperDump(tt.request); err == nil || containsSecret(err.Error()) {
				t.Fatalf("BuildHelperDump() error = %v", err)
			}
		})
	}
}

func TestOSBinaryExecutorCancelsTheDedicatedProcessGroup(t *testing.T) {
	if testing.Short() {
		t.Skip("process cancellation test")
	}
	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan ProcessResult, 1)
	go func() {
		resultCh <- OSBinaryExecutor{}.Run(ctx, ProcessSpec{Name: "sleep", Args: []string{"30"}, Timeout: time.Minute}, io.Discard)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case result := <-resultCh:
		if result.Outcome != ProcessCancelled {
			t.Fatalf("outcome = %#v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("process group was not terminated promptly")
	}
}

func TestCleanupHelperReconcilesNamedHelperAfterClientRecovery(t *testing.T) {
	executor := &fakeBinaryExecutor{stderr: []byte("db failed: " + processSecretSentinel)}
	run := HelperRun{Name: "alice-pg11-safe", OperationID: "operation", Spec: ProcessSpec{Name: "docker", Args: []string{"run"}, Timeout: time.Second}}
	var stdout bytes.Buffer
	result := executor.Run(context.Background(), run.Spec, &stdout)
	if result.Outcome != ProcessFailed || len(result.StderrCode) == 0 || containsSecret(fmt.Sprint(result, stdout.String())) {
		t.Fatalf("result = %#v stdout = %q", result, stdout.String())
	}
	if err := CleanupHelper(context.Background(), executor, run); err != nil {
		t.Fatalf("CleanupHelper() error = %v", err)
	}
	if len(executor.specs) != 2 || !equalStrings(executor.specs[1].Args, []string{"rm", "--force", run.Name}) {
		t.Fatalf("cleanup calls = %#v", executor.specs)
	}
	assertNoProcessLeak(t, executor.observed, executor.specs)
}

func TestRunHelperReconcilesAndRemovesCredentialsForEveryTerminalOutcome(t *testing.T) {
	for _, outcome := range []ProcessOutcome{ProcessSucceeded, ProcessFailed, ProcessTimedOut, ProcessCancelled} {
		t.Run(fmt.Sprintf("%d", outcome), func(t *testing.T) {
			credential, err := (CredentialTransport{TempRoot: t.TempDir()}).Prepare(testProcessConfig(processSecretSentinel))
			if err != nil {
				t.Fatal(err)
			}
			run := HelperRun{Name: "alice-pg11-safe", Spec: ProcessSpec{Name: "docker", Args: []string{"run"}}}
			executor := &terminalFakeExecutor{primary: ProcessResult{Outcome: outcome, StderrCode: "safe-code"}}

			got := RunHelper(context.Background(), executor, run, credential, io.Discard)

			if got.Outcome != outcome || got.StderrCode == processSecretSentinel {
				t.Fatalf("result = %#v", got)
			}
			if _, err := os.Stat(credential.HostPath()); !os.IsNotExist(err) {
				t.Fatalf("credential file remains after terminal outcome: %v", err)
			}
			if len(executor.specs) != 2 || !equalStrings(executor.specs[1].Args, []string{"rm", "--force", run.Name}) {
				t.Fatalf("cleanup calls = %#v", executor.specs)
			}
			assertNoProcessLeak(t, executor.specs, got)
		})
	}
}

func TestRunHelperReturnsRedactedTypedOutcomeWhenReconciliationFails(t *testing.T) {
	credential, err := (CredentialTransport{TempRoot: t.TempDir()}).Prepare(testProcessConfig(processSecretSentinel))
	if err != nil {
		t.Fatal(err)
	}
	run := HelperRun{Name: "alice-pg11-safe", Spec: ProcessSpec{Name: "docker", Args: []string{"run"}}}
	executor := &terminalFakeExecutor{primary: ProcessResult{Outcome: ProcessCancelled}, cleanup: ProcessResult{Outcome: ProcessFailed, StderrCode: processSecretSentinel}}

	got := RunHelper(context.Background(), executor, run, credential, io.Discard)

	if got != (ProcessResult{Outcome: ProcessCleanupFailed, StderrCode: "process-cleanup-failed"}) {
		t.Fatalf("result = %#v", got)
	}
	if _, err := os.Stat(credential.HostPath()); !os.IsNotExist(err) {
		t.Fatalf("credential file remains after failed reconciliation: %v", err)
	}
	assertNoProcessLeak(t, got, executor.specs)
}

func TestCleanupHelperDoesNotTreatInterruptedCleanupAsAbsence(t *testing.T) {
	run := HelperRun{Name: "alice-pg11-safe"}
	executor := &terminalFakeExecutor{cleanup: ProcessResult{Outcome: ProcessCancelled, StderrCode: "docker-container-absent"}}

	if err := CleanupHelper(context.Background(), executor, run); !errors.Is(err, ErrProcessPrecondition) {
		t.Fatalf("CleanupHelper() error = %v, want fail-closed precondition error", err)
	}
}

func TestRunHelperAcceptsDockerAutoRemovalAfterSuccessfulRun(t *testing.T) {
	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	docker := "#!/bin/sh\n" +
		"if [ \"$1\" = run ]; then printf 'archive'; exit 0; fi\n" +
		"if [ \"$1\" = rm ]; then echo 'Error response from daemon: No such container: alice-pg11-safe' >&2; exit 1; fi\n" +
		"exit 2\n"
	if err := os.WriteFile(dockerPath, []byte(docker), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	credential, err := (CredentialTransport{TempRoot: t.TempDir()}).Prepare(testProcessConfig(processSecretSentinel))
	if err != nil {
		t.Fatal(err)
	}
	run := HelperRun{
		Name: "alice-pg11-safe",
		Spec: ProcessSpec{Name: "docker", Args: []string{"run", "--rm", "--name", "alice-pg11-safe", "safe-image"}},
	}

	result := RunHelper(context.Background(), OSBinaryExecutor{}, run, credential, io.Discard)

	if result.Outcome != ProcessSucceeded {
		t.Fatalf("auto-removed helper outcome = %#v, want success", result)
	}
	if _, err := os.Stat(credential.HostPath()); !os.IsNotExist(err) {
		t.Fatalf("credential file remains after helper completion: %v", err)
	}
}

func TestOSBinaryExecutorTimesOutAndBoundsStderr(t *testing.T) {
	if testing.Short() {
		t.Skip("process timeout test")
	}
	stderr := &boundedStderr{limit: 4096}
	if _, err := stderr.Write(bytes.Repeat([]byte("x"), 8192)); err != nil || len(stderr.data) != 4096 {
		t.Fatalf("stderr bytes = %d, err = %v", len(stderr.data), err)
	}
	var stdout bytes.Buffer
	result := OSBinaryExecutor{}.Run(context.Background(), ProcessSpec{
		Name: "sh", Args: []string{"-c", "printf 'x%.0s' $(seq 1 8192) >&2; sleep 30"}, Timeout: 20 * time.Millisecond,
	}, &stdout)
	if result.Outcome != ProcessTimedOut || result.StderrCode != "process-timeout" {
		t.Fatalf("result = %#v", result)
	}
}

func TestOSBinaryExecutorCancellationTerminatesDescendantProcessGroup(t *testing.T) {
	if testing.Short() {
		t.Skip("process-tree cancellation test")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stdout lockedBuffer
	resultCh := make(chan ProcessResult, 1)
	go func() {
		resultCh <- OSBinaryExecutor{}.Run(ctx, ProcessSpec{Name: "sh", Args: []string{"-c", "sleep 30 & echo $!; wait"}, Timeout: time.Minute}, &stdout)
	}()
	var pid int
	deadline := time.After(time.Second)
	for pid == 0 {
		if _, err := fmt.Sscanf(strings.TrimSpace(stdout.String()), "%d", &pid); err == nil && pid > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("descendant PID was not emitted")
		case <-time.After(time.Millisecond):
		}
	}
	cancel()
	if result := <-resultCh; result.Outcome != ProcessCancelled {
		t.Fatalf("result = %#v", result)
	}
	deadline = time.After(time.Second)
	for {
		if err := syscall.Kill(pid, 0); err == syscall.ESRCH {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("descendant process %d still exists after group cancellation", pid)
		case <-time.After(time.Millisecond):
		}
	}
}

type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

type terminalFakeExecutor struct {
	primary ProcessResult
	cleanup ProcessResult
	specs   []ProcessSpec
}

func (f *terminalFakeExecutor) Run(_ context.Context, spec ProcessSpec, _ io.Writer) ProcessResult {
	f.specs = append(f.specs, spec)
	if len(spec.Args) > 0 && spec.Args[0] == "rm" {
		if f.cleanup.Outcome != ProcessSucceeded || f.cleanup.StderrCode != "" {
			return f.cleanup
		}
		return ProcessResult{Outcome: ProcessSucceeded}
	}
	return f.primary
}

func testProcessConfig(password string) ResolvedConfig {
	return ResolvedConfig{Environment: EnvironmentProduction, Dialect: DialectPostgreSQL, Database: "alice", Username: "guardian", Host: "127.0.0.1", Port: 5432, password: newSecret(password)}
}
func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func containsShell(args []string) bool {
	for _, arg := range args {
		if arg == "sh" || arg == "-c" {
			return true
		}
	}
	return false
}
func containsSecret(v string) bool { return strings.Contains(v, processSecretSentinel) }
func assertNoProcessLeak(t *testing.T, values ...any) {
	t.Helper()
	if rendered := fmt.Sprint(values...); containsSecret(rendered) {
		t.Fatalf("secret escaped observable boundary: %q", rendered)
	}
}
func assertArgsContainInOrder(t *testing.T, got []string, want ...string) {
	t.Helper()
	if !equalStrings(got, want) {
		t.Fatalf("argv = %#v\nwant = %#v", got, want)
	}
}
func equalStrings(a, b []string) bool {
	return len(a) == len(b) && func() bool {
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}()
}

type fakeBinaryExecutor struct {
	specs    []ProcessSpec
	stderr   []byte
	observed string
}

func (f *fakeBinaryExecutor) Run(_ context.Context, spec ProcessSpec, stdout io.Writer) ProcessResult {
	f.specs = append(f.specs, spec)
	f.observed = fmt.Sprint(spec)
	if len(spec.Args) > 0 && spec.Args[0] == "rm" {
		return ProcessResult{Outcome: ProcessSucceeded}
	}
	_, _ = stdout.Write([]byte("binary\x00dump"))
	return ProcessResult{Outcome: ProcessFailed, StderrCode: "process-failed"}
}
