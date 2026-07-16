package compose

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/jcaltamar/alice-installer/internal/platform"
)

// stderrLines is the maximum number of stderr tail lines to include in errors.
const stderrLines = 20

// BackendService is the only service the restore cutover may control.
const (
	BackendService      = "backend"
	PostgreSQLService   = "postgresql-master"
	PostgreSQLContainer = "alice_postgresql-master"
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
	Service   string
	Container string
	Status    string // Health column: "healthy" | "unhealthy" | "starting" | "none" | ""
	State     string // Lifecycle: "running" | "exited" | "restarting" | "paused" | "created" | "dead" | ""
}

// IsReady returns true when a service is acceptable for the verify stage.
//
// Rule: (Status=="healthy" AND State=="running") || (Status∈{"","none"} && State=="running")
//
// A healthy service that is NOT running (e.g. exited) is NOT ready.
// Services without a healthcheck that are running → ready.
// Services without a healthcheck that are restarting/exited → NOT ready.
func IsReady(s ServiceHealth) bool {
	if s.State != "running" {
		return false
	}
	if s.Status == "healthy" || s.Status == "" || s.Status == "none" {
		return true
	}
	return false
}

// PullProgressMsg carries a single line of progress from `docker compose pull`.
type PullProgressMsg struct {
	Service    string
	Status     string
	Percent    int
	HasPercent bool
	Raw        string
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
	Restart(ctx context.Context, files []string, envFile string) error
	Down(ctx context.Context, files []string, envFile string) error
	StopService(ctx context.Context, files []string, envFile, service string) error
	StartService(ctx context.Context, files []string, envFile, service string) error
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
// The leading "compose" is REQUIRED — without it the invocation becomes
// `docker -f ...` which the docker CLI rejects with "unknown shorthand flag: 'f'".
func baseArgs(files []string, envFile string, sub ...string) []string {
	args := []string{"compose"}
	args = append(args, ComposeArgs(files)...)
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
// On failure, stderr is captured and appended to the error so the user can see
// the actual cause (e.g. "manifest unknown", "pull access denied").
func (c *CLICompose) Pull(ctx context.Context, files []string, envFile string, progress chan<- PullProgressMsg) error {
	args := append([]string{"compose", "--progress", "plain"}, ComposeArgs(files)...)
	if envFile != "" {
		args = append(args, "--env-file", envFile)
	}
	args = append(args, "pull")
	var stderrBuf []string
	handleProgress := func(line string) PullProgressMsg {
		msg, _ := parsePullProgress(line)
		progress <- msg
		return msg
	}
	err := c.streamer.Stream(ctx,
		func(line string) {
			handleProgress(line)
		},
		func(line string) {
			msg := handleProgress(line)
			if msg.Service == "" || strings.HasPrefix(msg.Status, "Error") {
				stderrBuf = append(stderrBuf, msg.Raw)
			}
		},
		"docker", args...,
	)
	if err != nil && len(stderrBuf) > 0 {
		// Include the last stderrLines lines of stderr in the error message so the
		// ResultModel failure view can show the actual root cause.
		tail := stderrBuf
		if len(tail) > stderrLines {
			tail = tail[len(tail)-stderrLines:]
		}
		return fmt.Errorf("%w\n--- docker compose pull stderr ---\n%s", err, strings.Join(tail, "\n"))
	}
	return err
}

type pullProgressJSON struct {
	ID       string `json:"id"`
	ParentID string `json:"parent_id"`
	Status   string `json:"status"`
	Text     string `json:"text"`
	Details  string `json:"details"`
	Current  int64  `json:"current"`
	Total    int64  `json:"total"`
}

var plainByteProgress = regexp.MustCompile(`([0-9.]+)([kMGT]?B)\s*/\s*([0-9.]+)([kMGT]?B)`)

func parsePullProgress(line string) (PullProgressMsg, bool) {
	var event pullProgressJSON
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		if msg, ok := parsePlainPullProgress(line); ok {
			return msg, false
		}
		return PullProgressMsg{Status: parsePullStatus(line), Raw: line}, false
	}
	if event.ID == "" && event.ParentID == "" && event.Status == "" && event.Text == "" && event.Details == "" {
		return PullProgressMsg{Status: parsePullStatus(line), Raw: line}, false
	}

	resource := event.ParentID
	if resource == "" {
		resource = event.ID
	}
	service := pullResourceLabel(resource)
	if event.ParentID == "" && looksLikeDigest(service) {
		service = ""
	}

	status := event.Text
	if status == "" {
		status = event.Status
	}
	if strings.EqualFold(event.Status, "Error") {
		details := event.Details
		if details == "" && !strings.EqualFold(event.Text, "Error") {
			details = event.Text
		}
		status = "Error"
		if details != "" {
			status += ": " + details
		}
	}

	msg := PullProgressMsg{Service: service, Status: status}
	if event.Total > 0 && event.Current >= 0 {
		msg.Percent = min(int(float64(event.Current)*100/float64(event.Total)), 100)
		msg.HasPercent = true
	}
	msg.Raw = formatPullProgress(msg)
	return msg, true
}

func parsePlainPullProgress(line string) (PullProgressMsg, bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return PullProgressMsg{}, false
	}
	rest := strings.Join(fields[1:], " ")
	status := ""
	for _, candidate := range []string{"Pulling fs layer", "Download complete", "Verifying Checksum", "Already exists", "Pull complete", "Downloading", "Extracting", "Preparing", "Waiting", "Pulling", "Pulled", "Error"} {
		if rest == candidate || strings.HasPrefix(rest, candidate+" ") {
			status = candidate
			break
		}
	}
	if status == "" {
		return PullProgressMsg{}, false
	}
	service := fields[0]
	if looksLikeDigest(service) {
		service = "layer " + strings.TrimPrefix(service, "sha256:")[:12]
	}
	msg := PullProgressMsg{Service: service, Status: status}
	if values := plainByteProgress.FindStringSubmatch(rest); values != nil {
		current, _ := strconv.ParseFloat(values[1], 64)
		total, _ := strconv.ParseFloat(values[3], 64)
		scale := map[string]float64{"B": 1, "kB": 1e3, "MB": 1e6, "GB": 1e9, "TB": 1e12}
		current, total = current*scale[values[2]], total*scale[values[4]]
		if total > 0 {
			msg.Percent, msg.HasPercent = min(int(current*100/total), 100), true
		}
	}
	msg.Raw = formatPullProgress(msg)
	return msg, true
}

