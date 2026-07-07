# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **framework:** `AddUnaryInterceptor`, `AddStreamInterceptor`, and `AddHTTPMiddleware` on `App` — bundles can register gRPC interceptors and HTTP middleware during `Initialize`.
- **framework:** `WithLogging`, `WithObservability`, and `WithHealthRegistry` options to inject custom infrastructure managers, plus an `Observability()` accessor.
- **prometheus:** Automatic HTTP and gRPC request metrics — adding the bundle now records request metrics with no per-handler code.
- **jwt:** `StreamServerInterceptor` so streaming RPCs are authenticated (previously only unary RPCs were), and a `TrustedProxyHeader` config flag gating `X-Forwarded-Proto` trust.
- **httpclient:** Optional `AllowedHosts` allowlist (defense-in-depth SSRF guard) covering both requests and redirect targets.
- **health:** Background check runner honoring per-check `Interval`/`InitialDelay`; probe endpoints serve cached results so a slow dependency cannot stall Kubernetes probes.

### Changed (breaking)
- **API naming:** `Get`-prefixed getters renamed to drop the prefix — `Tracer`, `Meter`, `RegisteredChecks`, `CheckConfig`, `HealthySummary`, `MetricsHandler`, `SecureMetricsHandler`, `ConfigInfo`, `CircuitBreakerState`, `CircuitBreakerCounts`, and `jwt.ServiceID`/`jwt.ServiceName`. (`CredentialProvider.GetAPIKey`/`GetJWTToken` are unchanged — they are context-taking fetch operations, not accessors.)
- **health:** `HealthStatus` renamed to `Report`; constructors `NewHealthyStatus`/`NewUnhealthyStatus`/`NewUnknownStatus` renamed to `NewHealthyReport`/`NewUnhealthyReport`/`NewUnknownReport`.
- **errors package:** moved from `github.com/datariot/forge/errors` to `github.com/datariot/forge/forgeerrors` (package `forgeerrors`) so it no longer shadows the standard library `errors`.
- **postgresql:** switched the driver from `lib/pq` (maintenance mode) to `pgx/v5/stdlib`. Connection-string compatible; no config changes required.
- Health check HTTP responses no longer include raw per-check error strings (which could leak internal hostnames/ports); full detail remains in logs.
- gRPC reflection now requires an explicit `EnableReflection` flag instead of being on by default in development.

### Security
- Bumped `google.golang.org/grpc` (GO-2026-4762, authorization bypass) and `go.opentelemetry.io/otel/sdk` (GO-2026-4394, code execution); `govulncheck` reports no vulnerabilities affecting the code.
- **httpclient:** auth headers are stripped on cross-host redirects.
- Opt-in HTTP Basic Auth for the `/metrics` endpoint; pprof is now blocked outside development.

### Migration from 0.1.0

- Replace imports of `github.com/datariot/forge/errors` with `github.com/datariot/forge/forgeerrors` and update references from `errors.X` to `forgeerrors.X`.
- Drop the `Get` prefix from the renamed getters listed above.
- Replace `health.HealthStatus` with `health.Report` and the three `New*Status` constructors with `New*Report`.
- No action needed for the pgx driver swap unless you referenced the `"postgres"` driver name directly.

## [0.1.0] - 2026-02-25

### Added
- Optional gRPC support: framework runs in HTTP-only mode when gRPC is not configured
- OTEL metrics exporter with dual OTLP+Prometheus support
- `testutil` package with common testing helpers for framework consumers

### Fixed
- **httpclient:** Race condition in backoff configuration during concurrent client initialization
- **jwt:** Goroutine leak when JWT validation failed mid-request; goroutines now properly cleaned up on context cancellation
- **framework:** Incorrect shutdown ordering caused components to stop before their dependencies; ordering is now deterministic
- **errors:** `DomainError.Is()` now correctly implements error matching for use with `errors.Is()`
- **framework:** `framework.New()` now validates required config fields and returns an error instead of panicking
- **configloader:** Relative paths are now accepted and resolved against the working directory

### Changed
- All `fmt.Printf` and `fmt.Println` calls replaced with structured zerolog logging
- JWT regex patterns compiled once at package initialization instead of per-request

### Testing
- Overall coverage raised from 29.6% to 70.0%
- Added unit tests across all packages: errors (100%), health (96.5%), prometheus (76.4%), httpclient (75.9%), config (73.8%), testutil (71.1%), framework (70.0%), configloader (66.7%), jwt (62.0%), redis (36.0%), postgresql (30.6%)

[Unreleased]: https://github.com/datariot/forge/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/datariot/forge/releases/tag/v0.1.0
