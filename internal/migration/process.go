package migration

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	// PostgreSQL11Image is the only approved helper image identity. --pull=never
	// prevents silently replacing this reviewed deployment pin during an operation.
	ContainerPGPassPath   = "/run/alice-installer/pgpass"
	HelperCleanupLabel    = "alice-installer.migration-helper"
	HelperOperationLabel  = "alice-installer.migration-operation"
	defaultProcessTimeout = 30 * time.Minute
)

var ErrProcessPrecondition = errors.New("migration process precondition failed")

type CredentialTransport struct{ TempRoot string }

type CredentialFile struct {
	hostPath, root     string
	ownerUID, ownerGID int
}

func (f CredentialFile) HostPath() string { return f.hostPath }
func (f CredentialFile) Cleanup() error {
	if f.root == "" {
		return nil
	}
	return os.RemoveAll(f.root)
}

// Prepare creates the only secret-bearing host artifact. Its path is not exported
// through plans or results; it is mounted read-only at a fixed container path.
func (t CredentialTransport) Prepare(config ResolvedConfig) (CredentialFile, error) {
	defer config.Release()
	if !validConfig(config) || config.password.storage == nil || len(config.password.storage.value) == 0 {
		return CredentialFile{}, ErrProcessPrecondition
	}
	parent := t.TempRoot
	if parent == "" {
		parent = os.TempDir()
	}
	root, err := os.MkdirTemp(parent, "alice-pgpass-")
	if err != nil {
		return CredentialFile{}, ErrProcessPrecondition
	}
	if err := os.Chmod(root, 0o700); err != nil {
		_ = os.RemoveAll(root)
		return CredentialFile{}, ErrProcessPrecondition
	}
	path := filepath.Join(root, "pgpass")
	content := strings.Join([]string{pgpassField(config.Host), fmt.Sprintf("%d", config.Port), pgpassField(config.Database), pgpassField(config.Username), pgpassField(string(config.password.storage.value))}, ":") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		_ = os.RemoveAll(root)
		return CredentialFile{}, ErrProcessPrecondition
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = os.RemoveAll(root)
		return CredentialFile{}, ErrProcessPrecondition
	}
	fileInfo, err := os.Lstat(path)
	if err != nil {
		_ = os.RemoveAll(root)
		return CredentialFile{}, ErrProcessPrecondition
	}
	uid, gid, ok := numericOwner(fileInfo)
	if !ok {
		_ = os.RemoveAll(root)
		return CredentialFile{}, ErrProcessPrecondition
	}
	return CredentialFile{hostPath: path, root: root, ownerUID: uid, ownerGID: gid}, nil
}
func pgpassField(value string) string {
	return strings.NewReplacer("\\", "\\\\", ":", "\\:").Replace(value)
}

// HelperDumpRequest creates a non-secret direct Docker argv. Linux host networking
// is deliberate: the resolved legacy endpoint is used without rewriting it.
type HelperDumpRequest struct {
	GOOS       string
	Container  ContainerIdentity
	Config     ResolvedConfig
	Credential CredentialFile
	Timeout    time.Duration
}
type HelperRun struct {
	Name, OperationID string
	Image             ImageIdentity
	Spec              ProcessSpec
}

func (r HelperRun) CleanupSpec() ProcessSpec {
	return ProcessSpec{Name: "docker", Args: []string{"rm", "--force", r.Name}, Timeout: 30 * time.Second}
}

func BuildHelperDump(request HelperDumpRequest) (HelperRun, error) {
	if request.GOOS != "linux" || request.Container.Image != PostgreSQL11Image || !fullContainerID.MatchString(request.Container.ID) || !validConfig(request.Config) || request.Timeout < 0 || !safeCredential(request.Credential) {
		return HelperRun{}, ErrProcessPrecondition
	}
	timeout := request.Timeout
	if timeout == 0 {
		timeout = defaultProcessTimeout
	}
	id, err := randomToken()
	if err != nil {
		return HelperRun{}, ErrProcessPrecondition
	}
	name := "alice-pg11-" + id
	mount := "type=bind,src=" + request.Credential.hostPath + ",dst=" + ContainerPGPassPath + ",readonly"
	return HelperRun{Name: name, OperationID: id, Image: PostgreSQL11Image, Spec: ProcessSpec{Name: "docker", Args: []string{
		"run", "--rm", "--pull=never", "--name", name,
		"--label", HelperCleanupLabel + "=true", "--label", HelperOperationLabel + "=" + id,
		"--network", "host", "--mount", mount, "--env", "PGPASSFILE=" + ContainerPGPassPath,
		"--user", fmt.Sprintf("%d:%d", request.Credential.ownerUID, request.Credential.ownerGID),
		string(PostgreSQL11Image), "pg_dump", "--format=custom", "--no-password",
		"--host=" + request.Config.Host, fmt.Sprintf("--port=%d", request.Config.Port), "--username=" + request.Config.Username, "--dbname=" + request.Config.Database,
	}, Timeout: timeout}}, nil
}
func validConfig(c ResolvedConfig) bool {
	return c.Dialect == DialectPostgreSQL && c.Host != "" && c.Port > 0 && c.Database != "" && c.Username != ""
}
func safeCredential(f CredentialFile) bool {
	if f.root == "" || f.hostPath != filepath.Join(f.root, "pgpass") || !filepath.IsAbs(f.root) || f.ownerUID < 0 || f.ownerGID < 0 {
		return false
	}
	dir, err := os.Lstat(f.root)
	if err != nil || !dir.IsDir() || dir.Mode()&os.ModeSymlink != 0 || dir.Mode().Perm() != 0o700 || !ownedBy(dir, f.ownerUID, f.ownerGID) {
		return false
	}
	file, err := os.Lstat(f.hostPath)
	return err == nil && file.Mode().IsRegular() && file.Mode()&os.ModeSymlink == 0 && file.Mode().Perm() == 0o600 && ownedBy(file, f.ownerUID, f.ownerGID)
}
func randomToken() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}

