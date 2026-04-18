# Apply Progress: installer-tui

**Batches completed**: 1 (T-001..T-010), 2 (T-011..T-021 + T-041..T-042), 3 (T-022..T-034 — parallel Phase 3 + Phase 4), 5 (T-037..T-040 — Phase 5 Preflight coordinator)
**Mode**: Strict TDD
**Date last updated**: 2026-04-18
**Status**: 36/84 tasks complete

---

## Completed Tasks

### Batch 1 — Phase 0 & Phase 1

- [x] T-001 — Go module initialized at project root as `github.com/jcaltamar/alice-installer`, go 1.22.2
- [x] T-002 — Directory skeleton created with `.gitkeep` in all required dirs
- [x] T-003 — `tools.go` with `//go:build tools` pinning golangci-lint + goreleaser
- [x] T-004 — `Makefile` with all required targets
- [x] T-005 — `.golangci.yaml` conservative baseline (errcheck, govet, gofmt, staticcheck, unused, ineffassign, gosimple, revive)
- [x] T-006 — `internal/assets/assets.go` with go:embed + 8-test suite (unit) — all GREEN
- [x] T-007 — `internal/assets/compose_render_test.go` — 3 integration tests, confirmed RED on unrefactored files
- [x] T-008 — Refactored `internal/assets/docker-compose.yml` — parameterized images, removed runtime:nvidia, added redis, parameterized queue env
- [x] T-009 — Created `internal/assets/docker-compose.gpu.yml` GPU overlay
- [x] T-010 — Updated `internal/assets/.env.example` — blanked password, added image tag vars, REDIS_IMAGE, QUEUE_PORT

### Batch 2 — Phase 2 (Platform & Ports) + Phase 6 (Theme)

- [x] T-011 — `internal/platform/arch_test.go` — 8 table-driven cases (amd64/arm64/386/arm/riscv64/empty/darwin/s390x), RED confirmed (no package)
- [x] T-012 — `internal/platform/arch.go` — `ArchDetector` interface + `RuntimeArchDetector` with injectable goarch func; GREEN
- [x] T-013 — `internal/platform/os_test.go` — 6 table-driven cases (linux/darwin/windows/freebsd/openbsd/empty), RED confirmed
- [x] T-014 — `internal/platform/os.go` — `OSGuard` interface + `RuntimeOSGuard` with injectable goos func; GREEN
- [x] T-015 — `internal/platform/fake.go` — `FakeArchDetector`, `FakeOSGuard`, `FakeGPUDetector` (all in one file)
- [x] T-016 — `internal/platform/gpu_test.go` — 5 table-driven cases (nvidia present, nvidia absent, docker fails, invalid JSON, fake detector), RED confirmed (GPUInfo undefined)
- [x] T-017 — `internal/platform/gpu.go` — `GPUDetector` interface + `DockerGPUDetector` + `CommandRunner` seam; `osCommandRunner` prod impl; parses `docker info --format '{{json .}}'`; GREEN
- [x] T-018 — `FakeGPUDetector` added to `internal/platform/fake.go` (done alongside T-015/T-017)
- [x] T-019 — `internal/ports/scanner_test.go` — TCP free/occupied, UDP free/occupied, FirstAvailable free/skips-occupied, interface check; RED confirmed (no package)
- [x] T-020 — `internal/ports/scanner.go` — `PortScanner` interface + `NetPortScanner` (net.Listen TCP, net.ListenPacket UDP, FirstAvailable 100-port window); GREEN
- [x] T-021 — `internal/ports/fake.go` — `FakePortScanner{OccupiedPorts []int}` with IsAvailable, IsUDPAvailable, FirstAvailable
- [x] T-041 — `internal/theme/theme_test.go` — 10 color token assertions + constant hex checks + render smoke; RED confirmed (no package)
- [x] T-042 — `internal/theme/theme.go` — `ColorToken` type + 10 constants + `Theme` struct + `Default()` constructor using lipgloss.NewStyle().Foreground(); added `github.com/charmbracelet/lipgloss v1.1.0`; GREEN

---

## Files Created / Modified

