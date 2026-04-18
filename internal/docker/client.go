package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/jcaltamar/alice-installer/internal/platform"
)

// Runtimes holds the Docker container runtimes information.
type Runtimes struct {
	Names   []string // e.g. ["runc", "nvidia"]
	Default string
}

// Info holds relevant fields from `docker info`.
type Info struct {
	ServerVersion   string
	Architecture    string
	OperatingSystem string
	Runtimes        Runtimes
}

// Version holds Docker client and server version strings.
type Version struct {
	Client string
	Server string
}

// DockerClient is the interface for interacting with the Docker daemon.
type DockerClient interface {
	// Probe returns an error if the daemon is unreachable.
	Probe(ctx context.Context) error
	// Info returns daemon metadata parsed from `docker info --format '{{json .}}'`.
	Info(ctx context.Context) (Info, error)
	// Version returns client and server version strings.
	Version(ctx context.Context) (Version, error)
	// HasRuntime returns true if the named runtime is registered in the daemon.
	HasRuntime(ctx context.Context, name string) (bool, error)
}

// ---------------------------------------------------------------------------
// Production implementation
// ---------------------------------------------------------------------------

// dockerInfoResponse is the subset of `docker info --format '{{json .}}'` we parse.
type dockerInfoResponse struct {
	ServerVersion   string                     `json:"ServerVersion"`
	Architecture    string                     `json:"Architecture"`
	OperatingSystem string                     `json:"OperatingSystem"`
	Runtimes        map[string]json.RawMessage `json:"Runtimes"`
}

// dockerVersionResponse is the subset of `docker version --format '{{json .}}'` we parse.
type dockerVersionResponse struct {
	Client struct {
		Version string `json:"Version"`
	} `json:"Client"`
	Server struct {
		Components []struct {
			Version string `json:"Version"`
		} `json:"Components"`
	} `json:"Server"`
}

// CLIDocker implements DockerClient by shelling out to the `docker` CLI.
type CLIDocker struct {
	runner platform.CommandRunner
}

// NewCLIDocker creates a CLIDocker using the given CommandRunner.
// Pass nil to use the production OS runner embedded in the platform package.
func NewCLIDocker(runner platform.CommandRunner) *CLIDocker {
	if runner == nil {
		runner = &platform.OSCommandRunner{}
	}
	return &CLIDocker{runner: runner}
}

// Probe runs `docker info` and returns an error if the daemon is unreachable.
func (c *CLIDocker) Probe(ctx context.Context) error {
	_, _, err := c.runner.Run(ctx, "docker", "info", "--format", "{{json .}}")
	if err != nil {
		return fmt.Errorf("docker daemon unreachable: %w", err)
	}
	return nil
}

// Info runs `docker info --format '{{json .}}'` and parses the result.
func (c *CLIDocker) Info(ctx context.Context) (Info, error) {
	resp, err := c.fetchInfo(ctx)
	if err != nil {
		return Info{}, err
	}
	return infoFromResponse(resp), nil
}

// Version runs `docker version --format '{{json .}}'` and parses client+server.
func (c *CLIDocker) Version(ctx context.Context) (Version, error) {
	stdout, _, err := c.runner.Run(ctx, "docker", "version", "--format", "{{json .}}")
	if err != nil {
		return Version{}, fmt.Errorf("docker version failed: %w", err)
	}

	var resp dockerVersionResponse
	if err := json.Unmarshal(stdout, &resp); err != nil {
		return Version{}, fmt.Errorf("docker version parse error: %w", err)
	}

	server := ""
	if len(resp.Server.Components) > 0 {
		server = resp.Server.Components[0].Version
	}
	return Version{
		Client: resp.Client.Version,
		Server: server,
	}, nil
}

// HasRuntime returns true if the named runtime is present in daemon runtimes.
func (c *CLIDocker) HasRuntime(ctx context.Context, name string) (bool, error) {
	resp, err := c.fetchInfo(ctx)
	if err != nil {
		return false, err
	}
	_, ok := resp.Runtimes[name]
	return ok, nil
}

// fetchInfo is a shared helper that runs docker info and parses the JSON.
func (c *CLIDocker) fetchInfo(ctx context.Context) (dockerInfoResponse, error) {
	stdout, _, err := c.runner.Run(ctx, "docker", "info", "--format", "{{json .}}")
	if err != nil {
		return dockerInfoResponse{}, fmt.Errorf("docker info failed: %w", err)
	}
	var resp dockerInfoResponse
	if err := json.Unmarshal(stdout, &resp); err != nil {
		return dockerInfoResponse{}, fmt.Errorf("docker info parse error: %w", err)
	}
	return resp, nil
}

// infoFromResponse converts the raw JSON response to our Info type.
func infoFromResponse(resp dockerInfoResponse) Info {
	names := make([]string, 0, len(resp.Runtimes))
	for k := range resp.Runtimes {
		names = append(names, k)
	}
	sort.Strings(names)
	return Info{
		ServerVersion:   resp.ServerVersion,
		Architecture:    resp.Architecture,
		OperatingSystem: resp.OperatingSystem,
		Runtimes:        Runtimes{Names: names},
	}
}
