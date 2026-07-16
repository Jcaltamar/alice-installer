package migration

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

const LegacyApplicationRoot = "/opt/backend_alice_guardian/node"

var (
	ErrBackupPrecondition      = errors.New("backup precondition failed")
	ErrBackupEngineUnavailable = errors.New("backup engine unavailable")
	ErrResolvedConfigInvalid   = errors.New("resolved database configuration invalid")
	ErrLegacyContainerInvalid  = errors.New("legacy container identity invalid")
)

type BackupOutcome uint8

const (
	BackupStaged BackupOutcome = iota // retained only as the pre-publication internal boundary
	BackupValidated
	BackupCancelled
	BackupPreconditionFailed
	BackupDestinationFailed
	BackupDumpFailed
	BackupTimedOut
	BackupValidationFailed
)

type BackupStage uint8

const (
	BackupStagePreconditions BackupStage = iota
	BackupStageDestination
	BackupStageCredentials
	BackupStageDump
	BackupStageStagedFile
	BackupStageArchiveValidation
	BackupStagePublication
)

func (s BackupStage) String() string {
	labels := [...]string{
		"Backup preconditions",
		"Destination preparation",
		"Credential preparation",
		"Dump execution",
		"Staged file sync and non-empty check",
		"Archive validation",
		"Publication, checksum, and manifest",
	}
	if int(s) >= len(labels) {
		return "Unknown stage"
	}
	return labels[s]
}

type BackupStageStatus uint8

const (
	BackupStageNotRun BackupStageStatus = iota
	BackupStagePassed
	BackupStageFailed
)

func (s BackupStageStatus) String() string {
	if s == BackupStagePassed {
		return "passed"
	}
	if s == BackupStageFailed {
		return "failed"
	}
	return "not run"
}

type BackupStageResult struct {
	Stage  BackupStage
	Status BackupStageStatus
}

type BackupFailureCode uint8

const (
	BackupFailureNone BackupFailureCode = iota
	BackupFailurePrecondition
	BackupFailureEngineUnavailable
	BackupFailureResolvedConfigInvalid
	BackupFailureLegacyContainerInvalid
	BackupFailureLegacyContainerImageUntrusted
	BackupFailureLegacyContainerIdentityMismatch
	BackupFailureLegacyContainerEndpointMismatch
	BackupFailureLegacyContainerUnsafe
	BackupFailureLegacyContainerAmbiguous
	BackupFailureCancelled
	BackupFailureDestination
	BackupFailureCredentials
	BackupFailureHelperPrecondition
	BackupFailureDumpTimeout
	BackupFailureDump
	BackupFailureHelperCleanup
	BackupFailureStagedSync
	BackupFailureStagedClose
	BackupFailureStagedEmpty
	BackupFailureArchiveValidation
	BackupFailurePublication
	BackupFailureSudoDockerPermission
	BackupFailureLegacyContainerStopped
)

func (c BackupFailureCode) String() string {
	codes := [...]string{"", "backup-precondition", "backup-engine-unavailable", "backup-resolved-config-invalid", "backup-legacy-container-invalid", "backup-legacy-container-image-untrusted", "backup-legacy-container-identity-mismatch", "backup-legacy-container-endpoint-mismatch", "backup-legacy-container-unsafe", "backup-legacy-container-ambiguous", "backup-cancelled", "backup-destination", "backup-credentials", "backup-helper-precondition", "backup-dump-timeout", "backup-dump", "backup-helper-cleanup", "backup-staged-sync", "backup-staged-close", "backup-staged-empty", "backup-archive-validation", "backup-publication", "backup-sudo-docker-permission", "backup-legacy-container-stopped"}
	if int(c) >= len(codes) || c == BackupFailureNone {
		return "backup-failed"
	}
	return codes[c]
}

type BackupRemediation uint8

const (
	BackupRemediationNone BackupRemediation = iota
	BackupRemediationRetry
	BackupRemediationPrerequisites
	BackupRemediationDestination
	BackupRemediationCredentials
	BackupRemediationHelperImage
	BackupRemediationDatabase
	BackupRemediationDocker
	BackupRemediationStorage
	BackupRemediationArchive
	BackupRemediationRestartLegacyContainer
)

func (r BackupRemediation) String() string {
	hints := [...]string{
		"Review backup prerequisites and retry; do not continue migration.",
		"Retry the backup when ready.",
		"Verify the legacy database and backup destination, then retry.",
		"Verify destination permissions and free space, then retry.",
		"Verify local credential-file permissions, then retry.",
		"Verify the approved PostgreSQL helper image is available, then retry.",
		"Verify database availability and retry the backup.",
		"Verify Docker and the legacy database are available, then retry.",
		"Verify destination storage health and free space, then retry.",
		"Verify the pinned PostgreSQL helper image and retry; do not continue migration.",
		"Restart and verify the legacy PostgreSQL master, then retry the backup; the installer will not start it automatically.",
	}
	if int(r) >= len(hints) {
		return hints[0]
	}
	return hints[r]
}