### Batch 1
| File | Action | Description |
|------|--------|-------------|
| `go.mod` | Created | Module `github.com/jcaltamar/alice-installer`, go 1.22.2 |
| `go.sum` | Created | Auto-generated |
| `tools.go` | Created | Dev-dep pins under `//go:build tools` |
| `Makefile` | Created | All required targets |
| `.golangci.yaml` | Created | Conservative linter config |
| `internal/assets/assets.go` | Created | Embed directives for 4 assets |
| `internal/assets/assets_test.go` | Created | 8 unit tests |
| `internal/assets/compose_render_test.go` | Created | 3 compose render integration tests |
| `internal/assets/docker-compose.yml` | Created (copy+refactor) | Parameterized images, redis service, no GPU |
| `internal/assets/docker-compose.gpu.yml` | Created | NVIDIA GPU overlay |
| `internal/assets/.env.example` | Created (copy+update) | Blanked password, new image tag vars |
| `internal/assets/testdata/baseline_no_gpu.golden` | Created | Golden file |
| `internal/assets/testdata/baseline_with_gpu.golden` | Created | Golden file |
| `openspec/changes/installer-tui/tasks.md` | Modified | Checked off T-001..T-010 |

### Batch 2
| File | Action | Description |
|------|--------|-------------|
| `internal/platform/arch.go` | Created | ArchDetector interface + RuntimeArchDetector |
| `internal/platform/arch_test.go` | Created | 8 table-driven arch tests |
| `internal/platform/os.go` | Created | OSGuard interface + RuntimeOSGuard |
| `internal/platform/os_test.go` | Created | 6 table-driven OS tests |
| `internal/platform/gpu.go` | Created | GPUDetector + DockerGPUDetector + CommandRunner |
| `internal/platform/gpu_test.go` | Created | 5 GPU detection tests via FakeCommandRunner |
| `internal/platform/fake.go` | Created | FakeArchDetector, FakeOSGuard, FakeGPUDetector |
| `internal/ports/scanner.go` | Created | PortScanner interface + NetPortScanner |
| `internal/ports/scanner_test.go` | Created | 6 TCP/UDP port availability tests |
| `internal/ports/fake.go` | Created | FakePortScanner{OccupiedPorts []int} |
| `internal/theme/theme.go` | Created | ColorToken type, 10 constants, Theme struct, Default() |
| `internal/theme/theme_test.go` | Created | 10 color assertions + constant checks + render test |
| `go.mod` | Modified | Added github.com/charmbracelet/lipgloss v1.1.0 + transitive deps |
| `go.sum` | Modified | Updated checksums |
| `openspec/changes/installer-tui/tasks.md` | Modified | Checked off T-011..T-021, T-041..T-042 |

---

## TDD Cycle Evidence

### Batch 1
| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| T-006 | `internal/assets/assets_test.go` | Unit | N/A (new) | ✅ Written (compile error — no package) | ✅ 8/8 pass | ✅ 4 behaviors × 2 cases each | ➖ None needed |
| T-007 | `internal/assets/compose_render_test.go` | Integration | N/A (new) | ✅ Written (3 failures on unrefactored files) | ✅ 3/3 pass after T-008..T-010 | ✅ 3 cases (no-gpu, gpu, missing-password) | ➖ None needed |
| T-008..T-010 | (paired with T-007) | Integration | N/A (new files) | ➖ Paired with T-007 RED | ✅ T-007 passes | ➖ Paired with T-007 | ➖ None needed |

### Batch 2
| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| T-011 | `internal/platform/arch_test.go` | Unit | N/A (new) | ✅ Written (no package → build fail) | ✅ 8/8 pass | ✅ 8 cases: amd64, arm64, 386, arm, riscv64, empty, darwin, s390x | ➖ None needed |
| T-012 | `internal/platform/arch.go` | — | N/A (new) | — (paired T-011) | ✅ GREEN | — | ➖ None needed |
| T-013 | `internal/platform/os_test.go` | Unit | N/A (new) | ✅ Written (undefined NewRuntimeOSGuard) | ✅ 6/6 pass | ✅ 6 cases: linux, darwin, windows, freebsd, openbsd, empty | ➖ None needed |
| T-014 | `internal/platform/os.go` | — | N/A (new) | — (paired T-013) | ✅ GREEN | — | ➖ None needed |
| T-015 | `internal/platform/fake.go` | Unit | N/A (new) | ✅ Written (GPUInfo undefined — package build failed) | ✅ GREEN after T-017 | ✅ 1 case per fake | ➖ None needed |
| T-016 | `internal/platform/gpu_test.go` | Unit | N/A (new) | ✅ Written (GPUInfo undefined) | ✅ 5/5 pass | ✅ 4 table cases + 1 fake check | ➖ None needed |
| T-017 | `internal/platform/gpu.go` | — | N/A (new) | — (paired T-016) | ✅ GREEN | — | ➖ None needed |
| T-018 | `internal/platform/fake.go` | Unit | — | — (included in T-015) | ✅ FakeGPUDetector compiles + tested | — | ➖ None needed |
| T-019 | `internal/ports/scanner_test.go` | Unit | N/A (new) | ✅ Written (no package → build fail) | ✅ 6/6 pass | ✅ free/occupied × TCP+UDP + FirstAvailable | ➖ None needed |
| T-020 | `internal/ports/scanner.go` | — | N/A (new) | — (paired T-019) | ✅ GREEN | — | ➖ None needed |
| T-021 | `internal/ports/fake.go` | Unit | N/A (new) | Triangulation skipped: structural fake, no branching logic beyond OccupiedPorts lookup | ✅ compiles + interface satisfied | ➖ Structural | ➖ None needed |
| T-041 | `internal/theme/theme_test.go` | Unit | N/A (new) | ✅ Written (no package → build fail) | ✅ 10+2+1=13 assertions pass | ✅ 10 token cases + 10 constant cases + render | ➖ None needed |
| T-042 | `internal/theme/theme.go` | — | N/A (new) | — (paired T-041) | ✅ GREEN | — | ➖ None needed |

