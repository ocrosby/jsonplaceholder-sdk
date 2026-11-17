package jsonplaceholder_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	jp "github.com/ocrosby/jsonplaceholder-sdk"
)

// newServer starts a test server and returns a client pointed at it with fast retries.
func newServer(t *testing.T, h http.HandlerFunc, opts ...jp.Option) *jp.Client {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	all := append([]jp.Option{jp.WithBaseURL(ts.URL), jp.WithRetry(2, time.Millisecond)}, opts...)
	c, err := jp.NewClient(all...)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func TestNewClientRejectsBadOptions(t *testing.T) {
	cases := map[string]jp.Option{
		"empty base url":  jp.WithBaseURL(""),
		"non-http scheme": jp.WithBaseURL("ftp://example.test"),
		"nil http client": jp.WithHTTPClient(nil),
		"negative retry":  jp.WithRetry(-1, time.Millisecond),
		"retry above cap": jp.WithRetry(jp.MaxRetries+1, time.Millisecond),
		"nil logger":      jp.WithLogger(nil),
	}
	for name, opt := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := jp.NewClient(opt); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestDefaultsTargetTheRealService(t *testing.T) {
	c, err := jp.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	if c.BaseURL() != "https://jsonplaceholder.typicode.com" {
		t.Errorf("BaseURL = %q", c.BaseURL())
	}
	c2, _ := jp.NewClient(jp.WithBaseURL("http://localhost:9/"))
	if c2.BaseURL() != "http://localhost:9" {
		t.Errorf("trailing slash not trimmed: %q", c2.BaseURL())
	}
}

func TestGetDecodesAndSendsIdentity(t *testing.T) {
	var gotPath, gotUA, gotAccept string
	c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotUA, gotAccept = r.URL.Path, r.UserAgent(), r.Header.Get("Accept")
		_, _ = w.Write([]byte(`{"userId":1,"id":7,"title":"t","body":"b"}`))
	}, jp.WithUserAgent("my-app/2.0"))
	p, err := c.Posts.Get(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/posts/7" || gotUA != "my-app/2.0" || gotAccept != "application/json" {
		t.Errorf("path=%q ua=%q accept=%q", gotPath, gotUA, gotAccept)
	}
	if p.ID != 7 || p.UserID != 1 || p.Title != "t" || p.Body != "b" {
		t.Errorf("decoded %+v", p)
	}
}

func TestEveryResourceGetsFromItsOwnPath(t *testing.T) {
	var path string
	c := newServer(t, func(w http.ResponseWriter, r *http.Request) { path = r.URL.Path; _, _ = w.Write([]byte(`{"id":3}`)) })
	ctx := context.Background()
	calls := map[string]func() error{
		"/users/3":    func() error { _, e := c.Users.Get(ctx, 3); return e },
		"/posts/3":    func() error { _, e := c.Posts.Get(ctx, 3); return e },
		"/comments/3": func() error { _, e := c.Comments.Get(ctx, 3); return e },
		"/albums/3":   func() error { _, e := c.Albums.Get(ctx, 3); return e },
		"/photos/3":   func() error { _, e := c.Photos.Get(ctx, 3); return e },
		"/todos/3":    func() error { _, e := c.Todos.Get(ctx, 3); return e },
	}
	for want, call := range calls {
		if err := call(); err != nil || path != want {
			t.Errorf("want path %s, got %q (err %v)", want, path, err)
		}
	}
}

func TestInvalidIDIsRejectedBeforeAnyRequest(t *testing.T) {
	var hits atomic.Int32
	c := newServer(t, func(http.ResponseWriter, *http.Request) { hits.Add(1) })
	for _, id := range []int{0, -1} {
		_, err := c.Todos.Get(context.Background(), id)
		if !errors.Is(err, jp.ErrInvalidArgument) {
			t.Errorf("id %d: err = %v, want ErrInvalidArgument", id, err)
		}
	}
	if hits.Load() != 0 {
		t.Errorf("%d requests were made", hits.Load())
	}
}

