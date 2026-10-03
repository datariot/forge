---
layout: page
title: Architecture
permalink: /architecture/
---

# Architecture

A map of where things live in this repository, for a session that is about to change
something in it. This is the page escapement's ADR-009 asks every repo to carry. It is
deliberately about *structure*, not usage:

- `CLAUDE.md` — commands, conventions, troubleshooting.
- [`docs/getting-started.md`](getting-started.md) — walkthrough of a first service.
- [`docs/bundles.md`](bundles.md) — per-bundle configuration and options.

## Packages

Forge is one Go module, `github.com/datariot/forge`. The root `doc.go` is `package forge`:
documentation only, no code — the overview, quick start, and the list of HTTP endpoints a
service gets. Nothing imports it.

### `framework/` — lifecycle and servers

The core. Everything about how a service starts, serves, and stops is here.

- `framework/app.go` — the whole orchestration: the `App` struct, the functional-option
  builder `New`, `Run`/`Start`/`Stop`, the extension-point interfaces, and both server
  constructors (`startGRPCServer`, `startHTTPServer`). The largest and most important
  file in the repo.
- `framework/http.go` — the HTTP server: `HTTPServerConfig`/`DefaultHTTPServerConfig`,
  `HTTPServerBuilder`, and `buildHandler`, which assembles the routes and middleware.
  Health routes live under `HealthPathPrefix` (default `/health`, plus `/health/ready`
  and `/health/live`); `/metrics` is served when metrics are enabled, optionally behind
  basic auth; `/debug/pprof/*` is served only when `BaseConfig.EnablePprof` is set *and*
  the environment is development.
- `framework/shutdown.go` — `ShutdownOrchestrator`: named hooks, a whole-shutdown timeout,
  panic recovery per hook, and the reverse-registration-order execution the `App` relies on.
- `framework/logging.go` — `LoggingManager` (zerolog; console in development, JSON
  otherwise) and the adapter that lets the health registry log through it.
- `framework/observability.go` — `ObservabilityManager`: OTLP trace and metric providers,
  `Initialize`/`Shutdown`, `Tracer`/`Meter`.

### `health/` — checks and probe responses

- `health/check.go` — the `Check` interface (`Name`, `Liveness`, `Readiness`),
  `CheckConfig` and `DefaultCheckConfig` (required, 5s timeout, 30s interval), plus the
  ready-made `BasicCheck`, `AlwaysHealthyCheck` and `AlwaysUnhealthyCheck`.
- `health/registry.go` — `Registry`. Two modes: by default it runs all checks concurrently on
  every call; after `Start` a background goroutine per check runs on its `Interval` and
  reads are served from cache, so a probe never blocks on a hanging dependency. Also
  `Register`/`Unregister` (safe after `Start`) and `SetReady`, the explicit readiness gate.
- `health/status.go` — `Report` (the aggregate: status, message, per-check `Details`) with
  `HTTPStatus()`, `JSON()`, `Redacted()` — which strips raw check errors before they reach
  an unauthenticated endpoint — and the smaller `StatusResponse`.

### `config/` — the shared config struct

`config/base.go` only. `BaseConfig` carries every common field with `yaml` and `env` tags,
`DefaultBaseConfig()` supplies defaults (`:8080` gRPC, `:8081` HTTP, 30s shutdown
timeout), and `Validate()` is what `framework.New` calls before it will build an `App`.
`BaseConfig` reads neither files nor the environment itself — that is `bundles/configloader`.
Services embed `BaseConfig` and add their own fields; the `Validator` interface is the
convention for validating them.

### `bundles/` — optional integrations

One directory per bundle, each holding a single bundle file — `bundles/postgresql/bundle.go`
and so on — with a `NewBundle(Config)` constructor and a `DefaultConfig()`. Three report a
`Name()` that differs from their directory:

| Directory | `Name()` | What it provides | Health checks |
|---|---|---|---|
| `bundles/postgresql` | `postgresql` | `*sql.DB` pool via `DB()` | yes |
| `bundles/redis` | `redis` | client, cache, pub/sub, locks, rate limiters | yes |
| `bundles/prometheus` | `prometheus` | metric registry, recorders, auto HTTP/gRPC instrumentation | yes |
| `bundles/jwt` | `jwt-auth` | service-to-service tokens, gRPC interceptors, HTTP middleware | no |
| `bundles/httpclient` | `http-client` | resilient `Client()` with retries and circuit breaking | no |
| `bundles/configloader` | `config-loader` | YAML + env loading, optional file watching | no |

`bundles/prometheus` also ships `bundles/prometheus/grafana-dashboard.json`.

### `forgeerrors/` — error vocabulary

`forgeerrors/errors.go` only: `DomainError` (code, message, cause; `WithMessage`/`WithCause`, and
`Is`/`Unwrap` so `errors.Is` works), a set of shared sentinels such as
`ErrInvalidConfiguration` and `ErrRepositoryUnavailable`, and classifiers
(`IsTransientError`, `IsAuthenticationError`, `IsValidationError`,
`IsConfigurationError`, `IsRepositoryError`). Bundles return these from `Initialize`.

### `testutil/` and `examples/`

`testutil/testutil.go` holds the shared test helpers: `NewTestApp` (random ports,
automatic cleanup), `NewTestComponent`, `NewTestBundle`, `TestHTTPClient`, and the
`AssertX` helpers.

