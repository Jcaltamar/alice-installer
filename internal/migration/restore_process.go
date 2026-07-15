package migration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jcaltamar/alice-installer/internal/workspace"
)

const (
	PostgreSQLClientImage ImageIdentity = "postgres:16-alpine"
	validationOutputLimit               = 128
)

var ErrReplacementPrecondition = errors.New("restore replacement precondition failed")

// PrepareTargetCredential bridges a generated target config to the existing
// protected credential transport without exposing password bytes to migration.
func PrepareTargetCredential(transport CredentialTransport, config workspace.TargetDatabaseConfig) (CredentialFile, error) {
	parent := transport.TempRoot
	if parent == "" {
		parent = os.TempDir()
	}
	root, err := os.MkdirTemp(parent, "alice-pgpass-")
	if err != nil {
		return CredentialFile{}, ErrReplacementPrecondition
	}
	if err := os.Chmod(root, 0o700); err != nil {
		_ = os.RemoveAll(root)
		return CredentialFile{}, ErrReplacementPrecondition
	}
	path := filepath.Join(root, "pgpass")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		err = config.WritePGPass(file)
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
	}
	if err != nil || os.Chmod(path, 0o600) != nil {
		_ = os.RemoveAll(root)
		return CredentialFile{}, ErrReplacementPrecondition
	}
	fileInfo, err := os.Lstat(path)
	if err != nil {
		_ = os.RemoveAll(root)
		return CredentialFile{}, ErrReplacementPrecondition
	}
	uid, gid, ok := numericOwner(fileInfo)
	if !ok {
		_ = os.RemoveAll(root)
		return CredentialFile{}, ErrReplacementPrecondition
	}
	return CredentialFile{hostPath: path, root: root, ownerUID: uid, ownerGID: gid}, nil
}

func BuildTargetBackupDump(config workspace.TargetDatabaseConfig, credential CredentialFile, operationID string) (ProcessSpec, error) {
	if !targetConfigValid(config) || !safeCredential(credential) || operationID == "" {
		return ProcessSpec{}, ErrReplacementPrecondition
	}
	return ProcessSpec{Name: "docker", Args: []string{
		"run", "--rm", "--pull=never", "--network", "host",
		"--label", HelperCleanupLabel + "=true", "--label", HelperOperationLabel + "=" + operationID,
		"--mount", "type=bind,src=" + credential.hostPath + ",dst=" + ContainerPGPassPath + ",readonly",
		"--env", "PGPASSFILE=" + ContainerPGPassPath, string(PostgreSQLClientImage),
		"pg_dump", "--format=custom", "--file=-", "--no-password",
		"--host=" + config.Host, fmt.Sprintf("--port=%d", config.Port), "--username=" + config.User, "--dbname=" + config.Database,
	}, Timeout: defaultProcessTimeout}, nil
}

// PostgreSQLReachabilityAdapter proves that the expected PostgreSQL endpoint accepts
// a fresh authenticated connection without exposing credentials or diagnostics.
type PostgreSQLReachabilityAdapter struct {
	Executor BinaryExecutor
	Prepare  func(context.Context, workspace.TargetDatabaseConfig) (CredentialFile, error)
	Timeout  time.Duration
}

func (a PostgreSQLReachabilityAdapter) Reachable(ctx context.Context, cfg workspace.TargetDatabaseConfig) error {
	if a.Executor == nil || a.Prepare == nil || !targetConfigValid(cfg) || ctx.Err() != nil {
		return ErrReplacementPrecondition
	}
	credential, err := a.Prepare(ctx, cfg)
	if err != nil {
		return ErrReplacementPrecondition
	}
	defer credential.Cleanup()
	if !safeCredential(credential) {
		return ErrReplacementPrecondition
	}
	timeout := a.Timeout
	if timeout == 0 {
		timeout = defaultProcessTimeout
	}
	spec := ProcessSpec{Name: "docker", Args: []string{
		"run", "--rm", "--pull=never", "--network", "host",
		"--mount", "type=bind,src=" + credential.hostPath + ",dst=" + ContainerPGPassPath + ",readonly",
		"--env", "PGPASSFILE=" + ContainerPGPassPath, string(PostgreSQLClientImage),
		"psql", "--dbname=postgres", "--tuples-only", "--no-align", "--no-password",
		"--host=" + cfg.Host, fmt.Sprintf("--port=%d", cfg.Port), "--username=" + cfg.User, "--command=SELECT 1",
	}, Timeout: timeout}
	var output boundedOutput
	if result := a.Executor.Run(ctx, spec, &output); result.Outcome != ProcessSucceeded || ctx.Err() != nil || strings.TrimSpace(output.buffer.String()) != "1" {
		return ErrReplacementPrecondition
	}
	return nil
}

type TargetReplacementRequest struct {
	Config     workspace.TargetDatabaseConfig
	Credential CredentialFile
	DumpPath   string
	Timeout    time.Duration
}

