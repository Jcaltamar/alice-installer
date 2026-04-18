# Apply Progress: installer-docker-bootstrap

Status: COMPLETE

## Completed Tasks

- [x] T-DB-001/002: BootstrapEnv struct + envDetector.detect() — `bootstrap_env.go` + `bootstrap_env_test.go`
- [x] T-DB-003/004: PostActionBanner field added to Action struct in `messages.go`
- [x] T-DB-005/006: Action constructors (dockerInstallAction, systemdStartDockerAction, dockerGroupAddAction) + ActionID constants — `bootstrap.go`
- [x] T-DB-007/008: ClassifyBlockers signature updated to `(report, env, mediaDir, configDir)` — all callers updated
- [x] T-DB-009/010: Docker-missing case in ClassifyBlockers (DockerBinaryPresent=false → docker_install)
- [x] T-DB-011/012: User-not-in-group case (Binary=true, InGroup=false → docker_group_add with PostActionBanner)
- [x] T-DB-013/014: Systemctl case (Binary=true, InGroup=true, Systemd=true → systemd_start_docker)
- [x] T-DB-015: Non-systemd stuck case (Binary=true, InGroup=true, Systemd=false → nonFixable)
- [x] T-DB-016: Priority ordering (docker_install → dirs → systemctl → usermod)
- [x] T-DB-017/018: Banner screen in BootstrapModel (showingBanner, banners fields, View, Enter-to-dismiss)
- [x] T-DB-019: Env BootstrapEnv field added to Dependencies struct; model.go ClassifyBlockers call updated
- [x] T-DB-020: DetectEnv() called in cmd/installer/main.go newDependencies
- [x] T-DB-021: fullflow_docker_bootstrap_test.go — 5 integration scenarios

## Files Created

- `internal/tui/bootstrap_env.go` — BootstrapEnv struct, envDetector, DetectEnv()
- `internal/tui/bootstrap_env_test.go` — 14 tests for env detection
- `internal/tui/bootstrap_actions_test.go` — 4 tests for action constructors
- `internal/tui/fullflow_docker_bootstrap_test.go` — 5 integration scenarios

## Files Modified

- `internal/tui/messages.go` — PostActionBanner field on Action struct
- `internal/tui/bootstrap.go` — ActionID constants, action constructors, ClassifyBlockers signature + Docker cases, BootstrapModel banner screen
- `internal/tui/bootstrap_classify_test.go` — Updated to new ClassifyBlockers signature; added Docker test cases
- `internal/tui/bootstrap_model_test.go` — Added banner screen tests
- `internal/tui/model.go` — Env field in Dependencies; ClassifyBlockers call updated
- `internal/tui/model_bootstrap_test.go` — Updated for new signature; added docker-fixable tests
- `internal/tui/model_test.go` — buildTestDeps adds default non-systemd Env
- `internal/tui/fullflow_bootstrap_test.go` — Added Env to buildBootstrapFlowDeps
- `cmd/installer/main.go` — Env: tui.DetectEnv() in newDependencies

## Test Results

`go test -short ./...` → all 11 packages PASS
`go vet ./...` → clean
`go mod tidy` → no changes
