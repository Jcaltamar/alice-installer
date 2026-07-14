package migration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const secretSentinel = "synthetic-secret-must-not-escape"

func TestResolverResolveSupportedConfigurations(t *testing.T) {
	source := `module.exports = { production: { dialect: "postgres", database: process.env.DB_NAME || "alice", username: "guardian", password: process.env.DB_PASSWORD ?? "fallback", host: "db", }, development: { dialect: "postgres", database: "devdb", username: "devuser", password: "devpass", host: process.env.DEV_HOST || "127.0.0.1", port: process.env.DEV_PORT ?? 5435, }, };`
	for _, tt := range []struct {
		name string
		env  string
		vars map[string]string
		want ResolvedConfig
	}{
		{"default production uses environment and PostgreSQL default port", "", map[string]string{"DB_NAME": "operator-db", "DB_PASSWORD": secretSentinel}, ResolvedConfig{Environment: EnvironmentProduction, Dialect: DialectPostgreSQL, Database: "operator-db", Username: "guardian", Host: "db", Port: 5432}},
		{"empty environment override uses literal fallback", "", map[string]string{"DB_NAME": "", "DB_PASSWORD": secretSentinel}, ResolvedConfig{Environment: EnvironmentProduction, Dialect: DialectPostgreSQL, Database: "alice", Username: "guardian", Host: "db", Port: 5432}},
		{"explicit development selects only development", "development", map[string]string{"DEV_HOST": "dev-db", "DEV_PORT": "5544"}, ResolvedConfig{Environment: EnvironmentDevelopment, Dialect: DialectPostgreSQL, Database: "devdb", Username: "devuser", Host: "dev-db", Port: 5544}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (Resolver{Opener: staticOpener{source: source}, Environment: mapEnvironment(tt.vars)}).Resolve(context.Background(), ConfigRequest{Environment: tt.env})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			assertResolvedConfig(t, got, tt.want)
			assertNoSecret(t, secretSentinel, got, err)
		})
	}
}

func TestResolverSafeOpenReadBoundary(t *testing.T) {
	valid := `module.exports = { production: { dialect: "postgres", database: "db", username: "user", password: "` + secretSentinel + `", host: "host" } };`
	for _, tt := range []struct {
		name   string
		opener ConfigOpener
	}{
		{"symlink is rejected after open", staticOpener{source: valid, mode: fs.ModeSymlink}},
		{"non regular file is rejected after open", staticOpener{source: valid, mode: fs.ModeDir}},
		{"opened file over limit is rejected", staticOpener{source: strings.Repeat("x", DefaultConfigSizeLimit+1)}},
		{"missing file is redacted", staticOpener{openErr: errors.New(secretSentinel)}},
		{"open error is redacted", staticOpener{openErr: errors.New(secretSentinel)}},
		{"read error is redacted", staticOpener{source: valid, readErr: errors.New(secretSentinel)}},
		{"post validation replacement cannot escape opened file", &swappingOpener{opened: valid, replacement: `module.exports = { production: { dialect: "postgres", database: "escaped", username: "user", password: "` + secretSentinel + `", host: "host" } };`}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (Resolver{Opener: tt.opener, Environment: mapEnvironment(nil)}).Resolve(context.Background(), ConfigRequest{})
			if tt.name == "post validation replacement cannot escape opened file" {
				if err != nil || got.Database != "db" {
					t.Fatalf("Resolve() = %#v, %v", got, err)
				}
				if tt.opener.(*swappingOpener).opens != 1 {
					t.Fatal("resolver reopened the path after validation")
				}
				assertNoSecret(t, secretSentinel, got, err)
				return
			}
			if !errors.Is(err, ErrConfigInvalid) {
				t.Fatalf("Resolve() error = %v, want ErrConfigInvalid", err)
			}
			assertNoSecret(t, secretSentinel, err, got)
		})
	}
}

func TestConfigOpenerRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.js")
	link := filepath.Join(dir, "config.js")
	if err := os.WriteFile(target, []byte("secret source"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	file, err := newConfigOpener().Open(link)
	if err == nil {
		file.Close()
		t.Fatal("secure opener followed symlink")
	}
	assertNoSecret(t, "secret source", err)
}

func TestResolverRejectsUnsafeOrInvalidInputWithoutSecretLeak(t *testing.T) {
	valid := `module.exports = { production: { dialect: "postgres", database: "db", username: "user", password: "` + secretSentinel + `", host: "host" } };`
	for _, source := range []string{
		strings.Replace(valid, `"db"`, `makeDB()`, 1), `import x from "x"; ` + valid,
		strings.Replace(valid, "database:", `["database"]:`, 1), `module.exports = { production: { ...other } };`,
		strings.Replace(valid, `"db"`, "`db`", 1), strings.Replace(valid, "database:", "get database():", 1),
		strings.Replace(valid, `"db"`, `"d" + "b"`, 1), strings.Replace(valid, `"db"`, "config.database", 1),
		strings.Replace(valid, `"db"`, `process.env."DB_NAME" || "db"`, 1), strings.Replace(valid, "production:", "staging:", 1),
		strings.Replace(valid, "database:", `database: "other", database:`, 1), strings.Replace(valid, `"postgres"`, `"mysql"`, 1),
		strings.Replace(valid, `host: "host"`, `host: "host", port: 70000`, 1),
	} {
		_, err := (Resolver{Opener: staticOpener{source: source}, Environment: mapEnvironment(nil)}).Resolve(context.Background(), ConfigRequest{})
		if !errors.Is(err, ErrConfigInvalid) {
			t.Fatalf("Resolve() error = %v, want ErrConfigInvalid", err)
		}
		assertNoSecret(t, secretSentinel, err)
	}
}

func TestResolverResolveDevelopmentDefaultsOnlyWhenExplicit(t *testing.T) {
	source := `module.exports = { production: { dialect: "postgres", password: "prod" }, development: { dialect: "postgres", password: "dev" } };`
	resolver := Resolver{Opener: staticOpener{source: source}, Environment: mapEnvironment(nil)}
	got, err := resolver.Resolve(context.Background(), ConfigRequest{Environment: "development"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Host != "127.0.0.1" || got.Port != 5435 || got.Database != "alice_guardian" || got.Username != "postgres" {
		t.Fatalf("development defaults = %#v", got)
	}
	if _, err := resolver.Resolve(context.Background(), ConfigRequest{}); !errors.Is(err, ErrConfigInvalid) {
		t.Fatalf("Resolve(production) error = %v", err)
	}
}

func TestResolvedConfigSecretLifecycleAndObservableBoundaries(t *testing.T) {
	source := `module.exports = { production: { dialect: "postgres", database: "db", username: "user", password: "` + secretSentinel + `", host: "host" } };`
	got, err := (Resolver{Opener: staticOpener{source: source}, Environment: mapEnvironment(nil)}).Resolve(context.Background(), ConfigRequest{})
	if err != nil {
		t.Fatal(err)
	}
	assertNoSecret(t, secretSentinel, got, got.String(), fmt.Sprintf("%v", got), fmt.Sprintf("%+v", got), fmt.Sprintf("%#v", got))
	if string(got.password.storage.value) != secretSentinel {
		t.Fatal("resolver did not retain password")
	}
	copied := got
	got.Release()
	if got.password.storage.value != nil || copied.password.storage.value != nil {
		t.Fatal("Release() did not clear copied secret storage")
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	assertNoSecret(t, secretSentinel, string(encoded))
}

type staticOpener struct {
	source           string
	mode             fs.FileMode
	openErr, readErr error
}

func (o staticOpener) Open(string) (ConfigFile, error) {
	if o.openErr != nil {
		return nil, o.openErr
	}
	return &staticFile{Reader: strings.NewReader(o.source), mode: o.mode, size: int64(len(o.source)), readErr: o.readErr}, nil
}

type swappingOpener struct {
	opened, replacement string
	opens               int
}

func (o *swappingOpener) Open(string) (ConfigFile, error) {
	o.opens++
	return &staticFile{Reader: strings.NewReader(o.opened), size: int64(len(o.opened))}, nil
}

type staticFile struct {
	io.Reader
	mode    fs.FileMode
	size    int64
	readErr error
}

func (f *staticFile) Read(p []byte) (int, error) {
	if f.readErr != nil {
		return 0, f.readErr
	}
	return f.Reader.Read(p)
}
func (f *staticFile) Stat() (fs.FileInfo, error) { return staticInfo{mode: f.mode, size: f.size}, nil }
func (f *staticFile) Close() error               { return nil }

type staticInfo struct {
	mode fs.FileMode
	size int64
}

func (i staticInfo) Name() string { return "config.js" }
func (i staticInfo) Size() int64  { return i.size }
func (i staticInfo) Mode() fs.FileMode {
	if i.mode == 0 {
		return 0600
	}
	return i.mode
}
func (i staticInfo) ModTime() (z time.Time) { return }
func (i staticInfo) IsDir() bool            { return i.mode.IsDir() }
func (i staticInfo) Sys() any               { return nil }

type mapEnvironment map[string]string

func (e mapEnvironment) Lookup(name string) (string, bool) { value, ok := e[name]; return value, ok }
func assertResolvedConfig(t *testing.T, got, want ResolvedConfig) {
	t.Helper()
	if got.Environment != want.Environment || got.Dialect != want.Dialect || got.Database != want.Database || got.Username != want.Username || got.Host != want.Host || got.Port != want.Port {
		t.Fatalf("resolved public config = %#v, want %#v", got, want)
	}
}
func assertNoSecret(t *testing.T, secret string, values ...any) {
	t.Helper()
	for _, value := range values {
		if strings.Contains(stringify(value), secret) {
			t.Fatalf("secret escaped through observable value: %v", value)
		}
	}
}
func stringify(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(error); ok {
		return text.Error()
	}
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
