package jsonplaceholder

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Telemetry. The SDK is instrumented with the OpenTelemetry API only, and it
// is on by default. Until the application installs OpenTelemetry providers
// (for example with go-platform's otel.New) every call below is a no-op, so
// an uninstrumented application pays nothing. The SDK never installs providers
// or exporters itself: that is the application's decision.
//
// Signals:
//
//   - A client span per SDK call, "jsonplaceholder.<resource>.<operation>",
//     with one child HTTP span per attempt (from otelhttp) and a "retry" event
//     for each retry.
//   - Histogram jsonplaceholder.client.operation_duration (seconds), one
//     record per SDK call however many attempts it took.
//   - Counter jsonplaceholder.client.retries, one per retry.
//   - The otelhttp metrics (http.client.request.duration and friends), one
//     record per attempt, and W3C trace-context propagation to the service.
const (
	instrumentationName = "github.com/ocrosby/jsonplaceholder-sdk"

	// apiName is the "parent" of every resource this SDK reports: the API the
	// calls go to.
	apiName = "jsonplaceholder"

	metricOperationDuration = "jsonplaceholder.client.operation_duration"
	metricRetries           = "jsonplaceholder.client.retries"
)

// Attribute keys. The resource.* keys are the jedi-knights portfolio
// vocabulary defined in go-platform/otel/attrs.go (ADR-0019 resource taxonomy,
// resource_path = "<parent>/<kind>/<id>"), so these spans can be joined to
// audit and billing events by resource path. The key strings are mirrored here
// rather than imported because that package also holds the OpenTelemetry SDK
// bootstrap, which would add about 240 packages to every consumer of this
// library. The rest are OpenTelemetry semantic conventions.
const (
	attrResource       = "resource"
	attrResourceKind   = "resource.kind"
	attrResourceID     = "resource.id"
	attrResourceParent = "resource.parent"
	attrResourcePath   = "resource.path"

	attrErrorType  = "error.type"
	attrStatusCode = "http.response.status_code"
)

// resourceKindEndpoint is the ADR-0019 resource_kind for one HTTP endpoint.
const resourceKindEndpoint = "endpoint"

// durationBuckets are the explicit histogram bounds, in seconds, that the
// OpenTelemetry HTTP semantic conventions recommend for request durations.
var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10}

// operation identifies one SDK call for telemetry. route is a low-cardinality
// template such as "GET /posts/{id}", never a concrete URL.
type operation struct {
	resource string // collection, e.g. "posts"
	name     string // "get" or "list"
	route    string
}

// spanName follows the portfolio's short.snake_case convention anchored on
// the action: jsonplaceholder.posts.get.
func (o operation) spanName() string { return apiName + "." + o.resource + "." + o.name }

// attrs returns the portfolio resource attributes for the operation.
func (o operation) attrs() []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(attrResource, resourceKindEndpoint+":"+o.route),
		attribute.String(attrResourceKind, resourceKindEndpoint),
		attribute.String(attrResourceID, o.route),
		attribute.String(attrResourceParent, apiName),
		attribute.String(attrResourcePath, apiName+"/"+resourceKindEndpoint+"/"+o.route),
	}
}

// telemetry holds the instruments. A nil *telemetry is valid and does nothing,
// which is how WithoutTelemetry is implemented.
type telemetry struct {
	tracer   trace.Tracer
	duration metric.Float64Histogram
	retries  metric.Int64Counter
}

// initTelemetry builds the instruments and wraps the HTTP client's transport
// with otelhttp. It runs once, at the end of NewClient, after every option has
// been applied, so provider options may appear in any order. The caller's
// *http.Client is copied, never modified.
func (c *Client) initTelemetry() error {
	if c.telemetryOff {
		return nil
	}
	tp, mp, prop := c.tracerProvider, c.meterProvider, c.propagator
	if tp == nil {
		tp = otel.GetTracerProvider() // the global delegates, so providers installed later still apply
	}
	if mp == nil {
		mp = otel.GetMeterProvider()
	}
	if prop == nil {
		prop = otel.GetTextMapPropagator()
	}

	meter := mp.Meter(instrumentationName, metric.WithInstrumentationVersion(Version))
	duration, err := meter.Float64Histogram(metricOperationDuration,
		metric.WithUnit("s"),
		metric.WithDescription("Duration of one SDK call, including every retry attempt."),
		metric.WithExplicitBucketBoundaries(durationBuckets...))
	if err != nil {
		return err
	}
	retries, err := meter.Int64Counter(metricRetries,
		metric.WithUnit("{retry}"),
		metric.WithDescription("Retries of a transient failure."))
	if err != nil {
		return err
	}
	c.tel = &telemetry{
		tracer:   tp.Tracer(instrumentationName, trace.WithInstrumentationVersion(Version)),
		duration: duration,
		retries:  retries,
	}

	wrapped := *c.httpClient
	wrapped.Transport = otelhttp.NewTransport(c.httpClient.Transport,
		otelhttp.WithTracerProvider(tp), otelhttp.WithMeterProvider(mp), otelhttp.WithPropagators(prop))
	c.httpClient = &wrapped
	return nil
}

// start opens the operation span and returns the context to use for the call
// and a finish func that ends the span and records the duration. finish takes
// the call's final error (nil on success).
func (t *telemetry) start(ctx context.Context, op operation) (context.Context, func(error)) {
	if t == nil {
		return ctx, func(error) {}
	}
	ctx, span := t.tracer.Start(ctx, op.spanName(),
		trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(op.attrs()...))
	began := time.Now()
	return ctx, func(err error) {
		attrs := op.attrs()
		if err != nil {
			errType, status := classify(err)
			failure := []attribute.KeyValue{attribute.String(attrErrorType, errType)}
			if status > 0 {
				failure = append(failure, attribute.Int(attrStatusCode, status))
			}
			span.SetAttributes(failure...)
			span.RecordError(err)
			span.SetStatus(codes.Error, errType)
			attrs = append(attrs, failure...)
		}
		span.End()
		t.duration.Record(ctx, time.Since(began).Seconds(), metric.WithAttributes(attrs...))
	}
}

// retry records one retry on the operation span and the retries counter. ctx
// must carry the operation span.
func (t *telemetry) retry(ctx context.Context, op operation, attempt int, delay time.Duration, cause error) {
	if t == nil {
		return
	}
	errType, _ := classify(cause)
	trace.SpanFromContext(ctx).AddEvent("retry", trace.WithAttributes(
		attribute.Int("retry.attempt", attempt),
		attribute.Float64("retry.delay_seconds", delay.Seconds()),
		attribute.String(attrErrorType, errType),
	))
	t.retries.Add(ctx, 1, metric.WithAttributes(append(op.attrs(), attribute.String(attrErrorType, errType))...))
}

// classify turns an error into the bounded error.type value used on spans and
// metrics (the HTTP status for service errors), plus the status when known.
func classify(err error) (errorType string, status int) {
	var apiErr *APIError
	switch {
	case errors.As(err, &apiErr):
		return strconv.Itoa(apiErr.StatusCode), apiErr.StatusCode
	case errors.Is(err, ErrInvalidResponse):
		return "invalid_response", 0
	case errors.Is(err, context.Canceled):
		return "canceled", 0
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded", 0
	default:
		return "transport", 0
	}
}

// compile-time check that the transport wrapper is a RoundTripper.
var _ http.RoundTripper = (*otelhttp.Transport)(nil)
