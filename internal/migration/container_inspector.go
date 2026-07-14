package migration

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
)

// DockerRunner is deliberately limited to the migration inspector's read-only commands.
type DockerRunner interface {
	Run(context.Context, string, ...string) (stdout, stderr []byte, err error)
}

// DockerCLIInspector lists and inspects containers without reusing the general Docker client.
type DockerCLIInspector struct{ Runner DockerRunner }

func (i DockerCLIInspector) Candidates(ctx context.Context, image ImageIdentity) ([]ContainerSummary, error) {
	if i.Runner == nil || image != PostgreSQL11Image {
		return nil, ErrContainerPrecondition
	}
	stdout, _, err := i.Runner.Run(ctx, "docker", "ps", "--all", "--no-trunc", "--format", "{{.ID}}\t{{.Image}}")
	if err != nil {
		return nil, ErrContainerPrecondition
	}
	var candidates []ContainerSummary
	for _, line := range strings.Split(strings.TrimSpace(string(stdout)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 2 || !fullContainerID.MatchString(parts[0]) {
			return nil, ErrContainerPrecondition
		}
		if parts[1] == string(image) {
			candidates = append(candidates, ContainerSummary{ID: parts[0], Image: parts[1]})
		}
	}
	return candidates, nil
}

func (i DockerCLIInspector) Inspect(ctx context.Context, id string) (ContainerDetails, error) {
	if i.Runner == nil || !fullContainerID.MatchString(id) {
		return ContainerDetails{}, ErrContainerPrecondition
	}
	stdout, _, err := i.Runner.Run(ctx, "docker", "inspect", id)
	if err != nil {
		return ContainerDetails{}, ErrContainerPrecondition
	}
	var inspected []dockerInspect
	if json.Unmarshal(stdout, &inspected) != nil || len(inspected) != 1 {
		return ContainerDetails{}, ErrContainerPrecondition
	}
	details, ok := safeDetails(inspected[0])
	if !ok || details.ID != id {
		return ContainerDetails{}, ErrContainerPrecondition
	}
	return details, nil
}

type dockerInspect struct {
	ID     string `json:"Id"`
	Config struct {
		Image  string            `json:"Image"`
		Env    []string          `json:"Env"`
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	State struct {
		Running bool `json:"Running"`
		Health  *struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
	HostConfig struct {
		NetworkMode string `json:"NetworkMode"`
	} `json:"HostConfig"`
	Mounts []struct {
		Name        string `json:"Name"`
		Destination string `json:"Destination"`
	} `json:"Mounts"`
	NetworkSettings struct {
		Networks map[string]struct {
			Aliases []string `json:"Aliases"`
		} `json:"Networks"`
		Ports map[string][]struct {
			HostPort string `json:"HostPort"`
		} `json:"Ports"`
	} `json:"NetworkSettings"`
}

func safeDetails(raw dockerInspect) (ContainerDetails, bool) {
	if !fullContainerID.MatchString(raw.ID) || raw.Config.Image != string(PostgreSQL11Image) {
		return ContainerDetails{}, false
	}
	details := ContainerDetails{ID: raw.ID, Image: raw.Config.Image, NetworkMode: raw.HostConfig.NetworkMode, Health: HealthNone, Labels: SafeContainerLabels{ComposeProject: raw.Config.Labels["com.docker.compose.project"], ComposeService: raw.Config.Labels["com.docker.compose.service"]}}
	if raw.State.Running {
		details.State = ContainerRunning
	} else {
		details.State = ContainerStopped
	}
	if raw.State.Health != nil {
		switch raw.State.Health.Status {
		case "healthy":
			details.Health = HealthHealthy
		case "unhealthy":
			details.Health = HealthUnhealthy
		default:
			details.Health = HealthUnknown
		}
	}
	for _, env := range raw.Config.Env {
		key, value, found := strings.Cut(env, "=")
		if !found || value == "" {
			continue
		}
		switch key {
		case "POSTGRESQL_DATABASE":
			details.DatabaseNames = append(details.DatabaseNames, value)
		case "POSTGRESQL_USERNAME":
			details.Usernames = append(details.Usernames, value)
		}
	}
	for _, mount := range raw.Mounts {
		if mount.Name != "" && mount.Destination == "/bitnami/postgresql" {
			details.MountKinds = append(details.MountKinds, "bitnami-postgresql-data")
		}
	}
	for _, network := range raw.NetworkSettings.Networks {
		for _, alias := range network.Aliases {
			details.Endpoints = append(details.Endpoints, ContainerEndpoint{Host: alias, Port: 5432, ContainerLocal: true})
		}
	}
	for port := range raw.NetworkSettings.Ports {
		number, protocol, found := strings.Cut(port, "/")
		parsed, err := strconv.Atoi(number)
		if !found || protocol != "tcp" || err != nil {
			return ContainerDetails{}, false
		}
		// Published ports are deliberately not endpoints: host mapping never proves container-local routing.
		_ = parsed
	}
	return details, true
}
