# Tasks: installer-docker-bootstrap

TDD-paired. Format: [T-DB-NNN] RED task followed by GREEN task.

---

## Phase 1: BootstrapEnv + DetectEnv

- [x] T-DB-001: Write failing tests for `BootstrapEnv` struct and `envDetector.detect()` — table-driven, covering DockerBinaryPresent, UserInDockerGroup, SystemdPresent, UserName resolution (16 permutations → ~8 important ones). RED confirmed.
- [x] T-DB-002: Implement `bootstrap_env.go` with `BootstrapEnv`, `envDetector`, and `DetectEnv()`. GREEN confirmed.

## Phase 2: Action struct PostActionBanner field

- [x] T-DB-003: Write failing test asserting that `Action.PostActionBanner` field exists and is accessible. RED confirmed.
- [x] T-DB-004: Add `PostActionBanner string` field to `Action` struct in `messages.go`. GREEN confirmed.

## Phase 3: New Action constructors

- [x] T-DB-005: Write failing tests for `dockerInstallAction()`, `systemdStartDockerAction()`, `dockerGroupAddAction("alice")` — verify ID, Command, Args, PostActionBanner. RED confirmed.
- [x] T-DB-006: Implement action constructors in `bootstrap.go`. GREEN confirmed.

## Phase 4: ClassifyBlockers signature change + new cases

- [x] T-DB-007: Update ALL existing callers of `ClassifyBlockers` to compile (model.go, bootstrap_classify_test.go, model_bootstrap_test.go, fullflow_bootstrap_test.go). Add `BootstrapEnv` parameter with healthy default. Update existing test cases to pass healthy env. RED confirmed (compile errors are the red signal).
- [x] T-DB-008: Update `ClassifyBlockers` signature in `bootstrap.go`. Existing tests GREEN.
- [x] T-DB-009: Write failing tests for Docker-missing case (DockerBinaryPresent=false + CheckDockerDaemon=FAIL → docker_install action first). RED.
- [x] T-DB-010: Implement Docker-missing case in ClassifyBlockers. GREEN.
- [x] T-DB-011: Write failing tests for user-not-in-group case (Binary=true, InGroup=false → docker_group_add action last). RED.
- [x] T-DB-012: Implement user-not-in-group case. GREEN.
- [x] T-DB-013: Write failing tests for systemctl case (Binary=true, InGroup=true, Systemd=true → systemd_start_docker). RED.
- [x] T-DB-014: Implement systemctl case. GREEN.
- [x] T-DB-015: Write failing tests for non-systemd stuck case (Binary=true, InGroup=true, Systemd=false → nonFixable). RED.
- [x] T-DB-015b: Verify non-systemd stuck case (already handled by default in ClassifyBlockers). GREEN.

## Phase 5: Priority ordering

- [x] T-DB-016: Write failing tests asserting action ordering (docker install → dirs → systemctl → usermod). RED.
- [x] T-DB-016b: Implement priority ordering in ClassifyBlockers. GREEN.

## Phase 6: BootstrapModel banner screen

- [x] T-DB-017: Write failing tests for banner screen behavior (showingBanner=true after action with PostActionBanner; Enter emits BootstrapCompleteMsg; no banner for empty PostActionBanner). RED.
- [x] T-DB-018: Implement banner screen in BootstrapModel (fields + Update + View). GREEN.

## Phase 7: Dependencies wiring + main.go

- [x] T-DB-019: Add `Env BootstrapEnv` to `Dependencies` struct. Update model.go ClassifyBlockers call. Update test deps builders to compile. Verify tests pass.
- [x] T-DB-020: Add `deps.Env = tui.DetectEnv()` in `cmd/installer/main.go` newDependencies. Compile check.

## Phase 8: Integration tests

- [x] T-DB-021: Write `fullflow_docker_bootstrap_test.go` — 5 integration scenarios (docker missing, user-not-in-group, systemctl, non-systemd stuck, mixed). GREEN.