type BackupRequest struct {
	Destination       string
	SourceRoots       []string
	ConfigEnvironment string
}

// BackupPlan is created entirely from read-only preflight evidence. Its state is
// private so callers cannot substitute a different config, container, or destination after review.
type BackupPlan struct {
	config      ResolvedConfig
	container   ContainerIdentity
	destination DestinationPlan
	timeout     time.Duration
	goos        string
}

// BackupReview is the immutable, allowlisted pre-confirmation summary.
// It deliberately excludes credentials, pgpass details, and raw configuration.
type BackupReview struct {
	Environment EnvironmentName
	Endpoint    string
	Database    string
	User        string
	ContainerID string
	Image       ImageIdentity
	Destination string
}

func (p BackupPlan) Destination() string { return p.destination.Directory() }
func (p BackupPlan) ContainerID() string { return p.container.ID }

// Review returns only facts that are safe to display before confirmation.
func (p BackupPlan) Review() BackupReview {
	return BackupReview{
		Environment: p.config.Environment,
		Endpoint:    net.JoinHostPort(p.config.Host, fmt.Sprintf("%d", p.config.Port)),
		Database:    p.config.Database,
		User:        p.config.Username,
		ContainerID: p.container.ID,
		Image:       p.container.Image,
		Destination: p.destination.Directory(),
	}
}

func (p BackupPlan) Config() ResolvedConfig {
	return ResolvedConfig{Environment: p.config.Environment, Dialect: p.config.Dialect, Database: p.config.Database, Username: p.config.Username, Host: p.config.Host, Port: p.config.Port, Sources: cloneSources(p.config.Sources)}
}
func cloneSources(sources map[string]ValueSource) map[string]ValueSource {
	result := make(map[string]ValueSource, len(sources))
	for key, value := range sources {
		result[key] = value
	}
	return result
}

type BackupResult struct {
	Outcome      BackupOutcome
	DumpPath     string
	ManifestPath string
	SHA256       string
	Size         int64
	Stages       []BackupStageResult
	FailureCode  BackupFailureCode
	Remediation  BackupRemediation
}

func newBackupDiagnostics() []BackupStageResult {
	stages := make([]BackupStageResult, int(BackupStagePublication)+1)
	for stage := range stages {
		stages[stage].Stage = BackupStage(stage)
	}
	return stages
}

func backupFailure(outcome BackupOutcome, stages []BackupStageResult, failed BackupStage, code BackupFailureCode, remediation BackupRemediation) BackupResult {
	stages[failed].Status = BackupStageFailed
	return BackupResult{Outcome: outcome, Stages: stages, FailureCode: code, Remediation: remediation}
}

func BackupPreflightFailureResult(err error) BackupResult {
	code, stage, remediation := BackupFailurePrecondition, BackupStagePreconditions, BackupRemediationPrerequisites
	switch {
	case errors.Is(err, ErrSudoDockerPermission):
		code = BackupFailureSudoDockerPermission
	case errors.Is(err, ErrBackupEngineUnavailable):
		code = BackupFailureEngineUnavailable
	case errors.Is(err, ErrResolvedConfigInvalid):
		code = BackupFailureResolvedConfigInvalid
	case errors.Is(err, ErrNoExactImageCandidate):
		code = BackupFailureLegacyContainerImageUntrusted
	case errors.Is(err, ErrContainerIdentity):
		code = BackupFailureLegacyContainerIdentityMismatch
	case errors.Is(err, ErrContainerEndpoint):
		code = BackupFailureLegacyContainerEndpointMismatch
	case errors.Is(err, ErrContainerUnsafeState):
		code = BackupFailureLegacyContainerUnsafe
	case errors.Is(err, ErrContainerStopped):
		code, remediation = BackupFailureLegacyContainerStopped, BackupRemediationRestartLegacyContainer
	case errors.Is(err, ErrAmbiguousContainer):
		code = BackupFailureLegacyContainerAmbiguous
	case errors.Is(err, ErrLegacyContainerInvalid):
		code = BackupFailureLegacyContainerInvalid
	case errors.Is(err, ErrDestinationUnsafe), errors.Is(err, ErrDestinationSpace), errors.Is(err, ErrDestinationLocked), errors.Is(err, ErrDestinationFailure):
		code, stage, remediation = BackupFailureDestination, BackupStageDestination, BackupRemediationDestination
	}
	return backupFailure(BackupPreconditionFailed, newBackupDiagnostics(), stage, code, remediation)
}

