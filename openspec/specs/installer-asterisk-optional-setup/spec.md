# installer-asterisk-optional-setup Specification

## Purpose

Define the optional installer capability that prepares a host-managed Asterisk SIP Audio Server for Alice Guardian administration without making Asterisk part of the mandatory base install.

## Out of Scope

This capability does not include:

- Terminal registration.
- Dahua credentials.
- Terminal-specific endpoint lifecycle, SIP provisioning, or extension management.
- Backend or frontend application logic beyond installer-provided artifacts and environment wiring.

## Requirements

### Requirement: Optional TUI Entry for Asterisk SIP Audio Server

The installer MUST expose `Optional Packages -> Asterisk SIP Audio Server` in the TUI and MUST keep the package disabled by default. The optional package MUST only execute when the operator explicitly selects it.

#### Scenario: Optional package is visible but not selected

- GIVEN the installer is running in interactive mode on a supported host
- WHEN the optional packages screen is shown
- THEN `Asterisk SIP Audio Server` MUST be listed as an optional entry
- AND it MUST be unselected by default

#### Scenario: Operator skips the optional package

- GIVEN the operator does not select `Asterisk SIP Audio Server`
- WHEN the installer proceeds through the base install
- THEN the installer MUST continue without running any Asterisk setup steps
- AND the base install MUST complete with the same behavior as if the optional package never existed

### Requirement: Linux-Only Availability and Base Install Isolation

The optional Asterisk setup MUST be available only on supported Linux hosts. If the host is not supported or the package is skipped, the installer MUST preserve the existing base install behavior and MUST NOT fail the mandatory installation path.

#### Scenario: Supported Linux host

- GIVEN the installer is running on a supported Linux host
- WHEN optional packages are enumerated
- THEN `Asterisk SIP Audio Server` MUST be available for selection

#### Scenario: Unsupported host

- GIVEN the installer is running on a non-Linux host or an unsupported Linux environment
- WHEN optional packages are enumerated
- THEN `Asterisk SIP Audio Server` MUST be hidden or disabled
- AND the mandatory install flow MUST remain usable without Asterisk

#### Scenario: Optional package is skipped

- GIVEN the operator skips `Asterisk SIP Audio Server`
- WHEN the installation completes
- THEN the installer MUST behave as a normal base install
- AND no Asterisk-specific files, services, or environment references MUST be required for success

### Requirement: Host Asterisk Installation, Configuration, and Verification

When the optional package is selected, the installer MUST install host Asterisk, apply the required configuration for Alice Guardian administration, and verify the service before reporting success.

#### Scenario: Selected optional package completes successfully

- GIVEN the host is a supported Linux system
- AND the operator selected `Asterisk SIP Audio Server`
- WHEN the optional setup runs
- THEN the installer MUST install Asterisk on the host
- AND it MUST configure Asterisk for local administration
- AND it MUST verify the Asterisk service is reachable and ready before success is reported

#### Scenario: Asterisk verification fails

- GIVEN the optional package was selected
- AND Asterisk installation or configuration completed partially
- WHEN service verification or AMI verification fails
- THEN the installer MUST report the optional setup as failed
- AND it MUST NOT report the optional package as successfully installed

### Requirement: Localhost-Bound AMI Administration

The installer MUST configure Asterisk AMI for backend administration bound to `127.0.0.1:5038` only. AMI MUST NOT be exposed on external interfaces.

#### Scenario: AMI is configured for localhost access

- GIVEN the optional package is selected on a supported Linux host
- WHEN AMI configuration is generated
- THEN the AMI listener MUST be bound to `127.0.0.1`
- AND the AMI port MUST be `5038`
- AND the resulting configuration MUST be suitable for backend administration from the local host only

#### Scenario: AMI would be exposed externally

- GIVEN the generated or existing AMI configuration would bind to a non-localhost interface
- WHEN the installer validates the configuration
- THEN the installer MUST treat the setup as invalid
- AND it MUST fail the optional package rather than silently exposing AMI externally

### Requirement: Backend-Visible Asterisk Resource Bundle

When the optional package is selected, the installer MUST create a backend-visible resource bundle under `/opt/alice-config/asterisk`. The bundle MUST include `integration.env`, templates, sounds, and recordings, and MUST use secure filesystem permissions appropriate for host-managed shared configuration.

#### Scenario: Resource bundle is created

- GIVEN the operator selected `Asterisk SIP Audio Server`
- WHEN the optional setup completes
- THEN `/opt/alice-config/asterisk` MUST exist
- AND it MUST contain `integration.env`
- AND it MUST contain the resources required for templates, sounds, and recordings
- AND the created resources MUST be accessible to the backend through the shared host path

#### Scenario: Resource creation fails

- GIVEN the optional package is selected
- WHEN the installer cannot create or secure `/opt/alice-config/asterisk`
- THEN the installer MUST fail the optional setup
- AND it MUST surface the filesystem problem to the operator

### Requirement: Compose and Environment Integration for Backend Access

The installer MUST update compose and environment artifacts so the backend container can discover and access `/opt/alice-config/asterisk` via the shared `/opt/alice-config:/opt/alice-config` mount. The installer MUST provide the necessary environment references without implementing backend application logic.

#### Scenario: Backend access wiring is present

- GIVEN the optional package is selected
- WHEN installer artifacts are generated
- THEN the compose or environment configuration MUST include the shared `/opt/alice-config:/opt/alice-config` access path
- AND the backend-visible Asterisk resource location MUST be discoverable from the generated environment artifacts

#### Scenario: Optional package is skipped

- GIVEN the operator skips `Asterisk SIP Audio Server`
- WHEN compose and environment artifacts are generated
- THEN the installer MUST preserve existing base install behavior
- AND it MUST NOT require Asterisk-specific backend wiring for success

### Requirement: Scope Boundary Preservation

The installer MUST NOT implement terminal registration, Dahua credential handling, terminal-specific endpoint lifecycle, or backend/frontend application logic as part of this capability.

#### Scenario: Operator expects terminal provisioning

- GIVEN the optional Asterisk setup is selected
- WHEN the installer completes
- THEN terminal registration and endpoint lifecycle MUST remain unimplemented by this capability
- AND the installer MUST only provide the host-side resources and wiring defined in this specification

