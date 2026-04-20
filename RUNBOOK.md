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

### Standard upgrade (new binary, re-use existing .env)

```sh
# Download and extract the new binary
curl -LO https://github.com/Jcaltamar/alice-installer/releases/latest/download/alice-installer_<version>_linux_amd64.tar.gz
tar -xzf alice-installer_<version>_linux_amd64.tar.gz
chmod +x alice-installer

# Re-run against the existing .env
./alice-installer --env-output /opt/alice-media/.env
```

The installer will detect that `.env` already exists. If the workspace and ports have not changed, it will rewrite the file in place (atomic rename) and then pull new images and restart services.

### Check what changed

Read the release notes and `RUNBOOK.md` for the new version before upgrading. Pay special attention to the `⚠️ Security` section — it will list required actions (such as password rotation) for specific upgrade paths.

---

## Uninstall

```sh
# Stop and remove all containers, networks, and volumes
docker compose \
  -f /opt/alice-media/docker-compose.yml \
  --env-file /opt/alice-media/.env \
  down -v

# Remove the media and config directories (DESTRUCTIVE — all data will be lost)
sudo rm -rf /opt/alice-media /opt/alice-config

# Remove the installer binary
rm ./alice-installer
```

> **Warning**: `down -v` removes Docker volumes. All database data will be permanently deleted. Take a backup first if you need to preserve data.

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
