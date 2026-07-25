package whalealert

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestErrorsIsSentinelMapping(t *testing.T) {
	cases := []struct {
		status  int
		target  error
		matches bool
	}{
		{http.StatusUnauthorized, ErrUnauthorized, true},
		{http.StatusForbidden, ErrForbidden, true},
		{http.StatusNotFound, ErrNotFound, true},
		{http.StatusTooManyRequests, ErrRateLimited, true},
		{http.StatusBadRequest, ErrBadRequest, true},
		{http.StatusUnprocessableEntity, ErrValidation, true},
		{http.StatusInternalServerError, ErrProviderAPI, true},
		{http.StatusOK, ErrNotFound, false},
		{http.StatusOK, ErrProviderAPI, false},
	}
	for _, tc := range cases {
		apiErr := &APIError{StatusCode: tc.status}
		got := errors.Is(apiErr, tc.target)
		if got != tc.matches {
			t.Errorf("status %d: errors.Is(apiErr, %v) = %v, want %v", tc.status, tc.target, got, tc.matches)
		}
	}
}

func TestAPIErrorMessage(t *testing.T) {
	e := &APIError{StatusCode: 404, Message: "not found"}
	if !strings.Contains(e.Error(), "404") {
		t.Errorf("Error() should contain status code: %s", e.Error())
	}
	if !strings.Contains(e.Error(), "not found") {
		t.Errorf("Error() should contain message: %s", e.Error())
	}
}

func TestAPIErrorNoMessage(t *testing.T) {
	e := &APIError{StatusCode: 500}
	if !strings.Contains(e.Error(), "500") {
		t.Errorf("Error() should contain status code: %s", e.Error())
	}
}

func TestRetryAfterParsing(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "120")
	d, ok := parseRetryAfterHeader(h)
	if !ok {
		t.Fatal("expected ok")
	}
	if d != 120*time.Second {
		t.Errorf("duration = %v, want 120s", d)
	}
}

func TestRetryAfterInvalidValue(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "not-a-number")
	_, ok := parseRetryAfterHeader(h)
	if ok {
		t.Error("expected false for invalid value")
	}
}
