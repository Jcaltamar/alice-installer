package compose

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jcaltamar/alice-installer/internal/platform"
)

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

// Version holds Docker Compose version information.
type Version struct {
	V2Plugin bool
	Raw      string
}

// ServiceHealth holds the health status for a single compose service.
type ServiceHealth struct {
	Service string
	Status  string // "healthy" | "unhealthy" | "starting" | "none"
}

// PullProgressMsg carries a single line of progress from `docker compose pull`.
type PullProgressMsg struct {
	Service string
	Status  string // "Pulling" | "Downloading" | "Pulled" | "Error"
	Percent int    // 0-100 where applicable
	Raw     string
}

// UpProgressMsg carries a single line of progress from `docker compose up`.
type UpProgressMsg struct {
	Service string
	Status  string
	Raw     string
}

// ComposeRunner is the interface for driving Docker Compose operations.
type ComposeRunner interface {
	Version(ctx context.Context) (Version, error)
	Pull(ctx context.Context, files []string, envFile string, progress chan<- PullProgressMsg) error
	Up(ctx context.Context, files []string, envFile string, progress chan<- UpProgressMsg) error
	Down(ctx context.Context, files []string, envFile string) error
	HealthStatus(ctx context.Context, files []string, envFile string) ([]ServiceHealth, error)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// ComposeArgs converts a list of compose file paths into the -f flag sequence.
func ComposeArgs(files []string) []string {
	args := make([]string, 0, len(files)*2)
	for _, f := range files {
		args = append(args, "-f", f)
	}
	return args
}

// baseArgs builds the common args prefix for a compose sub-command.
func baseArgs(files []string, envFile string, sub ...string) []string {
	args := ComposeArgs(files)
	if envFile != "" {
		args = append(args, "--env-file", envFile)
	}
	args = append(args, sub...)
	return args
}

// ---------------------------------------------------------------------------
// Production implementation
// ---------------------------------------------------------------------------

// CLICompose implements ComposeRunner by shelling out to `docker compose`.
type CLICompose struct {
	runner   platform.CommandRunner
	streamer platform.StreamingCommandRunner
}

// NewCLICompose creates a CLICompose.
// runner is used for one-shot commands (Version, Down, HealthStatus).
// streamer is used for streaming commands (Pull, Up).
// Pass nil to use the OS-backed production implementations.
func NewCLICompose(runner platform.CommandRunner, streamer platform.StreamingCommandRunner) *CLICompose {
	if runner == nil {
		runner = &platform.OSCommandRunner{}
	}
	if streamer == nil {
		streamer = &platform.OSStreamingCommandRunner{}
	}
	return &CLICompose{runner: runner, streamer: streamer}
}

// Version runs `docker compose version --short` and validates the v2 plugin is present.
func (c *CLICompose) Version(ctx context.Context) (Version, error) {
	stdout, _, err := c.runner.Run(ctx, "docker", "compose", "version", "--short")
	if err != nil {
		return Version{}, fmt.Errorf("docker compose version failed: %w", err)
	}
	raw := strings.TrimSpace(string(stdout))
	if raw == "" {
		return Version{}, fmt.Errorf("docker compose version returned empty output; is docker compose v2 plugin installed?")
	}
	return Version{V2Plugin: true, Raw: raw}, nil
}

// Pull streams `docker compose pull` output; sends one PullProgressMsg per line.
// Closes the channel when done. Returns any execution error.
func (c *CLICompose) Pull(ctx context.Context, files []string, envFile string, progress chan<- PullProgressMsg) error {
	args := baseArgs(files, envFile, "pull")
	return c.streamer.Stream(ctx,
		func(line string) {
			progress <- PullProgressMsg{Raw: line, Status: parsePullStatus(line)}
		},
		func(_ string) {}, // stderr ignored at this layer — captured by error return
		"docker", args...,
	)
}

// Up streams `docker compose up --detach` output; sends one UpProgressMsg per line.
func (c *CLICompose) Up(ctx context.Context, files []string, envFile string, progress chan<- UpProgressMsg) error {
	args := baseArgs(files, envFile, "up", "--detach")
	return c.streamer.Stream(ctx,
		func(line string) {
			progress <- UpProgressMsg{Raw: line, Status: line}
		},
		func(_ string) {},
		"docker", args...,
	)
}

// Down runs `docker compose down` (one-shot).
func (c *CLICompose) Down(ctx context.Context, files []string, envFile string) error {
	args := baseArgs(files, envFile, "down")
	_, _, err := c.runner.Run(ctx, "docker", args...)
	if err != nil {
		return fmt.Errorf("docker compose down failed: %w", err)
	}
	return nil
}

// psLine is the shape of each JSON line from `docker compose ps --format json`.
type psLine struct {
	Service string `json:"Service"`
	Health  string `json:"Health"`
}

// HealthStatus runs `docker compose ps --format json` and parses line-delimited JSON.
func (c *CLICompose) HealthStatus(ctx context.Context, files []string, envFile string) ([]ServiceHealth, error) {
	args := baseArgs(files, envFile, "ps", "--format", "json")
	stdout, _, err := c.runner.Run(ctx, "docker", args...)
	if err != nil {
		return nil, fmt.Errorf("docker compose ps failed: %w", err)
	}

	var statuses []ServiceHealth
	scanner := bufio.NewScanner(bytes.NewReader(stdout))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var row psLine
		if jsonErr := json.Unmarshal([]byte(line), &row); jsonErr != nil {
			continue // skip malformed lines
		}
		statuses = append(statuses, ServiceHealth{
			Service: row.Service,
			Status:  row.Health,
		})
	}
	return statuses, nil
}

// ---------------------------------------------------------------------------
// parsePullStatus extracts a coarse status from a docker compose pull line.
// ---------------------------------------------------------------------------

func parsePullStatus(line string) string {
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, "pulling"):
		return "Pulling"
	case strings.Contains(lower, "downloading"):
		return "Downloading"
	case strings.Contains(lower, "pulled"):
		return "Pulled"
	case strings.Contains(lower, "error"):
		return "Error"
	default:
		return ""
	}
}
