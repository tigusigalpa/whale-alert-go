package whalealert

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// newGetRequest creates a GET request with standard headers.
func newGetRequest(ctx context.Context, c *Client, reqURL string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("whalealert: create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	return req, nil
}

// decodeResponse decodes an HTTP response body into the target type.
// On non-2xx status codes it returns a typed APIError.
func decodeResponse[T any](resp *http.Response) (T, error) {
	var zero T

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return zero, fmt.Errorf("whalealert: read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return zero, newAPIError(resp.StatusCode, body, resp.Header)
	}

	if len(body) == 0 {
		return zero, fmt.Errorf("whalealert: empty response body")
	}

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return zero, fmt.Errorf("whalealert: decode response: %w", err)
	}
	return result, nil
}
