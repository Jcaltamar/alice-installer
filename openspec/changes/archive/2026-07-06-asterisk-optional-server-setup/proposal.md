# Proposal: Optional Asterisk SIP Audio Server Setup

## Intent

Add an optional installer path that prepares a host-managed Asterisk SIP audio server for Alice Guardian administration. Operators can enable it from the TUI without making Asterisk mandatory.

## Scope

### In Scope
- Add `Optional Packages -> Asterisk SIP Audio Server` to the TUI.
- Install and configure host Asterisk, including AMI on `127.0.0.1:5038` for backend administration.
- Create `/opt/alice-config/asterisk` resources: `integration.env`, sounds, recordings, and templates.
- Ensure backend access through existing host networking and `/opt/alice-config:/opt/alice-config` mount.

### Out of Scope
- Registering Dahua terminals.
- Managing terminal-specific SIP endpoints, extensions, or provisioning lifecycle.
- Backend/frontend feature implementation outside alice-installer.

## Capabilities

### New Capabilities
- `installer-asterisk-optional-setup`: Opt-in TUI selection, host Asterisk setup, AMI integration, shared resource preparation, and verification.

### Modified Capabilities
- None.

## Approach

Model Asterisk as an optional post-install package with explicit selection and idempotent host actions. Reuse installer seams for command execution, filesystem writes, progress reporting, and test fakes. Prefer localhost AMI plus shared config under `/opt/alice-config/asterisk` so backend work can consume stable resources later.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/tui/` | Modified | Optional packages screen plus Asterisk setup progress/results. |
| `internal/bootstrap/` or new `internal/asterisk/` | New/Modified | Package install, config writes, service enable/restart, AMI verification. |
| `internal/envgen/` | Modified | Generate Asterisk integration env/resource paths when selected. |
| `internal/assets/` | Modified | Add templates/sounds/resources for `/opt/alice-config/asterisk`. |
| `cmd/installer/` | Modified | Wire dependencies and optional package flow. |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Package manager differences | Medium | Detect supported distros; fail with actionable guidance. |
| AMI exposure or weak credentials | Medium | Bind to `127.0.0.1`, generate secrets, restrict permissions. |
| Partial setup leaves broken service | Medium | Make steps idempotent; verify service and AMI before success. |
| Non-Linux targets cannot install Asterisk | Medium | Gate feature to supported Linux platforms. |

## Rollback Plan

Disable the optional package path and revert installer assets. On host failure, stop/disable installer-created Asterisk, restore backed-up configs, and leave `/opt/alice-config/asterisk` removable without affecting core compose services.

## Dependencies

- Supported Linux package manager and systemd-managed Asterisk package.
- Backend remains `network_mode: host` with `/opt/alice-config` mounted.

## Success Criteria

- [ ] Core install still works when Asterisk is not selected.
- [ ] Selecting Asterisk installs/configures host Asterisk and verifies AMI on `127.0.0.1:5038`.
- [ ] Backend-visible `/opt/alice-config/asterisk` resources are created with secure permissions.
- [ ] No Dahua terminal or endpoint provisioning is implemented in alice-installer.
