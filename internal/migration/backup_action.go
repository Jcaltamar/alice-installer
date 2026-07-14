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

var ErrBackupPrecondition = errors.New("backup precondition failed")

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
	Message      string
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

// Preflight does not create directories, locks, staging files, credentials, or processes.
func (a BackupAction) Preflight(ctx context.Context, request BackupRequest) (BackupPlan, error) {
	if ctx.Err() != nil || a.Resolver == nil || a.Inspector == nil || a.Store == nil {
		return BackupPlan{}, ErrBackupPrecondition
	}
	config, err := a.Resolver.Resolve(ctx, ConfigRequest{Environment: request.ConfigEnvironment})
	if err != nil || !validConfig(config) {
		return BackupPlan{}, ErrBackupPrecondition
	}
	if err := ctx.Err(); err != nil {
		return BackupPlan{}, ErrBackupPrecondition
	}
	container, err := a.Inspector.Discover(ctx, config)
	if err != nil || container.Image != PostgreSQL11Image || !fullContainerID.MatchString(container.ID) {
		return BackupPlan{}, ErrBackupPrecondition
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
	if ctx.Err() != nil {
		return BackupResult{Outcome: BackupCancelled, Message: "backup cancelled"}
	}
	if a.Store == nil || a.Executor == nil || a.Validator == nil || !validConfig(plan.config) || plan.container.Image != PostgreSQL11Image || !fullContainerID.MatchString(plan.container.ID) {
		return BackupResult{Outcome: BackupPreconditionFailed, Message: "backup precondition failed"}
	}
	staged, err := a.Store.Prepare(ctx, plan.destination)
	if err != nil {
		return BackupResult{Outcome: BackupDestinationFailed, Message: "backup destination failed"}
	}
	keep := false
	defer func() {
		if !keep {
			_ = staged.Cleanup()
		}
	}()
	if ctx.Err() != nil {
		return BackupResult{Outcome: BackupCancelled, Message: "backup cancelled"}
	}
	credential, err := a.Transport.Prepare(plan.config)
	if err != nil {
		return BackupResult{Outcome: BackupPreconditionFailed, Message: "backup precondition failed"}
	}
	run, err := BuildHelperDump(HelperDumpRequest{GOOS: plan.goos, Container: plan.container, Config: plan.config, Credential: credential, Timeout: plan.timeout})
	if err != nil {
		_ = credential.Cleanup()
		return BackupResult{Outcome: BackupPreconditionFailed, Message: "backup precondition failed"}
	}
	if ctx.Err() != nil {
		_ = credential.Cleanup()
		return BackupResult{Outcome: BackupCancelled, Message: "backup cancelled"}
	}
	result := RunHelper(ctx, a.Executor, run, credential, staged)
	if result.Outcome == ProcessCancelled {
		return BackupResult{Outcome: BackupCancelled, Message: "backup cancelled"}
	}
	if result.Outcome == ProcessTimedOut {
		return BackupResult{Outcome: BackupTimedOut, Message: "backup timed out"}
	}
	if result.Outcome != ProcessSucceeded {
		return BackupResult{Outcome: BackupDumpFailed, Message: "backup dump failed"}
	}
	if ctx.Err() != nil {
		return BackupResult{Outcome: BackupCancelled, Message: "backup cancelled"}
	}
	if err := staged.Sync(); err != nil {
		return BackupResult{Outcome: BackupDestinationFailed, Message: "backup destination failed"}
	}
	if err := staged.Close(); err != nil {
		return BackupResult{Outcome: BackupDestinationFailed, Message: "backup destination failed"}
	}
	if info, err := os.Stat(staged.Path()); err != nil || info.Size() == 0 {
		return BackupResult{Outcome: BackupDumpFailed, Message: "backup dump failed"}
	}
	if ctx.Err() != nil {
		return BackupResult{Outcome: BackupCancelled, Message: "backup cancelled"}
	}
	if err := a.Validator.Validate(ctx, staged.Path()); err != nil {
		return BackupResult{Outcome: BackupValidationFailed, Message: "backup validation failed"}
	}
	if ctx.Err() != nil {
		return BackupResult{Outcome: BackupCancelled, Message: "backup cancelled"}
	}
	publication, err := PublishBackupPair(ctx, ownedPublicationRequest(PublicationRequest{Directory: plan.destination.Directory(), DumpPath: staged.Path(), Container: plan.container, Config: plan.config}, staged.Path()))
	if err != nil {
		return BackupResult{Outcome: BackupDestinationFailed, Message: "backup destination failed"}
	}
	keep = true
	return BackupResult{Outcome: BackupValidated, DumpPath: publication.DumpPath, ManifestPath: publication.ManifestPath, SHA256: publication.SHA256, Size: publication.Size, Message: "backup validated"}
}
