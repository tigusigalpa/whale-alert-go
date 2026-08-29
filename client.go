package whalealert

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://leviathan.whale-alert.io"
	defaultTimeout = 30 * time.Second
	userAgent      = "whale-alert-go/1.0.0 (+https://github.com/tigusigalpa/whale-alert-go)"
)

// Client is the Whale Alert API HTTP client. It is safe for concurrent use
// by multiple goroutines. Do not mutate fields after construction.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	userAgent  string
	retry      RetryConfig
	hooks      []RequestHook

	Status       *StatusService
	Transactions *TransactionsService
	Blocks       *BlocksService
	Addresses    *AddressesService
}

// NewClient creates a new Whale Alert API client. The apiKey is required for
// authenticated endpoints; the public GET /status endpoint works without it.
func NewClient(apiKey string, opts ...ClientOption) *Client {
	c := &Client{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		userAgent: userAgent,
		retry: RetryConfig{
			MaxAttempts:  0,
			InitialDelay: 500 * time.Millisecond,
			MaxDelay:     10 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	c.Status = &StatusService{client: c}
	c.Transactions = &TransactionsService{client: c}
	c.Blocks = &BlocksService{client: c}
	c.Addresses = &AddressesService{client: c}

	return c
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithBaseURL overrides the default production base URL.
func WithBaseURL(u string) ClientOption {
	return func(c *Client) {
		c.baseURL = strings.TrimRight(u, "/")
	}
}

// WithHTTPClient replaces the default HTTP client.
func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *Client) {
		if hc != nil {
			c.httpClient = hc
		}
	}
}

// WithTimeout sets the HTTP client timeout.
func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) {
		c.httpClient.Timeout = d
	}
}

// WithUserAgent overrides the default User-Agent header.
func WithUserAgent(ua string) ClientOption {
	return func(c *Client) {
		c.userAgent = ua
	}
}

// WithRetry configures the retry policy for idempotent GET requests.
// Set maxAttempts to 0 to disable retries (the default).
func WithRetry(maxAttempts int, initialDelay, maxDelay time.Duration) ClientOption {
	return func(c *Client) {
		c.retry = RetryConfig{
			MaxAttempts:  maxAttempts,
			InitialDelay: initialDelay,
			MaxDelay:     maxDelay,
		}
	}
}

// RequestHook is called before each HTTP request is sent. The URL passed to
// the hook has the api_key query parameter redacted.
type RequestHook func(ctx context.Context, method, redactedURL string, body io.Reader)

// WithRequestHook adds a hook invoked before each HTTP request. Multiple
// hooks are called in registration order.
func WithRequestHook(hook RequestHook) ClientOption {
	return func(c *Client) {
		c.hooks = append(c.hooks, hook)
	}
}

// doRequest performs an HTTP request with retry handling and response decoding.
func (c *Client) doRequest(ctx context.Context, method, path string, params url.Values, out interface{}) error {
	reqURL := c.buildURL(path, params)
	return c.doRequestURL(ctx, method, reqURL, out)
}

// doRequestURL performs an HTTP request to an already constructed URL with
// retry handling and response decoding.
func (c *Client) doRequestURL(ctx context.Context, method, reqURL string, out interface{}) error {
	respBody, err := c.doRequestURLRaw(ctx, method, reqURL)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := decodeJSON(respBody, out); err != nil {
		return fmt.Errorf("whalealert: decode response: %w", err)
	}
	return nil
}

// doRequestRaw performs an HTTP request and returns the raw response body
// without decoding. Used for endpoints that may return non-JSON formats.
func (c *Client) doRequestRaw(ctx context.Context, method, path string, params url.Values) ([]byte, error) {
	return c.doRequestURLRaw(ctx, method, c.buildURL(path, params))
}

// doRequestURLRaw performs an HTTP request to an already constructed URL and
// returns the raw response body without decoding.
func (c *Client) doRequestURLRaw(ctx context.Context, method, reqURL string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	baseCtx := ctx
	if _, ok := ctx.Deadline(); !ok && c.httpClient.Timeout > 0 {
		var cancel context.CancelFunc
		baseCtx, cancel = context.WithTimeout(ctx, c.httpClient.Timeout)
		defer cancel()
	}

	for attempt := 0; attempt <= c.retry.MaxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(baseCtx, method, reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("whalealert: create request: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.userAgent)

		for _, hook := range c.hooks {
			hook(baseCtx, method, redactURL(reqURL), nil)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if baseCtx.Err() != nil {
				return nil, baseCtx.Err()
			}
			if attempt < c.retry.MaxAttempts && isRetryable(method) {
				if waitErr := c.sleep(baseCtx, c.backoff(attempt)); waitErr != nil {
					return nil, waitErr
				}
				continue
			}
			return nil, fmt.Errorf("whalealert: request failed: %w", err)
		}

		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("whalealert: read response: %w", readErr)
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return respBody, nil
		}

		apiErr := newAPIError(resp.StatusCode, respBody, resp.Header)

		if isRetryable(method) && shouldRetryStatus(resp.StatusCode) && attempt < c.retry.MaxAttempts {
			wait := c.backoff(attempt)
			if d, ok := parseRetryAfterHeader(resp.Header); ok && d > 0 {
				wait = d
			}
			if sleepErr := c.sleep(baseCtx, wait); sleepErr != nil {
				return nil, sleepErr
			}
			continue
		}

		return nil, apiErr
	}

	return nil, fmt.Errorf("whalealert: request exceeded maximum retry attempts")
}

// buildURL constructs the full request URL with query parameters.
// If the client has an API key and the path requires authentication,
// the api_key parameter is appended.
func (c *Client) buildURL(path string, params url.Values) string {
	u := c.baseURL + path
	clonedParams := make(url.Values, len(params))
	for key, values := range params {
		clonedParams[key] = append([]string(nil), values...)
	}
	params = clonedParams
	if c.apiKey != "" {
		params.Set("api_key", c.apiKey)
	}
	encoded := params.Encode()
	if encoded != "" {
		u += "?" + encoded
	}
	return u
}

// buildURLNoAuth constructs the full request URL without the api_key parameter.
func (c *Client) buildURLNoAuth(path string, params url.Values) string {
	u := c.baseURL + path
	if params != nil {
		encoded := params.Encode()
		if encoded != "" {
			u += "?" + encoded
		}
	}
	return u
}

func (c *Client) backoff(attempt int) time.Duration {
	delay := c.retry.InitialDelay * (1 << attempt)
	if delay > c.retry.MaxDelay || delay <= 0 {
		return c.retry.MaxDelay
	}
	return delay
}

func (c *Client) sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isRetryable(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

func shouldRetryStatus(status int) bool {
	return status == http.StatusTooManyRequests || (status >= 500 && status <= 599)
}

// redactURL replaces the api_key query parameter value with "REDACTED".
func redactURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	if q.Has("api_key") {
		q.Set("api_key", "REDACTED")
		u.RawQuery = q.Encode()
	}
	return u.String()
}
