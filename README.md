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
alice-installer update   # refresh an existing deployment in-place
alice-installer restart  # restart existing services in-place
```

## Interactive installation detection

After the splash screen, interactive mode checks the selected workspace before it enters install preflight. The check is read-only and shows only safe evidence categories.

| Detected state | Interactive action |
| --- | --- |
| No current or configured legacy evidence | Install |
| Complete current Compose artifacts | Update; Uninstall is shown but blocked |
| Confirmed legacy deployment on Linux amd64/arm64 | Migration Step 1: reviewed, confirmed backup |
| Partial, conflicting, unreadable, or ambiguous evidence | No lifecycle action; exit or use an explicit CLI route after manual verification |

Current Compose detection uses `.env` and `docker-compose.yml` in `--workspace-dir`. Legacy PM2 probing is supported only on Linux amd64 and arm64. Its default policy is intentionally empty: the installer does not guess PM2 process names, scripts, or deployment paths. Unsupported platforms do not infer a legacy installation.

Uninstall remains informational and blocked. On Linux amd64/arm64 only, a confirmed legacy deployment can enter Migration Step 1. The operator reviews a redacted production configuration, exact PostgreSQL 11 container identity, and protected default destination, then presses Enter to confirm. Nothing is created before that confirmation.

Migration is available only after interactive confirmation on Linux amd64/arm64. Before preflight or any installation side effect, it revalidates the legacy backup and selectively quiesces only a fully proven legacy PM2 process: `/opt/alice-guardian` on TCP `8080`, or `/opt/backend_alice_guardian` on TCP `9090` or `4550`. This requires `pm2`, `ss`, and readable Linux `/proc` process identity metadata; incomplete or ambiguous evidence blocks migration without installing. The existing deploy remains unchanged; then the migration flow waits exactly 60 seconds before it stops only Compose service `backend`. Compose identities are immutable: `backend`/`alice_backend` and `postgresql-master`/`alice_postgresql-master`. PostgreSQL stays running.

Before destructive replacement, the installer retains two validated custom-format backups under `/opt/alice/backups/`: the legacy dump and a newly created target rollback backup. It explicitly drops and recreates the target database, then restores with fail-fast `pg_restore --exit-on-error --no-owner --no-privileges`; it never merges data. A successful restore also requires a fresh connection, a non-system application table, PostgreSQL reachability, and backend health.

Any failure, cancellation, or abandoned migration after PM2 quiescence first completes database rollback when required, then attempts bounded recovery of exactly the PM2 identities it stopped. Final `InstallSuccessMsg` is the only completion that retains the proven legacy PM2 set stopped. Any database failure after replacement begins is an explicit partial cutover. The installer automatically restores only the validated target rollback backup, retains both backups, and starts `backend` only after recovery is healthy. If recovery cannot be proven, PostgreSQL remains running and `backend` remains stopped; follow `RUNBOOK.md`. Passwords, raw dump output, command output, and pgpass paths are never shown.

Install, Update, Restart, `--dry-run`, unattended/headless, Windows, and unsupported-platform routes remain unchanged and cannot invoke restore. Feature rollback removes the interactive restore action only; it never deletes operator backups.

## Update or restart an existing installation

Use `alice-installer update` to refresh containers using the existing workspace artifacts from a prior install.

```sh
# default workspace: ${XDG_CONFIG_HOME:-$HOME/.config}/alice-guardian
alice-installer update

# explicit workspace
alice-installer update --workspace-dir /opt/alice-media
```

Update mode is non-interactive. It reuses `docker-compose.yml` and `.env` from the selected workspace, then runs:

1. `docker compose pull`
2. `docker compose up -d`

If `.env` or `docker-compose.yml` are missing, update exits with an actionable error instead of falling back to install flows.

Use `alice-installer restart` when you only need to restart running services without pulling images or recreating containers.

```sh
# default workspace: ${XDG_CONFIG_HOME:-$HOME/.config}/alice-guardian
alice-installer restart

# explicit workspace
alice-installer restart --workspace-dir /opt/alice-media
```

Restart mode is non-interactive and reuses the same persisted artifact contract as update (`.env`, `docker-compose.yml`, optional `docker-compose.gpu.yml`). It executes exact `docker compose restart` semantics (no install fallback, no `down/up`, no `pull/up`).

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

### End-to-end tests

The E2E harness boots a real Ubuntu 22.04 container with systemd as PID 1 (no Docker pre-installed) and runs `alice-installer --unattended` inside it. The installer's bootstrap downloads and configures Docker from scratch, which is exactly what happens on a fresh production machine.

**Requirements:** a working local Docker daemon and ~500 MB of free disk (basic mode).

```sh
make e2e                   # basic mode — stops before pull + up
FULL_DEPLOY=1 make e2e     # full mode — pulls images (~3 GB) and brings services up
```

The basic mode validates:

- Docker is installed and the `docker compose` plugin works
- Docker and Compose operations work for non-root `testuser` through preauthorized `sudo -n`, without docker-group membership
- `/opt/alice-media` and `/opt/alice-config` are created and writable
- `.env` and both compose files are written to the workspace directory

`FULL_DEPLOY=1` additionally pulls all images and asserts that redis and postgres containers come up healthy.

## Release process

Tag a release on `main` to trigger the goreleaser workflow:

```sh
git tag v0.1.0
git push origin v0.1.0
```

GitHub Actions builds statically-linked binaries for `linux/amd64` and `linux/arm64`, publishes a release with `checksums.txt`, and makes the `scripts/install.sh` one-liner work against the new version.

## License

TBD.
