# Delta Spec: installer-docker-bootstrap

_Extends_: `installer-bootstrap` (dir-creation actions remain unchanged)

---

## REQ-DBS-1: Docker-missing detection and install action

When `CheckDockerDaemon` is FAIL and `BootstrapEnv.DockerBinaryPresent` is `false`, `ClassifyBlockers` MUST emit a fixable `Action` with ID `docker_install` that runs:

```
sudo sh -c "curl -fsSL https://get.docker.com | sh"
```

This action MUST be placed first in the fixable slice (highest priority) so it runs before any other actions that depend on Docker being installed.

### Scenarios

**GIVEN** a preflight report with CheckDockerDaemon=FAIL  
**AND** BootstrapEnv.DockerBinaryPresent=false  
**WHEN** ClassifyBlockers is called  
**THEN** fixable contains exactly one Action with ID "docker_install"  
**AND** Action.Command="sudo", Action.Args=["sh", "-c", "curl -fsSL https://get.docker.com | sh"]  
**AND** the action is first in the slice

**GIVEN** a preflight report with CheckDockerDaemon=FAIL  
**AND** BootstrapEnv.DockerBinaryPresent=true  
**WHEN** ClassifyBlockers is called  
**THEN** fixable does NOT contain an Action with ID "docker_install"

**GIVEN** a preflight report with CheckDockerDaemon=PASS  
**AND** BootstrapEnv.DockerBinaryPresent=false (hypothetical edge case)  
**WHEN** ClassifyBlockers is called  
**THEN** no action with ID "docker_install" is emitted (PASS items are skipped)

---

## REQ-DBS-2: Docker-daemon-stopped detection and systemctl action (systemd only)

When `CheckDockerDaemon` is FAIL, `BootstrapEnv.DockerBinaryPresent=true`, `BootstrapEnv.UserInDockerGroup=true`, and `BootstrapEnv.SystemdPresent=true`, `ClassifyBlockers` MUST emit a fixable `Action` with ID `systemd_start_docker` that runs:

```
sudo systemctl enable --now docker
```

When `SystemdPresent=false` under these same conditions, the check MUST be classified as **non-fixable**.

### Scenarios

**GIVEN** CheckDockerDaemon=FAIL  
**AND** BootstrapEnv{DockerBinaryPresent:true, UserInDockerGroup:true, SystemdPresent:true}  
**WHEN** ClassifyBlockers is called  
**THEN** fixable contains Action with ID "systemd_start_docker"  
**AND** Action.Command="sudo", Action.Args=["systemctl", "enable", "--now", "docker"]

**GIVEN** CheckDockerDaemon=FAIL  
**AND** BootstrapEnv{DockerBinaryPresent:true, UserInDockerGroup:true, SystemdPresent:false}  
**WHEN** ClassifyBlockers is called  
**THEN** nonFixable contains the CheckDockerDaemon result  
**AND** fixable does NOT contain any docker-related action

---

## REQ-DBS-3: User-not-in-docker-group detection and usermod action with post-action banner

When `CheckDockerDaemon` is FAIL, `BootstrapEnv.DockerBinaryPresent=true`, and `BootstrapEnv.UserInDockerGroup=false`, `ClassifyBlockers` MUST emit a fixable `Action` with ID `docker_group_add` that runs:

```
sudo usermod -aG docker <username>
```

Where `<username>` is resolved from `BootstrapEnv.UserName`.

The action MUST have a non-empty `PostActionBanner` instructing the user to log out/in or run `newgrp docker`.

After ALL bootstrap actions complete, if any action has a non-empty `PostActionBanner`, the `BootstrapModel` MUST:
1. Show all banners in `theme.Warning` color in an interstitial screen.
2. Wait for the user to press Enter.
3. Only then emit `BootstrapCompleteMsg`.

### Scenarios

**GIVEN** CheckDockerDaemon=FAIL  
**AND** BootstrapEnv{DockerBinaryPresent:true, UserInDockerGroup:false, UserName:"alice"}  
**WHEN** ClassifyBlockers is called  
**THEN** fixable contains Action with ID "docker_group_add"  
**AND** Action.Command="sudo", Action.Args=["usermod", "-aG", "docker", "alice"]  
**AND** Action.PostActionBanner is non-empty (mentions "log out" or "newgrp")

**GIVEN** a BootstrapModel with one completed action that has PostActionBanner="Log out and back in."  
**WHEN** all actions finish (last BootstrapActionResultMsg Err=nil)  
**THEN** model shows banner screen instead of immediately emitting BootstrapCompleteMsg  
**AND** pressing Enter emits BootstrapCompleteMsg

**GIVEN** a BootstrapModel with one completed action that has PostActionBanner=""  
**WHEN** last action finishes (Err=nil)  
**THEN** model immediately emits BootstrapCompleteMsg (no banner screen)

---

## REQ-DBS-4: Action ordering priority

`ClassifyBlockers` MUST return the fixable slice in this priority order:

1. Docker engine install (`docker_install`) — if applicable
2. Directory creation actions (`media_writable`, `config_writable`) — if applicable
3. Systemctl daemon start (`systemd_start_docker`) — if applicable
4. Docker group add (`docker_group_add`) — if applicable

### Scenarios

**GIVEN** a report with CheckDockerDaemon=FAIL (binary missing), CheckMediaWritable=FAIL, CheckConfigWritable=FAIL  
**AND** BootstrapEnv{DockerBinaryPresent:false, UserName:"bob"}  
**WHEN** ClassifyBlockers is called  
**THEN** fixable[0].ID = "docker_install"  
**AND** fixable[1].ID = string(CheckMediaWritable)  
**AND** fixable[2].ID = string(CheckConfigWritable)

**GIVEN** a report with CheckDockerDaemon=FAIL (binary present, group missing), CheckConfigWritable=FAIL  
**AND** BootstrapEnv{DockerBinaryPresent:true, UserInDockerGroup:false, UserName:"bob"}  
**WHEN** ClassifyBlockers is called  
**THEN** fixable[0].ID = string(CheckConfigWritable)  
**AND** fixable[1].ID = "docker_group_add"
