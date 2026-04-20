#!/usr/bin/env bash
# scripts/e2e/run.sh — end-to-end test for alice-installer inside a systemd container.
#
# The test boots a clean Ubuntu 22.04 container with systemd as PID 1 and NO
# Docker pre-installed.  The installer's bootstrap downloads Docker, creates
# /opt dirs, and adds the test user to the docker group.  Because group
# membership changes only take effect in a new login session, the installer
# exits with code 75 (EX_TEMPFAIL / ErrReloginRequired) after the usermod
# step.  A fresh docker exec then picks up the new group set.
#
# Multi-pass sequence handled here:
#   Pass 1 — installs Docker + /opt dirs → daemon not yet running → exit 1
#             (ErrPreflightStillFailing, because the get.docker.com script
#             does not guarantee the daemon is running after install on every
#             distro/config combination).
#             E2E helper: start the Docker daemon via systemd, then continue.
#   Pass 2 — Docker daemon running, user not yet in group → runs usermod,
#             exits 75 (ErrReloginRequired).
#   Pass 3 — fresh docker exec (new group set) → all checks pass → exit 0.
#
# Usage:
#   ./scripts/e2e/run.sh
#   FULL_DEPLOY=1 ./scripts/e2e/run.sh   # also pull + up (~3 GB)

set -euo pipefail

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
log()  { printf '[e2e] %s\n' "$*"; }
ok()   { printf '\033[32m[e2e] ✓ %s\033[0m\n' "$*"; }
fail() { printf '\033[31m[e2e] ✗ %s\033[0m\n' "$*" >&2; exit 1; }

ASSERTIONS_PASSED=0
assert() {
  local desc="$1"; shift
  if "$@" >/dev/null 2>&1; then
    ok "$desc"
    ASSERTIONS_PASSED=$((ASSERTIONS_PASSED + 1))
  else
    fail "ASSERTION FAILED: $desc"
  fi
}

START_TS=$(date +%s)

# ---------------------------------------------------------------------------
# 1. Repo root
# ---------------------------------------------------------------------------
REPO_ROOT=$(git -C "$(dirname "$0")" rev-parse --show-toplevel)
log "repo root: $REPO_ROOT"

# ---------------------------------------------------------------------------
# 2. Build static linux/amd64 binary
# ---------------------------------------------------------------------------
log "building static linux/amd64 binary…"
mkdir -p "$REPO_ROOT/dist"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -trimpath \
  -ldflags="-s -w" \
  -o "$REPO_ROOT/dist/alice-installer-e2e" \
  "$REPO_ROOT/cmd/installer"
log "binary written to dist/alice-installer-e2e"

# ---------------------------------------------------------------------------
# 3. Build test image
# ---------------------------------------------------------------------------
log "building test image alice-installer-e2e:latest…"
docker build \
  -t alice-installer-e2e:latest \
  -f "$REPO_ROOT/scripts/e2e/Dockerfile" \
  "$REPO_ROOT/scripts/e2e"

# ---------------------------------------------------------------------------
# Container name + cleanup trap
# ---------------------------------------------------------------------------
CID=""
CONTAINER_NAME="alice-e2e-$$"

