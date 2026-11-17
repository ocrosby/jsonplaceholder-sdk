package jsonplaceholder

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	// DefaultBaseURL is the public JSONPlaceholder service.
	DefaultBaseURL = "https://jsonplaceholder.typicode.com"

	defaultTimeout    = 30 * time.Second
	defaultMaxRetries = 2
	defaultRetryBase  = 200 * time.Millisecond

	// maxBody caps how much of one response is read. The largest legitimate
	// response, all 5000 photos, is under 1 MiB.
	maxBody = 8 << 20
	// requestIDHeader is the closest thing the service has to a request id:
	// JSONPlaceholder sits behind Cloudflare, which stamps every response.
	requestIDHeader = "cf-ray"
)

// Client talks to JSONPlaceholder. Create one with NewClient and reuse it; it
// is safe for concurrent use. Each resource is a field, so the method you want
// is one autocomplete away: client.Posts.List, client.Users.Get, and so on.
type Client struct {
	// Users, Posts, Comments, Albums, Photos and Todos are the six resources.
	Users    *UsersService
	Posts    *PostsService
	Comments *CommentsService
	Albums   *AlbumsService
	Photos   *PhotosService
	Todos    *TodosService

	baseURL    string
	httpClient *http.Client
	logger     *slog.Logger
	userAgent  string
	maxRetries int
	retryBase  time.Duration
	hook       RequestHook

	tracerProvider trace.TracerProvider
	meterProvider  metric.MeterProvider
	propagator     propagation.TextMapPropagator
	telemetryOff   bool
	tel            *telemetry
}

// NewClient builds a Client. With no options it targets DefaultBaseURL, times
// out each attempt after 30 seconds and retries transient failures twice.
// JSONPlaceholder needs no credentials, so there is no required argument.
// It returns an error if any option is invalid.
func NewClient(opts ...Option) (*Client, error) {
	c := &Client{
		baseURL:    DefaultBaseURL,
		httpClient: &http.Client{Timeout: defaultTimeout},
		logger:     slog.Default(),
		userAgent:  "jsonplaceholder-sdk-go/" + Version,
		maxRetries: defaultMaxRetries,
		retryBase:  defaultRetryBase,
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	if err := c.initTelemetry(); err != nil {
		return nil, err
	}
	c.Users = &UsersService{c}
	c.Posts = &PostsService{c}
	c.Comments = &CommentsService{c}
	c.Albums = &AlbumsService{c}
	c.Photos = &PhotosService{c}
	c.Todos = &TodosService{c}
	return c, nil
}

// BaseURL returns the base URL requests are sent to, without a trailing slash.
func (c *Client) BaseURL() string { return c.baseURL }

// get performs one SDK call: a GET with retries, decoded into out, wrapped in
// an operation span. It returns the final response headers. The retry loop is
// bounded by c.maxRetries, which WithRetry caps at MaxRetries.
func (c *Client) get(ctx context.Context, op operation, path string, q url.Values, out any) (http.Header, error) {
	ctx, finish := c.tel.start(ctx, op)
	hdr, err := c.getWithRetry(ctx, op, path, q, out)
	finish(err)
	return hdr, err
}

// getWithRetry is the retry loop behind get.
func (c *Client) getWithRetry(ctx context.Context, op operation, path string, q url.Values, out any) (http.Header, error) {
	target := c.baseURL + path
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	for attempt := 0; ; attempt++ {
		hdr, retry, retryAfter, err := c.attempt(ctx, target, out)
		if err == nil {
			return hdr, nil
		}
		if err := c.waitToRetry(ctx, op, target, attempt, retry, retryAfter, err); err != nil {
			return nil, err
		}
	}
}

// waitToRetry decides what follows a failed attempt. It returns nil after
// sleeping when the caller should try again, and the error to return when it
// should give up: the context ended, the failure is permanent, or the retry
// budget is spent.
func (c *Client) waitToRetry(ctx context.Context, op operation, target string, attempt int, retry bool, retryAfter time.Duration, cause error) error {
	if cerr := ctx.Err(); cerr != nil {
		return fmt.Errorf("jsonplaceholder: GET %s: %w", target, cerr)
	}
	if !retry || attempt >= c.maxRetries {
		return cause
	}
	delay := backoff(attempt, c.retryBase, retryAfter)
	// DebugContext, not Debug: a slog handler such as go-platform's
	// otel.SpanContextHandler reads the span from ctx to add trace_id and span_id.
	c.logger.DebugContext(ctx, "jsonplaceholder: retrying", "url", target, "attempt", attempt+1, "delay", delay, "err", cause)
	c.tel.retry(ctx, op, attempt+1, delay, cause)
	if serr := sleep(ctx, delay); serr != nil {
		return fmt.Errorf("jsonplaceholder: GET %s: %w", target, serr)
	}
	return nil
}

// attempt sends one request. retry says whether a failure is worth retrying;
// retryAfter carries the server's Retry-After, if any.
func (c *Client) attempt(ctx context.Context, target string, out any) (hdr http.Header, retry bool, retryAfter time.Duration, err error) {
	if c.hook != nil {
		c.hook(ctx, http.MethodGet, target)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, false, 0, fmt.Errorf("jsonplaceholder: building request for %s: %w", target, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, true, 0, fmt.Errorf("jsonplaceholder: GET %s: %w", target, err)
	}
	defer func() { _ = resp.Body.Close() }() // read-only body; a close error is not actionable

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10)) // drain so the connection can be reused; errors are not actionable
		return resp.Header, retryableStatus(resp.StatusCode), parseRetryAfter(resp.Header), &APIError{
			StatusCode: resp.StatusCode,
			Message:    http.StatusText(resp.StatusCode),
			RequestID:  resp.Header.Get(requestIDHeader),
			Method:     http.MethodGet,
			URL:        target,
		}
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(out); err != nil {
		return nil, false, 0, fmt.Errorf("jsonplaceholder: GET %s: %w: %w", target, ErrInvalidResponse, err)
	}
	return resp.Header, false, 0, nil
}

// getOne fetches /<collection>/<id> into a new T. what is the singular noun
// used in errors.
func getOne[T any](ctx context.Context, c *Client, collection string, id int) (*T, error) {
	if id < 1 {
		return nil, invalid("id must be 1 or greater, got %d", id)
	}
	op := operation{resource: collection, name: "get", route: "GET /" + collection + "/{id}"}
	var out T
	if _, err := c.get(ctx, op, "/"+collection+"/"+strconv.Itoa(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// setID adds key=value to q when id is set, rejecting non-positive ids.
func setID(q url.Values, key string, id *int) error {
	if id == nil {
		return nil
	}
	if *id < 1 {
		return invalid("%s must be 1 or greater, got %d", key, *id)
	}
	q.Set(key, strconv.Itoa(*id))
	return nil
}
