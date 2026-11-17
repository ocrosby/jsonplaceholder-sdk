# jsonplaceholder-sdk

A Go SDK for [JSONPlaceholder](https://jsonplaceholder.typicode.com), the free fake REST API. It is a reference implementation of the design in four posts on [omarcrosby.com](https://omarcrosby.com): small enough to read in one sitting, and complete enough to show every part of an SDK's surface working together. Its one dependency is [go-platform](https://github.com/jedi-knights/go-platform)'s zero-dependency `apperrors` package, so its errors work with services built on go-platform.

<p align="center">
  <a href="#install">Install</a> · <a href="#quick-start">Quick start</a> · <a href="#resources">Resources</a> · <a href="#paging">Paging</a> · <a href="#errors">Errors</a> · <a href="#retries-and-instrumentation">Retries and instrumentation</a> · <a href="#design">Design</a> · <a href="#development">Development</a>
</p>

## Install

Requires Go 1.26 or later.

```bash
go get github.com/ocrosby/jsonplaceholder-sdk
```

## Quick start

JSONPlaceholder needs no credentials, so there is nothing to configure:

```go
client, err := jsonplaceholder.NewClient()
if err != nil {
	return err
}

post, err := client.Posts.Get(ctx, 1)
if err != nil {
	return err
}
fmt.Println(post.Title)
```

Run the examples against the live service with `make examples`, or read them in [`examples/`](examples/).

## Resources

Each resource is a field on the client with the same three methods. The method you want is one autocomplete away.

| Resource | Records | `Get` | `List` and `All` filters |
|---|---|---|---|
| [`client.Users`](https://jsonplaceholder.typicode.com/users) | 10 | `Get(ctx, id)` | none |
| [`client.Posts`](https://jsonplaceholder.typicode.com/posts) | 100 | `Get(ctx, id)` | `UserID` |
| [`client.Comments`](https://jsonplaceholder.typicode.com/comments) | 500 | `Get(ctx, id)` | `PostID` |
| [`client.Albums`](https://jsonplaceholder.typicode.com/albums) | 100 | `Get(ctx, id)` | `UserID` |
| [`client.Photos`](https://jsonplaceholder.typicode.com/photos) | 5000 | `Get(ctx, id)` | `AlbumID` |
| [`client.Todos`](https://jsonplaceholder.typicode.com/todos) | 200 | `Get(ctx, id)` | `UserID`, `Completed` |

Optional filters are pointers, so "not set" is distinct from zero. Use `jsonplaceholder.Int(2)` and `jsonplaceholder.Bool(false)` to build them. The upstream nested routes such as `/posts/1/comments` are the same data as a filter (`CommentListParams{PostID: Int(1)}`), so they have no separate methods.

The service fakes writes and persists nothing, so the SDK is read-only.

## Paging

`List` returns one page. `All` returns an iterator over every record and fetches pages only as you consume them:

```go
// One page, with the size of the whole collection.
page, err := client.Photos.List(ctx, jsonplaceholder.PhotoListParams{
	AlbumID:     jsonplaceholder.Int(3),
	ListOptions: jsonplaceholder.ListOptions{Page: 2, Limit: 10},
})
fmt.Println(len(page.Items), "of", page.Total) // 10 of 50

// Every record, however many pages it takes. break stops fetching.
for photo, err := range client.Photos.All(ctx, jsonplaceholder.PhotoListParams{AlbumID: jsonplaceholder.Int(3)}) {
	if err != nil {
		return err
	}
	fmt.Println(photo.Title)
}
```

- **Zero `ListOptions` sends no paging parameters, and the service then returns every record.** For photos that is 5000. Set `Limit`, or filter, unless you want that.
- `Page.Total` comes from the service's `X-Total-Count` header, or is `-1` when the service did not report it. `All` uses it to stop without a final empty request.
- `All` fetches 100 per page unless you set `Limit`, and never more than `MaxPages` (1000) pages.

## Errors

Failures are typed values, so you branch on them instead of parsing strings:

```go
_, err := client.Posts.Get(ctx, 99999)

if jsonplaceholder.IsNotFound(err) {
	// no such post
}

var apiErr *jsonplaceholder.APIError
if errors.As(err, &apiErr) {
	log.Printf("%d %s, quote %s to support", apiErr.StatusCode, apiErr.Message, apiErr.RequestID)
}
```

| Error | When | Check with |
|---|---|---|
| `*APIError` | The service answered with a non-200 status | `errors.As`, `IsNotFound`, `IsRateLimited` |
| `ErrInvalidArgument` | A non-positive id or filter, or a negative page or limit. No request is made. | `errors.Is` |
| `ErrTooManyPages` | An `All` iterator hit `MaxPages` | `errors.Is` |
| `ErrInvalidResponse` | The service answered 200 with a body that is not the expected JSON. Not retried. | `errors.Is` |
| a wrapped network error | The request never reached the service | `errors.Is(err, context.Canceled)` and so on |

JSONPlaceholder sends no request-id header, so `APIError.RequestID` is Cloudflare's `cf-ray` value, which is what a support request can quote.

### Errors that work with go-platform

SDK errors carry a go-platform `*apperrors.AppError` in their chain, derived from the HTTP status, so `apperrors.IsNotFound(err)` and `httputil.WriteError(w, err)` work on them without translation. The `*APIError` stays reachable with `errors.As`.

| Status or error | `apperrors` code |
|---|---|
| 404 | `NOT_FOUND` |
| 429 | `RATE_LIMIT` |
| 401, 403, 409 | `UNAUTHORIZED`, `FORBIDDEN`, `CONFLICT` |
| any other 4xx, `ErrInvalidArgument` | `BAD_REQUEST` |
| 502, 503, 504, `ErrInvalidResponse` | `UNAVAILABLE` |
| any other 5xx, `ErrTooManyPages` | `INTERNAL` |

## Retries and instrumentation

By default a transient failure (HTTP 429, 500, 502, 503, 504, or a network error) is retried twice with jittered exponential backoff, and a `Retry-After` header is honored up to 30 seconds. Every call takes a `context.Context`, and cancelling it interrupts a backoff sleep immediately. Permanent errors such as 404 are never retried.

```go
client, err := jsonplaceholder.NewClient(
	jsonplaceholder.WithRetry(4, 100*time.Millisecond), // 0 disables retries; the cap is MaxRetries (10)
	jsonplaceholder.WithHTTPClient(&http.Client{Timeout: 5 * time.Second}),
	jsonplaceholder.WithUserAgent("my-app/2.0"),
	jsonplaceholder.WithRequestHook(func(ctx context.Context, method, url string) {
		log.Printf("%s %s", method, url) // called before every attempt, retries included
	}),
)
```

`WithBaseURL` points the client at a local [json-server](https://github.com/typicode/json-server) or a test double.

## Design

This SDK follows the series on omarcrosby.com. The table maps each part of the [anatomy post](https://omarcrosby.com/posts/the-anatomy-of-an-sdk-in-go/) to where it lives here.

| Anatomy part | Where |
|---|---|
| Client and construction (functional options, injectable `http.Client`) | `client.go`, `options.go` |
| Resource groupings (`client.Posts.List`) | one file per resource |
| Typed requests and responses | `models.go`, `*ListParams` types |
| Typed errors with a request id | `errors.go` |
| Pagination as a first-class iterator (`iter.Seq2`) | `pagination.go` |
| Retries and instrumentation | `retry.go`, `WithRequestHook` |
| Docs, samples, changelog | `doc.go`, `examples/`, `CHANGELOG.md` |

Graded honestly against the [eight-point rubric](https://omarcrosby.com/posts/what-makes-a-good-sdk/):

- **Met:** reads native in Go, boring auth (there is none), typed errors with a request id, sensible defaults with every knob reachable, pagination as a first-class primitive, docs and runnable samples, honest versioning.
- **Not applicable:** streaming (the service has none).
- **Not met:** types are hand-written, not generated from a spec, because JSONPlaceholder publishes no OpenAPI document. With six small resources that is a fair trade; with a large API it would drift.

Two choices are deliberate. The SDK is a thin, faithful client rather than a capability layer, because the API has no multi-call workflows worth hiding (see [capability-first design](https://omarcrosby.com/posts/capability-first-sdk-design-and-where-new-work-belongs/)). And it does not hide the service's behavior of returning every record when no paging is given; it documents it.

For when an SDK is worth building at all, see [*What an SDK actually is, and when to build one*](https://omarcrosby.com/posts/what-an-sdk-actually-is-and-when-to-build-one/). By that framework this API is too small to need one, which is exactly why it makes a good, readable reference.

## Development

```bash
make test       # go test -race ./...
make lint       # golangci-lint
make vuln       # govulncheck
make examples   # runs both examples against the live service
```

## License

[MIT](LICENSE)
