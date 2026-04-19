package envgen_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/jcaltamar/alice-installer/internal/envgen"
	"github.com/jcaltamar/alice-installer/internal/platform"
	"github.com/jcaltamar/alice-installer/internal/secrets"
)

// fixtureTemplate reads the shared test fixture.
func fixtureTemplate(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/env.example.txt")
	if err != nil {
		t.Fatalf("read testdata fixture: %v", err)
	}
	return data
}

// defaultInput returns a valid Input with amd64, no password generation.
func defaultInput() envgen.Input {
	return envgen.Input{
		Workspace:        "alice-prod",
		Arch:             platform.ArchAMD64,
		GeneratePassword: false,
		Ports: envgen.PortsConfig{
			PostgresPort:     5432,
			BackendPort:      9090,
			WebsocketPort:    4550,
			WebPort:          8080,
			RTSPPort:         8554,
			RedisPort:        6379,
			HLSPort:          8888,
			HLSPort2:         8889,
			HLSPort3:         8890,
			RTMPPort:         1935,
			MilvusPort:       19530,
			MinioAPIPort:     9000,
			MinioConsolePort: 9001,
		},
	}
}

// newTemplater returns a Templater with a controlled password generator.
func newTemplater(gen secrets.PasswordGenerator) *envgen.Templater {
	return &envgen.Templater{PasswordGen: gen}
}

// ---------------------------------------------------------------------------
// Workspace validation (14 cases)
// ---------------------------------------------------------------------------

