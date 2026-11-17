# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
follows [Semantic Versioning](https://semver.org/).

## [0.1.0] - 2026-11-17

### Added

- `Client` with `Users`, `Posts`, `Comments`, `Albums`, `Photos` and `Todos` services, each with `Get`, `List` and `All`.
- `Page[T]` results carrying `Total` from `X-Total-Count`, and `All` iterators built on `iter.Seq2`.
- `APIError` with status and request id (Cloudflare `cf-ray`), `IsNotFound`, `IsRateLimited`, `ErrInvalidArgument`, `ErrInvalidResponse`, `ErrTooManyPages`.
- Retries with jittered exponential backoff and `Retry-After` support, bounded by `MaxRetries`.
- Options: `WithBaseURL`, `WithHTTPClient`, `WithLogger`, `WithUserAgent`, `WithRetry`, `WithRequestHook`.
