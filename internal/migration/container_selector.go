package migration

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"sort"
)

const PostgreSQL11Image ImageIdentity = "bitnami/postgresql:11-debian-10"

func canonicalPostgreSQL11Image(reference string) (ImageIdentity, bool) {
	switch reference {
	case string(PostgreSQL11Image), "docker.io/bitnami/postgresql:11-debian-10":
		return PostgreSQL11Image, true
	default:
		return "", false
	}
}

var (
	ErrContainerPrecondition = errors.New("legacy database container precondition failed")
	ErrAmbiguousContainer    = errors.New("legacy database container is ambiguous")
	ErrNoExactImageCandidate = errors.New("no exact-image legacy database container candidate")
	ErrContainerIdentity     = errors.New("legacy database container identity mismatch")
	ErrContainerEndpoint     = errors.New("legacy database container endpoint mismatch")
	ErrContainerUnsafeState  = errors.New("legacy database container has unsafe state or provenance")
)

type ImageIdentity string

type ContainerState uint8

const (
	ContainerUnknown ContainerState = iota
	ContainerRunning
	ContainerStopped
)

type HealthStatus uint8

const (
	HealthNone HealthStatus = iota
	HealthHealthy
	HealthUnhealthy
	HealthUnknown
)

// ContainerSummary is the untrusted, minimal result of listing all Docker containers.
type ContainerSummary struct {
	ID    string
	Image string
}

// ContainerEndpoint is a declared endpoint. Only ContainerLocal endpoints can be used for backup.
type ContainerEndpoint struct {
	Host           string
	Port           int
	ContainerLocal bool
}

// PublishedPortBinding is the bounded subset of Docker port data used for loopback correlation.
type PublishedPortBinding struct {
	HostIP        string
	HostPort      int
	ContainerPort int
}

// SafeContainerLabels is the allowlist used for correlation; no arbitrary Docker labels escape inspection.
type SafeContainerLabels struct {
	ComposeProject string
	ComposeService string
}

// ContainerDetails contains only non-secret, allowlisted metadata required by the selector.
type ContainerDetails struct {
	ID             string
	Image          string
	Digest         string
	State          ContainerState
	Health         HealthStatus
	NetworkMode    string
	DatabaseNames  []string
	Usernames      []string
	Endpoints      []ContainerEndpoint
	PublishedPorts []PublishedPortBinding
	MountKinds     []string
	Labels         SafeContainerLabels
}

// ContainerIdentity is the immutable, safe result of successful correlation.
type ContainerIdentity struct {
	ID     string
	Image  ImageIdentity
	Digest string
}

// ContainerInspector is migration-specific. Implementations must list stopped containers and never expose env maps.
type ContainerInspector interface {
	Candidates(context.Context, ImageIdentity) ([]ContainerSummary, error)
	Inspect(context.Context, string) (ContainerDetails, error)
}

var fullContainerID = regexp.MustCompile(`^[a-f0-9]{64}$`)