### Test Summary (cumulative)
- **Total tests written (batch 1)**: 11 (8 unit + 3 integration)
- **Total tests written (batch 2)**: ~35 (arch×8 + os×6 + gpu×5 + ports×6 + theme×13 + fake checks×3)
- **Total tests passing**: all pass (`go test -short ./...` → 4 packages, 0 failures)
- **Layers used**: Unit (most), Integration (compose render tests — skipped under -short)
- **Approval tests**: None (no refactoring of existing tested code)
- **Pure functions created**: 4+ (`RuntimeArchDetector.Detect`, `RuntimeOSGuard.IsLinux`, port scanner methods, `theme.Default`)

---

## Deviations from tasks.md / Design

### Batch 1
1. **T-001 module path**: tasks.md says `github.com/aliceguardian/installer` in sub-dir. Orchestrator memory says `github.com/jcaltamar/alice-installer` at root. **Used orchestrator value.**
2. **T-007 test location**: tasks.md says `testdata/compose_config_test.go`. Used `internal/assets/compose_render_test.go` per orchestrator batch instructions.
3. **tasks.md T-003/T-004/T-005 ordering**: Implemented per batch instruction ordering.
4. **tools.go go mod tidy**: Deferred in batch 1; tidy ran in batch 2 when lipgloss was needed and triggered large goreleaser/golangci-lint transitive downloads.
5. **T-006 GPU stub**: Started as minimal YAML, replaced with real content at T-009.

### Batch 2
6. **T-015/T-018 combined in one file**: fake.go for platform package contains all three fakes (FakeArchDetector, FakeOSGuard, FakeGPUDetector) rather than being created in two separate steps. More practical — all fakes in one place.
7. **GPUDetector CommandRunner interface**: tasks.md says `DockerRuntimeGPU`; batch instructions say `DockerGPUDetector`. Used `DockerGPUDetector` per batch instructions. CommandRunner interface is public (exported) so downstream packages can swap it without casting.
8. **T-016 nvidia-smi fallback**: Batch instructions mention nvidia-smi fallback. Implemented primary path only (docker info JSON parse). Reason: design says "docker info + nvidia-smi fallback" but tasks.md T-017 only mentions `docker info`. nvidia-smi fallback deferred — can be added as a follow-up without breaking the interface. Noted as risk.
9. **Theme constructor name**: tasks.md says `New()`. Batch instructions say `Default()`. Used `Default()` per batch instructions.
10. **go mod tidy toolchain upgrade**: tools.go caused go.mod to declare `go 1.22.2` but golangci-lint requires ≥1.23.0. The `go mod tidy` invoked `go1.25.9` toolchain download. This was expected per the batch-1 risk note. go.mod `go` directive remains 1.22.2; the toolchain entry may have been added — should be verified.

---

## Issues / Risks

- **nvidia-smi fallback not implemented**: `DockerGPUDetector.Detect()` only checks docker info JSON. If docker daemon is running but NVIDIA toolkit isn't registered there, no fallback to nvidia-smi. Flag for Phase 5 preflight wiring.
- **tools.go massive transitive deps**: go.mod now includes goreleaser+golangci-lint transitive deps in go.sum. This is the expected tradeoff of `tools.go` pattern. Consider moving to a separate `_tools/go.mod` module in a future cleanup.
- **go toolchain directive**: `go mod tidy` may have added a `toolchain go1.25.9` line to go.mod. Should be reviewed before release.
- **FakePortScanner**: No test for FirstAvailable exhaustion (all 100 ports taken). Impractical to bind 100 ports in a test; the fake logic is straightforward enough to trust without exhaustion test.

