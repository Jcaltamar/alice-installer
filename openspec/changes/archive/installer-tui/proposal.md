# Proposal: Alice Guardian TUI Installer (v1, Linux)

## Intent

Ship a single self-contained binary that turns the Alice Guardian Docker Compose stack from "clone + hand-edit + hope" into a guided install. Operator runs one binary on a fresh Linux host, answers one question (`WORKSPACE`), and ends with a healthy running stack. Eliminates tribal knowledge about arch-specific image tags, port conflicts, NVIDIA toggling, and `.env` generation.

## Scope

### In Scope
- Go + Bubbletea TUI installer, Linux amd64 + arm64.
- Pre-flight: Docker daemon, Compose v2, arch detection, NVIDIA Toolkit detection, port availability.
- `.env` generation from `.env.example` defaults, with only `WORKSPACE` prompted interactively.
- Arch-aware image-tag selection written into `.env` (no runtime YAML rewriting).
- Dynamic NVIDIA handling via compose overlay (`docker-compose.gpu.yml`).
- Port-conflict resolution loop inside the TUI.
- Orchestration of `docker compose pull`, `up -d`, healthcheck polling with progress feedback.
- Cross-compiled binaries (`alice-installer-linux-amd64`, `alice-installer-linux-arm64`) via goreleaser.
- Compose-file cleanup: parameterize 3 hardcoded image tags, remove hardcoded `POSTGRES_PASSWORD` in `queue`, decide Redis fate.

### Out of Scope
- Windows and macOS installers (deferred to future changes).
- Binary signing, notarization, auto-update.
- Installing Docker itself (prerequisite, not responsibility).
- GUI or web-based installer.
- Backup / migration of existing installs.

## Capabilities

### New Capabilities
- `installer-preflight`: host probing — Docker, Compose, arch, NVIDIA, port scan.
- `installer-env-generation`: deterministic `.env` rendering from template + detected values + user input.
- `installer-compose-orchestration`: pull / up / health polling with overlay selection.
- `installer-tui-flow`: Bubbletea state machine and screen transitions.
- `installer-distribution`: cross-compilation + release packaging.

### Modified Capabilities
- None (no prior specs exist).

## Approach

