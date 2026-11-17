package jsonplaceholder_test

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	jp "github.com/ocrosby/jsonplaceholder-sdk"
)

const (
	opSpanGetPost = "jsonplaceholder.posts.get"
	durationName  = "jsonplaceholder.client.operation_duration"
	retriesName   = "jsonplaceholder.client.retries"
)

// telemetryHarness wires a client to in-memory trace and metric readers.
type telemetryHarness struct {
	client *jp.Client
	spans  *tracetest.SpanRecorder
	reader *sdkmetric.ManualReader
	tp     *sdktrace.TracerProvider
}

func newTelemetryClient(t *testing.T, h http.HandlerFunc, opts ...jp.Option) *telemetryHarness {
	t.Helper()
	spans := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	all := append([]jp.Option{
		jp.WithTracerProvider(tp), jp.WithMeterProvider(mp), jp.WithPropagators(propagation.TraceContext{}),
	}, opts...)
	return &telemetryHarness{client: newServer(t, h, all...), spans: spans, reader: reader, tp: tp}
}

func (h *telemetryHarness) metric(t *testing.T, name string) (metricdata.Metrics, bool) {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := h.reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return m, true
			}
		}
	}
	return metricdata.Metrics{}, false
}

// opSpan returns the operation span for Posts.Get, failing the test if absent.
func (h *telemetryHarness) opSpan(t *testing.T) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, s := range h.spans.Ended() {
		if s.Name() == opSpanGetPost {
			return s
		}
	}
	t.Fatalf("no span named %q among %v", opSpanGetPost, spanNames(h.spans.Ended()))
	return nil
}

func spanNames(spans []sdktrace.ReadOnlySpan) []string {
	out := make([]string, len(spans))
	for i, s := range spans {
		out[i] = s.Name()
	}
	return out
}

func attrOf(attrs []attribute.KeyValue, key string) (attribute.Value, bool) {
	for _, kv := range attrs {
		if string(kv.Key) == key {
			return kv.Value, true
		}
	}
	return attribute.Value{}, false
}