type ProcessSpec struct {
	Name    string
	Args    []string
	Timeout time.Duration
}
type ProcessOutcome uint8

const (
	ProcessSucceeded ProcessOutcome = iota
	ProcessFailed
	ProcessCancelled
	ProcessTimedOut
	ProcessCleanupFailed
)

type ProcessResult struct {
	Outcome    ProcessOutcome
	StderrCode string
}
type BinaryExecutor interface {
	Run(context.Context, ProcessSpec, io.Writer) ProcessResult
}

// OSBinaryExecutor streams stdout directly and kills the command's process group
// on caller cancellation or timeout. Stderr never crosses this typed boundary raw.
type OSBinaryExecutor struct{}

func (OSBinaryExecutor) Run(ctx context.Context, spec ProcessSpec, stdout io.Writer) ProcessResult {
	if spec.Name == "" || len(spec.Args) == 0 || stdout == nil {
		return ProcessResult{Outcome: ProcessFailed, StderrCode: "process-precondition"}
	}
	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = defaultProcessTimeout
	}
	processCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.Command(spec.Name, spec.Args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = stdout
	stderr := &boundedStderr{limit: 4096}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return ProcessResult{Outcome: ProcessFailed, StderrCode: "process-start"}
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	select {
	case err := <-wait:
		if err != nil {
			return ProcessResult{Outcome: ProcessFailed, StderrCode: classifyStderr(stderr)}
		}
		return ProcessResult{Outcome: ProcessSucceeded}
	case <-processCtx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-wait
		if errors.Is(processCtx.Err(), context.DeadlineExceeded) {
			return ProcessResult{Outcome: ProcessTimedOut, StderrCode: "process-timeout"}
		}
		return ProcessResult{Outcome: ProcessCancelled, StderrCode: "process-cancelled"}
	}
}

// CleanupHelper is explicit idempotent named cleanup for a client that regains
// control after interruption; docker run --rm remains the normal cleanup path.
func CleanupHelper(ctx context.Context, executor BinaryExecutor, run HelperRun) error {
	if executor == nil || run.Name == "" {
		return ErrProcessPrecondition
	}
	result := executor.Run(ctx, run.CleanupSpec(), io.Discard)
	if result.Outcome == ProcessSucceeded || result.Outcome == ProcessFailed && result.StderrCode == "docker-container-absent" {
		return nil
	}
	return ErrProcessPrecondition
}

// RunHelper owns every terminal cleanup path: reconciliation is attempted even
// after timeout or cancellation, using an independent bounded context.
func RunHelper(ctx context.Context, executor BinaryExecutor, run HelperRun, credential CredentialFile, stdout io.Writer) ProcessResult {
	if executor == nil || run.Name == "" || stdout == nil {
		return ProcessResult{Outcome: ProcessFailed, StderrCode: "process-precondition"}
	}
	result := executor.Run(ctx, run.Spec, stdout)
	cleanupCtx, cancel := context.WithTimeout(context.Background(), run.CleanupSpec().Timeout)
	defer cancel()
	cleanupErr := CleanupHelper(cleanupCtx, executor, run)
	credentialErr := credential.Cleanup()
	if cleanupErr != nil || credentialErr != nil {
		return ProcessResult{Outcome: ProcessCleanupFailed, StderrCode: "process-cleanup-failed"}
	}
	return result
}

type boundedStderr struct {
	mu    sync.Mutex
	data  []byte
	limit int
}

func (b *boundedStderr) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.data) < b.limit {
		remaining := b.limit - len(b.data)
		if remaining > len(p) {
			remaining = len(p)
		}
		b.data = append(b.data, p[:remaining]...)
	}
	return len(p), nil
}
func classifyStderr(stderr *boundedStderr) string {
	stderr.mu.Lock()
	defer stderr.mu.Unlock()
	if bytes.Contains(stderr.data, []byte("No such container:")) {
		return "docker-container-absent"
	}
	if bytes.Contains(stderr.data, []byte("a password is required")) || bytes.Contains(stderr.data, []byte("not allowed to execute")) || bytes.Contains(stderr.data, []byte("permission denied")) {
		return SudoDockerPermissionCode
	}
	return "process-failed"
}