cleanup() {
  if [ -n "$CID" ]; then
    log "removing container $CID…"
    docker rm -f "$CID" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# 4. Start container (privileged, detached, systemd as PID 1)
# ---------------------------------------------------------------------------
log "starting container $CONTAINER_NAME…"
CID=$(docker run \
  --name "$CONTAINER_NAME" \
  --privileged \
  --cgroupns=host \
  -v /sys/fs/cgroup:/sys/fs/cgroup:rw \
  --detach \
  alice-installer-e2e:latest)
log "container id: $CID"

# ---------------------------------------------------------------------------
# 5. Wait for systemd to be ready (up to 20 s)
# ---------------------------------------------------------------------------
log "waiting for systemd to be ready (up to 20 s)…"
SYSTEMD_READY=0
for i in $(seq 1 20); do
  STATE=$(docker exec "$CID" systemctl is-system-running 2>/dev/null || true)
  if [ "$STATE" = "running" ] || [ "$STATE" = "degraded" ]; then
    log "systemd state: $STATE (ready after ${i}s)"
    SYSTEMD_READY=1
    break
  fi
  sleep 1
done
if [ "$SYSTEMD_READY" -eq 0 ]; then
  log "systemd did not become ready in 20 s; current state:"
  docker exec "$CID" systemctl is-system-running 2>/dev/null || true
  docker logs "$CID" | tail -30
  fail "systemd never reached running/degraded"
fi

# ---------------------------------------------------------------------------
# 6. Copy binary into container
# ---------------------------------------------------------------------------
log "copying binary to container…"
docker cp "$REPO_ROOT/dist/alice-installer-e2e" "$CID:/home/testuser/alice-installer"
docker exec "$CID" chown testuser:testuser /home/testuser/alice-installer
docker exec "$CID" chmod +x /home/testuser/alice-installer

# ---------------------------------------------------------------------------
# Common installer flags
# ---------------------------------------------------------------------------
INSTALLER_FLAGS=(
  --unattended
  --workspace-name=e2e-test
  --workspace-dir=/home/testuser/.config/alice-guardian
  --media-dir=/opt/alice-media
  --config-dir=/opt/alice-config
  --deploy=false
)

# ---------------------------------------------------------------------------
# dump_diagnostics — called on unexpected non-zero exit.
# Optional args: service names to dump per-service compose logs for.
# ---------------------------------------------------------------------------
dump_diagnostics() {
  log "--- container logs (last 50 lines) ---"
  docker logs "$CID" 2>&1 | tail -50 || true
  log "--- journalctl docker ---"
  docker exec "$CID" journalctl -u docker --no-pager 2>/dev/null | tail -50 || true

  # When service names are provided, dump each one's compose logs.
  if [ "$#" -gt 0 ]; then
    local svc
    for svc in "$@"; do
      log "--- compose logs: $svc (last 50 lines) ---"
      docker exec "$CID" docker compose \
        -f /home/testuser/.config/alice-guardian/docker-compose.yml \
        --env-file /home/testuser/.config/alice-guardian/.env \
        logs --no-color --tail=50 "$svc" 2>&1 | tail -60 || true
    done
  fi
}

# ---------------------------------------------------------------------------
# 7. Pass 1
# ---------------------------------------------------------------------------
log "=== pass 1: expecting docker install + /opt dirs ==="
PASS1_EXIT=0
docker exec -u testuser "$CID" \
  /home/testuser/alice-installer "${INSTALLER_FLAGS[@]}" \
  || PASS1_EXIT=$?
log "pass 1 exit code: $PASS1_EXIT"

# ---------------------------------------------------------------------------
# Handle exit 1 from pass 1: Docker was installed but daemon not yet running.
# Start it via systemd, then run passes 2 and 3.
# ---------------------------------------------------------------------------
if [ "$PASS1_EXIT" -eq 1 ]; then
  # Verify Docker binary is now present — if not, it's a different error.
  if ! docker exec "$CID" which docker >/dev/null 2>&1; then
    log "Docker binary not found after pass 1 — not a daemon-not-running error"
    dump_diagnostics
    fail "pass 1 exited 1 and Docker was not installed"
  fi

  log "Docker installed but daemon not running — starting via systemd…"
  docker exec "$CID" systemctl start docker
  # Wait for the socket to be ready.
  DAEMON_READY=0
  for i in $(seq 1 10); do
    if docker exec "$CID" docker info >/dev/null 2>&1; then
      log "Docker daemon ready after ${i}s"
      DAEMON_READY=1
      break
    fi
    sleep 1
  done
  if [ "$DAEMON_READY" -eq 0 ]; then
    dump_diagnostics
    fail "Docker daemon did not become ready after systemctl start docker"
  fi

  # Pass 2: daemon running, user not yet in docker group → expect exit 75.
  log "=== pass 2: expecting usermod + exit 75 (ErrReloginRequired) ==="
  PASS2_EXIT=0
  docker exec -u testuser "$CID" \
    /home/testuser/alice-installer "${INSTALLER_FLAGS[@]}" \
    || PASS2_EXIT=$?
  log "pass 2 exit code: $PASS2_EXIT"

  if [ "$PASS2_EXIT" -eq 75 ]; then
    # Pass 3: fresh exec picks up new group set.
    log "=== pass 3: fresh exec with updated group set ==="
    PASS3_EXIT=0
    docker exec -u testuser "$CID" \
      /home/testuser/alice-installer "${INSTALLER_FLAGS[@]}" \
      || PASS3_EXIT=$?
    log "pass 3 exit code: $PASS3_EXIT"
    if [ "$PASS3_EXIT" -ne 0 ]; then
      dump_diagnostics
      fail "pass 3 (post-relogin) exited $PASS3_EXIT"
    fi

    # Pass 4: same-shell auto-reexec via sg.
    # A bash -lc login shell inherits the pre-group-add PAM environment, so it
    # exercises the stale-group detection and sg re-exec path.
    log "=== pass 4: same-shell auto-reexec via sg ==="
    PASS4_EXIT=0
    docker exec -u testuser "$CID" bash -lc \
      "/home/testuser/alice-installer ${INSTALLER_FLAGS[*]}" \
      || PASS4_EXIT=$?
    log "pass 4 exit code: $PASS4_EXIT"
    if [ "$PASS4_EXIT" -ne 0 ]; then
      dump_diagnostics
      fail "pass 4 (same-shell sg reexec) exited $PASS4_EXIT"
    fi
  elif [ "$PASS2_EXIT" -eq 0 ]; then
    log "pass 2 succeeded without usermod step (user already in group?)"
  else
    dump_diagnostics
    fail "pass 2 exited $PASS2_EXIT (expected 0 or 75)"
  fi

elif [ "$PASS1_EXIT" -eq 75 ]; then
  # Pass 1 exited 75: Docker already installed and running, user added to
  # group.  A fresh exec picks up the new group set.
  log "=== pass 2 (relogin after pass-1 usermod): expecting exit 0 ==="
  PASS2_EXIT=0
  docker exec -u testuser "$CID" \
    /home/testuser/alice-installer "${INSTALLER_FLAGS[@]}" \
    || PASS2_EXIT=$?
  log "pass 2 exit code: $PASS2_EXIT"
  if [ "$PASS2_EXIT" -ne 0 ]; then
    dump_diagnostics
    fail "pass 2 exited $PASS2_EXIT"
  fi

  # Pass 4 (in this branch pass 3 is skipped): same-shell auto-reexec via sg.
  log "=== pass 4: same-shell auto-reexec via sg ==="
  PASS4_EXIT=0
  docker exec -u testuser "$CID" bash -lc \
    "/home/testuser/alice-installer ${INSTALLER_FLAGS[*]}" \
    || PASS4_EXIT=$?
  log "pass 4 exit code: $PASS4_EXIT"
  if [ "$PASS4_EXIT" -ne 0 ]; then
    dump_diagnostics
    fail "pass 4 (same-shell sg reexec) exited $PASS4_EXIT"
  fi

elif [ "$PASS1_EXIT" -ne 0 ]; then
  dump_diagnostics
  fail "pass 1 exited $PASS1_EXIT"
fi

log "=== installer finished successfully ==="

# ---------------------------------------------------------------------------
# 8. Assertions
# ---------------------------------------------------------------------------
log "=== running assertions ==="

ENV_FILE=/home/testuser/.config/alice-guardian/.env

assert ".env exists and is readable" \
  docker exec "$CID" test -r "$ENV_FILE"

assert ".env contains WORKSPACE=e2e-test" \
  docker exec "$CID" grep -q "WORKSPACE=e2e-test" "$ENV_FILE"

assert "docker-compose.yml exists and is non-empty" \
  docker exec "$CID" bash -c 'test -s /home/testuser/.config/alice-guardian/docker-compose.yml'

assert "docker-compose.gpu.yml exists and is non-empty" \
  docker exec "$CID" bash -c 'test -s /home/testuser/.config/alice-guardian/docker-compose.gpu.yml'

assert "/opt/alice-media exists and is writable by testuser" \
  docker exec -u testuser "$CID" test -w /opt/alice-media

assert "/opt/alice-config exists and is writable by testuser" \
  docker exec -u testuser "$CID" test -w /opt/alice-config

assert "docker CLI is installed" \
  docker exec "$CID" docker --version

assert "docker compose plugin is installed" \
  docker exec "$CID" docker compose version --short

assert "testuser is in docker group" \
  docker exec "$CID" bash -c 'id testuser | grep -q docker'

# ---------------------------------------------------------------------------
# 9. Optional full deploy pass (FULL_DEPLOY=1)
# ---------------------------------------------------------------------------
if [ "${FULL_DEPLOY:-0}" = "1" ]; then
  log "=== FULL_DEPLOY=1: running pull + up ==="
  FULL_FLAGS=(
    --unattended
    --workspace-name=e2e-test
    --workspace-dir=/home/testuser/.config/alice-guardian
    --media-dir=/opt/alice-media
    --config-dir=/opt/alice-config
    --deploy=true
  )
  FULL_EXIT=0
  INSTALLER_LOG=$(mktemp)
  docker exec -u testuser "$CID" \
    /home/testuser/alice-installer "${FULL_FLAGS[@]}" 2>&1 | tee "$INSTALLER_LOG" \
    || FULL_EXIT=$?
  if [ "$FULL_EXIT" -ne 0 ]; then
    log "full deploy failed (exit $FULL_EXIT) — dumping diagnostics:"
    # Parse the last "unhealthy: svc1(state), svc2(state), ..." line from the
    # installer output and extract service names (the part before the first '(').
    UNHEALTHY_LINE=$(grep -o 'unhealthy: [^;]*' "$INSTALLER_LOG" | tail -1 || true)
    SVCS=()
    if [ -n "$UNHEALTHY_LINE" ]; then
      # Extract tokens like "svc(status/state)" → "svc"
      while IFS= read -r token; do
        svc="${token%%(*}"
        svc="${svc## }"
        svc="${svc%% }"
        [ -n "$svc" ] && SVCS+=("$svc")
      done < <(echo "$UNHEALTHY_LINE" | sed 's/unhealthy: //' | tr ',' '\n')
    fi
    rm -f "$INSTALLER_LOG"
    if [ "${#SVCS[@]}" -gt 0 ]; then
      dump_diagnostics "${SVCS[@]}"
    else
      dump_diagnostics
    fi
    fail "full deploy exited $FULL_EXIT"
  fi
  rm -f "$INSTALLER_LOG"
  log "container list after deploy:"
  docker exec "$CID" docker ps --format '{{.Names}}: {{.Status}}'
  assert "redis is running" \
    docker exec "$CID" bash -c "docker ps --format '{{.Names}}' | grep -q redis"
  assert "postgres is running" \
    docker exec "$CID" bash -c "docker ps --format '{{.Names}}' | grep -q postgres"
fi

# ---------------------------------------------------------------------------
# 10. Summary
# ---------------------------------------------------------------------------
END_TS=$(date +%s)
ELAPSED=$((END_TS - START_TS))

printf '\n\033[32m'
printf '╔══════════════════════════════════════════════════╗\n'
printf '║  alice-installer E2E  ✓  ALL ASSERTIONS PASSED  ║\n'
printf '╚══════════════════════════════════════════════════╝\n'
printf '\033[0m'
log "$ASSERTIONS_PASSED assertions passed in ${ELAPSED}s"
