# installer-tui-flow Specification

## Purpose

Defines the Bubbletea state machine governing the full TUI experience: screen sequencing, keyboard navigation, branding palette, progress rendering, error screen behavior, and accessibility constraints (terminal size, TTY detection).

## Requirements

### Requirement: REQ-TUI-1 — State machine sequence

The TUI MUST implement the following screen states in order:

`splash → preflight → workspace-input → port-scan → env-write → pull → deploy → verify → result`

Transitions MUST be triggered by explicit events (step completion or user action). The model MUST NOT skip states. Each state MUST be representable as a distinct Go type implementing the Bubbletea `Model` interface or a tagged union.

#### Scenario: Full happy-path traversal

- GIVEN all preflight checks pass and no port conflicts
- WHEN operator completes workspace input and advances
- THEN states transition in exact sequence: splash → preflight → workspace-input → port-scan → env-write → pull → deploy → verify → result(success)

#### Scenario: Preflight hard failure blocks advance

- GIVEN Docker daemon check fails
- WHEN TUI renders preflight state
- THEN the "Continue" / Enter action is disabled
- AND operator cannot advance past preflight until failure is resolved (or installer exits)

---

### Requirement: REQ-TUI-2 — Keyboard navigation

The installer MUST handle the following keys:
- `q` — quit at any screen (triggers rollback if compose has started)
- `Enter` — advance to next screen or confirm current action
- `Esc` — go back one screen where reversal is safe (safe: workspace-input, port-scan; unsafe: env-write onwards — must show confirmation prompt)
- `Ctrl+C` — treated identically to `q`

#### Scenario: Quit before env-write (no rollback needed)

- GIVEN TUI is on workspace-input or port-scan screen
- WHEN operator presses `q`
- THEN installer exits immediately with code 0, no rollback needed

#### Scenario: Quit during deploy (rollback needed)

- GIVEN TUI is on deploy or verify screen and compose is running
- WHEN operator presses `q`
- THEN TUI shows "Aborting… running rollback" spinner, calls `docker compose down`, then exits

#### Scenario: Esc on workspace-input returns to preflight

- GIVEN TUI is on workspace-input screen
- WHEN operator presses `Esc`
- THEN TUI returns to preflight screen without losing workspace text

#### Scenario: Esc after env-write shows confirmation

- GIVEN TUI is on pull screen
- WHEN operator presses `Esc`
- THEN TUI shows "Going back will require re-running env generation. Continue? [y/N]"

---

### Requirement: REQ-TUI-3 — Branding color palette

All TUI screens MUST use the following lipgloss color palette:

| Role | Hex | Usage |
|---|---|---|
| Background | `#0D1B2A` (dark navy) | Screen background |
| Accent | `#00D4FF` (cyan) | Titles, active borders, spinners |
| Success | `#00FF87` (green) | Completed steps, healthy services |
| Destructive | `#FF4444` (red) | Hard failures, errors |
| Warning | `#FFA500` (orange) | Warnings (NVIDIA absent, etc.) |
| Muted | `#6B7280` (gray) | Inactive items, help text |

The installer MUST NOT use hard-coded ANSI escape codes directly; all styling MUST go through the lipgloss palette defined in `internal/branding/`.

#### Scenario: Palette applied to preflight result screen

- GIVEN preflight completes with one warning and all passes
- WHEN TUI renders
- THEN pass items render in green, warning item in orange, screen title in cyan, background in dark navy

#### Scenario: Error screen uses destructive color

- GIVEN Docker daemon check fails
- WHEN error is rendered
- THEN error text and border use `#FF4444`

---

### Requirement: REQ-TUI-4 — Progress rendering for long operations

For pull, deploy, and verify steps, the TUI MUST render:
- A spinner (cyan) for indeterminate steps
- A progress bar (cyan fill, navy background) for operations with a known count (e.g. N services pulled)
- Per-service status rows that update in-place

#### Scenario: Pull progress with 4 services

- GIVEN 4 service images are being pulled
- WHEN pull is in progress
- THEN TUI shows 4 service rows, each with a spinner while pulling and a green checkmark when done

#### Scenario: Healthcheck polling progress

- GIVEN 3 services with healthchecks are starting
- WHEN healthcheck polling runs
- THEN TUI shows a progress bar "2/3 healthy" that updates every poll cycle

---

### Requirement: REQ-TUI-5 — Error screens with actionable remediation

Every error screen MUST include:
1. A red-bordered box with the error title and message
2. A "What to do" section with a numbered list of remediation steps
3. At least one actionable key binding (retry, back, or quit)

#### Scenario: Docker daemon down error screen

- GIVEN preflight detects Docker daemon unreachable
- WHEN error screen renders
- THEN screen shows: error title "Docker daemon not running", message with details, remediation "1. Run: sudo systemctl start docker  2. Re-run the installer", and `[R] Retry  [Q] Quit`

#### Scenario: Port conflict error screen

- GIVEN port 8080 is in use
- WHEN port-conflict screen renders
- THEN screen shows the conflicting port, the service using it, an input field for an alternative port, and `[Enter] Use this port  [Esc] Back`

---

### Requirement: REQ-TUI-6 — Minimum terminal size enforcement

The installer MUST require a minimum terminal size of 80 columns × 24 rows. If the terminal is smaller at startup, the installer MUST display a plain-text message (no lipgloss rendering) and wait for resize. On resize above the minimum, normal rendering resumes.

#### Scenario: Terminal too small at startup

- GIVEN terminal is 60×20
- WHEN installer starts
- THEN TUI displays "Terminal too small. Resize to at least 80×24 to continue." in plain text
- AND no other UI elements are rendered

#### Scenario: Terminal resized to sufficient size mid-flow

- GIVEN installer is waiting on resize and operator resizes to 82×26
- WHEN `tea.WindowSizeMsg` is received
- THEN TUI resumes normal rendering at the current screen state

---

### Requirement: REQ-TUI-7 — Non-TTY (CI) detection

If `os.Stdin` is not a TTY (e.g. piped input, CI environment), the installer MUST detect this, print "Non-interactive mode not supported. Run alice-installer in a TTY." to stderr, and exit with code 1. The TUI MUST NOT attempt to render in non-TTY environments.

#### Scenario: Non-TTY stdin (CI pipeline)

- GIVEN stdin is a pipe (not a TTY)
- WHEN installer starts
- THEN prints error to stderr and exits with code 1 immediately

---

### Requirement: REQ-TUI-8 — Window resize mid-flow

The TUI MUST handle `tea.WindowSizeMsg` at any screen. On resize, the current screen MUST re-render without losing state. Spinners and progress bars MUST adapt to the new width.

#### Scenario: Window resize during pull

- GIVEN pull is in progress and terminal is resized from 100 to 90 columns
- WHEN `WindowSizeMsg` is received
- THEN pull progress re-renders correctly at 90 columns with no state loss
