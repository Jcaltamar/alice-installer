.PHONY: test test-short test-integration cover build build-all build-snapshot release-local lint fmt tidy prescale-logo e2e

BINARY      := alice-installer
BIN_DIR     := bin
DIST_DIR    := dist
MODULE      := github.com/jcaltamar/alice-installer
CMD_PATH    := ./cmd/installer

LDFLAGS     := -s -w
GO_FLAGS    := CGO_ENABLED=0

# ── Test targets ─────────────────────────────────────────────────────────────

test:
	go test ./...

test-short:
	go test -short ./...

test-integration:
	go build -o $(DIST_DIR)/$(BINARY)-linux-amd64 $(CMD_PATH) && \
	go test -tags=integration ./...

cover:
	go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out

# ── Build targets ─────────────────────────────────────────────────────────────

build:
	$(GO_FLAGS) go build -trimpath -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) $(CMD_PATH)

build-all:
	mkdir -p $(DIST_DIR)
	GOOS=linux GOARCH=amd64  $(GO_FLAGS) go build -trimpath -ldflags="$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY)-linux-amd64  $(CMD_PATH)
	GOOS=linux GOARCH=arm64  $(GO_FLAGS) go build -trimpath -ldflags="$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY)-linux-arm64  $(CMD_PATH)

build-snapshot:
	goreleaser build --snapshot --clean

# release-local builds the per-arch tarballs + checksums.txt for manual upload
# to a GitHub release when the goreleaser CI workflow is unavailable. Pass
# VERSION=x.y.z; binary is placed at the root of each tarball (goreleaser-default
# layout) alongside RUNBOOK.md and README.md.
VERSION ?= 0.0.0-snapshot
release-local:
	@rm -rf $(DIST_DIR) && mkdir -p $(DIST_DIR)
	@for ARCH in amd64 arm64; do \
	  echo "→ Building linux/$$ARCH"; \
	  GOOS=linux GOARCH=$$ARCH $(GO_FLAGS) go build -trimpath \
	    -ldflags="$(LDFLAGS) -X main.version=v$(VERSION)" \
	    -o $(DIST_DIR)/$(BINARY) $(CMD_PATH) || exit 1; \
	  cp RUNBOOK.md README.md $(DIST_DIR)/; \
	  NAME=$(BINARY)_$(VERSION)_linux_$$ARCH; \
	  tar -czf $(DIST_DIR)/$$NAME.tar.gz -C $(DIST_DIR) $(BINARY) RUNBOOK.md README.md; \
	  rm $(DIST_DIR)/$(BINARY); \
	done
	@rm -f $(DIST_DIR)/RUNBOOK.md $(DIST_DIR)/README.md
	@cd $(DIST_DIR) && sha256sum $(BINARY)_*.tar.gz > checksums.txt
	@echo; echo "✓ Release assets ready in $(DIST_DIR)/:"; ls -lh $(DIST_DIR)/

# ── Code quality ─────────────────────────────────────────────────────────────

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .
	go vet ./...

tidy:
	go mod tidy

# ── E2E ──────────────────────────────────────────────────────────────────

# e2e runs the full end-to-end test harness inside a systemd Ubuntu container.
# Requires a working local Docker daemon and ~500 MB of disk (basic mode).
# Set FULL_DEPLOY=1 to also pull images and bring services up (~3 GB).
e2e:
	./scripts/e2e/run.sh

# ── Assets ───────────────────────────────────────────────────────────────────

# prescale-logo resamples logo_alice_security.png (~1 MB at 12501x12500) down
# to a 256x256 PNG embedded via //go:embed. Run whenever the source logo changes.
prescale-logo:
	go run ./scripts/prescale-logo