func pullResourceLabel(resource string) string {
	label := strings.TrimPrefix(resource, "Image ")
	if digest := strings.Index(label, "@sha256:"); digest >= 0 {
		label = label[:digest]
	}
	return label
}

func looksLikeDigest(value string) bool {
	value = strings.TrimPrefix(value, "sha256:")
	if len(value) < 12 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}

func formatPullProgress(msg PullProgressMsg) string {
	status := msg.Status
	if msg.HasPercent {
		status = fmt.Sprintf("%s %d%%", status, msg.Percent)
	}
	if msg.Service == "" {
		return status
	}
	return strings.TrimSpace(msg.Service + " " + status)
}

// Up streams `docker compose up --detach` output; sends one UpProgressMsg per line.
func (c *CLICompose) Up(ctx context.Context, files []string, envFile string, progress chan<- UpProgressMsg) error {
	args := baseArgs(files, envFile, "up", "--detach")
	var stderrBuf []string
	err := c.streamer.Stream(ctx,
		func(line string) {
			progress <- UpProgressMsg{Raw: line, Status: line}
		},
		func(line string) {
			stderrBuf = append(stderrBuf, line)
		},
		"docker", args...,
	)
	if err != nil && len(stderrBuf) > 0 {
		tail := stderrBuf
		if len(tail) > stderrLines {
			tail = tail[len(tail)-stderrLines:]
		}
		return fmt.Errorf("%w\n--- docker compose up stderr ---\n%s", err, strings.Join(tail, "\n"))
	}
	return err
}

// Restart runs `docker compose restart` (one-shot).
func (c *CLICompose) Restart(ctx context.Context, files []string, envFile string) error {
	args := baseArgs(files, envFile, "restart")
	_, _, err := c.runner.Run(ctx, "docker", args...)
	if err != nil {
		return fmt.Errorf("docker compose restart failed: %w", err)
	}
	return nil
}

func validateRestoreService(service string) error {
	if service != BackendService {
		return fmt.Errorf("compose service control rejected")
	}
	return nil
}

// StopService stops the one allowlisted restore service with direct argv.
func (c *CLICompose) StopService(ctx context.Context, files []string, envFile, service string) error {
	if err := validateRestoreService(service); err != nil {
		return err
	}
	_, _, err := c.runner.Run(ctx, "docker", baseArgs(files, envFile, "stop", BackendService)...)
	if err != nil {
		return fmt.Errorf("docker compose backend stop failed: %w", err)
	}
	return nil
}

// StartService starts the one allowlisted restore service with direct argv.
func (c *CLICompose) StartService(ctx context.Context, files []string, envFile, service string) error {
	if err := validateRestoreService(service); err != nil {
		return err
	}
	_, _, err := c.runner.Run(ctx, "docker", baseArgs(files, envFile, "start", BackendService)...)
	if err != nil {
		return fmt.Errorf("docker compose backend start failed: %w", err)
	}
	return nil
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
	Name    string `json:"Name"`
	State   string `json:"State"`
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
			return nil, fmt.Errorf("docker compose ps returned malformed output")
		}
		statuses = append(statuses, ServiceHealth{
			Service:   row.Service,
			Container: row.Name,
			Status:    row.Health,
			State:     row.State,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("docker compose ps output unreadable")
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