// TargetReplacementAdapter connects the coordinator to the reviewed direct-argv
// execution boundary. Prepare owns the private credential transport at composition.
type TargetReplacementAdapter struct {
	Executor BinaryExecutor
	Prepare  func(context.Context, workspace.TargetDatabaseConfig) (CredentialFile, error)
	Timeout  time.Duration
}

func (a TargetReplacementAdapter) Replace(ctx context.Context, cfg workspace.TargetDatabaseConfig, source ValidatedBackup) (DatabaseEvidence, bool, error) {
	if a.Executor == nil || a.Prepare == nil {
		return DatabaseEvidence{}, false, ErrReplacementPrecondition
	}
	credential, err := a.Prepare(ctx, cfg)
	if err != nil {
		return DatabaseEvidence{}, false, ErrReplacementPrecondition
	}
	plan, err := BuildTargetReplacement(TargetReplacementRequest{Config: cfg, Credential: credential, DumpPath: source.ref.DumpPath, Timeout: a.Timeout})
	if err != nil {
		_ = credential.Cleanup()
		return DatabaseEvidence{}, false, ErrReplacementPrecondition
	}
	result := RunTargetReplacement(ctx, a.Executor, plan, credential)
	if result.Process.Outcome != ProcessSucceeded {
		return DatabaseEvidence{}, result.Mutated, ErrReplacementPrecondition
	}
	evidence, ok := parseValidationEvidence(result.validationOutput)
	if !ok {
		return DatabaseEvidence{}, result.Mutated, ErrReplacementPrecondition
	}
	return evidence, result.Mutated, nil
}

// BuildTargetReplacement returns only direct Docker argv. SQL is fixed, mounted
// read-only, and receives the validated database identifier through psql -v.
func BuildTargetReplacement(request TargetReplacementRequest) (ReplacementPlan, error) {
	if !targetConfigValid(request.Config) || !safeCredential(request.Credential) || !safeDumpPath(request.DumpPath) || request.Timeout < 0 {
		return ReplacementPlan{}, ErrReplacementPrecondition
	}
	if err := writeReplacementSQL(request.Credential.root); err != nil {
		return ReplacementPlan{}, ErrReplacementPrecondition
	}
	timeout := request.Timeout
	if timeout == 0 {
		timeout = defaultProcessTimeout
	}
	common := replacementCommon(request.Credential, request.DumpPath)
	maintenance := []string{"--host=" + request.Config.Host, fmt.Sprintf("--port=%d", request.Config.Port), "--username=" + request.Config.User, "--no-password"}
	return ReplacementPlan{
		specs: [5]ProcessSpec{
			replacementSpec(common, timeout, "psql", append([]string{"--dbname=postgres", "--set=ON_ERROR_STOP=1", "-v", "target_db=" + request.Config.Database, "-f", "/run/alice-installer/terminate.sql"}, maintenance...)...),
			replacementSpec(common, timeout, "dropdb", append([]string{"--if-exists", "--force", "--maintenance-db=postgres"}, append(maintenance, request.Config.Database)...)...),
			replacementSpec(common, timeout, "createdb", append([]string{"--maintenance-db=postgres"}, append(maintenance, request.Config.Database)...)...),
			replacementSpec(common, timeout, "pg_restore", append([]string{"--exit-on-error", "--no-owner", "--no-privileges"}, append(maintenance, "--dbname="+request.Config.Database, "/run/alice-installer/legacy.dump")...)...),
			replacementSpec(common, timeout, "psql", append([]string{"--dbname=" + request.Config.Database, "--set=ON_ERROR_STOP=1", "--tuples-only", "--no-align", "-v", "target_db=" + request.Config.Database, "-f", "/run/alice-installer/validate.sql"}, maintenance...)...),
		},
	}, nil
}

