package whalealert

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Sentinel errors for use with errors.Is.
var (
	ErrUnauthorized  = errors.New("whalealert: unauthorized")
	ErrForbidden     = errors.New("whalealert: forbidden")
	ErrNotFound      = errors.New("whalealert: not found")
	ErrRateLimited   = errors.New("whalealert: rate limited")
	ErrBadRequest    = errors.New("whalealert: bad request")
	ErrValidation    = errors.New("whalealert: validation error")
	ErrTransport     = errors.New("whalealert: transport error")
	ErrDecoding      = errors.New("whalealert: decoding error")
	ErrProviderAPI   = errors.New("whalealert: provider API error")
	ErrMissingAPIKey = errors.New("whalealert: API key required for this endpoint")
)

// APIError represents an error returned by the Whale Alert API.
type APIError struct {
	StatusCode  int
	Message     string
	BodyExcerpt string
	Headers     http.Header
	RetryAfter  *time.Duration
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("whalealert API error %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("whalealert API error %d", e.StatusCode)
}

// Is reports whether the API error corresponds to a sentinel API error.
func (e *APIError) Is(target error) bool {
	switch target {
	case ErrUnauthorized:
		return e.StatusCode == http.StatusUnauthorized
	case ErrForbidden:
		return e.StatusCode == http.StatusForbidden
	case ErrNotFound:
		return e.StatusCode == http.StatusNotFound
	case ErrRateLimited:
		return e.StatusCode == http.StatusTooManyRequests
	case ErrBadRequest:
		return e.StatusCode == http.StatusBadRequest
	case ErrValidation:
		return e.StatusCode == http.StatusUnprocessableEntity
	case ErrProviderAPI:
		return e.StatusCode >= 400
	}
	return false
}

// As assigns this API error to a compatible target.
func (e *APIError) As(target interface{}) bool {
	switch t := target.(type) {
	case *APIError:
		*t = *e
		return true
	}
	return false
}

// newAPIError builds an APIError from an HTTP status, response body, and headers.
// The body excerpt is capped at 512 bytes and never contains the api_key value.
func newAPIError(status int, body []byte, headers http.Header) *APIError {
	excerpt := sanitizeBody(body, 512)

	msg := ""
	var errBody struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &errBody) == nil {
		if errBody.Error != "" {
			msg = errBody.Error
		} else if errBody.Message != "" {
			msg = errBody.Message
		}
	}
	if msg == "" {
		msg = http.StatusText(status)
	}

	apiErr := &APIError{
		StatusCode:  status,
		Message:     msg,
		BodyExcerpt: excerpt,
		Headers:     headers.Clone(),
	}

	if d, ok := parseRetryAfterHeader(headers); ok {
		apiErr.RetryAfter = &d
	}

	return apiErr
}

// sanitizeBody returns a capped body excerpt with api_key values redacted.
func sanitizeBody(body []byte, max int) string {
	s := string(body)
	if len(s) > max {
		s = s[:max]
	}
	// Redact api_key=VALUE patterns (value runs until & or end of string)
	result := ""
	remaining := s
	for {
		idx := strings.Index(remaining, "api_key=")
		if idx == -1 {
			result += remaining
			break
		}
		result += remaining[:idx] + "api_key=REDACTED"
		remaining = remaining[idx+len("api_key="):]
		for len(remaining) > 0 && remaining[0] != '&' && remaining[0] != '"' && remaining[0] != ' ' {
			remaining = remaining[1:]
		}
	}
	return result
}

// parseRetryAfterHeader extracts the Retry-After header as a duration.
func parseRetryAfterHeader(headers http.Header) (time.Duration, bool) {
	v := headers.Get("Retry-After")
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d, true
		}
		return 0, true
	}
	return 0, false
}
