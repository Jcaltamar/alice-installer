# alice-installer

TUI installer for the Alice Guardian Docker Compose stack. Written in Go with Bubbletea.

## Status

Early development. See `openspec/changes/installer-tui/` for the active change set:

- `proposal.md` — scope, approach, tradeoffs
- `design.md` — architecture, interfaces, state machine
- `specs/` — capability specifications (33 requirements, 72 scenarios)
- `tasks.md` — 84-task implementation checklist (strict TDD)

## Targets (v1)

- Linux amd64
- Linux arm64

macOS and Windows are planned for later iterations.

## Build

```sh
make test        # run unit tests
make test-short  # skip slow/integration
make build       # host arch binary in bin/
make build-all   # cross-compile to dist/ (linux/amd64, linux/arm64)
make lint        # golangci-lint
```

## Layout

```
cmd/installer/       entry point
internal/
  assets/            embedded docker-compose.yml, overlay, .env.example, logo
  compose/           compose runner wrapper
  docker/            docker client wrapper
  envgen/            .env template + password generation
  platform/          arch / OS / GPU detection
  ports/             port scanning + conflict resolution
  preflight/         pre-install checks coordinator
  secrets/           crypto/rand password generation
  theme/             Lipgloss color tokens from LogoNight.png
  tui/               Bubbletea Model/Update/View per state
openspec/            SDD planning artifacts
```

## License

TBD.
