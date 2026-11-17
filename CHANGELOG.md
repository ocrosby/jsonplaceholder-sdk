# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
follows [Semantic Versioning](https://semver.org/).

## [0.3.0] - 2026-11-17

### Added

- OpenTelemetry instrumentation, on by default and API-only: a client span per call (`jsonplaceholder.<resource>.<operation>`) with `retry` events and child HTTP spans per attempt, `jsonplaceholder.client.operation_duration` and `jsonplaceholder.client.retries` metrics, the `otelhttp` metrics, and W3C trace-context propagation.
- Portfolio `resource.*` attributes (go-platform ADR-0019) on spans and metrics.
- Options `WithTracerProvider`, `WithMeterProvider`, `WithPropagators` and `WithoutTelemetry`.

### Changed

- Retry log lines use `DebugContext` so slog handlers can add `trace_id` and `span_id`.
- The SDK now also depends on the OpenTelemetry API and `otelhttp`.

## [0.2.0] - 2026-11-17

### Added

- `*APIError.Unwrap` and the SDK sentinel errors now carry a go-platform `*apperrors.AppError`, so `apperrors.IsNotFound(err)` and `httputil.WriteError` work on SDK errors.

### Changed

- `ErrInvalidArgument`, `ErrInvalidResponse` and `ErrTooManyPages` are now `*apperrors.AppError` values (still `error`, still matched with `errors.Is`); their `Error()` text gains a `CODE: ` prefix.
- The SDK now depends on go-platform's `apperrors` (previously stdlib only).

## [0.1.0] - 2026-11-17

### Added

- `Client` with `Users`, `Posts`, `Comments`, `Albums`, `Photos` and `Todos` services, each with `Get`, `List` and `All`.
- `Page[T]` results carrying `Total` from `X-Total-Count`, and `All` iterators built on `iter.Seq2`.
- `APIError` with status and request id (Cloudflare `cf-ray`), `IsNotFound`, `IsRateLimited`, `ErrInvalidArgument`, `ErrInvalidResponse`, `ErrTooManyPages`.
- Retries with jittered exponential backoff and `Retry-After` support, bounded by `MaxRetries`.
- Options: `WithBaseURL`, `WithHTTPClient`, `WithLogger`, `WithUserAgent`, `WithRetry`, `WithRequestHook`.
