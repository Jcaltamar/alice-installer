# Design: Optional Asterisk SIP Audio Server Setup

## Technical Approach

Add Asterisk as an opt-in installer package, not a core dependency. The TUI records an `Asterisk SIP Audio Server` selection, env rendering emits only the backend handoff contract, then a new `internal/asterisk` installer performs Linux-only host setup before compose pull/deploy. The backend/frontend terminal registration lifecycle remains out of scope.

## Architecture Decisions

| Decision | Choice | Alternatives considered | Rationale |
|---|---|---|---|
| Package boundary | Create `internal/asterisk` with host-install, config, service, AMI verify, backups, and fakes. | Extend `internal/bootstrap`. | Bootstrap is preflight remediation; Asterisk is optional product setup and needs richer rollback/idempotency. |
| TUI placement | Insert `StateOptionalPackages` after workspace input and before port scan; insert `StateAsteriskSetup` after env-write and before pull when selected. | Run Asterisk before env-write. | Env-write must receive stable AMI credentials and compose handoff first; deploy must wait until shared resources exist. |
| Config ownership | Manage marked sections in Asterisk config files and backup before first edit. | Rewrite whole files. | Preserves operator config while keeping re-runs deterministic. |
| AMI exposure | `manager.conf` AMI binds only `127.0.0.1:5038` with generated credentials and least required read/write classes. | Bind all interfaces or static credentials. | Backend uses host networking; localhost avoids network exposure. |
| Backend handoff | `.env`, compose backend env, and `/opt/alice-config/asterisk/integration.env`. | Backend discovers host files implicitly. | Explicit contract is testable and stable for later backend work. |

## Data Flow

```text
Workspace -> OptionalPackages -> PortScan -> EnvWrite
                                      |
                                      v
             selected? yes -> AsteriskSetup -> Pull -> Deploy -> Verify
                       no  -------------------> Pull -> Deploy -> Verify
```

`AsteriskSetup` sequence:

```text
TUI model -> internal/asterisk.Installer
Installer -> PackageManager: install asterisk
Installer -> ManagedConfig: backup + replace managed sections
Installer -> SharedResources: mkdir/write /opt/alice-config/asterisk/*
Installer -> ServiceManager: enable/restart asterisk
Installer -> AMIProbe: login 127.0.0.1:5038
```

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/asterisk/installer.go` | Create | Orchestrates idempotent setup, rollback, and verification. |
| `internal/asterisk/interfaces.go` | Create | `PackageManager`, `ServiceManager`, `ManagedConfigStore`, `SharedResourceStore`, `AMIProbe`, `BackupStore`. |
| `internal/asterisk/fake.go` | Create | Test fakes for strict TDD and TUI/headless seams. |
| `internal/asterisk/templates.go` | Create | Renders managed sections and `integration.env`. |
| `internal/tui/optional_packages.go` | Create | Selection screen for optional packages. |
| `internal/tui/asterisk_setup.go` | Create | Progress/result screen for host Asterisk setup. |
| `internal/tui/model.go`, `messages.go` | Modify | Add states, selected option storage, and transition messages. |
| `internal/envgen/env.go` | Modify | Add optional Asterisk env fields to `Input` rendering. |
| `internal/assets/.env.example`, `docker-compose.yml` | Modify | Add backend env handoff variables only. |
| `cmd/installer/main.go` | Modify | Wire production Asterisk dependencies and config root. |

## Interfaces / Contracts

```go
type Options struct {
    Enabled bool
    ConfigRoot string // /opt/alice-config/asterisk
    AMIUser string
    AMIPassword string
}
type Installer interface { Install(context.Context, Options) error }
```

Backend contract when selected:

```env
ASTERISK_ENABLED=true
ASTERISK_AMI_HOST=127.0.0.1
ASTERISK_AMI_PORT=5038
ASTERISK_AMI_USERNAME=<generated>
ASTERISK_AMI_PASSWORD=<generated>
ASTERISK_CONFIG_DIR=/opt/alice-config/asterisk
ASTERISK_INTEGRATION_ENV=/opt/alice-config/asterisk/integration.env
```

When not selected, `ASTERISK_ENABLED=false` and no host action runs.

Managed system config sections use markers like `; BEGIN alice-installer managed` and `; END alice-installer managed`. Shared resources are `/opt/alice-config/asterisk/{integration.env,sounds,recordings,templates,backups}` with private credentials written `0600` and directories `0750` where backend needs read access.

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit | Linux gate, package-manager choice, managed-section replacement, rollback, AMI localhost config, env rendering. | RED first in `internal/asterisk`, `internal/envgen`, and assets tests using fakes/temp dirs. |
| TUI | Optional selection and selected/unselected transitions. | Bubbletea model tests mirroring existing fullflow tests. |
| Integration | Re-run idempotency and failure rollback. | Fake command/service/AMI dependencies; no real Asterisk in unit CI. |
| Regression | Core install without Asterisk unchanged. | Existing `go test ./...` plus new no-selection tests. |

## Migration / Rollout

No data migration required. Rollout is opt-in and Linux-only; non-Linux shows the option as unavailable with a clear message. On failure, restore backups for touched Asterisk configs, leave existing operator config outside managed sections intact, and return `InstallFailureMsg{Stage:"asterisk-setup"}` before compose deploy.

## Open Questions

None.
