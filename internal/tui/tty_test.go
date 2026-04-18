// Package tui — TTY detection notes.
//
// REQ-TUI-7 states: if stdin is not a TTY, print an error to stderr and exit 1.
// That check lives in cmd/installer/main.go (the entrypoint), not inside the
// tui package, because tea.Program itself requires a real TTY and we cannot
// mock os.Stdin at the package level without runtime hacks.
//
// The tui package itself is NOT responsible for TTY detection. It is the
// caller's responsibility to gate tea.NewProgram with an IsTerminal check
// before handing control to the TUI.
//
// Unit coverage: teatest.NewTestModel (used in fullflow_test.go) bypasses
// os.Stdin and provides a synthetic I/O pair, confirming the tui.Model itself
// does not hardcode any TTY dependency. This file exists to document that
// design decision and satisfy task T-065/T-066.
//
// Integration coverage: when running under CI (no TTY), tests that use
// teatest work because teatest creates its own in-memory reader/writer.
// The -short flag gates any test that would need a real terminal.

package tui
