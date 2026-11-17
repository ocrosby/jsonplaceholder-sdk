package jsonplaceholder_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jedi-knights/go-platform/apperrors"

	jp "github.com/ocrosby/jsonplaceholder-sdk"
)

// The SDK's errors carry a go-platform AppError in their chain, so services
// built on go-platform can use apperrors predicates and httputil.WriteError
// on them without translating.

func TestAPIErrorStatusMapsToAnAppErrorCode(t *testing.T) {
	cases := []struct {
		status int
		code   apperrors.ErrorCode
		is     func(error) bool
	}{
		{http.StatusNotFound, apperrors.ErrCodeNotFound, apperrors.IsNotFound},
		{http.StatusTooManyRequests, apperrors.ErrCodeRateLimit, apperrors.IsRateLimit},
		{http.StatusUnauthorized, apperrors.ErrCodeUnauthorized, apperrors.IsUnauthorized},
		{http.StatusForbidden, apperrors.ErrCodeForbidden, apperrors.IsForbidden},
		{http.StatusConflict, apperrors.ErrCodeConflict, apperrors.IsConflict},
		{http.StatusTeapot, apperrors.ErrCodeBadRequest, apperrors.IsBadRequest}, // any other 4xx
		{http.StatusInternalServerError, apperrors.ErrCodeInternal, apperrors.IsInternal},
		{http.StatusBadGateway, apperrors.ErrCodeUnavailable, apperrors.IsUnavailable},
		{http.StatusServiceUnavailable, apperrors.ErrCodeUnavailable, apperrors.IsUnavailable},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			c := newServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tc.status) }, jp.WithRetry(0, time.Millisecond))
			_, err := c.Posts.Get(context.Background(), 1)
			var appErr *apperrors.AppError
			if !errors.As(err, &appErr) || appErr.Code() != tc.code || !tc.is(err) {
				t.Fatalf("err = %v, want AppError code %s", err, tc.code)
			}
			var apiErr *jp.APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != tc.status {
				t.Errorf("the APIError must still be reachable: %v", err)
			}
		})
	}
}

func TestSDKSentinelsMapToAppErrorCodes(t *testing.T) {
	c := newServer(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`<html>`)) })

	_, err := c.Posts.Get(context.Background(), 0)
	if !errors.Is(err, jp.ErrInvalidArgument) || !apperrors.IsBadRequest(err) {
		t.Errorf("invalid argument: %v", err)
	}
	_, err = c.Posts.Get(context.Background(), 1)
	if !errors.Is(err, jp.ErrInvalidResponse) || !apperrors.IsUnavailable(err) {
		t.Errorf("invalid response: %v", err)
	}
	if !apperrors.IsInternal(jp.ErrTooManyPages) || !errors.Is(jp.ErrTooManyPages, jp.ErrTooManyPages) {
		t.Errorf("too many pages: %v", jp.ErrTooManyPages)
	}
}
