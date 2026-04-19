# Design: installer-tui (v1, Linux)

## Technical Approach

Single-binary Go+Bubbletea TUI. Orchestrates preflight → env gen → compose up → healthcheck on a fresh Linux host. Linux-only (amd64+arm64), CGO_ENABLED=0, no runtime deps. NVIDIA handled as a **compose overlay** (not YAML rewriting). Arch-specific image tags propagated via `.env` variable substitution. All system boundaries (Docker, FS, OS, ports) behind interfaces for strict-TDD unit isolation.

## Package Layout

```
cmd/installer/main.go                 entrypoint; wires real impls, launches tea.Program
internal/tui/                         Model, Update, View, state structs, messages, theme glue
internal/preflight/                   orchestrates probes; returns PreflightReport
internal/envgen/                      template .env.example + inputs → rendered .env
internal/compose/                     ComposeRunner impl; overlay -f flag selector; pull/up/down/ps
internal/ports/                       PortScanner (TCP+UDP); FirstAvailable fallback
internal/docker/                      DockerClient impl; shells to `docker`, parses JSON
internal/platform/                    ArchDetector, OSGuard, GPUDetector (stdlib + exec)
internal/secrets/                     crypto/rand password generator for POSTGRES_PASSWORD
internal/assets/                      //go:embed docker-compose.yml, docker-compose.gpu.yml, .env.example, logo ascii
internal/theme/                       Lipgloss Theme struct + semantic tokens
testdata/                             golden .env fixtures, mock docker info payloads
```

Each package exports one or two narrow interfaces + one prod struct. `internal/tui` depends on interfaces only — never on prod implementations.

## Architecture Decisions

| # | Decision | Choice | Alternatives | Rationale |
|---|----------|--------|--------------|-----------|
| 1 | GPU toggle | Overlay file `docker-compose.gpu.yml` | Runtime YAML rewrite; compose profiles | Keeps baseline diffable in git; profiles can't toggle `runtime:` |
| 2 | Image tags | `.env` substitution (`${BACKEND_IMAGE}` etc.) | Render-at-install; two compose files | Leverages existing `${WEB_IMAGE}` pattern; no YAML mutation |
| 3 | Docker access | Shell out to `docker` CLI | Docker SDK | CGO_ENABLED=0; smaller binary; no version coupling |
| 4 | GPU detection | `docker info --format {{json .Runtimes}}` + fallback `nvidia-smi` | NVML cgo bindings | No CGO; works without drivers installed on host |
| 5 | DI style | Interface fields on `Model` struct | Package globals; functional options | Unit-testable; strict TDD mandate |
| 6 | Long ops | `tea.Cmd` returning progress `tea.Msg` ticks | Goroutine + channels direct to View | Idiomatic Bubbletea; Update loop stays non-blocking |
| 7 | Redis | Add `redis:7-alpine` service, `network_mode: host` | External prerequisite | Closes .env-vs-compose gap; self-contained deploy |
| 8 | Password gen | `crypto/rand` 32-byte base64 on first-run | Reuse `.env.example` value | Example value is leaked in git — MUST rotate |
| 9 | Distribution | goreleaser | Manual `go build` matrix | Reproducible; checksums; GitHub Release integration |
| 10 | Port UDP | `net.ListenPacket("udp", ...)` for 8189 | TCP probe only | 8189 is WebRTC ICE UDP — TCP probe misses conflicts |

## Key Interfaces

```go
// internal/docker
type DockerClient interface {
    Version(ctx) (string, error)
    ComposeVersion(ctx) (string, error)       // v2 plugin check
    Runtimes(ctx) ([]string, error)           // reads `docker info`
    Info(ctx) (DockerInfo, error)
}

// internal/compose
type ComposeRunner interface {
    Pull(ctx, files []string, env []string) <-chan ProgressEvent
    Up(ctx, files []string, env []string) <-chan ProgressEvent
    Down(ctx, files []string) error
    PS(ctx, files []string) ([]ServiceStatus, error)
    WaitHealthy(ctx, files []string, timeout) (HealthReport, error)
}

// internal/ports
type PortScanner interface {
    IsAvailableTCP(port int) bool
    IsAvailableUDP(port int) bool
    FirstAvailableTCP(start int) (int, error)
}

// internal/platform
type ArchDetector interface { GOARCH() string }   // "amd64" | "arm64"
type OSGuard     interface { IsSupportedLinux() (bool, string) }
type GPUDetector interface { HasNVIDIA(ctx) (bool, string, error) }

// internal/envgen
type EnvTemplater interface { Render(in EnvInput) (string, error) }

// internal/secrets
type PasswordGen interface { Generate() (string, error) }

// internal/fs (fs.go)
type FilesystemOps interface {
    Mkdir(path string, mode os.FileMode) error
    WriteAtomic(path string, data []byte, mode os.FileMode) error
    Exists(path string) bool
    Read(path string) ([]byte, error)
}
```

