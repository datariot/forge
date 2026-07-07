# Contributing to Forge

Thanks for your interest in improving Forge. This guide covers how to get set up, what's expected of a change, and how to get it merged.

## Prerequisites

- **Go 1.25+**
- **[Task](https://taskfile.dev)** (optional but recommended — `brew install go-task`)
- **Docker + Docker Compose** (only for integration tests)
- **[golangci-lint](https://golangci-lint.run) v2.12.x** — CI pins this version; match it locally to avoid surprises.

## Getting started

```bash
git clone https://github.com/datariot/forge
cd forge
task test        # or: go test ./...
```

## Development workflow

```bash
task test            # unit tests with the race detector
task test:coverage   # coverage report (opens coverage.html)
task test:integration # integration tests — starts PostgreSQL + Redis via Docker, runs, tears down
task lint            # gofmt check + go vet + golangci-lint
task fmt             # format the tree
task build:all       # build the framework and every example
task --list          # all available tasks
```

The raw `go` commands (`go test ./...`, `go build ./...`, `go vet ./...`) work without Task if you prefer.

## What a change needs

Before opening a pull request, make sure:

1. **It builds and passes** — `go build ./...`, `go test ./... -race`, and every module under `examples/` builds. CI runs all three.
2. **It's formatted** — `gofmt -l .` reports nothing. CI fails on unformatted code.
3. **It lints clean** — `golangci-lint run` reports 0 issues.
4. **It's tested** — new behavior comes with tests. The project targets **70%+ coverage** overall (CI enforces the threshold); the health and framework core aim higher.
5. **Exported symbols are documented** — godoc comments on exported types, functions, and methods, starting with the symbol name. pkg.go.dev is the API reference, so this matters.

### Adding a bundle

Bundles live under `bundles/<name>/` and follow a consistent shape:

- A `Config` struct with a `Validate() error` method and a `DefaultConfig()` constructor.
- `NewBundle(config Config) *Bundle`.
- `Name() string`, `Initialize(app *framework.App) error`, and `Stop(ctx context.Context) error`.
- Health checks via the `HealthContributor` interface where the bundle manages a dependency.
- **Evaluate third-party dependencies carefully** — Forge is "batteries included" but deliberately lightweight. A new direct dependency should earn its place; prefer the standard library where it's close enough.

Model new bundles on `bundles/postgresql` or `bundles/redis`.

## Commit and PR conventions

- Use [Conventional Commits](https://www.conventionalcommits.org) (`feat:`, `fix:`, `docs:`, `refactor:`, `chore:`; use `!` for breaking changes, e.g. `refactor!:`).
- Keep pull requests focused; a reviewable diff beats a sweeping one.
- Note any breaking API change in the PR description and in [CHANGELOG.md](CHANGELOG.md) under "Unreleased".
- CI (unit tests with race detection, formatting, linting, example builds, coverage) must be green before merge.

## Reporting issues

Open a GitHub issue with the Forge version (or commit), Go version, a minimal reproduction if you can, and what you expected versus what happened.
