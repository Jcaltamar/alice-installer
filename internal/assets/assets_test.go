package assets_test

import (
	"bytes"
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