func TestTemplater_Render_WorkspaceValidation(t *testing.T) {
	tpl := newTemplater(secrets.FakeGenerator{Val: "pw"})
	tmpl := fixtureTemplate(t)

	tests := []struct {
		name      string
		workspace string
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "valid simple name",
			workspace: "alice-prod",
			wantErr:   false,
		},
		{
			name:      "valid hyphens only chars",
			workspace: "a-b-c",
			wantErr:   false,
		},
		{
			name:      "valid digits only",
			workspace: "12345",
			wantErr:   false,
		},
		{
			name:      "valid underscore",
			workspace: "alice_prod",
			wantErr:   false,
		},
		{
			name:      "valid mixed case",
			workspace: "AliceProd",
			wantErr:   false,
		},
		{
			name:      "empty",
			workspace: "",
			wantErr:   true,
			errSubstr: "empty",
		},
		{
			name:      "whitespace only",
			workspace: "   ",
			wantErr:   true,
			errSubstr: "empty",
		},
		{
			name:      "leading dot",
			workspace: ".hidden",
			wantErr:   true,
			errSubstr: "dot",
		},
		{
			name:      "contains forward slash",
			workspace: "foo/bar",
			wantErr:   true,
			errSubstr: "separator",
		},
		{
			name:      "contains backslash",
			workspace: `foo\bar`,
			wantErr:   true,
			errSubstr: "separator",
		},
		{
			name:      "interior space",
			workspace: "my workspace",
			wantErr:   true,
			errSubstr: "whitespace",
		},
		{
			name:      "65 chars (over limit)",
			workspace: strings.Repeat("a", 65),
			wantErr:   true,
			errSubstr: "64",
		},
		{
			name:      "exactly 64 chars (on limit — valid)",
			workspace: strings.Repeat("a", 64),
			wantErr:   false,
		},
		{
			name:      "unicode letter (non-ASCII)",
			workspace: "café",
			wantErr:   true,
			errSubstr: "alphanumeric",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := defaultInput()
			in.Workspace = tt.workspace

			_, err := tpl.Render(tmpl, in)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("Render() expected error (substr %q), got nil", tt.errSubstr)
				}
				if tt.errSubstr != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errSubstr)) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
			} else {
				if err != nil {
					t.Fatalf("Render() unexpected error: %v", err)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Arch substitution
// ---------------------------------------------------------------------------

func TestTemplater_Render_ArchSubstitution(t *testing.T) {
	tpl := newTemplater(secrets.FakeGenerator{Val: "pw"})
	tmpl := fixtureTemplate(t)

	tests := []struct {
		name    string
		arch    platform.Arch
		backend string
		socket  string
		web     string
	}{
		{
			name:    "amd64 uses plain tags",
			arch:    platform.ArchAMD64,
			backend: "jcaltamare/aliceguardian:backend",
			socket:  "jcaltamare/aliceguardian:socket1",
			web:     "jcaltamare/aliceguardian:web_ag",
		},
		{
			name:    "arm64 uses -arm suffix",
			arch:    platform.ArchARM64,
			backend: "jcaltamare/aliceguardian:backend-arm",
			socket:  "jcaltamare/aliceguardian:socket1-arm",
			web:     "jcaltamare/aliceguardian:web_ag-arm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := defaultInput()
			in.Arch = tt.arch

			out, err := tpl.Render(tmpl, in)
			if err != nil {
				t.Fatalf("Render() error: %v", err)
			}

			s := string(out)
			assertKeyValue(t, s, "BACKEND_IMAGE", tt.backend)
			assertKeyValue(t, s, "WEBSOCKET_IMAGE", tt.socket)
			assertKeyValue(t, s, "WEB_IMAGE", tt.web)

			// Redis is multi-arch — always the same
			assertKeyValue(t, s, "REDIS_IMAGE", "redis:7-alpine")
		})
	}
}

// ---------------------------------------------------------------------------
// Password injection
// ---------------------------------------------------------------------------

func TestTemplater_Render_PasswordInjection(t *testing.T) {
	tmpl := fixtureTemplate(t)

	t.Run("password override beats generator", func(t *testing.T) {
		gen := secrets.FakeGenerator{Val: "should-not-be-used"}
		tpl := newTemplater(gen)

		in := defaultInput()
		in.GeneratePassword = false
		in.PasswordOverride = "my-override-password"

		out, err := tpl.Render(tmpl, in)
		if err != nil {
			t.Fatalf("Render() error: %v", err)
		}

		assertKeyValue(t, string(out), "POSTGRES_PASSWORD", "my-override-password")
	})

	t.Run("generator called when no override and GeneratePassword true", func(t *testing.T) {
		gen := secrets.FakeGenerator{Val: "generated-password"}
		tpl := newTemplater(gen)

		in := defaultInput()
		in.GeneratePassword = true
		in.PasswordOverride = ""

		out, err := tpl.Render(tmpl, in)
		if err != nil {
			t.Fatalf("Render() error: %v", err)
		}

		assertKeyValue(t, string(out), "POSTGRES_PASSWORD", "generated-password")
	})

	t.Run("no generation and no override leaves password empty", func(t *testing.T) {
		tpl := newTemplater(secrets.FakeGenerator{Val: "should-not-be-used"})

		in := defaultInput()
		in.GeneratePassword = false
		in.PasswordOverride = ""

		out, err := tpl.Render(tmpl, in)
		if err != nil {
			t.Fatalf("Render() error: %v", err)
		}

		assertKeyValue(t, string(out), "POSTGRES_PASSWORD", "")
	})
}

// ---------------------------------------------------------------------------
// Preservation of comments, blank lines, unmanaged keys
// ---------------------------------------------------------------------------

func TestTemplater_Render_Preservation(t *testing.T) {
	tpl := newTemplater(secrets.FakeGenerator{Val: "pw"})
	tmpl := fixtureTemplate(t)

	out, err := tpl.Render(tmpl, defaultInput())
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	s := string(out)

	// Comments must be preserved verbatim
	if !strings.Contains(s, "# Alice Guardian Environment — fixture for tests") {
		t.Error("header comment not preserved")
	}
	if !strings.Contains(s, "# Database") {
		t.Error("section comment '# Database' not preserved")
	}

	// Unmanaged keys pass through unchanged
	assertKeyValue(t, s, "SOME_CUSTOM_KEY", "custom_value")
	assertKeyValue(t, s, "ANOTHER_KEY", "another_value")
	assertKeyValue(t, s, "NODE_ENV", "production")

	// Blank lines preserved (at least one blank line in output)
	if !strings.Contains(s, "\n\n") {
		t.Error("blank lines not preserved in output")
	}
}

// ---------------------------------------------------------------------------
// Port substitution (all 14 ports)
// ---------------------------------------------------------------------------

func TestTemplater_Render_PortSubstitution(t *testing.T) {
	tpl := newTemplater(secrets.FakeGenerator{Val: "pw"})
	tmpl := fixtureTemplate(t)

	in := defaultInput()
	in.Ports = envgen.PortsConfig{
		PostgresPort:     15432,
		BackendPort:      19090,
		WebsocketPort:    14550,
		WebPort:          18080,
		RTSPPort:         18554,
		RedisPort:        16379,
		HLSPort:          18888,
		HLSPort2:         18889,
		HLSPort3:         18890,
		RTMPPort:         11935,
		MilvusPort:       19531,
		MinioAPIPort:     19000,
		MinioConsolePort: 19001,
	}

	out, err := tpl.Render(tmpl, in)
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	s := string(out)
	assertKeyValue(t, s, "POSTGRES_PORT", "15432")
	assertKeyValue(t, s, "BACKEND_PORT", "19090")
	assertKeyValue(t, s, "WEBSOCKET_PORT", "14550")
	assertKeyValue(t, s, "WEB_PORT", "18080")
	assertKeyValue(t, s, "RTSP_PORT", "18554")
	assertKeyValue(t, s, "REDIS_PORT", "16379")
	assertKeyValue(t, s, "HLS_PORT", "18888")
	assertKeyValue(t, s, "HLS_PORT2", "18889")
	assertKeyValue(t, s, "HLS_PORT3", "18890")
	assertKeyValue(t, s, "RTMP_PORT", "11935")
	assertKeyValue(t, s, "MILVUS_PORT", "19531")
	assertKeyValue(t, s, "MINIO_API_PORT", "19000")
	assertKeyValue(t, s, "MINIO_CONSOLE_PORT", "19001")
}

// ---------------------------------------------------------------------------
// Idempotency
// ---------------------------------------------------------------------------

func TestTemplater_Render_Idempotency(t *testing.T) {
	tpl := newTemplater(secrets.FakeGenerator{Val: "pw"})
	tmpl := fixtureTemplate(t)

	in := defaultInput()
	in.PasswordOverride = "fixed-password"
	in.GeneratePassword = false

	first, err := tpl.Render(tmpl, in)
	if err != nil {
		t.Fatalf("first Render() error: %v", err)
	}

	second, err := tpl.Render(tmpl, in)
	if err != nil {
		t.Fatalf("second Render() error: %v", err)
	}

	if !bytes.Equal(first, second) {
		t.Errorf("Render is not idempotent with fixed password override\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

// ---------------------------------------------------------------------------
// Output ends with newline
// ---------------------------------------------------------------------------

func TestTemplater_Render_EndsWithNewline(t *testing.T) {
	tpl := newTemplater(secrets.FakeGenerator{Val: "pw"})
	tmpl := fixtureTemplate(t)

	out, err := tpl.Render(tmpl, defaultInput())
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	if len(out) == 0 || out[len(out)-1] != '\n' {
		t.Errorf("output does not end with newline; last byte: %q", out[len(out)-1])
	}
}

// ---------------------------------------------------------------------------
// WORKSPACE key substituted
// ---------------------------------------------------------------------------

func TestTemplater_Render_WorkspaceSubstituted(t *testing.T) {
	tpl := newTemplater(secrets.FakeGenerator{Val: "pw"})
	tmpl := fixtureTemplate(t)

	in := defaultInput()
	in.Workspace = "my-custom-workspace"

	out, err := tpl.Render(tmpl, in)
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	assertKeyValue(t, string(out), "WORKSPACE", "my-custom-workspace")
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------
// T-084 — Embedded-asset smoke test
// Renders the real .env.example from internal/assets through Templater.Render
// and asserts critical fields: WORKSPACE, POSTGRES_PASSWORD length, REDIS_IMAGE.
// ---------------------------------------------------------------------------

func TestTemplater_Render_EmbeddedAssetSmoke(t *testing.T) {
	// We import the real embedded asset to simulate what the binary does at runtime.
	// This verifies the full code path: embedded bytes → Render → .env content.
	//
	// The workspace must appear literally as WORKSPACE=my-site.
	// The password must be base64 (44 chars for 32 random bytes: ceil(32/3)*4 = 44).
	// REDIS_IMAGE must be present (multi-arch, no suffix).
	const (
		workspace = "my-site"
		wantPass  = 44 // base64(32 bytes) = 44 chars
	)

	gen := &secrets.CryptoRandGenerator{}
	tr := newTemplater(gen)

	// Use the real embedded .env.example bytes.
	// We read the file directly from the assets package's path to avoid coupling
	// the envgen package to the assets package (no import cycle).
	assetPath := "../assets/.env.example"
	template, err := os.ReadFile(assetPath)
	if err != nil {
		t.Skipf("embedded asset not readable at %s: %v", assetPath, err)
	}

	in := envgen.Input{
		Workspace:        workspace,
		Arch:             platform.ArchAMD64,
		GeneratePassword: true,
		Ports:            envgen.PortsConfig{RedisPort: 6379},
	}

	out, err := tr.Render(template, in)
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	rendered := string(out)

	// 1. WORKSPACE=my-site appears literally.
	if !strings.Contains(rendered, "WORKSPACE="+workspace) {
		t.Errorf("rendered .env missing WORKSPACE=%s\n---\n%s", workspace, rendered)
	}

	// 2. POSTGRES_PASSWORD is non-empty and 44 chars (base64-encoded 32 bytes).
	var gotPassword string
	for _, line := range strings.Split(rendered, "\n") {
		trimmed := strings.TrimRight(line, "\r")
		if strings.HasPrefix(trimmed, "POSTGRES_PASSWORD=") {
			gotPassword = strings.TrimPrefix(trimmed, "POSTGRES_PASSWORD=")
			break
		}
	}
	if gotPassword == "" {
		t.Error("POSTGRES_PASSWORD line not found in rendered .env")
	} else if len(gotPassword) != wantPass {
		t.Errorf("POSTGRES_PASSWORD length = %d, want %d (base64 of 32 bytes)", len(gotPassword), wantPass)
	}

	// 3. REDIS_IMAGE is present (multi-arch, no -arm suffix needed).
	if !strings.Contains(rendered, "REDIS_IMAGE=") {
		t.Error("rendered .env missing REDIS_IMAGE line")
	}
}

// ---------------------------------------------------------------------------

// assertKeyValue checks that the rendered output contains exactly KEY=VALUE on a line.
func assertKeyValue(t *testing.T, output, key, value string) {
	t.Helper()
	target := key + "=" + value
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimRight(line, "\r")
		if trimmed == target {
			return
		}
	}
	t.Errorf("expected line %q in output\ngot:\n%s", target, output)
}
