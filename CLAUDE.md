# jsonplaceholder-sdk

Go SDK for jsonplaceholder.typicode.com. A reference implementation of the design in the SDK series on omarcrosby.com (anatomy, rubric, capability-first). Dependencies are deliberately minimal: the OpenTelemetry **API** (never the OTel SDK, exporters, gRPC or Prometheus in non-test code), `otelhttp`, and go-platform's zero-dependency `apperrors`. Never import go-platform's `otel` (the application bootstrap, about 240 packages) or its server-side packages from library code. Test-only code may import the OTel SDK.

## Layout

One package, `jsonplaceholder`, at the repo root: `client.go` (transport, retry loop), `options.go`, `errors.go`, `pagination.go` (`Page[T]`, `all`), one file per resource, `models.go`. `examples/` are runnable programs against the live service.

## Conventions

- Every request method takes `ctx` first. Every loop has a named bound (`MaxRetries`, `MaxPages`).
- `*APIError.Unwrap` exposes a go-platform `*AppError` (code from the HTTP status); every sentinel error is an `*AppError` too, so a new sentinel needs an `apperrors` code.
- Failures are typed: `*APIError` (inspect with `errors.As`), `ErrInvalidArgument` (checked before any request), `ErrTooManyPages`. Transport errors are plain wrapped errors, never `*APIError`.
- Zero `ListOptions` sends no paging, and the service then returns every record (5000 photos). That is faithful to the service; document it, do not paper over it.
- The service is read-only fake data, so there are no write methods. Do not add them.
- Public API changes follow semver and need a `CHANGELOG.md` entry. A new resource field, filter or option is additive; renaming anything exported is breaking.

## Telemetry (on by default, API only)

- Spans are `jsonplaceholder.<resource>.<operation>`; metrics are `jsonplaceholder.client.*`. Every operation carries the `resource.*` attributes mirrored from `go-platform/otel/attrs.go` (ADR-0019); keep them in sync by value, since importing that package is not allowed.
- `resource.id` is always a route template (`GET /posts/{id}`), never a concrete URL or id: it is a metric attribute and must stay low-cardinality.
- A new resource or method needs an `operation{resource, name, route}` passed to `Client.get`, so it is instrumented automatically.
- Log retries with `DebugContext(ctx, ...)`, never `Debug`, so slog handlers can add `trace_id`.

## Commands

`make test` · `make lint` · `make vuln` · `make examples` (hits the live service)