Each has a prod struct (`RealDockerClient`, `OSScanner`, etc.) and a fake struct in `*_test.go` satisfying the interface with programmable fixtures (table-driven). No gomock/mockery — hand-rolled fakes.

## Bubbletea State Machine

```
[Splash] --Tick(1.5s)--> [Preflight]
[Preflight] --ok--> [WorkspaceInput]   --err--> [PreflightError] --retry--> [Preflight] / --abort--> [Exit]
[WorkspaceInput] --submit--> [PortScan]
[PortScan] --all-free--> [EnvWrite]    --conflict--> [PortConflict] --resolve--> [PortScan]
[EnvWrite] --ok--> [ComposePull]       --err--> [FatalError]
[ComposePull] --ok--> [ComposeUp]      --err--> [FatalError]
[ComposeUp] --ok--> [Healthcheck]      --err--> [PartialSuccess]
[Healthcheck] --all-healthy--> [Success]
              --timeout--> [PartialSuccess]
Global: Ctrl+C → [AbortConfirm] → [Exit]; q (on terminal states) → [Exit]
        Esc → previous state (where meaningful); WindowSizeMsg → re-layout
```

### Messages

```go
type PreflightResult   struct { Report PreflightReport; Err error }
type PortScanResult    struct { Conflicts []PortConflict }
type PortResolved      struct { Name string; Port int }
type EnvWritten        struct { Path string; Err error }
type PullProgress      struct { Service, Layer string; Pct float64 }
type PullDone          struct { Err error }
type DeployProgress    struct { Service string; State string }
type DeployDone        struct { Err error }
type HealthTick        struct { Statuses []ServiceHealth }
type HealthReport      struct { AllHealthy bool; Failed []string }
type ErrorMsg          struct { Stage string; Err error; Recoverable bool }
type AbortMsg          struct{}
```

### Long-running ops

`Pull` and `Up` return `<-chan ProgressEvent`. A `tea.Cmd` wraps the first receive; each subsequent tick re-issues a Cmd that reads the next event. Healthcheck uses `tea.Tick(2s)` polling `PS` + `WaitHealthy` until all services report `healthy` or a 120s overall budget elapses.

## Compose Overlay Mechanics

**Baseline `docker-compose.yml`** (post-refactor):

- `backend.image: ${BACKEND_IMAGE}` (was hardcoded arm tag)
- `websocket.image: ${WEBSOCKET_IMAGE}` (was hardcoded arm tag)
- `web.image: ${WEB_IMAGE}` (already parameterized — keep)
- `queue.image: ${QUEUE_IMAGE}` (was hardcoded arm tag)
- `queue.environment.POSTGRES_PASSWORD=${POSTGRES_PASSWORD}` (was leaked plaintext)
- `queue.environment.POSTGRES_USER=${POSTGRES_USER}` / `POSTGRES_DATABASE=${POSTGRES_DATABASE}` (de-hardcode)
- **Remove** `backend.runtime: nvidia` + `backend.deploy.resources.reservations.devices` block → moves to overlay
- **Add** `redis` service: `image: redis:7-alpine`, `network_mode: host`, healthcheck `redis-cli -p ${REDIS_PORT} ping`

**Overlay `docker-compose.gpu.yml`**:

```yaml
services:
  backend:
    runtime: nvidia
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu]
```

**Invocation**:

| GPU | Command |
|-----|---------|
| present | `docker compose -f docker-compose.yml -f docker-compose.gpu.yml --env-file .env up -d` |
| absent | `docker compose -f docker-compose.yml --env-file .env up -d` |

Assets are embedded via `//go:embed` in `internal/assets/assets.go`. Installer extracts them to `$PWD/alice-guardian/` on first run (or updates if checksums differ, preserving user-edited `.env`).

## Image Tag Resolution

| Service | amd64 | arm64 |
|---------|-------|-------|
| backend | `jcaltamare/aliceguardian:backend` | `jcaltamare/aliceguardian:backend-arm` |
| websocket | `jcaltamare/aliceguardian:socket1` | `jcaltamare/aliceguardian:socket1-arm` |
| web | `jcaltamare/aliceguardian:web_ag` | `jcaltamare/aliceguardian:web_ag-arm` |
| queue | `jcaltamare/aliceguardian:queue` | `jcaltamare/aliceguardian:queue-arm` |
| rtsp | `bluenviron/mediamtx:latest-ffmpeg` | same (manifest multi-arch) |
| postgres | `timescale/timescaledb:latest-pg15` | same (manifest multi-arch) |
| redis | `redis:7-alpine` | same (manifest multi-arch) |

`envgen` maps `runtime.GOARCH` → suffix (`""` for amd64, `-arm` for arm64) → writes `BACKEND_IMAGE`, `WEBSOCKET_IMAGE`, `WEB_IMAGE`, `QUEUE_IMAGE` into `.env`.

## Theme (Lipgloss)

```go
type Theme struct {
    Background  lipgloss.Color // #0f172a dark navy
    Surface     lipgloss.Color // #1e293b
    Primary     lipgloss.Color // #22d3ee cyan (logo)
    Accent      lipgloss.Color // #4fd1c5 teal
    Success     lipgloss.Color // #22c55e green
    Warning     lipgloss.Color // #f59e0b orange
    Danger      lipgloss.Color // #ef4444 red
    TextPrimary lipgloss.Color // #f1f5f9
    TextMuted   lipgloss.Color // #64748b
    Border      lipgloss.Color // #334155
}
```

| State | Uses |
|-------|------|
| Splash | Background + Primary wordmark + TextMuted tagline |
| Preflight | Border frame; per-check line: Success check / Warning dot / Danger X |
| PortConflict | Danger banner title; TextPrimary body; Primary highlight on suggested port |
| Pull/Deploy | Primary progress bar; Accent for pulling layer; TextMuted for pulled |
| Success | Success banner; Accent service list |
| PartialSuccess | Warning banner; Danger per-failed-service line |
| FatalError | Danger banner; TextPrimary stderr excerpt |

## Testing Strategy

| Layer | What | Approach |
|-------|------|----------|
| Unit `tui` | state transitions, keybindings | teatest.NewTestModel + `SendKey`/`SendMsg`; golden View snapshots |
| Unit `preflight` | probe orchestration | fake DockerClient, GPUDetector, PortScanner; table-driven |
| Unit `envgen` | template render | table: (input → expected .env string); golden file compare |
| Unit `ports` | scan + first-available | listen on ephemeral port to simulate conflict |
| Unit `compose` | arg construction | assert correct `-f` flags for GPU vs non-GPU; fake exec.Command |
| Unit `platform` | arch/os/gpu | inject fake `runtime.GOARCH`; fake `docker info` JSON |
| Integration | end-to-end | `-tags=integration` build tag; runs against real Docker on CI; creates `/tmp/alice-test-$X`, spins stack, asserts healthy, tears down |
| Matrix | arm64 | GitHub Actions `docker/setup-qemu-action` + binfmt_misc; runs integration tests under arm64 emulation |

Key teatest flows (10):

1. splash → preflight happy path
2. preflight fails: no Docker → error state → retry → abort
3. preflight warns: no GPU → proceed non-GPU path
4. workspace input validation (empty, too long, valid)
5. port scan clean → straight to env-write
6. port conflict resolution loop (user accepts suggested port)
7. port conflict: user rejects, enters custom port
8. pull progress ticks render
9. deploy + healthcheck timeout → partial success
10. Ctrl+C during pull → abort confirm → compose down cleanup

