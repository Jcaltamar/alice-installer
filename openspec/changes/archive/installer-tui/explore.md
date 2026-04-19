# Exploration: installer-tui

**Change**: `installer-tui`
**Project**: `ag-docker-compose`
**Date**: 2026-04-18
**Artifact Store**: hybrid (engram + openspec)

---

## Current State

The project is **pre-code**. The repo contains:
- `docker-compose.yml` — 6 services: `postgresql-master`, `backend`, `websocket`, `web`, `rtsp`, `queue`
- `.env.example` — all env vars with defaults; only `WORKSPACE` is site-specific
- `LogoNight.png` — branding asset: transparent/dark background, bright cyan-green "ALICE" wordmark (confirmed by visual inspection)
- `openspec/` — SDD artifact store initialized, empty changes/specs

There is **no Go code yet**. This is a greenfield TUI installer.

---

## Affected Areas

- `/home/dev/Documents/Apps/AG-DOCKER-COMPOSE/docker-compose.yml` — source of truth for services, ports, image names, volumes
- `/home/dev/Documents/Apps/AG-DOCKER-COMPOSE/.env.example` — template for `.env` generation
- `/home/dev/Documents/Apps/AG-DOCKER-COMPOSE/LogoNight.png` — branding for splash screen
- `(new) cmd/installer/main.go` — TUI entry point
- `(new) internal/tui/` — Bubbletea model, views, styles
- `(new) internal/preflight/` — Docker/NVIDIA/port detection
- `(new) internal/compose/` — image selection, env generation, deploy
- `(new) internal/config/` — .env read/write
- `(new) .github/workflows/` — CI multi-arch integration tests

---

## Analysis by Topic

### 1. Go TUI Framework: Bubbletea Ecosystem

**Recommendation**: Bubbletea + Lipgloss + Bubbles (selected components)

| Component | Use case | Notes |
|-----------|----------|-------|
| `bubbletea` | Core event loop, Model/Update/View | Required |
| `lipgloss` | Styling (colors, borders, layout) | Required — matches logo palette |
| `bubbles/spinner` | Pull/deploy progress indication | Required |
| `bubbles/progress` | Step-by-step progress bar | Required |
| `bubbles/textinput` | WORKSPACE field entry | Required |
| `bubbles/list` | Port conflict selection / service list | Optional — may be overkill for this flow |
| `bubbles/viewport` | Scrollable log output during `docker compose` | Recommended for deploy logs |