**A. Compose rendering — Overlay pattern (option ii).** Commit GPU-less baseline `docker-compose.yml` + `docker-compose.gpu.yml` (adds only `runtime: nvidia` + `deploy.resources.reservations`). Installer chooses `-f` flags at `up` time. Keeps YAML auditable in git, no templating engine, diff-friendly, future-compatible with profiles if needed. Rejected (i) render-at-install (loses git diff, drifts from source of truth) and (iii) profiles (don't cleanly toggle `runtime:` / `deploy.resources`).

**B. Image tag selection — env-var substitution.** Refactor `docker-compose.yml`:
- line 24 `jcaltamare/aliceguardian:backend-arm` → `${BACKEND_IMAGE}`
- line 79 `jcaltamare/aliceguardian:socket1-arm` → `${WEBSOCKET_IMAGE}`
- line 129 `jcaltamare/aliceguardian:queue-arm` → `${QUEUE_IMAGE}`
- line 96 already `${WEB_IMAGE}` — keep.
Installer picks `amd64` vs `arm64`-suffixed tag per `runtime.GOARCH` and writes all four to `.env`.

**C. Module layout.**
```
cmd/installer/main.go
internal/tui/        # Bubbletea Model/Update/View, screens, styles
internal/preflight/  # DockerProbe, ArchDetector, GPUDetector, PortScanner (interfaces)
internal/env/        # template parser, renderer, .env writer
internal/compose/    # compose runner (pull/up/ps), overlay selector
internal/platform/   # shell-out Runner interface (exec.Command wrapper — mockable)
internal/branding/   # lipgloss palette + logo banner
testdata/            # golden .env, sample compose outputs
```
Interface seams on model struct for DI: `Runner`, `FS`, `PortScanner`, `ArchDetector`, `GPUDetector`. Never globals.

**D. TUI state flow.**
`splash → preflight → (preflight-error?) → workspace-input → port-scan → (port-conflict loop?) → env-write → compose-pull → compose-up → healthcheck → result(success|partial|error)`. Each state is a named Bubbletea sub-model returning transition messages.

**E. Compose fixes (pre-code).**
1. Parameterize 3 hardcoded image tags (see B).
2. `queue` service: replace hardcoded `POSTGRES_PASSWORD=pi4aK2u...` and other hardcoded DB values with `${...}` references — the value leaked in git is a security issue and must be rotated.
3. **Redis**: add as service to compose (`redis:7-alpine`, host network, `REDIS_PORT`). Installer pre-flight also verifies the port. Declaring internally avoids silent failures when operators forget to install it externally.

**F. Testing strategy.**
- Unit: `teatest` per state, `Runner`/`FS`/`PortScanner` mocks, golden `.env` files.
- Integration: GitHub Actions matrix. `ubuntu-latest` = amd64 native; `ubuntu-latest` + QEMU binfmt = arm64 emulated. Build tag `-tags=integration` gates tests that require a real Docker daemon. `-short` skips slow paths for local dev.
- TDD strict: every Model transition has a failing teatest before implementation.

**G. Distribution.**
- goreleaser config, two targets: `linux/amd64`, `linux/arm64`.
- `CGO_ENABLED=0`, static binaries, no runtime deps.
- Output names: `alice-installer-linux-amd64`, `alice-installer-linux-arm64`.
- `Makefile` wrapper: `make build`, `make test`, `make release`.
- **No signing in v1** (explicitly documented; deferred).

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `docker-compose.yml` | Modified | Parameterize 3 image tags; fix `queue` env; add `redis` service; remove `runtime: nvidia` + GPU `deploy` block |
| `docker-compose.gpu.yml` | New | GPU overlay with `runtime: nvidia` + reservations |
| `.env.example` | Modified | Add `BACKEND_IMAGE`, `WEBSOCKET_IMAGE`, `QUEUE_IMAGE`; remove leaked password |
| `cmd/installer/` | New | Binary entry point |
| `internal/**` | New | TUI, preflight, env, compose, platform, branding packages |
| `testdata/` | New | Golden files for env render + compose selection |
| `.github/workflows/` | New | CI matrix (amd64 + arm64 via QEMU), goreleaser |
| `Makefile` | New | Dev/build/test/release wrapper |
| `go.mod` | New | Module bootstrap |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Leaked `POSTGRES_PASSWORD` in git history (`queue` service) | High | Rotate in production before release; parameterize in fix step; document in release notes |
| QEMU arm64 emulation too slow / flaky in CI | Med | Use `-short` to skip heavy paths; schedule full run nightly; consider self-hosted arm64 runner later |
| `docker compose` behaviors differ across minor versions | Med | Require Compose v2.x; pin minimum in preflight; fail fast with readable error |
| Redis addition changes existing deployments' behavior | Low | Default `REDIS_HOST=127.0.0.1`; document migration note |
| Host networking / `/opt` paths break on SELinux-enforcing hosts | Low | Detect and warn; do not auto-configure SELinux |

## Rollback Plan

The installer is additive: it creates binaries + writes `.env` + runs `docker compose`. Rollback:
1. `docker compose down -v` to stop stack (operator-driven).
2. Delete generated `.env` (restore from `.env.example` if needed).
3. Revert compose-file changes via git — baseline restored from the commit tagged `pre-installer-tui`.
4. Uninstall = delete the binary. No system-level changes, no background services, no privileged files written.

A pre-installer git tag will be cut before the compose-file refactor merges.

## Dependencies

- Go 1.22+, `charmbracelet/bubbletea`, `bubbles`, `lipgloss`, `teatest`.
- Docker Engine 24+ and Compose v2 plugin (operator prerequisite; not installed by us).
- NVIDIA Container Toolkit (optional; probed at runtime).
- goreleaser for releases; GitHub Actions for CI.

## Success Criteria

- [ ] Fresh Linux host (amd64 OR arm64) with Docker installed → one command → healthy stack, zero extra manual config.
- [ ] GPU host: NVIDIA overlay applied automatically; backend container shows GPU access.
- [ ] Non-GPU host: backend runs CPU-only; no fatal error, only a warning in the TUI summary.
- [ ] Port conflict on any of the 15 tracked ports → TUI prompts for alternative and persists it to `.env`.
- [ ] Every Bubbletea state has a passing teatest unit test.
- [ ] One end-to-end integration test per arch passes on CI against a clean container.
- [ ] Two binaries published per release: `alice-installer-linux-amd64`, `alice-installer-linux-arm64`.
- [ ] Leaked DB password removed from `docker-compose.yml` and rotated in production.
