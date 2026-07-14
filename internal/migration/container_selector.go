package migration

import (
	"context"
	"errors"
	"regexp"
	"sort"
)

const PostgreSQL11Image ImageIdentity = "bitnami/postgresql:11-debian-10"

var (
	ErrContainerPrecondition = errors.New("legacy database container precondition failed")
	ErrAmbiguousContainer    = errors.New("legacy database container is ambiguous")
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

// SafeContainerLabels is the allowlist used for correlation; no arbitrary Docker labels escape inspection.
type SafeContainerLabels struct {
	ComposeProject string
	ComposeService string
}

// ContainerDetails contains only non-secret, allowlisted metadata required by the selector.
type ContainerDetails struct {
	ID            string
	Image         string
	Digest        string
	State         ContainerState
	Health        HealthStatus
	NetworkMode   string
	DatabaseNames []string
	Usernames     []string
	Endpoints     []ContainerEndpoint
	MountKinds    []string
	Labels        SafeContainerLabels
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
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	// Inspect every list candidate before making any decision: partial inspection is not evidence.
	detailsByID := make(map[string]ContainerDetails, len(candidates))
	inspectionFailed := false
	for _, candidate := range candidates {
		if candidate.Image != string(PostgreSQL11Image) || !fullContainerID.MatchString(candidate.ID) {
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
	for _, candidate := range candidates {
		details := detailsByID[candidate.ID]
		corroborated, unsafe := correlateCandidate(details, config)
		if unsafe {
			return ContainerIdentity{}, ErrContainerPrecondition
		}
		if corroborated {
			matches = append(matches, ContainerIdentity{ID: details.ID, Image: PostgreSQL11Image, Digest: details.Digest})
		}
	}
	if len(matches) == 0 {
		return ContainerIdentity{}, ErrContainerPrecondition
	}
	if len(matches) != 1 {
		return ContainerIdentity{}, ErrAmbiguousContainer
	}
	return matches[0], nil
}

// correlateCandidate distinguishes insufficient unrelated evidence from unsafe contradictions.
// It is called only after every exact-image candidate has been inspected successfully.
func correlateCandidate(details ContainerDetails, config ResolvedConfig) (corroborated, unsafe bool) {
	if !fullContainerID.MatchString(details.ID) || details.Image != string(PostgreSQL11Image) {
		return false, true
	}
	endpointMatches, endpointConflicts := containerLocalEndpointEvidence(details.Endpoints, config.Host, config.Port)
	identityMatches := contains(details.DatabaseNames, config.Database) && contains(details.Usernames, config.Username) && endpointMatches
	if !identityMatches {
		return false, false
	}
	if endpointConflicts || details.State != ContainerRunning || details.Health == HealthUnhealthy || details.Health == HealthUnknown {
		return false, true
	}
	return len(details.MountKinds) > 0 || details.Labels.ComposeProject != "" || details.Labels.ComposeService != "", false
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

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
