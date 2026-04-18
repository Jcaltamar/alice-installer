// Package envgen renders a .env file from a .env.example template plus installer inputs.
package envgen

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jcaltamar/alice-installer/internal/platform"
	"github.com/jcaltamar/alice-installer/internal/secrets"
)

// PortsConfig holds the resolved port numbers for all services.
type PortsConfig struct {
	PostgresPort     int
	BackendPort      int
	WebsocketPort    int
	WebPort          int
	RTSPPort         int
	RedisPort        int
	QueuePort        int
	HLSPort          int
	HLSPort2         int
	HLSPort3         int
	RTMPPort         int
	MilvusPort       int
	MinioAPIPort     int
	MinioConsolePort int
}

// Input holds all installer-supplied values needed to render the .env template.
type Input struct {
	Workspace        string
	Arch             platform.Arch
	Ports            PortsConfig
	GeneratePassword bool   // when true, calls PasswordGen; ignored if PasswordOverride != ""
	PasswordOverride string // explicit override; wins over GeneratePassword
}

// EnvTemplater renders .env.example bytes into a deployment-ready .env.
type EnvTemplater interface {
	Render(template []byte, in Input) ([]byte, error)
}

// Templater is the concrete implementation.
// Inject PasswordGen for password generation; use FakeGenerator in tests.
type Templater struct {
	PasswordGen secrets.PasswordGenerator
}

// workspaceRe allows only alphanumeric, dash, and underscore characters.
var workspaceRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ValidateWorkspace is the exported workspace name validator.
// It returns an error if the name is not safe for use as a filesystem path component.
// Rules: non-empty, alphanumeric/dash/underscore only, no whitespace, no leading dot,
// no path separators, max 64 chars.
func ValidateWorkspace(ws string) error {
	return validateWorkspace(ws)
}

// validateWorkspace returns an error if the workspace name is not safe for the filesystem.
func validateWorkspace(ws string) error {
	trimmed := strings.TrimSpace(ws)

	if trimmed == "" {
		return fmt.Errorf("WORKSPACE cannot be empty")
	}

	// Reject interior whitespace that survived trim
	if strings.ContainsAny(ws, " \t\n\r") {
		return fmt.Errorf("WORKSPACE must not contain whitespace")
	}

	if strings.HasPrefix(trimmed, ".") {
		return fmt.Errorf("WORKSPACE must not start with a dot")
	}

	if strings.ContainsAny(trimmed, "/\\") {
		return fmt.Errorf("WORKSPACE must not contain path separators (/ or \\)")
	}

	if len(trimmed) > 64 {
		return fmt.Errorf("WORKSPACE must not exceed 64 characters (got %d)", len(trimmed))
	}

	if !workspaceRe.MatchString(trimmed) {
		return fmt.Errorf("WORKSPACE must contain only alphanumeric characters, hyphens, or underscores (got %q)", trimmed)
	}

	return nil
}

// imageTags returns the arch-specific image tag map.
func imageTags(arch platform.Arch) map[string]string {
	suffix := ""
	if arch == platform.ArchARM64 {
		suffix = "-arm"
	}

	return map[string]string{
		"BACKEND_IMAGE":   "jcaltamare/aliceguardian:backend" + suffix,
		"WEBSOCKET_IMAGE": "jcaltamare/aliceguardian:socket1" + suffix,
		"WEB_IMAGE":       "jcaltamare/aliceguardian:web_ag" + suffix,
		"QUEUE_IMAGE":     "jcaltamare/aliceguardian:queue" + suffix,
		"REDIS_IMAGE":     "redis:7-alpine", // multi-arch manifest, no suffix
	}
}

// Render applies all inputs to template and returns the fully-rendered .env bytes.
//
// Rules:
//  1. Validate workspace.
//  2. Resolve password (override > generator > empty).
//  3. Walk template line by line; substitute managed keys; preserve everything else.
//  4. Ensure trailing newline.
func (t *Templater) Render(template []byte, in Input) ([]byte, error) {
	if err := validateWorkspace(in.Workspace); err != nil {
		return nil, err
	}

	// Resolve password
	password := in.PasswordOverride
	if password == "" && in.GeneratePassword {
		var err error
		password, err = t.PasswordGen.Generate(32)
		if err != nil {
			return nil, fmt.Errorf("generate password: %w", err)
		}
	}

	// Build substitution map
	subs := imageTags(in.Arch)
	subs["WORKSPACE"] = in.Workspace
	subs["POSTGRES_PASSWORD"] = password

	p := in.Ports
	subs["POSTGRES_PORT"] = fmt.Sprintf("%d", p.PostgresPort)
	subs["BACKEND_PORT"] = fmt.Sprintf("%d", p.BackendPort)
	subs["WEBSOCKET_PORT"] = fmt.Sprintf("%d", p.WebsocketPort)
	subs["WEB_PORT"] = fmt.Sprintf("%d", p.WebPort)
	subs["RTSP_PORT"] = fmt.Sprintf("%d", p.RTSPPort)
	subs["REDIS_PORT"] = fmt.Sprintf("%d", p.RedisPort)
	subs["QUEUE_PORT"] = fmt.Sprintf("%d", p.QueuePort)
	subs["HLS_PORT"] = fmt.Sprintf("%d", p.HLSPort)
	subs["HLS_PORT2"] = fmt.Sprintf("%d", p.HLSPort2)
	subs["HLS_PORT3"] = fmt.Sprintf("%d", p.HLSPort3)
	subs["RTMP_PORT"] = fmt.Sprintf("%d", p.RTMPPort)
	subs["MILVUS_PORT"] = fmt.Sprintf("%d", p.MilvusPort)
	subs["MINIO_API_PORT"] = fmt.Sprintf("%d", p.MinioAPIPort)
	subs["MINIO_CONSOLE_PORT"] = fmt.Sprintf("%d", p.MinioConsolePort)

	// Process template line by line
	lines := strings.Split(string(template), "\n")

	// Remove a single trailing empty element produced by a trailing newline in the template
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	var out strings.Builder
	for _, line := range lines {
		// Preserve comments and blank lines verbatim
		stripped := strings.TrimRight(line, "\r")
		if stripped == "" || strings.HasPrefix(stripped, "#") {
			out.WriteString(stripped)
			out.WriteByte('\n')
			continue
		}

		// Parse KEY=VALUE
		idx := strings.IndexByte(stripped, '=')
		if idx < 0 {
			// Not a key=value line — preserve as-is
			out.WriteString(stripped)
			out.WriteByte('\n')
			continue
		}

		key := stripped[:idx]

		if val, managed := subs[key]; managed {
			out.WriteString(key)
			out.WriteByte('=')
			out.WriteString(val)
			out.WriteByte('\n')
		} else {
			// Unmanaged key — preserve verbatim
			out.WriteString(stripped)
			out.WriteByte('\n')
		}
	}

	result := out.String()

	// Guarantee trailing newline
	if len(result) == 0 || result[len(result)-1] != '\n' {
		result += "\n"
	}

	return []byte(result), nil
}