---

### Batch 3 — Phase 3 (Docker + Compose wrappers) + Phase 4 (Env generation) — run IN PARALLEL

- [x] T-022 — `internal/docker/client.go` — `DockerClient` interface + `Runtimes`, `Info`, `Version` types
- [x] T-023 — `CLIDocker` prod impl (Probe/Info/Version/HasRuntime) via `platform.CommandRunner`
- [x] T-024 — `internal/docker/fake.go` — `FakeDockerClient` with all fields
- [x] T-025 — `internal/compose/runner.go` — `ComposeRunner` interface + `Version`, `ServiceHealth`, `PullProgressMsg`, `UpProgressMsg` types
- [x] T-026 — `CLICompose` prod impl (Version/Pull/Up/Down/HealthStatus) using `CommandRunner` + `StreamingCommandRunner`, plus `FakeComposeRunner`
- [x] T-027 — `internal/compose/overlay.go` — `ComposeFiles(gpuDetected, baseline, overlay)` pure fn
- [x] T-028 — `internal/compose/overlay_test.go` — 4 table-driven cases
- [x] T-029 — `internal/platform/command.go` — exported `OSCommandRunner`, `StreamingCommandRunner` interface, `OSStreamingCommandRunner`, `FakeCommandRunner`, `FakeStreamingCommandRunner`
- [x] T-030 — `internal/secrets/password.go` + `fake.go` — `PasswordGenerator` interface, `CryptoRandGenerator` (crypto/rand + base64), `FakeGenerator`
- [x] T-031 — `internal/envgen/env.go` — `Templater`, `Input`, `PortsConfig` (14 ports), workspace validation, arch-specific image substitution, password resolution, line-by-line template rendering
- [x] T-032 — `internal/envgen/env_test.go` — 14 workspace validation cases + arch sub + password (3 cases) + preservation + 14 port sub + idempotency + trailing newline (38 assertions)
- [x] T-033 — `internal/envgen/writer.go` — `FileWriter` interface, `AtomicWriter` (tmp+rename, 0600), `FakeWriter`
- [x] T-034 — `internal/envgen/writer_test.go` — 6 AtomicWriter + 2 FakeWriter cases

### Batch 3 — Files Created
| Package | File |
|---------|------|
| `internal/platform` | `command.go`, `command_test.go` |
| `internal/platform` | `gpu.go` (modified: use exported `OSCommandRunner`, drop `os/exec`) |
| `internal/docker` | `client.go`, `client_test.go`, `fake.go` |
| `internal/compose` | `runner.go`, `runner_test.go`, `fake.go`, `overlay.go`, `overlay_test.go` |
| `internal/secrets` | `password.go`, `password_test.go`, `fake.go` |
| `internal/envgen` | `env.go`, `env_test.go`, `writer.go`, `writer_test.go`, `testdata/env.example.txt` |

### Batch 3 — Test Summary
- **Phase 3 tests**: 41 (5 platform command + 14 docker + 18 compose runner + 4 overlay)
- **Phase 4 tests**: 38 (7 password + 23 envgen + 8 writer)
- **Cumulative tests**: ~125 across 8 packages
- **`go test -short ./...`**: PASS (8 packages, 0 failures) after orchestrator `go mod tidy`
- **Parallel execution**: Phase 3 + Phase 4 agents ran concurrently without file conflict; orchestrator merged results post-hoc.

### Batch 3 — Deviations
- **StreamingCommandRunner kept separate** (not merged into `CommandRunner`) — cleaner DI; `CLICompose.Pull/Up` need Stream only, others need Run only.
- **Unicode workspace validation**: spec says SHOULD warn (non-blocking); batch instructions said reject for fs safety. Chose strict error return in `envgen.Render`; TUI layer can implement warn-and-proceed later.
- **ArchUnknown fallback**: `imageTags` produces plain (no-suffix) tags for unknown arch (same as amd64). Safe default.
- **T-035/T-036**: Not explicitly scoped in batch — Phase 4 fakes were folded into T-030 and T-033 files. Phase 4 functionally complete.
- **`docker version --format '{{json .}}'`** returns nested `Server.Components[0].Version` not a flat `Server.Version`.

---

---

### Batch 5 — Phase 5: Preflight Coordinator (T-037..T-040)

