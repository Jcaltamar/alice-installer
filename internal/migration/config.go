// Package migration provides fail-closed, non-destructive legacy migration inputs.
package migration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
)

const LegacyConfigPath = "/opt/backend_alice_guardian/node/config/config.js"
const DefaultConfigSizeLimit = 1 << 20

var ErrConfigInvalid = errors.New("legacy configuration is unsupported")

type EnvironmentName string

const (
	EnvironmentProduction  EnvironmentName = "production"
	EnvironmentDevelopment EnvironmentName = "development"
	DialectPostgreSQL                      = "postgres"
)

// Environment provides only the exact variable names referenced by the static config.
type Environment interface{ Lookup(string) (string, bool) }

// ConfigFile is the opened config handle. Stat applies to this handle, not a path that can be replaced.
type ConfigFile interface {
	io.Reader
	Stat() (fs.FileInfo, error)
	Close() error
}

// ConfigOpener opens the fixed legacy config without following a final symlink.
// Unsupported platforms must fail closed rather than use an unsafe fallback.
type ConfigOpener interface {
	Open(string) (ConfigFile, error)
}

type osEnvironment struct{}

func (osEnvironment) Lookup(name string) (string, bool) { return os.LookupEnv(name) }

type ConfigRequest struct{ Environment string }

type ValueSource string

const (
	ValueSourceLiteral            ValueSource = "literal"
	ValueSourceEnvironment        ValueSource = "environment"
	ValueSourceDevelopmentDefault ValueSource = "development-default"
	ValueSourcePostgreSQLDefault  ValueSource = "postgresql-default"
)

// ResolvedConfig intentionally exposes only values that are safe for plans and UI summaries.
type ResolvedConfig struct {
	Environment EnvironmentName        `json:"environment"`
	Dialect     string                 `json:"dialect"`
	Database    string                 `json:"database"`
	Username    string                 `json:"username"`
	Host        string                 `json:"host"`
	Port        int                    `json:"port"`
	Sources     map[string]ValueSource `json:"sources"`
	password    secret
}

func (c ResolvedConfig) String() string {
	return fmt.Sprintf("migration config environment=%s dialect=%s database=%s username=%s host=%s port=%d", c.Environment, c.Dialect, c.Database, c.Username, c.Host, c.Port)
}

// Release clears the password as soon as the downstream process boundary has copied it.
func (c *ResolvedConfig) Release() { c.password.clear() }

// Resolver reads only the fixed legacy configuration path. It never executes JavaScript.
type Resolver struct {
	Opener      ConfigOpener
	Environment Environment
	MaxBytes    int
}

func (r Resolver) Resolve(ctx context.Context, request ConfigRequest) (ResolvedConfig, error) {
	if err := ctx.Err(); err != nil {
		return ResolvedConfig{}, configError()
	}
	opener := r.Opener
	if opener == nil {
		opener = newConfigOpener()
	}
	file, err := opener.Open(LegacyConfigPath)
	if err != nil {
		return ResolvedConfig{}, configError()
	}
	defer file.Close()
	limit := r.MaxBytes
	if limit == 0 {
		limit = DefaultConfigSizeLimit
	}
	info, err := file.Stat()
	if err != nil || info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > int64(limit) {
		return ResolvedConfig{}, configError()
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil || len(data) > limit {
		return ResolvedConfig{}, configError()
	}
	selected, err := selectEnvironment(request.Environment)
	if err != nil {
		return ResolvedConfig{}, err
	}
	root, err := parseConfig(data)
	if err != nil {
		return ResolvedConfig{}, err
	}
	fields, ok := root[string(selected)]
	if !ok {
		return ResolvedConfig{}, configError()
	}
	env := r.Environment
	if env == nil {
		env = osEnvironment{}
	}
	return resolveFields(selected, fields, env)
}

func selectEnvironment(request string) (EnvironmentName, error) {
	if request == "" || request == string(EnvironmentProduction) {
		return EnvironmentProduction, nil
	}
	if request == string(EnvironmentDevelopment) {
		return EnvironmentDevelopment, nil
	}
	return "", configError()
}
func configError() error { return ErrConfigInvalid }

type secret struct{ storage *secretStorage }
type secretStorage struct{ value []byte }

func newSecret(value string) secret { return secret{storage: &secretStorage{value: []byte(value)}} }
func (s *secret) clear() {
	if s.storage == nil {
		return
	}
	for i := range s.storage.value {
		s.storage.value[i] = 0
	}
	s.storage.value = nil
}
