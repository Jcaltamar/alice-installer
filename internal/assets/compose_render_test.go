package assets_test

// compose_render_test.go — snapshot / integration-style tests that validate
// the docker-compose files in this package against `docker compose config`.
//
// Guard: tests skip automatically when:
//   - `docker` binary is not in PATH
//   - `-short` flag is passed (unit/CI fast runs)
//
// Golden files in internal/assets/testdata/*.golden are regenerated with:
//
//	go test ./internal/assets/... -run TestComposeRender -update
//
// REQ-CO-1, REQ-ENV-6

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "regenerate golden files")

// composeConfigArgs builds the `docker compose config` command.
// overlays is a list of additional -f files beyond the baseline.
func composeConfigArgs(baselineFile, envFile string, overlays []string) []string {
	args := []string{"compose", "-f", baselineFile}
	for _, o := range overlays {
		args = append(args, "-f", o)
	}
	args = append(args, "--env-file", envFile, "config")
	return args
}

// writeEnvFile writes key=value pairs to a temp file and returns its path.
func writeEnvFile(t *testing.T, kv map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	var buf strings.Builder
	for k, v := range kv {
		buf.WriteString(k)
		buf.WriteByte('=')
		buf.WriteString(v)
		buf.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(buf.String()), 0600); err != nil {
		t.Fatalf("writeEnvFile: %v", err)
	}
	return path
}

// fullEnv returns a minimal but complete env map that satisfies all ${VAR}
// substitutions in the refactored docker-compose.yml.
func fullEnv() map[string]string {
	return map[string]string{
		"BACKEND_IMAGE":                    "jcaltamare/aliceguardian:backend",
		"WEBSOCKET_IMAGE":                  "jcaltamare/aliceguardian:socket1",
		"WEB_IMAGE":                        "jcaltamare/aliceguardian:web_ag",
		"QUEUE_IMAGE":                      "jcaltamare/aliceguardian:queue",
		"REDIS_IMAGE":                      "redis:7-alpine",
		"REDIS_PORT":                       "6379",
		"POSTGRES_HOST":                    "127.0.0.1",
		"POSTGRES_PORT":                    "5432",
		"POSTGRES_USER":                    "postgres",
		"POSTGRES_PASSWORD":                "test-password",
		"POSTGRES_DATABASE":                "alice_guardian",
		"POSTGRES_SSL":                     "false",
		"POSTGRES_SSL_REJECT_UNAUTHORIZED": "false",
		"NODE_ENV":                         "production",
		"QUEUE_PORT":                       "3000",
		"REDIS_HOST":                       "127.0.0.1",
		"WEBSOCKET_HOST":                   "127.0.0.1",
		"BACKEND_HOST":                     "127.0.0.1",
		"TELEGRAM_API":                     "http://127.0.0.1:9190",
		"HOST_MQTT":                        "127.0.0.1",
		"HOST_MQTT_USER":                   "user",
		"HOST_MQTT_PWD":                    "pass",
		"WORKSPACE":                        "alice",
		"APIFACE":                          "http://127.0.0.1:18080",
		"HOSTDOCKER":                       "http://127.0.0.1",
		"PORTDOCKER":                       "2375",
		"HOST_RTSP":                        "127.0.0.1:8554",
	}
}

func thisDir(t *testing.T) string {
	t.Helper()
	// The source files live in internal/assets/ relative to project root.
	// We use the embed package path resolution: the test binary CWD is the
	// package directory when run via `go test`.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return wd
}

func dockerBin(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("docker binary not in PATH — skipping compose render tests")
	}
	return bin
}

