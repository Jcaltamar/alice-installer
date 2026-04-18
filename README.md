# alice-installer

TUI installer for the Alice Guardian Docker Compose stack. Written in Go with Bubbletea.

## Install (one-liner)

```sh
curl -fsSL https://raw.githubusercontent.com/Jcaltamar/alice-installer/main/scripts/install.sh | bash
```

The script detects your OS + CPU architecture, downloads the matching binary from the latest GitHub release, verifies the SHA256 checksum, and installs to `~/.local/bin/alice-installer`. Override the destination with `INSTALL_DIR=/usr/local/bin` or pin a version with `VERSION=v0.1.0`.

After install:

```sh
alice-installer          # launch the interactive TUI
alice-installer --help   # list flags
alice-installer --dry-run  # run preflight only (no writes, no deploy)
```

## Manual install

Grab the archive for your platform from [Releases](https://github.com/Jcaltamar/alice-installer/releases), verify against `checksums.txt`, extract, and drop the `alice-installer` binary anywhere on your `PATH`.

## Targets (v1)

- Linux amd64
- Linux arm64

macOS and Windows are planned for later iterations.

## SDD artifacts

See `openspec/changes/` for the specification-driven development artifacts that produced this binary:

- `installer-tui/` — base installer (proposal, design, 5 capability specs, 84-task breakdown)
- `installer-bootstrap/` — sudo auto-elevation for /opt directory creation
- `installer-docker-bootstrap/` — Docker install + systemd + usermod actions

## Build from source

```sh
git clone https://github.com/Jcaltamar/alice-installer.git
cd alice-installer
make test        # run unit tests
make test-short  # skip slow/integration
make build       # host arch binary in bin/
make build-all   # cross-compile to dist/ (linux/amd64, linux/arm64)
make lint        # golangci-lint
```

## Layout

```
cmd/installer/       entry point
internal/
  assets/            embedded docker-compose.yml, overlay, .env.example, logo
  compose/           compose runner wrapper
  docker/            docker client wrapper
  envgen/            .env template + password generation
  platform/          arch / OS / GPU detection
  ports/             port scanning + conflict resolution
  preflight/         pre-install checks coordinator
  secrets/           crypto/rand password generation
  theme/             Lipgloss color tokens from LogoNight.png
  tui/               Bubbletea Model/Update/View per state
openspec/            SDD planning artifacts
```

## Release process

Tag a release on `main` to trigger the goreleaser workflow:

```sh
git tag v0.1.0
git push origin v0.1.0
```

GitHub Actions builds statically-linked binaries for `linux/amd64` and `linux/arm64`, publishes a release with `checksums.txt`, and makes the `scripts/install.sh` one-liner work against the new version.

## License

TBD.
