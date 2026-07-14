# alice-installer — Operations Runbook

## Table of Contents

1. [First-time install](#first-time-install)
2. [Password rotation](#password-rotation)
3. [Upgrade](#upgrade)
4. [Uninstall](#uninstall)
5. [Common issues](#common-issues)

---

## First-time install

### Prerequisites

- Linux (amd64 or arm64)
- Docker Engine ≥ 24.0 with the Compose v2 plugin (`docker compose version`)
- A user in the `docker` group (`sudo usermod -aG docker $USER`)
- Free TCP ports: 5432, 6379, 9090, 4550, 8080, 3000, 8554, 8888, 8889, 8890, 1935, 19530, 9000, 9001

### Run the installer

```sh
# Download the binary for your architecture
curl -LO https://github.com/Jcaltamar/alice-installer/releases/latest/download/alice-installer_<version>_linux_amd64.tar.gz
tar -xzf alice-installer_<version>_linux_amd64.tar.gz
chmod +x alice-installer

# (Optional) verify the checksum
sha256sum -c checksums.txt

# Run the interactive installer (requires a real TTY)
./alice-installer
```

The installer will:

1. Run preflight checks (OS, arch, Docker, ports, directories).
2. Ask for a **WORKSPACE** name (e.g. `my-site` — alphanumeric, dash, underscore, max 64 chars).
3. Scan required ports and let you pick alternates for any conflicts.
4. Write a `.env` file to the media directory (`/opt/alice-media/.env` by default) with mode `0600`.
5. Pull container images.
6. Start services with `docker compose up -d`.
7. Poll healthchecks until all services are healthy.

### What WORKSPACE means

`WORKSPACE` is a human-readable site identifier written into `.env` as `WORKSPACE=<value>`. It is used by the backend to namespace data. It is **not** a filesystem path — it is a label. You cannot change it without a migration.

### Dry-run mode (preflight only)

```sh
./alice-installer --dry-run
```

This runs all preflight checks and prints a report, then exits without writing files or starting services. Useful in CI or before committing to an install.

---

## Password rotation

### Why this is critical

Early versions of `docker-compose.yml` had a hardcoded `POSTGRES_PASSWORD` value committed to git history. If you deployed from that file **before the installer was used**, your database password is in git history and must be rotated immediately.

### How to rotate

**1. Generate a new password**

```sh
openssl rand -base64 32
# or
head -c 32 /dev/urandom | base64
```

Note the new password — you will use it in the next steps.

**2. Update the running database**

```sh
# Connect to the running Postgres container
docker compose -f /opt/alice-media/docker-compose.yml exec postgres \
  psql -U postgres -c "ALTER USER postgres WITH PASSWORD 'NEW_PASSWORD_HERE';"
```

Replace `NEW_PASSWORD_HERE` with the password you generated.

**3. Update .env**

```sh
# Edit /opt/alice-media/.env and update the POSTGRES_PASSWORD line
# Use a text editor — do not commit the .env file to git
nano /opt/alice-media/.env
```

Set `POSTGRES_PASSWORD=NEW_PASSWORD_HERE`.

**4. Restart services**

```sh
docker compose -f /opt/alice-media/docker-compose.yml down
docker compose -f /opt/alice-media/docker-compose.yml --env-file /opt/alice-media/.env up -d
```

**5. Verify**

```sh
docker compose -f /opt/alice-media/docker-compose.yml ps
# All services should be "healthy" or "running"
```

**6. (Optional) Purge git history**

If the old password was committed:

```sh
# Using git-filter-repo (recommended over BFG)
pip install git-filter-repo
git filter-repo --path docker-compose.yml --path .env --invert-paths
```

Or use [BFG Repo-Cleaner](https://rtyley.github.io/bfg-repo-cleaner/) to remove the old values from history.

---

## Upgrade

### Standard upgrade (new binary, in-place update)

```sh
# Download and extract the new binary
curl -LO https://github.com/Jcaltamar/alice-installer/releases/latest/download/alice-installer_<version>_linux_amd64.tar.gz
tar -xzf alice-installer_<version>_linux_amd64.tar.gz
chmod +x alice-installer

# Refresh an existing install from persisted workspace artifacts
./alice-installer update --workspace-dir /opt/alice-media
```

`update` is non-interactive and does not regenerate install artifacts. It requires:

- `/opt/alice-media/.env`
- `/opt/alice-media/docker-compose.yml`

Then it performs `docker compose pull` followed by `docker compose up -d` using those exact files.

### In-place service restart (no image refresh)

```sh
# Restart running services using existing workspace artifacts
./alice-installer restart --workspace-dir /opt/alice-media
```

`restart` is non-interactive and uses the same artifact requirements as `update`:

- `/opt/alice-media/.env`
- `/opt/alice-media/docker-compose.yml`

If `/opt/alice-media/docker-compose.gpu.yml` exists and GPU toolkit detection is positive, it is included after the base compose file (same ordering contract as `update`).

`restart` executes exact `docker compose restart` semantics. It does **not** run install flows and does **not** substitute `down/up` or `pull/up`.

Cross-platform behavior is intentionally equivalent on Linux (amd64/arm64) and Windows (amd64): command recognition, artifact targeting, success signaling, and failure signaling remain the same.

### Check what changed

Read the release notes and `RUNBOOK.md` for the new version before upgrading. Pay special attention to the `⚠️ Security` section — it will list required actions (such as password rotation) for specific upgrade paths.

---

## Contextual installation menu

Interactive mode performs a read-only detection step after the splash screen and before preflight. It uses the selected `--workspace-dir` and the required `.env` and `docker-compose.yml` artifacts. Partial, unreadable, malformed, conflicting, or otherwise ambiguous evidence blocks lifecycle actions rather than treating the host as clean.

Legacy PM2 probing is Linux amd64/arm64 only. It requires exact, configured Alice-specific identifiers with corroborating deployment paths. The production policy is intentionally empty until historical identifiers are confirmed, so generic PM2 processes never qualify. Unsupported platforms show no legacy positive result.

Use the explicit `update` or `restart` commands when their artifact requirements are met. Uninstall remains informational and blocked.

## Legacy migration and recovery

Legacy restore is offered only after a confirmed interactive Migration selection on Linux amd64 or arm64. It is never offered by Windows, unsupported platforms, Install, Update, Restart, `--dry-run`, unattended, or headless routes; those routes retain their existing behavior.

### Cutover contract

1. Before preflight or any installation side effect, the installer revalidates the legacy backup and stops only a PM2 identity proven by canonical Linux process and socket evidence: `/opt/alice-guardian` on TCP `8080`, or `/opt/backend_alice_guardian` on TCP `9090` or `4550`. `pm2`, `ss`, and readable `/proc` identity metadata are required; missing, ambiguous, or changed evidence blocks migration.
2. The existing deployment completes unchanged. The restore then performs one cancellable **60-second** wait before it stops only Compose service `backend`.
3. Names are immutable: application `backend` / `alice_backend`; database `postgresql-master` / `alice_postgresql-master`. PostgreSQL stays running; the installer never uses whole-stack shutdown or PostgreSQL service control.
4. Before replacement, two validated custom-format backups are retained under `/opt/alice/backups/`: the legacy archive and a distinct target rollback archive. Neither is overwritten or deleted automatically.
5. The target is destructively replaced: sessions are terminated, the database is dropped and recreated, then `pg_restore --exit-on-error --no-owner --no-privileges` restores the legacy archive. This is not a merge.
6. Success requires restore exit evidence, a new target connection, at least one non-system application table, PostgreSQL reachability, and a healthy restarted backend. Only then does the installer consume the PM2 lease and intentionally leave the proven legacy PM2 set stopped.

### Partial cutover recovery

Any failure after the drop/recreate boundary is a **partial cutover**, never an install success. The installer automatically revalidates and restores only the retained target rollback backup, then starts `backend` only after database validation and backend health succeed. Both backups remain available.

If automatic rollback cannot be proven, leave PostgreSQL running and keep `backend` stopped. On every install failure, restore failure, cancellation, terminal abandonment, or verification failure after PM2 quiescence, database rollback completes first when needed and the installer then attempts bounded recovery of exactly the acknowledged stopped PM2 identities. Preserve `/opt/alice/backups/` unchanged, record the redacted stage/code shown by the installer, and escalate with those paths and checksums. Do not retry with a merge, a different dump, `docker compose down`, broad PM2 commands, or manual PostgreSQL service restart. Passwords, pgpass paths, raw dump output, and raw command diagnostics are intentionally not displayed.

Feature rollback removes the interactive restore action and returns Migration to its validated-backup completion state. It does not delete either operator backup or change non-migration routes.

## Uninstall

Uninstall is not implemented by the installer or its contextual menu. Do not infer a deletion procedure from this runbook. A future operation requires an approved ownership, backup, confirmation, recovery, and rollback contract before it can be documented or exposed.

---

## Common issues

### Docker not running

**Symptom**: Preflight fails with `Docker daemon unreachable`.

**Fix**:

```sh
sudo systemctl start docker
sudo systemctl enable docker  # start on boot
sudo usermod -aG docker $USER  # then log out and back in
```

### GPU toolkit missing

**Symptom**: Preflight shows `WARN: NVIDIA Container Toolkit not detected`.

This is non-blocking — the backend will run on CPU. To enable GPU acceleration:

```sh
# Install nvidia-container-toolkit
curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg
# ... follow https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html
sudo systemctl restart docker
```

Re-run the installer to pick up the GPU overlay.

### Port conflict

**Symptom**: Preflight shows `WARN: Port conflicts`.

**Fix**: The installer will ask you to pick alternate ports during the interactive session. Run the installer and provide alternate ports when prompted. Common conflicts:

- **5432 (Postgres)**: another Postgres instance running — use `sudo systemctl stop postgresql`
- **6379 (Redis)**: another Redis instance running — use `sudo systemctl stop redis`
- **8080 (Web)**: a web server on port 80 or 8080 — stop the conflicting service or choose an alternate port

### Terminal too small

**Symptom**: The TUI shows "Terminal too small. Resize to at least 80×24."

**Fix**: Resize your terminal window to at least 80 columns × 24 rows. Most modern terminals support this. If running over SSH, ensure your SSH client passes the correct terminal dimensions.

### Installer exits with "stdin is not a terminal"

**Symptom**: `alice-installer: stdin is not a terminal. Run interactively in a TTY.`

**Fix**: The installer requires a real TTY. Do not pipe input to it. If running over SSH, use `ssh -t user@host` to allocate a pseudo-TTY. For non-interactive use, use `--dry-run`.

### Stale docker-group recovery

**Symptom**: You ran `sudo usermod -aG docker $USER` and re-ran the installer in the same terminal session, but it still reports `Docker daemon unreachable (permission denied on /var/run/docker.sock)`.

**What happens automatically**: The installer detects at startup that your user is in `/etc/group:docker` but the docker GID is absent from the current process's supplementary groups (classic post-`usermod` stale-session state). It automatically re-execs itself via `sg docker -c <argv>` so the replacement process inherits the docker group — no logout required.

**Requirement**: The `sg` binary must be present (provided by the `login` package on Ubuntu/Debian; typically pre-installed on any standard distribution).

**Fallback — if `sg` is not available**: The installer exits with code `75` (`EX_TEMPFAIL`) and prints a copy-paste-ready command:

```
newgrp docker && alice-installer <original flags>
```

Run that command to get a subshell with the updated group and re-run the installer in one step.