func okPost(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"id":7}`)) }

func TestOperationSpanDescribesTheCallWithThePortfolioVocabulary(t *testing.T) {
	h := newTelemetryClient(t, okPost)
	if _, err := h.client.Posts.Get(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	op := h.opSpan(t)
	if op.SpanKind() != trace.SpanKindClient || op.Status().Code == codes.Error {
		t.Errorf("kind=%v status=%v", op.SpanKind(), op.Status())
	}
	want := map[string]string{
		"resource":        "endpoint:GET /posts/{id}",
		"resource.kind":   "endpoint",
		"resource.id":     "GET /posts/{id}",
		"resource.parent": "jsonplaceholder",
		"resource.path":   "jsonplaceholder/endpoint/GET /posts/{id}",
	}
	for k, v := range want {
		if got, ok := attrOf(op.Attributes(), k); !ok || got.AsString() != v {
			t.Errorf("attribute %s = %v (present=%v), want %q", k, got.String(), ok, v)
		}
	}
	if _, ok := attrOf(op.Attributes(), "error.type"); ok {
		t.Error("error.type must be absent on success")
	}
}

func TestEachHTTPAttemptIsAChildOfTheOperationSpan(t *testing.T) {
	h := newTelemetryClient(t, okPost)
	_, _ = h.client.Posts.Get(context.Background(), 7)
	op := h.opSpan(t)
	var children int
	for _, s := range h.spans.Ended() {
		if s.Name() != opSpanGetPost && s.Parent().SpanID() == op.SpanContext().SpanID() {
			children++
			if s.SpanContext().TraceID() != op.SpanContext().TraceID() || s.SpanKind() != trace.SpanKindClient {
				t.Errorf("attempt span %q: trace=%v kind=%v", s.Name(), s.SpanContext().TraceID(), s.SpanKind())
			}
		}
	}
	if children != 1 {
		t.Errorf("%d attempt spans under the operation, want 1; spans = %v", children, spanNames(h.spans.Ended()))
	}
}

func TestFailureMarksTheSpanAndRecordsTheErrorType(t *testing.T) {
	h := newTelemetryClient(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })
	_, _ = h.client.Posts.Get(context.Background(), 7)
	op := h.opSpan(t)
	if op.Status().Code != codes.Error {
		t.Errorf("status = %v, want Error", op.Status())
	}
	if v, ok := attrOf(op.Attributes(), "error.type"); !ok || v.AsString() != "404" {
		t.Errorf("error.type = %v", v.String())
	}
	if v, ok := attrOf(op.Attributes(), "http.response.status_code"); !ok || v.AsInt64() != 404 {
		t.Errorf("http.response.status_code = %v", v.String())
	}
	var recorded bool
	for _, e := range op.Events() {
		recorded = recorded || e.Name == "exception"
	}
	if !recorded {
		t.Error("the error was not recorded on the span")
	}
}

func TestOperationDurationIsRecordedOncePerOperation(t *testing.T) {
	var attempts atomic.Int32
	h := newTelemetryClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		okPost(w, nil)
	})
	if _, err := h.client.Posts.Get(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	m, ok := h.metric(t, durationName)
	if !ok {
		t.Fatal("operation duration metric missing")
	}
	hist, isHist := m.Data.(metricdata.Histogram[float64])
	if !isHist || m.Unit != "s" || len(hist.DataPoints) != 1 || hist.DataPoints[0].Count != 1 {
		t.Fatalf("metric = %+v, want one point with count 1 in seconds (two attempts, one operation)", m)
	}
	if v, ok := attrOf(hist.DataPoints[0].Attributes.ToSlice(), "resource.id"); !ok || v.AsString() != "GET /posts/{id}" {
		t.Errorf("resource.id = %v", v.String())
	}
	if _, ok := attrOf(hist.DataPoints[0].Attributes.ToSlice(), "error.type"); ok {
		t.Error("a retried-then-successful operation must not carry error.type")
	}
}

func TestRetriesAreCountedAndEventedOnTheSpan(t *testing.T) {
	var attempts atomic.Int32
	h := newTelemetryClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		okPost(w, nil)
	})
	_, _ = h.client.Posts.Get(context.Background(), 7)

	m, ok := h.metric(t, retriesName)
	if !ok {
		t.Fatal("retries counter missing")
	}
	sum, isSum := m.Data.(metricdata.Sum[int64])
	if !isSum || len(sum.DataPoints) != 1 || sum.DataPoints[0].Value != 1 {
		t.Fatalf("retries = %+v, want one retry", m.Data)
	}
	if v, ok := attrOf(sum.DataPoints[0].Attributes.ToSlice(), "error.type"); !ok || v.AsString() != "503" {
		t.Errorf("retry error.type = %v", v.String())
	}
	var found bool
	for _, e := range h.opSpan(t).Events() {
		if e.Name == "retry" {
			found = true
			if v, ok := attrOf(e.Attributes, "retry.attempt"); !ok || v.AsInt64() != 1 {
				t.Errorf("retry.attempt = %v", v.String())
			}
		}
	}
	if !found {
		t.Error("no retry event on the operation span")
	}
}

func TestTraceContextIsPropagatedToTheService(t *testing.T) {
	var traceparent atomic.Value
	h := newTelemetryClient(t, func(w http.ResponseWriter, r *http.Request) {
		traceparent.Store(r.Header.Get("traceparent"))
		okPost(w, r)
	})
	ctx, parent := h.tp.Tracer("caller").Start(context.Background(), "caller-span")
	if _, err := h.client.Posts.Get(ctx, 7); err != nil {
		t.Fatal(err)
	}
	parent.End()
	got, _ := traceparent.Load().(string)
	if !strings.Contains(got, parent.SpanContext().TraceID().String()) {
		t.Errorf("traceparent %q does not carry the caller's trace id %s", got, parent.SpanContext().TraceID())
	}
	if op := h.opSpan(t); op.Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Error("the operation span is not a child of the caller's span")
	}
}

func TestWithoutTelemetryEmitsAndInjectsNothing(t *testing.T) {
	var traceparent atomic.Value
	h := newTelemetryClient(t, func(w http.ResponseWriter, r *http.Request) {
		traceparent.Store(r.Header.Get("traceparent"))
		okPost(w, r)
	}, jp.WithoutTelemetry())
	ctx, parent := h.tp.Tracer("caller").Start(context.Background(), "caller-span")
	_, _ = h.client.Posts.Get(ctx, 7)
	parent.End()
	if got, _ := traceparent.Load().(string); got != "" {
		t.Errorf("traceparent = %q, want none", got)
	}
	for _, s := range h.spans.Ended() {
		if s.Name() != "caller-span" {
			t.Errorf("unexpected span %q", s.Name())
		}
	}
	if _, ok := h.metric(t, durationName); ok {
		t.Error("metrics were recorded with telemetry disabled")
	}
}

func TestGlobalProvidersAreUsedByDefault(t *testing.T) { // mutates process-wide state, so no t.Parallel
	spans := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans)))
	t.Cleanup(func() { otel.SetTracerProvider(noop.NewTracerProvider()) })

	c := newServer(t, okPost) // no provider options at all
	if _, err := c.Posts.Get(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, s := range spans.Ended() {
		found = found || s.Name() == opSpanGetPost
	}
	if !found {
		t.Errorf("no operation span reached the global provider; got %v", spanNames(spans.Ended()))
	}
}

// ctxCapturingHandler records the context every log record is handled with.
type ctxCapturingHandler struct {
	mu   *sync.Mutex
	ctxs *[]context.Context
}

func (h ctxCapturingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h ctxCapturingHandler) Handle(ctx context.Context, _ slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	*h.ctxs = append(*h.ctxs, ctx)
	return nil
}
func (h ctxCapturingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h ctxCapturingHandler) WithGroup(string) slog.Handler      { return h }

func TestRetryLogsCarryTheSpanContext(t *testing.T) {
	var attempts atomic.Int32
	var mu sync.Mutex
	var ctxs []context.Context
	logger := slog.New(ctxCapturingHandler{mu: &mu, ctxs: &ctxs})
	h := newTelemetryClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		okPost(w, nil)
	}, jp.WithLogger(logger), jp.WithRetry(2, time.Millisecond))
	_, _ = h.client.Posts.Get(context.Background(), 7)

	mu.Lock()
	defer mu.Unlock()
	if len(ctxs) == 0 {
		t.Fatal("no retry log record")
	}
	if !trace.SpanContextFromContext(ctxs[0]).IsValid() {
		t.Error("the retry log record was not handled with the span's context, so a slog handler cannot add trace_id")
	}
}