func TestNotFoundIsTypedAndCarriesRequestID(t *testing.T) {
	c := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("cf-ray", "abc123-ATL")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{}`))
	})
	_, err := c.Posts.Get(context.Background(), 99999)
	if !jp.IsNotFound(err) {
		t.Fatalf("IsNotFound(%v) = false", err)
	}
	var apiErr *jp.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 404 || apiErr.RequestID != "abc123-ATL" {
		t.Fatalf("APIError = %+v", apiErr)
	}
	if !strings.Contains(err.Error(), "abc123-ATL") || !strings.Contains(err.Error(), "404") {
		t.Errorf("Error() = %q, want status and request id", err.Error())
	}
}

func TestListBuildsQueryAndReportsTotal(t *testing.T) {
	var q string
	c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		q = r.URL.RawQuery
		w.Header().Set("X-Total-Count", "100")
		_, _ = w.Write([]byte(`[{"userId":2,"id":11,"title":"a","body":"b"}]`))
	})
	page, err := c.Posts.List(context.Background(), jp.PostListParams{UserID: jp.Int(2), ListOptions: jp.ListOptions{Page: 3, Limit: 5}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"userId=2", "_page=3", "_limit=5"} {
		if !strings.Contains(q, want) {
			t.Errorf("query %q lacks %s", q, want)
		}
	}
	if len(page.Items) != 1 || page.Items[0].ID != 11 || page.Total != 100 || page.Page != 3 || page.Limit != 5 {
		t.Errorf("page = %+v", page)
	}
}

func TestZeroOptionsSendNoPagingAndUnknownTotalIsMinusOne(t *testing.T) {
	var q string
	c := newServer(t, func(w http.ResponseWriter, r *http.Request) { q = r.URL.RawQuery; _, _ = w.Write([]byte(`[]`)) })
	page, err := c.Users.List(context.Background(), jp.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if q != "" || page.Total != -1 {
		t.Errorf("query=%q total=%d, want empty and -1", q, page.Total)
	}
}

func TestTodoFilters(t *testing.T) {
	var q string
	c := newServer(t, func(w http.ResponseWriter, r *http.Request) { q = r.URL.RawQuery; _, _ = w.Write([]byte(`[]`)) })
	_, err := c.Todos.List(context.Background(), jp.TodoListParams{UserID: jp.Int(4), Completed: jp.Bool(false)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q, "userId=4") || !strings.Contains(q, "completed=false") {
		t.Errorf("query = %q", q)
	}
}

func TestFilterIDsMustBePositive(t *testing.T) {
	c := newServer(t, func(http.ResponseWriter, *http.Request) { t.Error("request made") })
	_, err := c.Comments.List(context.Background(), jp.CommentListParams{PostID: jp.Int(0)})
	if !errors.Is(err, jp.ErrInvalidArgument) {
		t.Errorf("err = %v", err)
	}
	_, err = c.Posts.List(context.Background(), jp.PostListParams{ListOptions: jp.ListOptions{Limit: -1}})
	if !errors.Is(err, jp.ErrInvalidArgument) {
		t.Errorf("negative limit: err = %v", err)
	}
}

func TestAllWalksEveryPageAndStopsOnShortPage(t *testing.T) {
	var pages atomic.Int32
	c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		pages.Add(1)
		n := map[string]int{"1": 3, "2": 3, "3": 1}[r.URL.Query().Get("_page")]
		_, _ = w.Write([]byte(`[` + strings.TrimSuffix(strings.Repeat(`{"id":1},`, n), ",") + `]`))
	})
	count := 0
	for _, err := range c.Posts.All(context.Background(), jp.PostListParams{ListOptions: jp.ListOptions{Limit: 3}}) {
		if err != nil {
			t.Fatal(err)
		}
		count++
	}
	if count != 7 || pages.Load() != 3 {
		t.Errorf("items=%d pages=%d, want 7 and 3", count, pages.Load())
	}
}

func TestAllStopsFetchingWhenCallerBreaks(t *testing.T) {
	var pages atomic.Int32
	c := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		pages.Add(1)
		_, _ = w.Write([]byte(`[{"id":1},{"id":2}]`))
	})
	for range c.Posts.All(context.Background(), jp.PostListParams{ListOptions: jp.ListOptions{Limit: 2}}) {
		break
	}
	if pages.Load() != 1 {
		t.Errorf("fetched %d pages after break, want 1", pages.Load())
	}
}

func TestAllYieldsTheErrorAndStops(t *testing.T) {
	c := newServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(404) })
	var got error
	n := 0
	for _, err := range c.Albums.All(context.Background(), jp.AlbumListParams{}) {
		n++
		got = err
	}
	if n != 1 || !jp.IsNotFound(got) {
		t.Errorf("iterations=%d err=%v", n, got)
	}
}

func TestRetriesTransientFailuresThenSucceeds(t *testing.T) {
	var attempts atomic.Int32
	c := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"id":1}`))
	})
	if _, err := c.Posts.Get(context.Background(), 1); err != nil {
		t.Fatalf("expected success after retries: %v", err)
	}
	if attempts.Load() != 3 {
		t.Errorf("attempts = %d, want 3", attempts.Load())
	}
}

