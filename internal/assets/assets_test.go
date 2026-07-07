package assets_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jcaltamar/alice-installer/internal/assets"
	"gopkg.in/yaml.v3"
)

// pngMagic is the 8-byte PNG file signature (ISO 15948 §12.12).
var pngMagic = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

func TestDockerComposeYAML_NonEmpty(t *testing.T) {
	if len(assets.DockerComposeYAML) == 0 {
		t.Fatal("DockerComposeYAML is empty")
	}
}

func TestDockerComposeYAML_ValidYAML(t *testing.T) {
	var out interface{}
	if err := yaml.Unmarshal(assets.DockerComposeYAML, &out); err != nil {
		t.Fatalf("DockerComposeYAML is not valid YAML: %v", err)
	}
	if out == nil {
		t.Fatal("DockerComposeYAML parsed to nil — file may be empty")
	}
}

func TestDockerComposeGPU_NonEmpty(t *testing.T) {
	if len(assets.DockerComposeGPU) == 0 {
		t.Fatal("DockerComposeGPU is empty")
	}
}

func TestDockerComposeGPU_ValidYAML(t *testing.T) {
	var out interface{}
	if err := yaml.Unmarshal(assets.DockerComposeGPU, &out); err != nil {
		t.Fatalf("DockerComposeGPU is not valid YAML: %v", err)
	}
	if out == nil {
		t.Fatal("DockerComposeGPU parsed to nil — file may be empty")
	}
}

func TestEnvExample_NonEmpty(t *testing.T) {
	if len(assets.EnvExample) == 0 {
		t.Fatal("EnvExample is empty")
	}
}

func TestEnvExample_ContainsWorkspace(t *testing.T) {
	if !bytes.Contains(assets.EnvExample, []byte("WORKSPACE=")) {
		t.Fatal("EnvExample does not contain WORKSPACE= key")
	}
}

func TestEnvExample_ContainsDisabledAsteriskContract(t *testing.T) {
	for _, want := range [][]byte{
		[]byte("ASTERISK_ENABLED=false"),
		[]byte("ASTERISK_AMI_HOST=127.0.0.1"),
		[]byte("ASTERISK_AMI_PORT=5038"),
		[]byte("ASTERISK_AMI_USERNAME="),
		[]byte("ASTERISK_AMI_PASSWORD="),
		[]byte("ASTERISK_CONFIG_DIR=/opt/alice-config/asterisk"),
		[]byte("ASTERISK_INTEGRATION_ENV=/opt/alice-config/asterisk/integration.env"),
	} {
		if !bytes.Contains(assets.EnvExample, want) {
			t.Fatalf("EnvExample missing %q", want)
		}
	}
}

func TestDockerComposeYAML_BackendAsteriskEnvPreservesSharedConfigMount(t *testing.T) {
	compose := string(assets.DockerComposeYAML)
	for _, want := range []string{
		"- ASTERISK_ENABLED=${ASTERISK_ENABLED:-false}",
		"- ASTERISK_AMI_HOST=${ASTERISK_AMI_HOST:-127.0.0.1}",
		"- ASTERISK_AMI_PORT=${ASTERISK_AMI_PORT:-5038}",
		"- ASTERISK_AMI_USERNAME=${ASTERISK_AMI_USERNAME:-}",
		"- ASTERISK_AMI_PASSWORD=${ASTERISK_AMI_PASSWORD:-}",
		"- ASTERISK_CONFIG_DIR=${ASTERISK_CONFIG_DIR:-/opt/alice-config/asterisk}",
		"- ASTERISK_INTEGRATION_ENV=${ASTERISK_INTEGRATION_ENV:-/opt/alice-config/asterisk/integration.env}",
		"- /opt/alice-config:/opt/alice-config",
	} {
		if !strings.Contains(compose, want) {
			t.Fatalf("docker-compose.yml missing %q\n%s", want, compose)
		}
	}
	if strings.Count(compose, "/opt/alice-config:/opt/alice-config") != 1 {
		t.Fatalf("backend shared config mount should remain exactly once, got %d", strings.Count(compose, "/opt/alice-config:/opt/alice-config"))
	}
}

func TestLogoNight_NonEmpty(t *testing.T) {
	if len(assets.LogoNight) == 0 {
		t.Fatal("LogoNight is empty")
	}
}

func TestLogoNight_PNGMagicBytes(t *testing.T) {
	if len(assets.LogoNight) < len(pngMagic) {
		t.Fatalf("LogoNight too short to be a PNG (%d bytes)", len(assets.LogoNight))
	}
	if !bytes.Equal(assets.LogoNight[:len(pngMagic)], pngMagic) {
		t.Fatalf("LogoNight does not start with PNG magic bytes; got: %x", assets.LogoNight[:8])
	}
}
