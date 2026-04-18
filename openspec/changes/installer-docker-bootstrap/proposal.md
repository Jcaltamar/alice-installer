# Proposal: installer-docker-bootstrap

## Problem

Today, when the alice-installer preflight detects that Docker is unreachable (CheckDockerDaemon = FAIL), the check is classified as **non-fixable** regardless of the root cause. This means users without Docker installed — the most common new-machine scenario — hit a hard wall with no automated remediation. They must leave the installer, perform multiple manual steps (install Docker, add group membership, restart), and re-run the installer. The installer currently knows how to fix directory-permission blockers but goes blind when the blocker is Docker itself.

## Approach

We extend `ClassifyBlockers` to handle three Docker-related failure sub-cases, all resolved via `tea.ExecProcess` so sudo password prompts reach the real TTY (see engram `installer-tui/sudo-pattern` — this is non-negotiable and applies to every privileged command).

A new `BootstrapEnv` struct captures the host environment at startup: Docker binary presence (via `exec.LookPath`), current user's `docker` group membership (via `os/user`), and systemd availability (via `exec.LookPath("systemctl")` + `/run/systemd/system`). `ClassifyBlockers` receives this env alongside the preflight report to decide which action to offer:

1. **Docker binary missing** → install via `sudo sh -c "curl -fsSL https://get.docker.com | sh"` — the official Docker upstream script, maintained by Docker Inc, tested across Ubuntu/Debian/Fedora/CentOS/Raspbian, installs engine + Compose v2 plugin in one shot.
2. **Daemon stopped (systemd host, user already in group)** → `sudo systemctl enable --now docker`.
3. **User not in docker group (binary present)** → `sudo usermod -aG docker <user>` with a mandatory post-action banner instructing to log out/in or run `newgrp docker` before proceeding.

The `Action` struct gains an optional `PostActionBanner` field. When any completed action has a non-empty banner, the bootstrap model shows an interstitial screen (theme.Warning color, Enter to dismiss) before emitting `BootstrapCompleteMsg`. Existing dir-creation actions are unaffected (zero value = no banner).

## Out of Scope

- **Rootless Docker** setup (requires user-level systemd, different group structure).
- **Non-systemd init systems** (OpenRC, runit, SysV): the daemon-start action is only offered when systemd is confirmed present; other init systems result in a non-fixable classification with a RUNBOOK entry.
- **Docker Desktop** installation on Linux or detection of existing Docker Desktop setups.
- **Compose-only** installation when Docker engine is already installed but Compose plugin is missing — that remains a non-fixable check until a future change.
