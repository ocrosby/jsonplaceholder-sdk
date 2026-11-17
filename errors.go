package jsonplaceholder

import (
	"errors"
	"fmt"
	"net/http"
)

// ErrInvalidArgument is wrapped by every error returned for a bad argument
// (a non-positive id, a negative page or limit). The SDK rejects these before
// making any request. Test for it with errors.Is.
var ErrInvalidArgument = errors.New("jsonplaceholder: invalid argument")

// ErrTooManyPages is yielded by an All iterator that reached MaxPages without
// the service reporting the end of the collection.
var ErrTooManyPages = errors.New("jsonplaceholder: exceeded MaxPages while paging")

// APIError is returned when the service answers with a non-200 status.
// Inspect it with errors.As:
//
//	var apiErr *jsonplaceholder.APIError
//	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusTooManyRequests { ... }
type APIError struct {
	// StatusCode is the HTTP status of the final response.
	StatusCode int
	// Message is the status text, for example "Not Found".
	Message string
	// RequestID identifies the request to the service's CDN. JSONPlaceholder
	// sends no request-id header, so this is Cloudflare's cf-ray value, which
	// is the identifier a support request can quote. It is empty when absent.
	RequestID string
	// Method and URL describe the request that failed.
	Method string
	URL    string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return fmt.Sprintf("jsonplaceholder: %s %s: %d %s [request_id=%s]", e.Method, e.URL, e.StatusCode, e.Message, e.RequestID)
}

// IsNotFound reports whether err is an APIError with status 404.
func IsNotFound(err error) bool { return hasStatus(err, http.StatusNotFound) }

// IsRateLimited reports whether err is an APIError with status 429.
func IsRateLimited(err error) bool { return hasStatus(err, http.StatusTooManyRequests) }

// hasStatus reports whether err wraps an APIError with the given status.
func hasStatus(err error, status int) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == status
}

// invalid builds an error that wraps ErrInvalidArgument.
func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidArgument, fmt.Sprintf(format, a...))
}
