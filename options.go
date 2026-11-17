package jsonplaceholder

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Retry limits. They are named constants so the bound on every retry loop is
// discoverable and reviewable.
const (
	// MaxRetries is the largest retry count WithRetry accepts.
	MaxRetries = 10
	// maxRetryBase is the largest base delay WithRetry accepts, which also
	// keeps the exponential shift in backoff from overflowing.
	maxRetryBase = time.Minute
)

// RequestHook observes each request attempt just before it is sent, including
// retries. It is the SDK's instrumentation seam: use it for metrics, tracing
// or request logs. The hook must not block and must be safe for concurrent use.
type RequestHook func(ctx context.Context, method, url string)

// Option configures a Client. Options are validated by NewClient.
type Option func(*Client) error

// WithBaseURL points the client at another JSONPlaceholder-compatible server,
// such as a local json-server or a test double. The URL must be http or https.
func WithBaseURL(raw string) Option {
	return func(c *Client) error {
		trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
		u, err := url.Parse(trimmed)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("jsonplaceholder: base URL must be an http(s) URL, got " + raw)
		}
		c.baseURL = trimmed
		return nil
	}
}

// WithHTTPClient replaces the HTTP client. Use it to add tracing, a custom
// transport or a proxy. The client's own Timeout applies to each attempt.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) error {
		if h == nil {
			return errors.New("jsonplaceholder: HTTP client must not be nil")
		}
		c.httpClient = h
		return nil
	}
}

// WithLogger sets the logger used for debug lines about retries.
func WithLogger(l *slog.Logger) Option {
	return func(c *Client) error {
		if l == nil {
			return errors.New("jsonplaceholder: logger must not be nil")
		}
		c.logger = l
		return nil
	}
}

// WithUserAgent sets the User-Agent header, for example "my-app/2.0".
func WithUserAgent(ua string) Option {
	return func(c *Client) error {
		if strings.TrimSpace(ua) == "" {
			return errors.New("jsonplaceholder: user agent must not be empty")
		}
		c.userAgent = ua
		return nil
	}
}

// WithRetry sets how many times a transient failure (HTTP 429, 500, 502, 503,
// 504, or a network error) is retried and the base delay of the exponential
// backoff. maxRetries is 0 to MaxRetries; 0 disables retrying. base must be
// positive and at most one minute.
func WithRetry(maxRetries int, base time.Duration) Option {
	return func(c *Client) error {
		if maxRetries < 0 || maxRetries > MaxRetries {
			return errors.New("jsonplaceholder: retries must be between 0 and MaxRetries")
		}
		if base <= 0 || base > maxRetryBase {
			return errors.New("jsonplaceholder: retry base delay must be positive and at most one minute")
		}
		c.maxRetries, c.retryBase = maxRetries, base
		return nil
	}
}

// WithRequestHook registers a RequestHook.
func WithRequestHook(h RequestHook) Option {
	return func(c *Client) error {
		if h == nil {
			return errors.New("jsonplaceholder: request hook must not be nil")
		}
		c.hook = h
		return nil
	}
}

// WithTracerProvider sets the OpenTelemetry TracerProvider the SDK's spans are
// created with. The default is the global provider, which is a no-op until the
// application installs one. Pass one explicitly to keep the SDK's telemetry
// separate from the global setup, or in tests.
func WithTracerProvider(tp trace.TracerProvider) Option {
	return func(c *Client) error {
		if tp == nil {
			return errors.New("jsonplaceholder: tracer provider must not be nil")
		}
		c.tracerProvider = tp
		return nil
	}
}

// WithMeterProvider sets the OpenTelemetry MeterProvider the SDK's metrics are
// created with. The default is the global provider.
func WithMeterProvider(mp metric.MeterProvider) Option {
	return func(c *Client) error {
		if mp == nil {
			return errors.New("jsonplaceholder: meter provider must not be nil")
		}
		c.meterProvider = mp
		return nil
	}
}

// WithPropagators sets the propagator used to inject trace context into
// outgoing requests. The default is the global propagator, which the
// application sets (go-platform's otel.New installs W3C TraceContext and Baggage).
func WithPropagators(p propagation.TextMapPropagator) Option {
	return func(c *Client) error {
		if p == nil {
			return errors.New("jsonplaceholder: propagator must not be nil")
		}
		c.propagator = p
		return nil
	}
}

// WithoutTelemetry turns off the SDK's spans, metrics and trace-context
// propagation, and leaves the HTTP client untouched. Use it when the HTTP
// client you pass to WithHTTPClient is already instrumented and you do not
// want every attempt recorded twice.
func WithoutTelemetry() Option {
	return func(c *Client) error {
		c.telemetryOff = true
		return nil
	}
}
