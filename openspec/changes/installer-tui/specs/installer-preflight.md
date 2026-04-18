# installer-preflight Specification

## Purpose

Validates the host environment before any installation step begins. Runs synchronously, produces a structured result for each check, and blocks the TUI from advancing if any MUST-pass check fails. Warns (non-blocking) for optional capabilities such as NVIDIA.

## Requirements

### Requirement: REQ-PF-1 — Linux-only enforcement

The installer MUST detect the host OS at startup. If the OS is not Linux, the installer MUST print a human-readable error and exit with code 1. No TUI is launched on non-Linux hosts.

#### Scenario: Non-Linux host (macOS/Windows)

- GIVEN the installer binary is executed on a non-Linux OS
- WHEN the OS check runs
- THEN the installer prints "Alice Guardian installer requires Linux (amd64 or arm64). Current OS: <os>." and exits with code 1
- AND no TUI is started

#### Scenario: Linux host

- GIVEN the installer is executed on a Linux host
- WHEN the OS check runs
- THEN the check passes and execution continues

---

### Requirement: REQ-PF-2 — Architecture detection

The installer MUST detect the CPU architecture. MUST accept `amd64` and `arm64`. MUST exit with code 1 on any other architecture, printing the detected arch and a "not supported" message.

#### Scenario: Supported arch (amd64)

- GIVEN a Linux/amd64 host
- WHEN arch detection runs
- THEN result is `{arch: "amd64", ok: true}`

#### Scenario: Supported arch (arm64)

- GIVEN a Linux/arm64 host
- WHEN arch detection runs
- THEN result is `{arch: "arm64", ok: true}`

#### Scenario: Unsupported arch (e.g. 386)

- GIVEN a Linux/386 host
- WHEN arch detection runs
- THEN installer exits with code 1 and prints "Unsupported architecture: 386. Supported: amd64, arm64."

---

### Requirement: REQ-PF-3 — Docker daemon check

The installer MUST verify the Docker daemon is reachable via `docker info`. MUST check the Docker Engine version is >= 20.10. If the daemon is unreachable, the preflight MUST fail with a human-readable remediation message. If the version is below minimum, the preflight MUST fail with the installed vs required version.

#### Scenario: Docker daemon running, version sufficient

- GIVEN Docker daemon is running and version >= 20.10
- WHEN Docker check runs
- THEN result is `{docker: "ok", version: "<x.y.z>"}`

#### Scenario: Docker daemon not running

- GIVEN Docker daemon is stopped
- WHEN Docker check runs
- THEN result is `{docker: "fail", reason: "daemon unreachable"}` and TUI shows "Start the Docker daemon and retry."

#### Scenario: Docker version too old

- GIVEN Docker version is 19.03
- WHEN Docker check runs
- THEN result is `{docker: "fail", reason: "version 19.03 < required 20.10"}` and TUI shows upgrade instructions

---

### Requirement: REQ-PF-4 — Compose v2 plugin check

The installer MUST verify `docker compose version` (v2 plugin, not `docker-compose` standalone). MUST require version >= 2.0. If only Compose v1 (`docker-compose`) is found, the preflight MUST fail with a distinct "Compose v2 plugin required" message.

#### Scenario: Compose v2 present and sufficient

- GIVEN `docker compose version` returns >= 2.0
- WHEN Compose check runs
- THEN result is `{compose: "ok", version: "2.x.y"}`

#### Scenario: Only Compose v1 present

- GIVEN `docker compose` not available but `docker-compose` is
- WHEN Compose check runs
- THEN result is `{compose: "fail", reason: "v1 only"}` and TUI shows "Install the Compose v2 plugin: https://docs.docker.com/compose/install/"

#### Scenario: Compose absent entirely

- GIVEN neither `docker compose` nor `docker-compose` is available
- WHEN Compose check runs
- THEN result is `{compose: "fail", reason: "not found"}` with install link

---

### Requirement: REQ-PF-5 — NVIDIA Container Toolkit detection

The installer SHOULD detect whether NVIDIA Container Toolkit is installed. If detected, the preflight MUST set `gpu: true`. If absent, the preflight MUST set `gpu: false`, emit an orange WARNING to the TUI ("No NVIDIA GPU detected — running CPU-only"), and continue. The installer MUST NOT fail or exit on GPU absence.

#### Scenario: NVIDIA toolkit present

- GIVEN `nvidia-container-toolkit` is installed and `nvidia-smi` succeeds
- WHEN GPU check runs
- THEN result is `{gpu: true}` and GPU overlay will be applied at compose step

#### Scenario: NVIDIA kernel module present but container-toolkit missing

- GIVEN `nvidia-smi` succeeds (driver loaded) but `nvidia-container-toolkit` is not installed
- WHEN GPU check runs
- THEN result is `{gpu: false, reason: "toolkit missing"}` and TUI shows orange warning "NVIDIA driver detected but container-toolkit not installed. Install nvidia-container-toolkit to enable GPU. Continuing CPU-only."

#### Scenario: No NVIDIA hardware

- GIVEN no NVIDIA driver or toolkit
- WHEN GPU check runs
- THEN result is `{gpu: false}` and TUI shows orange warning "No NVIDIA GPU detected. Running CPU-only."

---

### Requirement: REQ-PF-6 — Filesystem writability check

The installer MUST verify that `/opt/alice-media` and `/opt/alice-config` are either writable (if they exist) or can be created by the current user. If either path is not writable, the preflight MUST fail with a message identifying which path(s) failed and remediation instructions (e.g. `sudo chown $USER /opt/alice-media`).

#### Scenario: Both paths writable

- GIVEN current user has write access to `/opt/alice-media` and `/opt/alice-config`
- WHEN writability check runs
- THEN result is `{paths: "ok"}`

#### Scenario: One path not writable

- GIVEN `/opt/alice-config` exists but is owned by root with no write permission for current user
- WHEN writability check runs
- THEN result is `{paths: "fail", blocked: ["/opt/alice-config"]}` and TUI shows remediation command

#### Scenario: Path doesn't exist, can be created

- GIVEN `/opt/alice-media` does not exist but parent `/opt` is writable
- WHEN writability check runs
- THEN result is `{paths: "ok"}` (creation will happen at install time)

---

### Requirement: REQ-PF-7 — All checks produce structured output

The preflight MUST return a typed result struct consumable by the TUI model, containing per-check status, version strings, and remediation messages. The TUI MUST render a checklist of all checks with pass/warn/fail icons.

#### Scenario: All checks pass

- GIVEN all preflight checks succeed
- WHEN TUI renders preflight screen
- THEN each check shows a green checkmark and a version or "OK" label

#### Scenario: Mix of pass and warn

- GIVEN Docker and Compose pass, NVIDIA absent, paths writable
- WHEN TUI renders preflight screen
- THEN Docker and Compose show green, NVIDIA shows orange warning, paths show green
- AND the "Continue" action is available (warn does not block)

#### Scenario: Hard failure present

- GIVEN Docker check fails
- WHEN TUI renders preflight screen
- THEN Docker check shows red, "Continue" action is disabled, and remediation text is visible
