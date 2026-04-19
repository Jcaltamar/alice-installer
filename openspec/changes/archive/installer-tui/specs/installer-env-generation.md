# installer-env-generation Specification

## Purpose

Generates a `.env` file from `.env.example` defaults, requires the operator to provide only `WORKSPACE`, validates it, substitutes arch-specific image tags, and writes the result. The operation is idempotent; re-running prompts for confirmation before overwriting. No secrets are added or leaked.

## Requirements

### Requirement: REQ-ENV-1 — WORKSPACE is the only required input

The installer MUST prompt the operator to enter `WORKSPACE`. All other variables MUST default from `.env.example`. The TUI MUST NOT ask for any other input.

#### Scenario: Operator provides valid WORKSPACE

- GIVEN the TUI displays the workspace-input screen
- WHEN the operator types a valid name (e.g. `myhome`) and presses Enter
- THEN `WORKSPACE=myhome` is written to `.env`
- AND all other variables are copied from `.env.example` defaults

#### Scenario: Operator submits empty WORKSPACE

- GIVEN the workspace-input field is empty
- WHEN the operator presses Enter
- THEN the TUI shows inline error "WORKSPACE cannot be empty" and does not advance

---

### Requirement: REQ-ENV-2 — WORKSPACE validation rules

The installer MUST reject `WORKSPACE` values that:
- Are empty or whitespace-only
- Contain `/`, `\`, or `.` as the first character
- Contain any path separator characters
- Contain null bytes or non-printable ASCII characters

The installer SHOULD warn (non-blocking) if `WORKSPACE` contains Unicode characters outside ASCII range.

#### Scenario: WORKSPACE with path separator

- GIVEN operator enters `foo/bar` as WORKSPACE
- WHEN validation runs
- THEN TUI shows "WORKSPACE must not contain path separators" and clears the input

#### Scenario: WORKSPACE with leading dot

- GIVEN operator enters `.hidden` as WORKSPACE
- WHEN validation runs
- THEN TUI shows "WORKSPACE must not start with a dot" and clears the input

#### Scenario: WORKSPACE with whitespace

- GIVEN operator enters `my workspace` (with space)
- WHEN validation runs
- THEN TUI shows "WORKSPACE must not contain whitespace" and clears the input

#### Scenario: WORKSPACE with Unicode (non-ASCII)

- GIVEN operator enters `café` as WORKSPACE
- WHEN validation runs
- THEN TUI shows an orange warning "WORKSPACE contains non-ASCII characters — this may cause issues" but allows the operator to proceed

#### Scenario: Valid WORKSPACE

- GIVEN operator enters `alice-prod`
- WHEN validation runs
- THEN no error is shown and the TUI advances

---

### Requirement: REQ-ENV-3 — Arch-specific image tag substitution

The installer MUST substitute the following variables in `.env` based on detected architecture:

| Variable | amd64 value | arm64 value |
|---|---|---|
| `BACKEND_IMAGE` | `jcaltamare/aliceguardian:backend` | `jcaltamare/aliceguardian:backend-arm` |
| `WEBSOCKET_IMAGE` | `jcaltamare/aliceguardian:socket1` | `jcaltamare/aliceguardian:socket1-arm` |
| `WEB_IMAGE` | `jcaltamare/aliceguardian:web` | `jcaltamare/aliceguardian:web-arm` |
| `QUEUE_IMAGE` | `jcaltamare/aliceguardian:queue` | `jcaltamare/aliceguardian:queue-arm` |

The installer MUST write the resolved image tags, not the placeholder strings, to `.env`.

#### Scenario: amd64 arch substitution

- GIVEN detected arch is `amd64`
- WHEN env generation runs
- THEN `.env` contains `BACKEND_IMAGE=jcaltamare/aliceguardian:backend`
- AND `QUEUE_IMAGE=jcaltamare/aliceguardian:queue`

#### Scenario: arm64 arch substitution

- GIVEN detected arch is `arm64`
- WHEN env generation runs
- THEN `.env` contains `BACKEND_IMAGE=jcaltamare/aliceguardian:backend-arm`
- AND `QUEUE_IMAGE=jcaltamare/aliceguardian:queue-arm`

---

### Requirement: REQ-ENV-4 — Idempotent re-runs

If `.env` already exists, the installer MUST detect it and prompt the operator with "`.env` already exists — overwrite? [y/N]". If the operator declines, the installer MUST keep the existing file and skip env-write. If the operator confirms, the installer MUST overwrite.

#### Scenario: .env already exists, operator declines overwrite

- GIVEN `.env` already exists
- WHEN the installer reaches the env-write step
- THEN TUI shows overwrite prompt
- AND operator presses N (or Enter for default)
- THEN `.env` is unchanged and the installer continues with the existing file

#### Scenario: .env already exists, operator confirms overwrite

- GIVEN `.env` already exists
- WHEN overwrite prompt is shown and operator presses Y
- THEN `.env` is overwritten with freshly generated content

---

### Requirement: REQ-ENV-5 — .env.example availability

The installer MUST locate `.env.example` relative to its working directory. If `.env.example` is missing, the installer MUST fail at the env-generation step with a clear message: "`.env.example` not found at `<path>`. Cannot generate configuration." The installer MUST NOT continue to compose steps.

#### Scenario: .env.example missing

- GIVEN `.env.example` does not exist in the working directory
- WHEN env-generation runs
- THEN installer shows error screen "`.env.example` not found" with file path and exits env-write step
- AND compose orchestration does not start

#### Scenario: .env.example present but empty

- GIVEN `.env.example` exists but has 0 bytes
- WHEN env-generation runs
- THEN installer warns "`.env.example` is empty — generated `.env` will only contain WORKSPACE and image tags"
- AND continues

---

### Requirement: REQ-ENV-6 — No secret leakage

The installer MUST NOT write the POSTGRES_PASSWORD value from the existing compose file (the hardcoded leaked value) into `.env`. If `.env.example` contains `POSTGRES_PASSWORD=`, the installer MUST require the operator to set a new value OR generate a random 32-char alphanumeric string and display it once with a "Save this password" warning.

#### Scenario: POSTGRES_PASSWORD generation

- GIVEN `.env.example` contains `POSTGRES_PASSWORD=`
- WHEN env-generation runs
- THEN a random 32-char alphanumeric value is generated
- AND written to `.env` as `POSTGRES_PASSWORD=<generated>`
- AND TUI displays the value in a yellow box: "Generated DB password — save it now: <value>"

#### Scenario: Operator manually provides POSTGRES_PASSWORD via env before running

- GIVEN environment variable `POSTGRES_PASSWORD` is set in the shell
- WHEN env-generation runs
- THEN the pre-existing value is used and no generation occurs
