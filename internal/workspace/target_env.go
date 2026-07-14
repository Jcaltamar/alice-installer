package workspace

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const targetEnvMaxBytes = 64 << 10

type secretValue struct{ bytes []byte }

// TargetDatabaseConfig contains only the generated target identity; its password is private.
type TargetDatabaseConfig struct {
	Host     string
	Port     uint16
	User     string
	Database string
	password secretValue
}

func (c TargetDatabaseConfig) String() string {
	return fmt.Sprintf("target database host=%s port=%d user=%s database=%s", c.Host, c.Port, c.User, c.Database)
}
func (c *TargetDatabaseConfig) Release() {
	for i := range c.password.bytes {
		c.password.bytes[i] = 0
	}
	c.password.bytes = nil
}

// WritePGPass emits a target credential only to the caller's protected writer.
// It never returns the password and consumes the private value after use.
func (c *TargetDatabaseConfig) WritePGPass(w io.Writer) error {
	if w == nil || len(c.password.bytes) == 0 {
		return targetEnvError("target-env-credential")
	}
	_, err := fmt.Fprintf(w, "%s:%d:*:%s:%s\n", pgpassField(c.Host), c.Port, pgpassField(c.User), pgpassField(string(c.password.bytes)))
	c.Release()
	return err
}

func pgpassField(value string) string {
	return strings.NewReplacer("\\", "\\\\", ":", "\\:").Replace(value)
}

type TargetEnvReader interface {
	ReadTargetDatabase(context.Context, string) (TargetDatabaseConfig, error)
}
type TargetEnvFileReader struct{}
type targetEnvError string

func (e targetEnvError) Error() string { return string(e) }
func (TargetEnvFileReader) ReadTargetDatabase(ctx context.Context, path string) (TargetDatabaseConfig, error) {
	if err := ctx.Err(); err != nil {
		return TargetDatabaseConfig{}, targetEnvError("target-env-cancelled")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return TargetDatabaseConfig{}, targetEnvError("target-env-unsafe-file")
	}
	file, err := os.Open(path)
	if err != nil {
		return TargetDatabaseConfig{}, targetEnvError("target-env-unsafe-file")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return TargetDatabaseConfig{}, targetEnvError("target-env-unsafe-file")
	}
	if opened.Size() < 0 || opened.Size() > targetEnvMaxBytes {
		return TargetDatabaseConfig{}, targetEnvError("target-env-too-large")
	}
	data, err := io.ReadAll(io.LimitReader(file, targetEnvMaxBytes+1))
	if err != nil || len(data) > targetEnvMaxBytes {
		return TargetDatabaseConfig{}, targetEnvError("target-env-too-large")
	}
	return parseTargetEnv(data)
}
func parseTargetEnv(data []byte) (TargetDatabaseConfig, error) {
	if strings.ContainsAny(string(data), "\x00\r") {
		return TargetDatabaseConfig{}, targetEnvError("target-env-malformed")
	}
	values := map[string]string{}
	allowed := map[string]bool{"POSTGRES_HOST": true, "POSTGRES_PORT": true, "POSTGRES_USER": true, "POSTGRES_PASSWORD": true, "POSTGRES_DATABASE": true}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.Contains(value, "=") || strings.TrimSpace(key) != key || key == "" || strings.HasPrefix(key, "export ") {
			return TargetDatabaseConfig{}, targetEnvError("target-env-malformed")
		}
		if !allowed[key] {
			continue
		}
		if _, exists := values[key]; exists {
			return TargetDatabaseConfig{}, targetEnvError("target-env-duplicate-key")
		}
		if value == "" {
			return TargetDatabaseConfig{}, targetEnvError("target-env-empty-key")
		}
		if strings.ContainsAny(value, "\\\"'$#") || strings.Contains(value, " ") {
			return TargetDatabaseConfig{}, targetEnvError("target-env-malformed")
		}
		values[key] = value
	}
	for _, key := range []string{"POSTGRES_HOST", "POSTGRES_PORT", "POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DATABASE"} {
		if values[key] == "" {
			return TargetDatabaseConfig{}, targetEnvError("target-env-missing-key")
		}
	}
	if values["POSTGRES_HOST"] != "127.0.0.1" {
		return TargetDatabaseConfig{}, targetEnvError("target-env-invalid-host")
	}
	port, err := strconv.ParseUint(values["POSTGRES_PORT"], 10, 16)
	if err != nil || values["POSTGRES_PORT"] != strconv.FormatUint(port, 10) || port == 0 {
		return TargetDatabaseConfig{}, targetEnvError("target-env-invalid-port")
	}
	if !targetIdentifier(values["POSTGRES_USER"]) {
		return TargetDatabaseConfig{}, targetEnvError("target-env-invalid-user")
	}
	if !targetIdentifier(values["POSTGRES_DATABASE"]) || values["POSTGRES_DATABASE"] == "postgres" {
		return TargetDatabaseConfig{}, targetEnvError("target-env-invalid-database")
	}
	return TargetDatabaseConfig{Host: values["POSTGRES_HOST"], Port: uint16(port), User: values["POSTGRES_USER"], Database: values["POSTGRES_DATABASE"], password: secretValue{bytes: []byte(values["POSTGRES_PASSWORD"])}}, nil
}
func targetIdentifier(value string) bool {
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