func TestComposeRender(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compose render tests under -short")
	}
	docker := dockerBin(t)
	dir := thisDir(t)

	baselineFile := filepath.Join(dir, "docker-compose.yml")
	gpuFile := filepath.Join(dir, "docker-compose.gpu.yml")

	goldenDir := filepath.Join(dir, "testdata")
	if err := os.MkdirAll(goldenDir, 0755); err != nil {
		t.Fatalf("mkdir testdata: %v", err)
	}

	tests := []struct {
		name     string
		overlays []string
		envKV    map[string]string
		// wantContains: substrings that MUST appear in rendered YAML
		wantContains []string
		// wantAbsent: substrings that MUST NOT appear in rendered YAML
		wantAbsent []string
		// wantExitZero: if false, we expect non-zero exit (compose config fails)
		wantExitZero bool
		golden       string
	}{
		{
			name:     "baseline_no_gpu",
			overlays: nil,
			envKV:    fullEnv(),
			// docker compose config resolves ${VAR} — so we check the resolved values.
			wantContains: []string{
				// resolved image values from fullEnv()
				"jcaltamare/aliceguardian:backend",  // BACKEND_IMAGE resolved (no -arm suffix)
				"jcaltamare/aliceguardian:socket1",  // WEBSOCKET_IMAGE resolved (no -arm suffix)
				"jcaltamare/aliceguardian:queue",    // QUEUE_IMAGE resolved (no -arm suffix)
				"alice_redis",                        // redis service present
				"redis:7-alpine",                    // redis image resolved
			},
			// runtime: nvidia must NOT appear in baseline; leaked password must be gone
			wantAbsent: []string{
				"pi4aK2uBQa",                          // hardcoded leaked password fragment
				"jcaltamare/aliceguardian:backend-arm", // old hardcoded image tag
				"jcaltamare/aliceguardian:socket1-arm", // old hardcoded image tag
				"jcaltamare/aliceguardian:queue-arm",   // old hardcoded image tag
				"runtime: nvidia",                       // GPU-only — must be absent from baseline
			},
			wantExitZero: true,
			golden:       "baseline_no_gpu.golden",
		},
		{
			name:     "baseline_with_gpu_overlay",
			overlays: []string{gpuFile},
			envKV:    fullEnv(),
			// GPU overlay must inject runtime: nvidia under backend
			wantContains: []string{
				"nvidia",
			},
			wantAbsent:   nil,
			wantExitZero: true,
			golden:       "baseline_with_gpu.golden",
		},
		{
			name: "baseline_missing_postgres_password",
			// POSTGRES_PASSWORD intentionally absent — docker compose config should
			// still exit 0 (docker compose substitutes missing vars with empty string)
			// but the rendered YAML must not contain the old hardcoded value.
			overlays:     nil,
			envKV:        withoutKey(fullEnv(), "POSTGRES_PASSWORD"),
			wantAbsent:   []string{"pi4aK2uBQa"},
			wantExitZero: true, // compose config succeeds; blank substitution is fine
			golden:       "",   // no golden — just check absent pattern
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			envFile := writeEnvFile(t, tt.envKV)
			args := composeConfigArgs(baselineFile, envFile, tt.overlays)

			// #nosec G204 — controlled args, no user input
			cmd := exec.Command(docker, args...)
			out, err := cmd.CombinedOutput()

			if tt.wantExitZero && err != nil {
				t.Fatalf("docker compose config exited with error:\n%s\nerr: %v", out, err)
			}
			if !tt.wantExitZero && err == nil {
				t.Fatalf("expected non-zero exit but got success; output:\n%s", out)
			}

			rendered := string(out)

			for _, want := range tt.wantContains {
				if !strings.Contains(rendered, want) {
					t.Errorf("rendered YAML missing %q\noutput:\n%s", want, rendered)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(rendered, absent) {
					t.Errorf("rendered YAML contains forbidden string %q\noutput:\n%s", absent, rendered)
				}
			}

			// Golden file handling
			if tt.golden == "" {
				return
			}
			goldenPath := filepath.Join(goldenDir, tt.golden)
			if *updateGolden {
				if err2 := os.WriteFile(goldenPath, out, 0644); err2 != nil {
					t.Fatalf("write golden: %v", err2)
				}
				t.Logf("updated golden: %s", goldenPath)
				return
			}
			goldenBytes, readErr := os.ReadFile(goldenPath)
			if os.IsNotExist(readErr) {
				t.Fatalf("golden file missing: %s — run with -update to generate it", goldenPath)
			}
			if readErr != nil {
				t.Fatalf("read golden: %v", readErr)
			}
			if !bytes.Equal(goldenBytes, out) {
				t.Errorf("rendered output does not match golden %s\n--- want ---\n%s\n--- got ---\n%s",
					goldenPath, goldenBytes, out)
			}
		})
	}
}

// withoutKey returns a copy of m without the specified key.
func withoutKey(m map[string]string, key string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		if k != key {
			out[k] = v
		}
	}
	return out
}