func (r BackupResult) String() string {
	return fmt.Sprintf("backup outcome=%d dump=%s manifest=%s size=%d", r.Outcome, r.DumpPath, r.ManifestPath, r.Size)
}

type StaticConfigResolver interface {
	Resolve(context.Context, ConfigRequest) (ResolvedConfig, error)
}
type ContainerDiscovery interface {
	Discover(context.Context, ResolvedConfig) (ContainerIdentity, error)
}

type InspectorDiscovery struct{ Inspector ContainerInspector }

func (d InspectorDiscovery) Discover(ctx context.Context, config ResolvedConfig) (ContainerIdentity, error) {
	return DiscoverContainer(ctx, d.Inspector, config)
}

type BackupAction struct {
	Resolver  StaticConfigResolver
	Inspector ContainerDiscovery
	Store     DestinationStore
	Executor  BinaryExecutor
	Transport CredentialTransport
	Validator ArchiveValidator
	GOOS      string
	Timeout   time.Duration
}

type BackupProgressStage uint8

const (
	BackupProgressPreparing BackupProgressStage = iota
	BackupProgressDumping
	BackupProgressSyncing
	BackupProgressValidating
	BackupProgressPublishing
)

// Preflight does not create directories, locks, staging files, credentials, or processes.
func (a BackupAction) Preflight(ctx context.Context, request BackupRequest) (BackupPlan, error) {
	if ctx.Err() != nil {
		return BackupPlan{}, ErrBackupPrecondition
	}
	if a.Resolver == nil || a.Inspector == nil || a.Store == nil {
		return BackupPlan{}, ErrBackupEngineUnavailable
	}
	config, err := a.Resolver.Resolve(ctx, ConfigRequest{Environment: request.ConfigEnvironment})
	if err != nil || !validConfig(config) {
		return BackupPlan{}, ErrResolvedConfigInvalid
	}
	if err := ctx.Err(); err != nil {
		return BackupPlan{}, ErrBackupPrecondition
	}
	container, err := a.Inspector.Discover(ctx, config)
	if err != nil {
		return BackupPlan{}, fmt.Errorf("%w: %w", ErrLegacyContainerInvalid, err)
	}
	if container.Image != PostgreSQL11Image || !fullContainerID.MatchString(container.ID) {
		return BackupPlan{}, ErrLegacyContainerInvalid
	}
	if err := ctx.Err(); err != nil {
		return BackupPlan{}, ErrBackupPrecondition
	}
	sources := append([]string(nil), request.SourceRoots...)
	sources = append(sources, filepath.Clean(LegacyApplicationRoot))
	destination, err := a.Store.Preflight(ctx, DestinationRequest{Directory: request.Destination, SourceRoots: sources})
	if err != nil {
		return BackupPlan{}, ErrBackupPrecondition
	}
	return BackupPlan{config: config, container: container, destination: destination, timeout: a.Timeout, goos: a.GOOS}, nil
}

// Run is intentionally limited to unvalidated staging. Slice 4.5 owns archive
// validation, checksum, manifest construction, and every final rename.
func (a BackupAction) Run(ctx context.Context, plan BackupPlan) BackupResult {
	return a.RunWithProgress(ctx, plan, nil)
}