Coverage target: ≥80% on `internal/*`. `cmd/installer` covered by integration smoke only.

## Cross-Compile / Release

`.goreleaser.yaml` (outline):

```yaml
builds:
  - id: alice-installer
    main: ./cmd/installer
    env: [CGO_ENABLED=0]
    flags: [-trimpath]
    ldflags: ["-s -w -X main.version={{.Version}} -X main.commit={{.ShortCommit}}"]
    goos: [linux]
    goarch: [amd64, arm64]
archives:
  - format: binary
    name_template: "alice-installer-{{.Os}}-{{.Arch}}"
checksum: { name_template: "checksums.txt" }
```

**GitHub Actions** (`.github/workflows/ci.yml`):

```
jobs:
  test:          ubuntu-latest, go test -race -short ./...
  test-arm64:    ubuntu-latest + QEMU, go test -short ./... under arm64
  integration:   ubuntu-latest, docker installed, go test -tags=integration ./...
  release:       on tag v*, goreleaser --clean; uploads to GitHub Release
```

`Makefile` wraps common dev loops: `make test`, `make test-integration`, `make build`, `make release-dry`.

## Security & Hygiene Decisions

| Issue | Action |
|-------|--------|
| Leaked `POSTGRES_PASSWORD` in `queue` service env (git history) | Remove; substitute `${POSTGRES_PASSWORD}`. Release notes MUST instruct operators to rotate on any existing deployment. |
| Example `POSTGRES_PASSWORD` also committed in `.env.example` | Installer **generates** a new 32-byte base64 password via `crypto/rand` on first run; `.env.example` stripped to placeholder `POSTGRES_PASSWORD=` (empty) |
| `.env` contains secrets | Written with 0600; parent dir 0700 |
| `PORTDOCKER=2375` unauth TCP | Preflight warning: "Docker daemon TCP port 2375 exposed — consider firewalling" |
| Binary unsigned v1 | Document SHA256 checksums in release; signing deferred to v2 |

## Data Flow

```
     .env.example (embedded) ──┐
                               ▼
 Arch + GPU detection ──→  EnvTemplater ──→ .env (0600)
                               │
                               ▼
         PortScanner ──→ ConflictResolver ──→ .env (ports patched)
                               │
                               ▼
                   ComposeRunner.Pull ──progress──→ TUI
                               │
                               ▼
                   ComposeRunner.Up(overlay?) ──progress──→ TUI
                               │
                               ▼
                   WaitHealthy (poll ps) ──→ HealthReport ──→ Success/Partial
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `docker-compose.yml` | Modify | Parameterize image tags; de-hardcode queue env; remove GPU block; add redis service |
| `docker-compose.gpu.yml` | Create | GPU overlay: runtime+devices on backend only |
| `.env.example` | Modify | Remove committed password value; add `BACKEND_IMAGE`/`WEBSOCKET_IMAGE`/`QUEUE_IMAGE` placeholders |
| `go.mod` / `go.sum` | Create | Module init; bubbletea, lipgloss, bubbles deps |
| `cmd/installer/main.go` | Create | Entrypoint |
| `internal/**` | Create | All packages listed in Package Layout |
| `.goreleaser.yaml` | Create | Release config |
| `.github/workflows/ci.yml` | Create | CI: test + arm64 + integration + release |
| `Makefile` | Create | Dev loop wrapper |
| `testdata/**` | Create | Golden fixtures |

## Migration / Rollout

No data migration. Existing deployments (if any) continue using pre-installer-tui compose file. Release notes must flag:

1. Rotate `POSTGRES_PASSWORD` (leaked in git)
2. New `BACKEND_IMAGE`/`WEBSOCKET_IMAGE`/`QUEUE_IMAGE` env vars required — document the mapping table
3. GPU hosts: ensure installer sees `nvidia` runtime in `docker info`
4. Redis now in-compose (prior external Redis will conflict on 6379 → installer preflight catches this)

## Open Questions

- [ ] Do we need a `--non-interactive` flag for CI/unattended installs? (v1 scope: no — defer)
- [ ] Should installer self-update? (v1 scope: no — users redownload)
- [ ] Log output destination: stdout only, or also `$PWD/alice-installer.log`? (lean: file log for post-mortem)
