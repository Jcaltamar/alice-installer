// Package assets embeds the Docker Compose configuration files and
// supporting assets into the binary so the installer runs without any
// filesystem prerequisites on the target host.
//
// Single source of truth: the files in this directory are what the
// installer ships. The repo-root copies (docker-compose.yml, .env.example,
// LogoNight.png) are the canonical developer view; the copies here are
// kept in sync during development and become the embedded artifacts.
package assets

import _ "embed"

// DockerComposeYAML is the baseline docker-compose.yml embedded at build time.
//
//go:embed docker-compose.yml
var DockerComposeYAML []byte

// DockerComposeGPU is the NVIDIA GPU overlay (docker-compose.gpu.yml)
// embedded at build time. Applied via `-f` when the GPU runtime is detected.
//
//go:embed docker-compose.gpu.yml
var DockerComposeGPU []byte

// EnvExample is the .env.example template embedded at build time.
// The installer renders this into a per-deployment .env file.
//
//go:embed .env.example
var EnvExample []byte

// LogoNight is the Alice Guardian branding PNG for the splash screen.
//
//go:embed LogoNight.png
var LogoNight []byte

// LogoAliceSecurity is the Alice Security brand mark rendered on the splash
// screen via pixterm's half-block ANSI renderer. The embedded copy is a
// 256x256-fit resample of the full-resolution source at the repo root;
// regenerate with `go run ./scripts/prescale-logo`.
//
//go:embed logo_alice_security.png
var LogoAliceSecurity []byte
