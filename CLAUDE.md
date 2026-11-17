# jsonplaceholder-sdk

Go SDK for jsonplaceholder.typicode.com. A reference implementation of the design in the SDK series on omarcrosby.com (anatomy, rubric, capability-first). Stdlib only: do not add dependencies.

## Layout

One package, `jsonplaceholder`, at the repo root: `client.go` (transport, retry loop), `options.go`, `errors.go`, `pagination.go` (`Page[T]`, `all`), one file per resource, `models.go`. `examples/` are runnable programs against the live service.

## Conventions

- Every request method takes `ctx` first. Every loop has a named bound (`MaxRetries`, `MaxPages`).
- Failures are typed: `*APIError` (inspect with `errors.As`), `ErrInvalidArgument` (checked before any request), `ErrTooManyPages`. Transport errors are plain wrapped errors, never `*APIError`.
- Zero `ListOptions` sends no paging, and the service then returns every record (5000 photos). That is faithful to the service; document it, do not paper over it.
- The service is read-only fake data, so there are no write methods. Do not add them.
- Public API changes follow semver and need a `CHANGELOG.md` entry. A new resource field, filter or option is additive; renaming anything exported is breaking.

## Commands

`make test` · `make lint` · `make vuln` · `make examples` (hits the live service)
