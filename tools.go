//go:build tools

// tools.go pins development tool dependencies so they appear in go.mod/go.sum
// and produce reproducible toolchains. Run `go mod tidy` after editing.
// Do NOT `go install` from this file; use `make lint` / goreleaser directly.
package tools

import (
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/goreleaser/goreleaser"
)