// DiscoverContainer inspects every exact-image candidate and selects only one fully corroborated identity.
func DiscoverContainer(ctx context.Context, inspector ContainerInspector, config ResolvedConfig) (ContainerIdentity, error) {
	if ctx.Err() != nil || inspector == nil || config.Host == "" || config.Port < 1 || config.Database == "" || config.Username == "" {
		return ContainerIdentity{}, ErrContainerPrecondition
	}
	candidates, err := inspector.Candidates(ctx, PostgreSQL11Image)
	if err != nil {
		return ContainerIdentity{}, ErrContainerPrecondition
	}
	if len(candidates) == 0 {
		return ContainerIdentity{}, classifiedContainerError(ErrNoExactImageCandidate)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	// Inspect every list candidate before making any decision: partial inspection is not evidence.
	detailsByID := make(map[string]ContainerDetails, len(candidates))
	inspectionFailed := false
	for _, candidate := range candidates {
		if _, trusted := canonicalPostgreSQL11Image(candidate.Image); !trusted || !fullContainerID.MatchString(candidate.ID) {
			inspectionFailed = true
			continue
		}
		details, inspectErr := inspector.Inspect(ctx, candidate.ID)
		if inspectErr != nil {
			inspectionFailed = true
			continue
		}
		detailsByID[candidate.ID] = details
	}
	if inspectionFailed {
		return ContainerIdentity{}, ErrContainerPrecondition
	}
	matches := make([]ContainerIdentity, 0, len(candidates))
	identityMismatch, endpointMismatch, unsafeCandidate := false, false, false
	for _, candidate := range candidates {
		details := detailsByID[candidate.ID]
		corroborated, reason := correlateCandidate(details, config)
		identityMismatch = identityMismatch || reason == ErrContainerIdentity
		endpointMismatch = endpointMismatch || reason == ErrContainerEndpoint
		unsafeCandidate = unsafeCandidate || reason == ErrContainerUnsafeState
		if corroborated {
			matches = append(matches, ContainerIdentity{ID: details.ID, Image: PostgreSQL11Image, Digest: details.Digest})
		}
	}
	if len(matches) == 0 {
		switch {
		case unsafeCandidate:
			return ContainerIdentity{}, classifiedContainerError(ErrContainerUnsafeState)
		case endpointMismatch:
			return ContainerIdentity{}, classifiedContainerError(ErrContainerEndpoint)
		case identityMismatch:
			return ContainerIdentity{}, classifiedContainerError(ErrContainerIdentity)
		default:
			return ContainerIdentity{}, ErrContainerPrecondition
		}
	}
	if len(matches) != 1 {
		return ContainerIdentity{}, ErrAmbiguousContainer
	}
	return matches[0], nil
}

// correlateCandidate distinguishes insufficient unrelated evidence from unsafe contradictions.
// It is called only after every exact-image candidate has been inspected successfully.
func correlateCandidate(details ContainerDetails, config ResolvedConfig) (bool, error) {
	if _, trusted := canonicalPostgreSQL11Image(details.Image); !fullContainerID.MatchString(details.ID) || !trusted {
		return false, ErrContainerIdentity
	}
	if !contains(details.DatabaseNames, config.Database) || !contains(details.Usernames, config.Username) {
		return false, ErrContainerIdentity
	}
	endpointMatches, endpointConflicts := containerLocalEndpointEvidence(details.Endpoints, config.Host, config.Port)
	if loopbackFamily(config.Host) != 0 {
		endpointMatches = endpointMatches || publishedEndpointEvidence(details.PublishedPorts, config.Host, config.Port)
	}
	if !endpointMatches {
		return false, ErrContainerEndpoint
	}
	if endpointConflicts || details.State != ContainerRunning || details.Health == HealthUnhealthy || details.Health == HealthUnknown {
		return false, ErrContainerUnsafeState
	}
	if len(details.MountKinds) == 0 && details.Labels.ComposeProject == "" && details.Labels.ComposeService == "" {
		return false, ErrContainerUnsafeState
	}
	return true, nil
}

func classifiedContainerError(reason error) error {
	return fmt.Errorf("%w: %w", ErrContainerPrecondition, reason)
}

func containerLocalEndpointEvidence(endpoints []ContainerEndpoint, host string, port int) (matches, conflicts bool) {
	for _, endpoint := range endpoints {
		if !endpoint.ContainerLocal || endpoint.Host != host {
			continue
		}
		if endpoint.Port == port {
			matches = true
			continue
		}
		conflicts = true
	}
	return matches, conflicts
}

func publishedEndpointEvidence(bindings []PublishedPortBinding, host string, hostPort int) bool {
	family := loopbackFamily(host)
	if family == 0 {
		return false
	}
	for _, binding := range bindings {
		if binding.HostPort == hostPort && binding.ContainerPort == 5432 && bindingReachableFromLoopback(binding.HostIP, family) {
			return true
		}
	}
	return false
}

// loopbackFamily returns 4, 6, or 46 for localhost, which may resolve to either family.
func loopbackFamily(host string) int {
	if host == "localhost" {
		return 46
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return 0
	}
	if ip.To4() != nil {
		return 4
	}
	return 6
}

func bindingReachableFromLoopback(hostIP string, family int) bool {
	ip := net.ParseIP(hostIP)
	if ip == nil || !ip.IsUnspecified() && !ip.IsLoopback() {
		return false
	}
	bindingFamily := 6
	if ip.To4() != nil {
		bindingFamily = 4
	}
	return family == 46 || family == bindingFamily
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