`examples/` has seven runnable services — `examples/simple-service`,
`examples/config-service`, `examples/httpclient-service`, `examples/jwt-service`,
`examples/postgresql-service`, `examples/prometheus-service`, `examples/redis-service`.
Each is its **own Go module** with its own `go.mod`, so `go build ./...` at the root does
not cover them; the gate builds each one separately.

## Lifecycle

All of it is in `framework/app.go`. `Run` is the usual entry point: it calls `Start`,
waits on SIGINT/SIGTERM via `signal.NotifyContext`, then calls `Stop` with a context
bounded by `ShutdownTimeout`. `Start` and `Stop` are exported, which is how tests drive
an app without signals.

`New` runs first and is not part of `Start`: it applies the options, requires a config,
calls `config.Validate()`, and initializes logging.

`Start`, in order:

1. `observability.Initialize` — tracing and metrics.
2. Bundles — `Initialize(app)` in registration order.
3. Startup hooks, in registration order.
4. `healthRegistry.Start` — the background check runner. From here on probes read cached
   results. It is started *after* bundles and hooks so their checks are already
   registered, and before any server accepts traffic.
5. gRPC server — **only when at least one `Registrar` is registered**. An HTTP-only
   service never binds `GRPCAddr`. When it does start, the standard gRPC health service
   is registered, and reflection only if `EnableReflection` is set.
6. HTTP server — always started.
7. Components — `Start(ctx)` in registration order.
8. Ready — `SetReady(true)` immediately, or after `ReadinessInitialDelay` via a
   cancellable timer so a `Stop` during the delay prevents the flip.

`Stop` marks the service not-ready first, stops the health runner, then builds a
`ShutdownOrchestrator`. The orchestrator executes hooks in **reverse** registration
order, and `Stop` registers them bundles → components → user hooks → gRPC → HTTP →
observability. So the actual execution order is:

1. User shutdown hooks (reverse registration order).
2. Components, reverse registration order.
3. Bundles, reverse registration order.
4. gRPC `GracefulStop`, forced `Stop` on timeout.
5. HTTP `Shutdown`.
6. Observability flush — last, so logging survives the rest.

If the overall timeout expires mid-sequence, remaining hooks are skipped and reported as
an error rather than run past the deadline.

## Extension points

What a service implements, all declared in `framework/app.go`:

```go
type Component interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

type Bundle interface {
	Name() string
	Initialize(app *App) error
	Stop(ctx context.Context) error
}

type HealthContributor interface {
	HealthChecks() []health.Check
}

type Registrar interface {
	RegisterGRPC(server *grpc.Server) error
}

type HTTPRegistrar interface {
	RegisterHTTPRoutes(mux *http.ServeMux)
}

type StartupHook func(ctx context.Context, app *App) error
type ShutdownHook func(ctx context.Context, app *App) error
```

What it passes to `New`: `WithConfig` (required), `WithVersion`, `WithComponent`,
`WithBundle`, `WithGRPCRegistrar`, `WithHealthContributor`, `WithStartupHook`,
`WithShutdownHook`, `WithHTTPServerConfig`, `WithUnaryInterceptor`,
`WithStreamInterceptor`, and the injection options `WithLogging`, `WithObservability`,
`WithHealthRegistry` for replacing the framework's defaults in tests.

Two asymmetries worth knowing before you go looking for them:

- `WithHealthContributor` is not a separate registry; it appends a **startup hook** that
  registers the contributor's checks. So its checks land in the registry at step 3 above.
- There is no `WithHTTPRegistrar`. A registered `Component` is picked up as an
  `HTTPRegistrar` by type assertion inside `startHTTPServer` — implementing the interface
  is enough.

A bundle reaches the request pipeline from its `Initialize` via three `App` methods:
`AddUnaryInterceptor`, `AddStreamInterceptor`, `AddHTTPMiddleware`. All three are no-ops
once the corresponding server has been built, which is why `Initialize` (step 2) is the
place to call them. HTTP middleware is applied outermost-first in registration order, so
the first middleware added sees the request first (`buildHandler` in `framework/http.go`).
`bundles/prometheus` wires itself up this way; `bundles/jwt` does not — its interceptors
and middleware are exposed for the caller to pass in explicitly.

## Tests

Tests are co-located `_test.go` files beside the code: `framework/app_test.go`,
`framework/app_lifecycle_test.go`, `framework/app_http_test.go`,
`framework/app_middleware_test.go`, `framework/http_test.go`,
`framework/http_stream_test.go`, `framework/shutdown_test.go`, `health/check_test.go`,
`health/registry_test.go`, `health/status_test.go`, `config/base_test.go`,
`forgeerrors/errors_test.go`, `testutil/testutil_test.go`, and a bundle test beside every
bundle (`bundles/redis/bundle_test.go` and so on).

**No file in this repo carries a build tag.** `go test ./...` runs everything there is.
`TESTING.md` and `docker-compose.test.yml` describe an intended integration suite that
does not exist yet — do not assume a `-tags=integration` run exercises anything.

`scripts/gate.sh` is the merge gate, and it is what CI and the escapement kernel both
run: `gofmt -l`, `go vet ./...`, `go build ./...`, `go test -race -count=1` with a **70%
total coverage floor**, then a `go build` inside each `examples/*/` module. Run it before
you call a change done.
