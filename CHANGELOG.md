# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

[0.1.0]: https://github.com/datariot/forge/releases/tag/v0.1.0
