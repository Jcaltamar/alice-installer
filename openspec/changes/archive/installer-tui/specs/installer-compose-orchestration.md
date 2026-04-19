# installer-compose-orchestration Specification

## Purpose

Drives the Docker Compose lifecycle: select overlay files based on GPU detection, pull images with live progress, start the stack, poll healthchecks until all services are healthy or a timeout is reached, and expose per-service diagnostics on failure. Supports graceful rollback on abort.

## Requirements

### Requirement: REQ-CO-1 — Overlay selection

The installer MUST select Compose files at runtime:
- GPU absent: `docker compose -f docker-compose.yml up`
- GPU present: `docker compose -f docker-compose.yml -f docker-compose.gpu.yml up`

The installer MUST NOT modify any Compose YAML file at install time. Overlay selection MUST be purely via `-f` flags.

#### Scenario: GPU absent — baseline only

- GIVEN preflight result has `gpu: false`
- WHEN compose command is built
- THEN `-f docker-compose.gpu.yml` is NOT included in the command
- AND `docker compose -f docker-compose.yml pull` is the pull command

#### Scenario: GPU present — overlay applied

- GIVEN preflight result has `gpu: true`
- WHEN compose command is built
- THEN command includes `-f docker-compose.yml -f docker-compose.gpu.yml`

---

### Requirement: REQ-CO-2 — Image pull with live progress

The installer MUST run `docker compose pull` before `up`. Pull output MUST be streamed to the TUI in real time. Each service image MUST show a progress indicator (spinner or bar). Partial pull failures (one image fails) MUST surface the failing image name and error, block the up step, and show a retry option.

#### Scenario: Successful pull

- GIVEN all images are accessible from the registry
- WHEN `docker compose pull` runs
- THEN TUI shows per-service pull progress
- AND on completion all services show green "Pulled"

#### Scenario: Pull fails — network error

- GIVEN the registry is unreachable during pull
- WHEN pull runs and fails
- THEN TUI shows red error: "Failed to pull <image>: <error>" with a "Retry pull" action
- AND the up step does NOT start

#### Scenario: Pull fails — image not found

- GIVEN an image tag does not exist in the registry
- WHEN pull runs for that service
- THEN TUI shows "Image not found: <image>. Verify BACKEND_IMAGE in .env." with an edit-env action

---

### Requirement: REQ-CO-3 — Stack startup

The installer MUST run `docker compose up -d` after a successful pull. If any service fails to start (non-zero exit within 10 seconds of `up`), the installer MUST surface the container name, exit code, and last 20 lines of logs.

#### Scenario: All services start successfully

- GIVEN all containers start and remain running after `up -d`
- WHEN up completes
- THEN TUI shows "Stack started — waiting for healthchecks…"

#### Scenario: Service exits immediately after up

- GIVEN one service exits with code 1 within 10 seconds of `up -d`
- WHEN the installer detects the exit
- THEN TUI shows the service name, exit code, and last 20 log lines
- AND a rollback prompt is shown

#### Scenario: Port conflict detected at up-time (race condition after port-scan)

- GIVEN a port that was free during port-scan is now occupied when `up -d` runs
- WHEN Compose reports "address already in use"
- THEN TUI shows "Port <N> conflict at startup. Free the port and retry, or go back to reassign." with retry and back actions

---

### Requirement: REQ-CO-4 — Healthcheck polling

The installer MUST poll `docker inspect --format='{{.State.Health.Status}}'` for each service that declares a healthcheck. Polling interval MUST be 3 seconds. The installer MUST consider the stack healthy when ALL services with healthchecks report `healthy`. The installer MUST time out after 120 seconds and surface per-service status.

#### Scenario: All services become healthy within timeout

- GIVEN all services transition to `healthy` within 120 seconds
- WHEN polling completes
- THEN TUI transitions to the result(success) screen

#### Scenario: One service never becomes healthy (timeout)

- GIVEN service `backend` stays in `starting` for 120+ seconds
- WHEN timeout is reached
- THEN TUI shows "Healthcheck timeout for: backend" with the last known health log
- AND partial success is shown for healthy services
- AND a "View logs" action is available for the failing service

#### Scenario: Service has no healthcheck declared

- GIVEN a service in docker-compose.yml has no `healthcheck` block
- WHEN polling runs
- THEN that service is considered healthy immediately (not polled)
- AND TUI notes it as "no healthcheck"

---

### Requirement: REQ-CO-5 — Graceful rollback on abort

If the operator presses `q` or `Ctrl+C` during pull, up, or healthcheck polling, the installer MUST:
1. Stop the current operation
2. Run `docker compose down` to remove started containers
3. Show "Rollback complete — no containers are running" before exiting

The installer MUST NOT leave dangling containers on abort.

#### Scenario: Operator aborts during pull

- GIVEN `docker compose pull` is in progress
- WHEN operator presses `q`
- THEN pull is cancelled, `docker compose down` is run, TUI shows rollback complete message
- AND installer exits with code 0

#### Scenario: Operator aborts during healthcheck polling

- GIVEN healthcheck polling is active (some services healthy, some starting)
- WHEN operator presses `q`
- THEN polling stops, `docker compose down` is run, rollback complete shown
- AND installer exits with code 0

#### Scenario: Rollback itself fails

- GIVEN `docker compose down` returns non-zero
- WHEN rollback runs
- THEN TUI shows "Rollback may be incomplete. Run: docker compose down" with the command for manual cleanup