func targetConfigValid(c workspace.TargetDatabaseConfig) bool {
	return c.Host == "127.0.0.1" && c.Port > 0 && replacementIdentifier(c.User) && replacementIdentifier(c.Database) && c.Database != "postgres"
}
func replacementIdentifier(value string) bool {
	if len(value) == 0 || len(value) > 63 {
		return false
	}
	for i := range value {
		c := value[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || (i > 0 && c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}
func safeDumpPath(path string) bool {
	return filepath.IsAbs(path) && filepath.Base(path) != "." && !strings.Contains(path, "..")
}
func replacementCommon(credential CredentialFile, dumpPath string) []string {
	return []string{"run", "--rm", "--pull=never", "--network", "host", "--mount", "type=bind,src=" + credential.hostPath + ",dst=" + ContainerPGPassPath + ",readonly", "--mount", "type=bind,src=" + filepath.Join(credential.root, "terminate.sql") + ",dst=/run/alice-installer/terminate.sql,readonly", "--mount", "type=bind,src=" + filepath.Join(credential.root, "validate.sql") + ",dst=/run/alice-installer/validate.sql,readonly", "--mount", "type=bind,src=" + dumpPath + ",dst=/run/alice-installer/legacy.dump,readonly", "--env", "PGPASSFILE=" + ContainerPGPassPath, string(PostgreSQLClientImage)}
}
func replacementSpec(common []string, timeout time.Duration, command string, args ...string) ProcessSpec {
	argv := append(append([]string{}, common...), command)
	return ProcessSpec{Name: "docker", Args: append(argv, args...), Timeout: timeout}
}
func writeReplacementSQL(root string) error {
	files := map[string]string{
		"terminate.sql": "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = :'target_db' AND pid <> pg_backend_pid();\n",
		"validate.sql":  "SELECT current_database() = :'target_db'; SELECT count(*) FROM pg_tables WHERE schemaname NOT IN ('pg_catalog','information_schema','pg_toast'); SELECT true;\n",
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// ReplacementStep is stable, coordinator-observable execution evidence.
type ReplacementStep uint8

const (
	ReplacementTerminateSessions ReplacementStep = iota
	ReplacementDropDatabase
	ReplacementCreateDatabase
	ReplacementRestoreArchive
	ReplacementValidate
)

type ReplacementResult struct {
	Process          ProcessResult
	FailedStep       ReplacementStep
	Mutated          bool
	validationOutput string
}

// ReplacementPlan fixes the only executable step order to the reviewed builder.
// Specs returns a copy for exact, secret-free observability in tests and evidence.
type ReplacementPlan struct{ specs [5]ProcessSpec }

func (p ReplacementPlan) Specs() []ProcessSpec {
	specs := make([]ProcessSpec, len(p.specs))
	for i, spec := range p.specs {
		specs[i] = spec
		specs[i].Args = append([]string(nil), spec.Args...)
	}
	return specs
}

// RunTargetReplacement executes ordered direct-argv steps. Mutated becomes true
// immediately before dropdb is submitted; no command output crosses this boundary.
func RunTargetReplacement(ctx context.Context, executor BinaryExecutor, plan ReplacementPlan, credential CredentialFile) (result ReplacementResult) {
	defer func() {
		panicked := recover() != nil
		cleanupErr := credential.Cleanup()
		if panicked || cleanupErr != nil {
			result.Process = ProcessResult{Outcome: ProcessCleanupFailed, StderrCode: "restore-cleanup-failed"}
		}
	}()
	if executor == nil {
		return ReplacementResult{Process: ProcessResult{Outcome: ProcessFailed, StderrCode: "restore-process-precondition"}}
	}
	var validation boundedOutput
	for step, spec := range plan.specs {
		if err := ctx.Err(); err != nil {
			return ReplacementResult{Process: ProcessResult{Outcome: ProcessCancelled, StderrCode: "restore-cancelled"}, FailedStep: ReplacementStep(step), Mutated: result.Mutated}
		}
		if step == int(ReplacementDropDatabase) {
			result.Mutated = true
		}
		stdout := io.Writer(io.Discard)
		if step == int(ReplacementValidate) {
			stdout = &validation
		}
		process := executor.Run(ctx, spec, stdout)
		if step == int(ReplacementValidate) && validation.Len() > validationOutputLimit {
			return ReplacementResult{Process: ProcessResult{Outcome: ProcessFailed, StderrCode: "restore-validation-output"}, FailedStep: ReplacementStep(step), Mutated: result.Mutated}
		}
		if process.Outcome != ProcessSucceeded {
			return ReplacementResult{Process: ProcessResult{Outcome: process.Outcome, StderrCode: "restore-process-failed"}, FailedStep: ReplacementStep(step), Mutated: result.Mutated}
		}
	}
	result.Process = ProcessResult{Outcome: ProcessSucceeded}
	result.validationOutput = validation.buffer.String()
	return result
}

func parseValidationEvidence(output string) (DatabaseEvidence, bool) {
	lines := strings.Fields(output)
	if len(lines) != 3 || lines[0] != "t" || lines[2] != "t" {
		return DatabaseEvidence{}, false
	}
	tables, err := strconv.ParseUint(lines[1], 10, 64)
	if err != nil || tables == 0 {
		return DatabaseEvidence{}, false
	}
	return DatabaseEvidence{RestoreExitOK: true, ConnectionOK: true, ApplicationTables: tables, PostgreSQLReachable: true}, true
}

// boundedOutput retains only the small, numeric validation protocol.
type boundedOutput struct{ buffer bytes.Buffer }

func (o *boundedOutput) Write(p []byte) (int, error) {
	remaining := validationOutputLimit - o.buffer.Len()
	if remaining <= 0 {
		return 0, io.ErrShortWrite
	}
	if len(p) > remaining {
		_, _ = o.buffer.Write(p[:remaining])
		return remaining, io.ErrShortWrite
	}
	return o.buffer.Write(p)
}

func (o *boundedOutput) Len() int { return o.buffer.Len() }