func (a BackupAction) RunWithProgress(ctx context.Context, plan BackupPlan, progress func(BackupProgressStage)) BackupResult {
	stages := newBackupDiagnostics()
	if ctx.Err() != nil {
		return backupFailure(BackupCancelled, stages, BackupStagePreconditions, BackupFailureCancelled, BackupRemediationRetry)
	}
	if a.Store == nil || a.Executor == nil || a.Validator == nil {
		return backupFailure(BackupPreconditionFailed, stages, BackupStagePreconditions, BackupFailureEngineUnavailable, BackupRemediationPrerequisites)
	}
	if !validConfig(plan.config) {
		return backupFailure(BackupPreconditionFailed, stages, BackupStagePreconditions, BackupFailureResolvedConfigInvalid, BackupRemediationPrerequisites)
	}
	if plan.container.Image != PostgreSQL11Image || !fullContainerID.MatchString(plan.container.ID) {
		return backupFailure(BackupPreconditionFailed, stages, BackupStagePreconditions, BackupFailureLegacyContainerInvalid, BackupRemediationPrerequisites)
	}
	stages[BackupStagePreconditions].Status = BackupStagePassed
	emitBackupProgress(progress, BackupProgressPreparing)
	staged, err := a.Store.Prepare(ctx, plan.destination)
	if err != nil {
		return backupFailure(BackupDestinationFailed, stages, BackupStageDestination, BackupFailureDestination, BackupRemediationDestination)
	}
	stages[BackupStageDestination].Status = BackupStagePassed
	keep := false
	defer func() {
		if !keep {
			_ = staged.Cleanup()
		}
	}()
	if ctx.Err() != nil {
		return backupFailure(BackupCancelled, stages, BackupStageCredentials, BackupFailureCancelled, BackupRemediationRetry)
	}
	credential, err := a.Transport.Prepare(plan.config)
	if err != nil {
		return backupFailure(BackupPreconditionFailed, stages, BackupStageCredentials, BackupFailureCredentials, BackupRemediationCredentials)
	}
	stages[BackupStageCredentials].Status = BackupStagePassed
	run, err := BuildHelperDump(HelperDumpRequest{GOOS: plan.goos, Container: plan.container, Config: plan.config, Credential: credential, Timeout: plan.timeout})
	if err != nil {
		_ = credential.Cleanup()
		return backupFailure(BackupPreconditionFailed, stages, BackupStageDump, BackupFailureHelperPrecondition, BackupRemediationHelperImage)
	}
	if ctx.Err() != nil {
		_ = credential.Cleanup()
		return backupFailure(BackupCancelled, stages, BackupStageDump, BackupFailureCancelled, BackupRemediationRetry)
	}
	emitBackupProgress(progress, BackupProgressDumping)
	result := RunHelper(ctx, a.Executor, run, credential, staged)
	if result.Outcome == ProcessCancelled {
		return backupFailure(BackupCancelled, stages, BackupStageDump, BackupFailureCancelled, BackupRemediationRetry)
	}
	if result.Outcome == ProcessTimedOut {
		return backupFailure(BackupTimedOut, stages, BackupStageDump, BackupFailureDumpTimeout, BackupRemediationDatabase)
	}
	if result.Outcome != ProcessSucceeded {
		code := BackupFailureDump
		if result.Outcome == ProcessCleanupFailed {
			code = BackupFailureHelperCleanup
		}
		return backupFailure(BackupDumpFailed, stages, BackupStageDump, code, BackupRemediationDocker)
	}
	stages[BackupStageDump].Status = BackupStagePassed
	emitBackupProgress(progress, BackupProgressSyncing)
	if ctx.Err() != nil {
		return backupFailure(BackupCancelled, stages, BackupStageStagedFile, BackupFailureCancelled, BackupRemediationRetry)
	}
	if err := staged.Sync(); err != nil {
		return backupFailure(BackupDestinationFailed, stages, BackupStageStagedFile, BackupFailureStagedSync, BackupRemediationStorage)
	}
	if err := staged.Close(); err != nil {
		return backupFailure(BackupDestinationFailed, stages, BackupStageStagedFile, BackupFailureStagedClose, BackupRemediationStorage)
	}
	if info, err := os.Stat(staged.Path()); err != nil || info.Size() == 0 {
		return backupFailure(BackupDumpFailed, stages, BackupStageStagedFile, BackupFailureStagedEmpty, BackupRemediationStorage)
	}
	stages[BackupStageStagedFile].Status = BackupStagePassed
	emitBackupProgress(progress, BackupProgressValidating)
	if ctx.Err() != nil {
		return backupFailure(BackupCancelled, stages, BackupStageArchiveValidation, BackupFailureCancelled, BackupRemediationRetry)
	}
	if err := a.Validator.Validate(ctx, staged.Path()); err != nil {
		return backupFailure(BackupValidationFailed, stages, BackupStageArchiveValidation, BackupFailureArchiveValidation, BackupRemediationArchive)
	}
	stages[BackupStageArchiveValidation].Status = BackupStagePassed
	emitBackupProgress(progress, BackupProgressPublishing)
	if ctx.Err() != nil {
		return backupFailure(BackupCancelled, stages, BackupStagePublication, BackupFailureCancelled, BackupRemediationRetry)
	}
	publication, err := PublishBackupPair(ctx, ownedPublicationRequest(PublicationRequest{Directory: plan.destination.Directory(), DumpPath: staged.Path(), Container: plan.container, Config: plan.config}, staged.Path()))
	if err != nil {
		return backupFailure(BackupDestinationFailed, stages, BackupStagePublication, BackupFailurePublication, BackupRemediationStorage)
	}
	stages[BackupStagePublication].Status = BackupStagePassed
	keep = true
	return BackupResult{Outcome: BackupValidated, DumpPath: publication.DumpPath, ManifestPath: publication.ManifestPath, SHA256: publication.SHA256, Size: publication.Size, Stages: stages}
}

func emitBackupProgress(progress func(BackupProgressStage), stage BackupProgressStage) {
	if progress != nil {
		progress(stage)
	}
}