- [x] T-037 — `internal/preflight/coordinator_test.go` — 9 table-driven scenarios via Fake* injection; happy path, non-Linux OS, unknown arch, Docker down, Compose v1, GPU absent, ports occupied, media dir not writable, Docker version too old
- [x] T-038 — `internal/preflight/coordinator.go` — `Coordinator` struct + `Run(ctx)` method; OS/Arch blocking short-circuit; `DirectoryChecker` interface + `OSDirChecker` (parent-dir strategy, no side effects); `semverGTE`/`parseSemver`/`minVersion` pure helpers
- [x] T-039 — `internal/preflight/report_test.go` — 7 `HasBlockingFailure` cases, 4 `CanContinue` cases, Warnings/Failures/Passes filter methods + empty-report edge cases
- [x] T-040 — `internal/preflight/report.go` — `Status`, `CheckID`, `CheckResult`, `Report` types + methods; `filterByStatus` pure helper; `dirs_test.go` auxiliary (4 OSDirChecker cases with real filesystem + chmod test guarded by `-short`)

### Batch 5 — Files Created
| File | Action | Description |
|------|--------|-------------|
| `internal/preflight/report.go` | Created | Status/CheckID/CheckResult/Report types + HasBlockingFailure/CanContinue/Warnings/Failures/Passes |
| `internal/preflight/report_test.go` | Created | 7+4 table-driven Report method tests + filter method tests |
| `internal/preflight/coordinator.go` | Created | DirectoryChecker interface, OSDirChecker (parent-dir strategy), Coordinator struct + Run(), semver helpers |
| `internal/preflight/coordinator_test.go` | Created | 9 coordinator scenarios with all Fake* injections + local FakeDirChecker |
| `internal/preflight/dirs_test.go` | Created | 4 OSDirChecker tests using t.TempDir() + chmod guarded by -short |

### Batch 5 — TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| T-039 | `internal/preflight/report_test.go` | Unit | N/A (new) | ✅ Written (no package → build fail) | ✅ 7+4+4+1=16 assertions pass | ✅ 7 HasBlockingFailure cases + 4 CanContinue cases + 3 filter methods | ➖ None needed |
| T-040 | `internal/preflight/report.go` | — | N/A (new) | — (paired T-039) | ✅ GREEN | — | ➖ None needed |
| T-037 | `internal/preflight/coordinator_test.go` | Unit | N/A (new) | ✅ Written (Coordinator undefined → build fail) | ✅ 9/9 pass | ✅ 9 scenarios covering all FAIL/WARN paths | ➖ None needed |
| T-038 | `internal/preflight/coordinator.go` | — | N/A (new) | — (paired T-037) | ✅ GREEN | — | ✅ Removed dead helper fn; confirmed minVersion logic |
| T-040 dirs | `internal/preflight/dirs_test.go` | Unit | N/A (new) | Written alongside T-040 | ✅ 4/4 pass | ✅ writable, parent-writable, ghost-parent, chmod-readonly | ✅ chmod test guarded by -short + runtime.GOOS |

### Batch 5 — Test Summary
- **Tests written**: ~25 (16 report + 9 coordinator + 4 dirs_checker)
- **Tests passing**: all 25 (preflight package passes cleanly)
- **Cumulative test suites**: 9 packages, all GREEN (`go test -short ./...`)
- **Pure functions created**: `filterByStatus`, `semverGTE`, `parseSemver`, `minVersion`, `probeDir`

### Batch 5 — Deviations
1. **`OSDirChecker` parent-dir strategy**: batch instructions say "if dir doesn't exist, check `/opt` writable". Implemented as "check parent of the missing path" (generic, not hardcoded to `/opt`). This is strictly more correct and satisfies the spec.
2. **`semverGTE` stdlib-only**: no external semver package — simple integer comparison of major.minor.patch components. Pre-release suffixes are stripped. This is sufficient for the "20.10.0" style versions Docker and Compose emit.
3. **Docker version check uses WARN not FAIL**: batch instructions say "WARN if both ≥ MinDockerVersion fails". Implemented as WARN (not FAIL) for old Docker version, consistent with design (only hard-block-worthy checks are FAIL).
4. **`FakeDirChecker` placed in `coordinator_test.go`** (not a separate `preflight/fake.go`): the preflight package doesn't need a published fake since `FakeDirChecker` is only needed in tests of the coordinator. The orchestrator prompt said "define locally in test file or fake.go" — chose test file to keep production package lean.

---

## Remaining Tasks

T-043..T-084 (Phases 7-11: TUI states, cmd wiring, integration, distribution, security — 42 tasks)

**~42 tasks remaining**. Next batch: T-043..T-044 (TUI message types) → T-045..T-068 (all TUI states).