**Do NOT use**: `bubbletea-textarea` (multi-line, unnecessary), `huh` (Charm's form library is heavier than needed for a single-field flow).

The installer has a mostly **linear wizard flow** — deep component reuse is less critical than clean state machine design.

---

### 2. Architecture Detection

Use `runtime.GOOS` + `runtime.GOARCH` from Go stdlib. This is detected at **process start**, before the TUI renders.

```go
// internal/arch/detect.go
type Platform struct {
    OS   string // "linux", "windows", "darwin"
    Arch string // "amd64", "arm64"
}

func Detect() Platform {
    return Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
}
```

**Tradeoffs**:
- `runtime.GOOS/GOARCH` reflects the **compiled target**, not the host — but since we cross-compile, the binary is already the right arch/OS. This is correct behavior.
- No CGO required.
- Windows: `runtime.GOOS == "windows"` triggers alternate network_mode handling.

**Image tag mapping** (derived from docker-compose.yml + .env.example inspection):

| Service | amd64 | arm64 |
|---------|-------|-------|
| backend | `jcaltamare/aliceguardian:backend` | `jcaltamare/aliceguardian:backend-arm` |
| websocket | `jcaltamare/aliceguardian:socket1` | `jcaltamare/aliceguardian:socket1-arm` |
| web | `jcaltamare/aliceguardian:web_ag` | `jcaltamare/aliceguardian:web_ag-arm` |
| queue | `jcaltamare/aliceguardian:queue` | `jcaltamare/aliceguardian:queue-arm` |
| rtsp | `bluenviron/mediamtx:latest-ffmpeg` | same (multi-arch manifest) |
| postgresql | `timescale/timescaledb:latest-pg15` | same (multi-arch manifest) |

Note: The current `docker-compose.yml` hardcodes arm tags for backend, websocket, and queue. The installer must **overwrite** these based on detected arch.

---

### 3. Compose Image Selection Strategy

**Two approaches**:

**A. Template docker-compose.yml at install time**
- Installer reads `docker-compose.yml`, substitutes image fields, writes final `docker-compose.yml`
- Pros: The deployed file is self-contained and readable by `docker compose` directly without extra env vars
- Cons: Requires Go YAML manipulation (e.g., `gopkg.in/yaml.v3`); risk of losing comments/formatting; harder to diff

**B. Env var substitution (`${BACKEND_IMAGE}` in compose + set in `.env`)**
- The compose file uses `${BACKEND_IMAGE}`, `${WEBSOCKET_IMAGE}`, `${QUEUE_IMAGE}`, `${WEB_IMAGE}` variables
- `.env` already has `WEB_IMAGE` — pattern is partially established
- Installer writes the correct image tags to `.env` during env generation step
- Pros: Compose file stays clean and version-controlled; standard Docker Compose convention; `.env` is the single source of truth
- Cons: Requires updating `docker-compose.yml` to use `${BACKEND_IMAGE}` etc. for the 3 hardcoded services

**Recommendation**: **Option B (env var substitution)**. The compose file already uses `${WEB_IMAGE}` — extending this pattern to backend/websocket/queue is a one-line change per service. The installer only needs to write vars to `.env`. No YAML parsing required.

**Required compose.yml changes** (to be done in the installer repo, not by the installer at runtime):
- `backend.image`: `jcaltamare/aliceguardian:backend-arm` → `${BACKEND_IMAGE}`
- `websocket.image`: `jcaltamare/aliceguardian:socket1-arm` → `${WEBSOCKET_IMAGE}`
- `queue.image`: `jcaltamare/aliceguardian:queue-arm` → `${QUEUE_IMAGE}`

---

### 4. Port Scanning

**Ports to check** (from .env.example + docker-compose.yml):

| Port | Service | Env var |
|------|---------|---------|
| 5432 | PostgreSQL | `POSTGRES_PORT` |
| 6379 | Redis | `REDIS_PORT` |
| 9090 | Backend | `BACKEND_PORT` |
| 4550 | WebSocket | `WEBSOCKET_PORT` |
| 8080 | Web UI | `WEB_PORT` |
| 8554 | RTSP | `RTSP_PORT` |
| 1935 | RTMP | `RTMP_PORT` |
| 8888 | HLS | `HLS_PORT` |
| 8889 | HLS/WebRTC | `HLS_PORT2` (also `MTX_WEBRTCADDRESS`) |
| 8890 | HLS alt | `HLS_PORT3` |
| 8189 | WebRTC ICE UDP | `MTX_WEBRTCICEUDP` |
| 9190 | Telegram API | `TELEGRAM_API` |
| 18080 | APIFACE | `APIFACE` |
| 2375 | Docker daemon | `PORTDOCKER` |
| 3000 | Queue | `QUEUE_PORT` |

**Detection approach** (Go stdlib, no deps):

```go
// Try to listen; if it fails with EADDRINUSE → port occupied
func IsPortFree(port int) bool {
    ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
    if err != nil {
        return false
    }
    ln.Close()
    return true
}
```

**Tradeoffs**:
- `net.Listen` is stdlib, cross-platform, no root required on Linux
- On Windows, `net.Listen` may require firewall permissions for some ranges — acceptable for installer context
- Alternative (`net.Dial` to detect if something is already listening): complementary approach for checking if a service is ALREADY running (not just free)
- For UDP ports (8189 WebRTC ICE), use `net.ListenPacket("udp", ":8189")`

**TUI flow for port conflicts**: If port is occupied → show conflict dialog → user enters alternative → update `.env` in memory → proceed.

---

### 5. Docker / NVIDIA Toolkit Detection

**Three options**:

**A. Shell out to `docker info`**
- `exec.Command("docker", "info", "--format", "{{json .}}")` → parse JSON
- Pros: Simple, no extra dependencies, works everywhere Docker CLI is installed
- Cons: Spawns a subprocess; `docker info` can be slow (~500ms) if daemon is under load; JSON parsing requires struct definition

**B. Shell out with targeted format**
- `exec.Command("docker", "info", "--format", "{{.Runtimes}}")` for NVIDIA check
- `exec.Command("docker", "version")` for daemon liveness
- Pros: Minimal parsing, fast, focused
- Cons: Two subprocess calls

**C. Docker Go SDK (`github.com/docker/docker/client`)**
- `client.NewClientWithOpts(client.FromEnv)` → `client.Info(ctx)`
- Pros: Strongly typed, no subprocess, better error handling
- Cons: Heavy dependency (~10MB added to binary); overkill for pre-flight checks; CGO complications on some platforms

**Recommendation**: **Option B (targeted shell-out)**. Keep the binary lean. Two subprocess calls for preflight is perfectly acceptable. Use:

```go
// Docker daemon alive?
exec.Command("docker", "version").Run() // exit 0 = running

// Compose v2 plugin?
exec.Command("docker", "compose", "version").Run()

// NVIDIA runtime?
out, _ := exec.Command("docker", "info", "--format", "{{json .Runtimes}}").Output()
// parse: check if "nvidia" key exists in the map
```

For NVIDIA toolkit specifically, also check: `exec.Command("nvidia-smi").Run()` as a secondary signal.

---

### 6. Logo Rendering in Terminal

**Visual inspection result**: Logo is a thin cyan-green "ALICE" wordmark on transparent/dark background. It's text-based, not a complex illustration — ASCII art or styled text can faithfully reproduce it.

**Three options**:

| Option | Description | Cross-platform | Fidelity | Effort |
|--------|-------------|---------------|---------|--------|
| A. Styled Lipgloss text banner | `lipgloss.NewStyle().Foreground(lipgloss.Color("#22d3ee")).Bold(true)` with ALICE ASCII lettering | Excellent | Good | Low |
| B. ASCII art from PNG | Use `go-ascii-art` or pre-generate with `figlet` + embed | Good | Medium | Medium |
| C. Sixel/Kitty graphics | Render PNG pixels in terminal — terminal-dependent | Poor (requires kitty/iTerm2/WezTerm) | Excellent | High |

**Recommendation**: **Option A (Lipgloss text banner)**. The logo IS text ("ALICE") — a bold cyan Lipgloss render at large size (e.g., a simple custom ASCII block font for "ALICE" embedded as a string constant) is portable, fast, and requires zero image decoding. Sixel/Kitty is not acceptable for a production installer targeting headless Linux servers.

**Color palette confirmed from logo**:
- Primary text/accent: `#22d3ee` (cyan, confirmed — matches the wordmark exactly)
- Background: transparent/dark → use `#0f172a` (dark navy for styled panels)
- Status green: `#22c55e`
- Error red: `#ef4444`
- Warning orange: `#f59e0b`
- Muted: `#64748b`

---

### 7. Windows Strategy

**The problem**: All 6 services in `docker-compose.yml` use `network_mode: host`. On Linux, this binds container ports directly to the host network stack. On Windows + Docker Desktop, `network_mode: host` is **silently ignored** (or partially supported via WSL2 backend in newer Docker Desktop 4.29+).

**Concrete implications**:
1. Services that rely on `localhost` connectivity between containers (backend → postgres, queue → postgres) work on Linux via host networking. On Windows, they'd need explicit `ports:` mapping AND internal DNS/networking.
2. The `runtime: nvidia` GPU requirement for backend is effectively incompatible with Windows unless the user has WSL2 + NVIDIA CUDA for WSL2 configured — a very niche setup.
3. Redis is not in the compose file at all (only env vars reference it) — possibly expected to be pre-installed on the host.

**Strategic options**:

**A. Windows = "Docker Desktop + WSL2 backend" only**
- Installer detects Windows → warns user: "Docker Desktop with WSL2 backend required"
- Does NOT rewrite `network_mode`
- Relies on Docker Desktop's partial `host` mode support (experimental in 4.29+)
- Pros: Minimal Windows-specific code; honest about limitations
- Cons: host-mode + NVIDIA on Windows is still fragile

**B. Installer rewrites compose for Windows**
- On Windows, replace `network_mode: host` with explicit `ports:` mappings per service + create a Docker bridge network
- Pros: Actually works on Windows
- Cons: Significantly more complexity; requires YAML rewrite; network topology changes break service-to-service `localhost` references (services must use container names or network aliases)

**C. Windows support: pre-flight gate only**
- Installer on Windows shows a clear "NOT SUPPORTED" preflight failure with explanation
- Defers Windows support to a future version
- Pros: Zero risk of broken installs; honest
- Cons: Excludes Windows users entirely

**Recommendation**: **Option A for now, with Option C as a fallback gate**. The installer should:
1. Detect Windows
2. Check Docker Desktop version (4.29+) and WSL2 backend flag
3. If conditions met → warn user and proceed (experimental)
4. If conditions not met → show preflight failure: "Windows requires Docker Desktop 4.29+ with WSL2 backend. NVIDIA GPU passthrough requires WSL2 + NVIDIA CUDA for WSL2."

This is honest, safe, and doesn't over-promise. The GPU requirement alone makes Windows a hard case.

---

### 8. Testing Strategy: Strict TDD + Multi-Arch

**Unit tests (fast, no Docker)**:
- Bubbletea `Model.Update()` state transitions: use `m.Update(tea.KeyMsg{...})` directly, cast result to `Model`
- For interactive flows: `teatest.NewTestModel(t, m)` + `tm.Send(...)` + `tm.WaitFinished(t, ...)`
- Mock Docker/FS/NVIDIA via **interface injection on Model struct** (not global vars)
- Mock `exec.Command` via a `CommandRunner` interface: `type Runner interface { Run(name string, args ...string) error }`
- Golden file tests for TUI views: store in `testdata/*.golden`, use `-update` flag to regenerate
- `t.TempDir()` for all file system operations

**Integration tests (real Docker, per arch)**:

| Approach | Pros | Cons | Verdict |
|----------|------|------|---------|
| GitHub Actions arm64 runners (native) | Fast, no emulation, official GHA | Paid runners (ARM not free) | Best for amd64 Linux |
| QEMU user-mode (`tonistiigi/binfmt`) | Free, Docker multi-arch builds | 3-5x slower; some syscalls fail | Acceptable for arm64 CI |
| Multipass VMs | Clean instances, realistic | Complex setup, slow cold start | Good for local dev |
| Vagrant | Mature, repeatable | Heavy, slow, VirtualBox dep | Overkill |
| Docker-in-Docker | Simpler for unit-ish tests | Not clean instance | Not suitable for arch tests |

**Recommendation for CI**:
- `linux/amd64`: GitHub Actions `ubuntu-latest` (free, native)
- `linux/arm64`: GitHub Actions arm64 runner OR QEMU + `tonistiigi/binfmt` in a privileged container
- `windows/amd64`: GitHub Actions `windows-latest` (free) — runs installer preflight/env generation only (no actual Docker Compose deploy in CI)

**CI pipeline structure**:
```yaml
jobs:
  unit:
    runs-on: ubuntu-latest
    steps: [go test ./... -short]
  
  integration-linux-amd64:
    runs-on: ubuntu-latest
    steps: [install docker compose v2, go test ./... -tags=integration]
  
  integration-linux-arm64:
    runs-on: ubuntu-latest  # QEMU emulation
    steps: [setup qemu, go test ./... -tags=integration]
  
  integration-windows:
    runs-on: windows-latest
    steps: [install docker desktop, go test ./... -tags=integration -run TestPreflightWindows]
```

---

### 9. Distribution

**Cross-compile matrix**:

```makefile
GOOS=linux   GOARCH=amd64  go build -o dist/installer-linux-amd64
GOOS=linux   GOARCH=arm64  go build -o dist/installer-linux-arm64
GOOS=windows GOARCH=amd64  go build -o dist/installer-windows-amd64.exe
```

**CGO**: Must be disabled (`CGO_ENABLED=0`) for cross-compilation to work cleanly. This means no CGO-dependent libraries. The Docker SDK (if used) requires pure-Go client mode — another reason to prefer shell-out over SDK.

**Signing/Notarization**:
- Linux: Not required. Can optionally sign with `cosign` for supply chain integrity.
- Windows: Unsigned `.exe` will trigger SmartScreen warnings. Options:
  - (a) Accept warnings for internal/ops tooling — simplest
  - (b) Self-signed cert + enterprise trust — for managed deployments
  - (c) EV code signing cert — expensive, overkill for internal tool
- **Recommendation**: Accept SmartScreen for now (this is an ops tool, not consumer software). Document in README.

**Distribution**: GitHub Releases with goreleaser is the cleanest approach. Produces checksums + release notes automatically.

```yaml
# .goreleaser.yml
builds:
  - goos: [linux, windows]
    goarch: [amd64, arm64]
    env: [CGO_ENABLED=0]
archives:
  - format: binary
```

---

### 10. TUI State Machine

**States** (Bubbletea model phases):

```
splash
  └─► preflight
        ├─ [fail] ─► preflight-error (show failures, exit)
        └─ [pass] ─► workspace-input
                        └─► port-check
                              ├─ [conflict] ─► port-conflict (per conflicted port, loop)
                              └─ [clear] ─► env-generation
                                              └─► pull
                                                    └─► deploy
                                                          └─► healthcheck
                                                                ├─ [all healthy] ─► success
                                                                └─ [timeout] ─► partial-success (show unhealthy services)
```

**Bubbletea Model design**:

```go
type Phase int

const (
    PhaseSplash Phase = iota
    PhasePreflight
    PhasePreflightError
    PhaseWorkspaceInput
    PhasePortCheck
    PhasePortConflict
    PhaseEnvGeneration
    PhasePull
    PhaseDeploy
    PhaseHealthcheck
    PhaseSuccess
    PhaseError
)

type Model struct {
    phase       Phase
    platform    arch.Platform
    config      config.Config       // workspace, ports (with user overrides)
    preflight   preflight.Results
    portStatus  []PortStatus
    textInput   textinput.Model     // for WORKSPACE + port conflict inputs
    spinner     spinner.Model
    progress    progress.Model
    viewport    viewport.Model      // for deploy logs
    logs        []string
    err         error
}
```

**Key state transitions**:
- `PhaseSplash` → auto-advance after 1.5s (or keypress) → `PhasePreflight`
- `PhasePreflight` → run checks as `tea.Cmd` (async) → `PhasePreflightError` or `PhaseWorkspaceInput`
- `PhaseWorkspaceInput` → `Enter` key → `PhasePortCheck`
- `PhasePortCheck` → iterate ports → if conflict: `PhasePortConflict` (loop per conflict) else `PhaseEnvGeneration`
- `PhasePortConflict` → user enters new port → back to `PhasePortCheck` for remaining ports
- `PhaseEnvGeneration` → write `.env` → `PhasePull`
- `PhasePull` / `PhaseDeploy` → stream `docker compose pull/up -d` output to viewport → `PhaseHealthcheck`
- `PhaseHealthcheck` → poll service health via `docker inspect` or compose ps → `PhaseSuccess` or `PhaseError`

---

## Recommendation

**Approach: Env-var substitution + targeted shell-out + Lipgloss text banner + Phase A Windows strategy**

1. Modify `docker-compose.yml` (pre-code, in the repo) to use `${BACKEND_IMAGE}`, `${WEBSOCKET_IMAGE}`, `${QUEUE_IMAGE}` for the 3 hardcoded services
2. Installer detects `runtime.GOOS/GOARCH`, sets image tag vars in memory
3. Copies `.env.example` → `.env`, writes arch-specific image tags + user's `WORKSPACE` value
4. No YAML manipulation at runtime — only `.env` writes
5. All Docker checks via targeted `exec.Command` shell-out (lean binary, CGO-free)
6. Logo: Lipgloss styled text in `#22d3ee` on `#0f172a` background
7. Windows: preflight gate with Docker Desktop 4.29+ + WSL2 check, warn-and-proceed
8. CI: GHA ubuntu-latest (amd64) + QEMU (arm64) + windows-latest (preflight only)
9. Distribution: goreleaser with 3 targets, unsigned binaries, CGO_ENABLED=0

---

## Risks

- **NVIDIA on non-NVIDIA hosts**: The backend service has `runtime: nvidia` — if the host has no NVIDIA GPU, `docker compose up` will fail. The installer must detect NVIDIA and either warn/skip or remove the runtime block. This is a hard requirement.
- **Redis not in compose**: `docker-compose.yml` has no Redis service, but `.env.example` has `REDIS_HOST/REDIS_PORT`. The installer must warn if Redis is not running externally.
- **Windows network_mode: host**: Even with Docker Desktop 4.29+ WSL2 backend, `host` mode on Windows is experimental and services using `localhost` inter-service comms may break silently.
- **Port 2375 (Docker daemon TCP)**: `PORTDOCKER=2375` in .env — this suggests the app connects to Docker daemon over TCP (unauthenticated). This is a security risk on production hosts. The installer should warn about this.
- **Hardcoded credentials in queue service**: `docker-compose.yml` has `POSTGRES_PASSWORD=pi4aK2uBQa+...` hardcoded in the queue service environment block — not using env var substitution. The installer should detect and warn, or override with the password from `.env`.
- **Binary size**: CGO_ENABLED=0 is required for cross-compilation. All dependencies must be pure Go. The Docker SDK adds ~10MB — confirmed reason to prefer shell-out.
- **Goreleaser CGO**: Must validate that all transitive dependencies (lipgloss, bubbles, bubbletea) are pure Go — they are, but must be confirmed when go.mod is written.

---

## Ready for Proposal

**Yes.** All 10 investigation areas are covered with concrete tradeoffs and recommendations. The proposal phase can proceed with:
- Framework: Bubbletea + Lipgloss + Bubbles (spinner, progress, textinput, viewport)
- Image selection: env-var substitution strategy (requires 3-line change to docker-compose.yml)
- Detection: runtime.GOOS/GOARCH + shell-out for Docker/NVIDIA
- Windows: preflight gate (warn + proceed if Docker Desktop 4.29+ WSL2)
- Testing: teatest unit + GHA multi-arch CI with QEMU for arm64
- Distribution: goreleaser, 3 targets, CGO_ENABLED=0, unsigned
- State machine: 12-phase linear wizard with looping port-conflict resolution

**Key open question for user before proposal**: Does the backend service ALWAYS require NVIDIA GPU, or should the installer make it optional (skip GPU reservation if no NVIDIA detected)? This affects whether the install can succeed on non-GPU Linux hosts.
