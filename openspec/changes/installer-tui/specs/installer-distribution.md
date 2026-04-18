# installer-distribution Specification

## Purpose

Defines how the installer binary is cross-compiled, named, checksummed, and published. Targets Linux amd64 and arm64 only. Single static binary per arch with no runtime dependencies. Artifacts are uploadable to GitHub Releases.

## Requirements

### Requirement: REQ-DIST-1 — Cross-compilation targets

The build system MUST produce exactly two binaries per release:
- `alice-installer-linux-amd64` (GOOS=linux, GOARCH=amd64)
- `alice-installer-linux-arm64` (GOOS=linux, GOARCH=arm64)

Both MUST be built with `CGO_ENABLED=0`. No other targets (Windows, macOS, 386, etc.) are produced in v1.

#### Scenario: goreleaser build produces exactly 2 binaries

- GIVEN goreleaser is run with the project's `.goreleaser.yml`
- WHEN the build completes
- THEN the `dist/` directory contains exactly `alice-installer-linux-amd64` and `alice-installer-linux-arm64`
- AND no other platform binaries exist

#### Scenario: Build on non-Linux host (CI, macOS dev machine)

- GIVEN goreleaser runs on a macOS or non-Linux CI runner
- WHEN cross-compilation runs with `CGO_ENABLED=0`
- THEN both Linux binaries are produced correctly without requiring a Linux host

---

### Requirement: REQ-DIST-2 — Static binaries (no runtime deps)

Both binaries MUST be statically linked. Running `ldd alice-installer-linux-amd64` on a Linux host MUST return "not a dynamic executable". The binary MUST run on any Linux system without installing additional libraries.

#### Scenario: amd64 binary has no dynamic deps

- GIVEN the amd64 binary is produced by goreleaser
- WHEN `ldd alice-installer-linux-amd64` is run
- THEN output is "not a dynamic executable"

#### Scenario: arm64 binary runs on minimal Alpine container

- GIVEN the arm64 binary is copied into a minimal Alpine Linux arm64 container with no extra packages
- WHEN `./alice-installer-linux-arm64` is executed
- THEN the binary launches without "no such file" or library errors

---

### Requirement: REQ-DIST-3 — Checksums

The build MUST produce a `checksums.txt` file in SHA256 format containing entries for both binaries. The format MUST be: `<sha256>  <filename>`. This file MUST be included in the GitHub Release upload alongside the binaries.

#### Scenario: Checksums file generated

- GIVEN goreleaser build completes
- WHEN `checksums.txt` is inspected
- THEN it contains exactly 2 lines, one per binary, each starting with a 64-char hex sha256

#### Scenario: Checksum verification

- GIVEN a downloaded binary and `checksums.txt`
- WHEN `sha256sum -c checksums.txt` is run
- THEN both entries show "OK"

---

### Requirement: REQ-DIST-4 — Binary naming convention

Binaries MUST be named `alice-installer-linux-<arch>` with no file extension. The `<arch>` segment MUST be `amd64` or `arm64` exactly (no aliases like `x86_64` or `aarch64`).

#### Scenario: amd64 binary name

- GIVEN goreleaser build completes
- WHEN directory listing is inspected
- THEN binary is named exactly `alice-installer-linux-amd64`

---

### Requirement: REQ-DIST-5 — GitHub Releases upload

The release workflow MUST upload both binaries and `checksums.txt` to the GitHub Release for the tagged version. No signing is required in v1. The release notes MUST include installation instructions with `curl` download commands for each arch.

#### Scenario: Release artifacts uploaded

- GIVEN a git tag `v0.1.0` is pushed
- WHEN the goreleaser GitHub Actions workflow runs
- THEN the GitHub Release for `v0.1.0` contains: `alice-installer-linux-amd64`, `alice-installer-linux-arm64`, `checksums.txt`
- AND release notes include download commands

---

### Requirement: REQ-DIST-6 — Makefile build wrapper

The project MUST provide a `Makefile` target `make build` that invokes goreleaser locally. A `make build-snapshot` target MUST allow building without a git tag (goreleaser `--snapshot`). Build output MUST go to `dist/`.

#### Scenario: Local snapshot build

- GIVEN developer runs `make build-snapshot` on a Linux or macOS host
- WHEN goreleaser runs in snapshot mode
- THEN `dist/alice-installer-linux-amd64` and `dist/alice-installer-linux-arm64` are produced without requiring a git tag

---

### Requirement: REQ-DIST-7 — arm64 CI parity via QEMU

The CI pipeline MUST run the arm64 binary through at least a smoke test on every PR. The smoke test MAY use QEMU emulation (`ubuntu-latest` + `qemu-user-static`). The smoke test MUST at minimum execute `alice-installer-linux-arm64 --version` and verify exit code 0.

#### Scenario: arm64 smoke test via QEMU

- GIVEN a GitHub Actions `ubuntu-latest` runner with `qemu-user-static` installed
- WHEN `alice-installer-linux-arm64 --version` is executed
- THEN exit code is 0 and version string is printed
- AND the test is not skipped with `-short` (it is fast enough)

#### Scenario: arm64 QEMU integration test skipped under -short

- GIVEN tests are run with `go test -short ./...`
- WHEN the arm64 integration test is encountered
- THEN `t.Skip("skipping arm64 QEMU test in -short mode")` is called
- AND the test does not block the fast feedback loop