func TestRetryBudgetIsBounded(t *testing.T) {
	var attempts atomic.Int32
	c := newServer(t, func(w http.ResponseWriter, _ *http.Request) { attempts.Add(1); w.WriteHeader(502) })
	_, err := c.Posts.Get(context.Background(), 1)
	var apiErr *jp.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 502 {
		t.Fatalf("err = %v", err)
	}
	if attempts.Load() != 3 { // 1 try + 2 retries
		t.Errorf("attempts = %d, want 3", attempts.Load())
	}
}

func TestPermanentErrorsAreNotRetried(t *testing.T) {
	var attempts atomic.Int32
	c := newServer(t, func(w http.ResponseWriter, _ *http.Request) { attempts.Add(1); w.WriteHeader(404) })
	_, _ = c.Posts.Get(context.Background(), 1)
	if attempts.Load() != 1 {
		t.Errorf("attempts = %d, want 1", attempts.Load())
	}
}

func TestRetryDisabledMakesOneAttempt(t *testing.T) {
	var attempts atomic.Int32
	c := newServer(t, func(w http.ResponseWriter, _ *http.Request) { attempts.Add(1); w.WriteHeader(500) }, jp.WithRetry(0, time.Millisecond))
	_, _ = c.Posts.Get(context.Background(), 1)
	if attempts.Load() != 1 {
		t.Errorf("attempts = %d, want 1", attempts.Load())
	}
}

func TestCancellingDuringBackoffReturnsPromptly(t *testing.T) {
	var attempts atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	c := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		time.AfterFunc(50*time.Millisecond, cancel) // cancel while the client is sleeping before its retry
		w.WriteHeader(http.StatusServiceUnavailable)
	}, jp.WithRetry(5, time.Minute)) // the sleep would last seconds if it were not interruptible
	start := time.Now()
	_, err := c.Posts.Get(ctx, 1)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("took %v to notice the cancellation", elapsed)
	}
	if attempts.Load() != 1 {
		t.Errorf("attempts = %d, want 1", attempts.Load())
	}
}

func TestTransportErrorsAreNotAPIErrors(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	url := ts.URL
	ts.Close()
	c, _ := jp.NewClient(jp.WithBaseURL(url), jp.WithRetry(0, time.Millisecond))
	_, err := c.Posts.Get(context.Background(), 1)
	var apiErr *jp.APIError
	if err == nil || errors.As(err, &apiErr) {
		t.Errorf("err = %v; a refused connection must not look like an API response", err)
	}
}

func TestUndecodableBodyIsAnError(t *testing.T) {
	c := newServer(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`<html>`)) })
	if _, err := c.Posts.Get(context.Background(), 1); err == nil {
		t.Error("expected a decode error")
	}
}

func TestRequestHookSeesEveryAttempt(t *testing.T) {
	var seen []string
	var attempts atomic.Int32
	c := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(500)
			return
		}
		_, _ = w.Write([]byte(`{"id":1}`))
	}, jp.WithRequestHook(func(_ context.Context, method, url string) { seen = append(seen, method+" "+url) }))
	if _, err := c.Posts.Get(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || !strings.HasPrefix(seen[0], "GET http://") || !strings.HasSuffix(seen[0], "/posts/1") {
		t.Errorf("hook saw %v", seen)
	}
}

func TestInjectedHTTPClientIsUsed(t *testing.T) {
	var used atomic.Bool
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		used.Store(true)
		return http.DefaultTransport.RoundTrip(r)
	})
	c := newServer(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"id":1}`)) },
		jp.WithHTTPClient(&http.Client{Transport: rt}))
	if _, err := c.Posts.Get(context.Background(), 1); err != nil || !used.Load() {
		t.Errorf("err=%v used=%v", err, used.Load())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRetryAfterIsHonored(t *testing.T) {
	var attempts atomic.Int32
	c := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"id":1}`))
	})
	start := time.Now()
	if _, err := c.Posts.Get(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Errorf("retried after %v, want at least the 1s Retry-After", elapsed)
	}
}

