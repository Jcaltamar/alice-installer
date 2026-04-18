.PHONY: test test-short test-integration cover build build-all lint fmt tidy

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

# ── Code quality ─────────────────────────────────────────────────────────────

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .
	go vet ./...

tidy:
	go mod tidy
