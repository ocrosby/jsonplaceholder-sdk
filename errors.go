package jsonplaceholder

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/jedi-knights/go-platform/apperrors"
)

// ErrInvalidArgument is wrapped by every error returned for a bad argument
// (a non-positive id, a negative page or limit). The SDK rejects these before
// making any request. Test for it with errors.Is.
var ErrInvalidArgument error = apperrors.New(apperrors.ErrCodeBadRequest, "jsonplaceholder: invalid argument")

// ErrTooManyPages is yielded by an All iterator that reached MaxPages without
// the service reporting the end of the collection.
var ErrTooManyPages error = apperrors.New(apperrors.ErrCodeInternal, "jsonplaceholder: exceeded MaxPages while paging")

// ErrInvalidResponse is wrapped by the error returned when the service answers
// 200 but the body is not the JSON the SDK expected, for example an HTML error
// page from a proxy. It is not retried. Test for it with errors.Is.
var ErrInvalidResponse error = apperrors.New(apperrors.ErrCodeUnavailable, "jsonplaceholder: invalid response")

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

// Unwrap exposes a go-platform *apperrors.AppError whose code is derived from
// the HTTP status (404 NOT_FOUND, 429 RATE_LIMIT, 401 UNAUTHORIZED, 403
// FORBIDDEN, 409 CONFLICT, other 4xx BAD_REQUEST, 502, 503 and 504
// UNAVAILABLE, other 5xx INTERNAL). It lets a service built on go-platform
// use apperrors.IsNotFound(err) and httputil.WriteError(w, err) on SDK errors
// directly. The *APIError itself stays reachable with errors.As.
func (e *APIError) Unwrap() error {
	return apperrors.New(codeForStatus(e.StatusCode), e.Message)
}

// statusCodes maps the statuses with a dedicated go-platform code. Any other
// 4xx is BAD_REQUEST and anything else is INTERNAL (see codeForStatus).
var statusCodes = map[int]apperrors.ErrorCode{
	http.StatusNotFound:           apperrors.ErrCodeNotFound,
	http.StatusTooManyRequests:    apperrors.ErrCodeRateLimit,
	http.StatusUnauthorized:       apperrors.ErrCodeUnauthorized,
	http.StatusForbidden:          apperrors.ErrCodeForbidden,
	http.StatusConflict:           apperrors.ErrCodeConflict,
	http.StatusBadGateway:         apperrors.ErrCodeUnavailable,
	http.StatusServiceUnavailable: apperrors.ErrCodeUnavailable,
	http.StatusGatewayTimeout:     apperrors.ErrCodeUnavailable,
}

// codeForStatus maps an HTTP status to the go-platform error code.
func codeForStatus(status int) apperrors.ErrorCode {
	if code, ok := statusCodes[status]; ok {
		return code
	}
	if status >= 400 && status < 500 {
		return apperrors.ErrCodeBadRequest
	}
	return apperrors.ErrCodeInternal
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