func TestRateLimitedIsDetectable(t *testing.T) {
	c := newServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) }, jp.WithRetry(0, time.Millisecond))
	_, err := c.Posts.Get(context.Background(), 1)
	if !jp.IsRateLimited(err) || jp.IsNotFound(err) {
		t.Errorf("err = %v", err)
	}
}

func TestAllStopsAtReportedTotalWithoutAnExtraRequest(t *testing.T) {
	var pages atomic.Int32
	c := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		pages.Add(1)
		w.Header().Set("X-Total-Count", "4")
		_, _ = w.Write([]byte(`[{"id":1},{"id":2}]`)) // full pages of 2; total 4 means exactly two pages
	})
	n := 0
	for _, err := range c.Todos.All(context.Background(), jp.TodoListParams{ListOptions: jp.ListOptions{Limit: 2}}) {
		if err != nil {
			t.Fatal(err)
		}
		n++
	}
	if n != 4 || pages.Load() != 2 {
		t.Errorf("items=%d pages=%d, want 4 and 2", n, pages.Load())
	}
}

func TestAllIsBoundedByMaxPages(t *testing.T) {
	var pages atomic.Int32
	c := newServer(t, func(w http.ResponseWriter, _ *http.Request) { pages.Add(1); _, _ = w.Write([]byte(`[{"id":1}]`)) })
	var last error
	for _, err := range c.Photos.All(context.Background(), jp.PhotoListParams{ListOptions: jp.ListOptions{Limit: 1}}) {
		last = err
	}
	if !errors.Is(last, jp.ErrTooManyPages) || int(pages.Load()) != jp.MaxPages {
		t.Errorf("err=%v pages=%d, want ErrTooManyPages after %d", last, pages.Load(), jp.MaxPages)
	}
}

func TestEveryResourceListsFromItsOwnPath(t *testing.T) {
	var path string
	c := newServer(t, func(w http.ResponseWriter, r *http.Request) { path = r.URL.Path; _, _ = w.Write([]byte(`[]`)) })
	ctx := context.Background()
	calls := map[string]func() error{
		"/users":    func() error { _, e := c.Users.List(ctx, jp.ListOptions{}); return e },
		"/posts":    func() error { _, e := c.Posts.List(ctx, jp.PostListParams{}); return e },
		"/comments": func() error { _, e := c.Comments.List(ctx, jp.CommentListParams{PostID: jp.Int(1)}); return e },
		"/albums":   func() error { _, e := c.Albums.List(ctx, jp.AlbumListParams{UserID: jp.Int(1)}); return e },
		"/photos":   func() error { _, e := c.Photos.List(ctx, jp.PhotoListParams{AlbumID: jp.Int(1)}); return e },
		"/todos":    func() error { _, e := c.Todos.List(ctx, jp.TodoListParams{}); return e },
	}
	for want, call := range calls {
		if err := call(); err != nil || path != want {
			t.Errorf("want path %s, got %q (err %v)", want, path, err)
		}
	}
	for range c.Users.All(ctx, jp.ListOptions{}) { // empty collection ends the iteration immediately
		t.Error("expected no items")
	}
}

func TestEveryFilterValidatesItsID(t *testing.T) {
	c := newServer(t, func(http.ResponseWriter, *http.Request) { t.Error("request made") })
	ctx, zero := context.Background(), jp.Int(0)
	errs := []error{}
	_, e := c.Posts.List(ctx, jp.PostListParams{UserID: zero})
	errs = append(errs, e)
	_, e = c.Albums.List(ctx, jp.AlbumListParams{UserID: zero})
	errs = append(errs, e)
	_, e = c.Photos.List(ctx, jp.PhotoListParams{AlbumID: zero})
	errs = append(errs, e)
	_, e = c.Todos.List(ctx, jp.TodoListParams{UserID: zero})
	errs = append(errs, e)
	for i, err := range errs {
		if !errors.Is(err, jp.ErrInvalidArgument) {
			t.Errorf("case %d: err = %v", i, err)
		}
	}
}
