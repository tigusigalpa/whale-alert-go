package whalealert

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, srv *httptest.Server, opts ...ClientOption) *Client {
	t.Helper()
	allOpts := append([]ClientOption{WithBaseURL(srv.URL)}, opts...)
	return NewClient("test-api-key", allOpts...)
}

func TestNewClient_Defaults(t *testing.T) {
	c := NewClient("my-key")
	if c.apiKey != "my-key" {
		t.Errorf("apiKey = %q, want %q", c.apiKey, "my-key")
	}
	if c.baseURL != defaultBaseURL {
		t.Errorf("baseURL = %q, want %q", c.baseURL, defaultBaseURL)
	}
	if c.httpClient.Timeout != defaultTimeout {
		t.Errorf("timeout = %v, want %v", c.httpClient.Timeout, defaultTimeout)
	}
	if c.userAgent != userAgent {
		t.Errorf("userAgent = %q, want %q", c.userAgent, userAgent)
	}
	if c.retry.MaxAttempts != 0 {
		t.Errorf("retry.MaxAttempts = %d, want 0", c.retry.MaxAttempts)
	}
	if c.Status == nil || c.Transactions == nil || c.Blocks == nil || c.Addresses == nil {
		t.Error("services should not be nil")
	}
}

func TestNewClient_NoAPIKey(t *testing.T) {
	c := NewClient("")
	if c.apiKey != "" {
		t.Errorf("apiKey = %q, want empty", c.apiKey)
	}
}

func TestWithBaseURL(t *testing.T) {
	c := NewClient("k", WithBaseURL("https://custom.example.com/"))
	if c.baseURL != "https://custom.example.com" {
		t.Errorf("baseURL = %q, want %q", c.baseURL, "https://custom.example.com")
	}
}

func TestWithTimeout(t *testing.T) {
	c := NewClient("k", WithTimeout(5*time.Second))
	if c.httpClient.Timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", c.httpClient.Timeout)
	}
}

func TestWithHTTPClient_NilPreservesDefaultClient(t *testing.T) {
	c := NewClient("k", WithHTTPClient(nil))
	if c.httpClient == nil {
		t.Fatal("httpClient should retain the default client")
	}
}

func TestWithUserAgent(t *testing.T) {
	c := NewClient("k", WithUserAgent("custom/1.0"))
	if c.userAgent != "custom/1.0" {
		t.Errorf("userAgent = %q, want %q", c.userAgent, "custom/1.0")
	}
}

func TestWithRetry(t *testing.T) {
	c := NewClient("k", WithRetry(3, 100*time.Millisecond, 5*time.Second))
	if c.retry.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", c.retry.MaxAttempts)
	}
	if c.retry.InitialDelay != 100*time.Millisecond {
		t.Errorf("InitialDelay = %v, want 100ms", c.retry.InitialDelay)
	}
	if c.retry.MaxDelay != 5*time.Second {
		t.Errorf("MaxDelay = %v, want 5s", c.retry.MaxDelay)
	}
}

func TestDoRequest_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("api_key"); got != "test-api-key" {
			t.Errorf("api_key = %q, want %q", got, "test-api-key")
		}
		if got := r.Header.Get("User-Agent"); got != userAgent {
			t.Errorf("User-Agent = %q, want %q", got, userAgent)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"start_height":100,"end_height":200,"block_count":101}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)

	var result BlockchainStatus
	err := c.doRequest(context.Background(), "GET", "/bitcoin/status", nil, &result)
	if err != nil {
		t.Fatalf("doRequest error: %v", err)
	}
	if result.StartHeight != 100 || result.EndHeight != 200 || result.BlockCount != 101 {
		t.Errorf("result = %+v, want {100 200 101}", result)
	}
}

func TestDoRequest_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{invalid json`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)

	var result BlockchainStatus
	err := c.doRequest(context.Background(), "GET", "/bitcoin/status", nil, &result)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "decode response") {
		t.Errorf("error should mention decode, got: %v", err)
	}
}

func TestDoRequest_RetriesOn429ThenSucceeds(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limited"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"start_height":1,"end_height":2,"block_count":2}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, WithRetry(5, 1*time.Millisecond, 10*time.Millisecond))

	var result BlockchainStatus
	err := c.doRequest(context.Background(), "GET", "/bitcoin/status", nil, &result)
	if err != nil {
		t.Fatalf("doRequest error: %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Fatalf("attempts = %d, want 3", got)
	}
}

func TestDoRequest_ExhaustsRetriesReturnsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, WithRetry(2, 1*time.Millisecond, 5*time.Millisecond))

	err := c.doRequest(context.Background(), "GET", "/bitcoin/status", nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err type = %T, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusTooManyRequests {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusTooManyRequests)
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Error("errors.Is(err, ErrRateLimited) = false, want true")
	}
}

func TestDoRequest_NoRetryByDefault(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)

	err := c.doRequest(context.Background(), "GET", "/bitcoin/status", nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Fatalf("attempts = %d, want 1 (no retries by default)", got)
	}
}

func TestDoRequest_RetriesOn5xxThenSucceeds(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"start_height":1,"end_height":2,"block_count":2}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, WithRetry(3, 1*time.Millisecond, 10*time.Millisecond))

	var result BlockchainStatus
	err := c.doRequest(context.Background(), "GET", "/bitcoin/status", nil, &result)
	if err != nil {
		t.Fatalf("doRequest error: %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
}

func TestDoRequest_NoRetryOn400(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, WithRetry(5, 1*time.Millisecond, 5*time.Millisecond))

	err := c.doRequest(context.Background(), "GET", "/bitcoin/status", nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Fatalf("attempts = %d, want 1 (no retry on 400)", got)
	}
}

func TestDoRequest_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := newTestClient(t, srv, WithRetry(5, 1*time.Second, 5*time.Second))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := c.doRequest(ctx, "GET", "/bitcoin/status", nil, nil)
	if err == nil {
		t.Fatal("expected error due to context deadline, got nil")
	}
}

func TestDoRequest_UnauthorizedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)

	err := c.doRequest(context.Background(), "GET", "/bitcoin/status", nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("errors.Is(err, ErrUnauthorized) = false, want true")
	}
}

func TestDoRequest_NotFoundError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)

	err := c.doRequest(context.Background(), "GET", "/bitcoin/transaction/abc", nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) = false, want true")
	}
}

func TestRedactURL(t *testing.T) {
	cases := []struct {
		input  string
		expect string
	}{
		{
			"https://leviathan.whale-alert.io/status?api_key=secret123",
			"https://leviathan.whale-alert.io/status?api_key=REDACTED",
		},
		{
			"https://leviathan.whale-alert.io/status",
			"https://leviathan.whale-alert.io/status",
		},
	}
	for _, tc := range cases {
		got := redactURL(tc.input)
		if got != tc.expect {
			t.Errorf("redactURL(%q) = %q, want %q", tc.input, got, tc.expect)
		}
	}
}

func TestRequestHook_RedactedURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"start_height":1,"end_height":2,"block_count":2}`))
	}))
	defer srv.Close()

	var capturedURL string

	c := newTestClient(t, srv, WithRequestHook(func(_ context.Context, _, u string, _ io.Reader) {
		capturedURL = u
	}))

	var result BlockchainStatus
	_ = c.doRequest(context.Background(), "GET", "/bitcoin/status", nil, &result)

	if strings.Contains(capturedURL, "test-api-key") {
		t.Errorf("hook URL should not contain api_key value, got: %s", capturedURL)
	}
	if !strings.Contains(capturedURL, "REDACTED") {
		t.Errorf("hook URL should contain REDACTED, got: %s", capturedURL)
	}
}

func TestBuildURL_WithAPIKey(t *testing.T) {
	c := NewClient("my-secret-key")
	u := c.buildURL("/status", nil)
	if !strings.Contains(u, "api_key=my-secret-key") {
		t.Errorf("URL should contain api_key, got: %s", u)
	}
}

func TestBuildURL_DoesNotMutateParams(t *testing.T) {
	c := NewClient("my-secret-key")
	params := url.Values{"limit": {"10"}}
	_ = c.buildURL("/status", params)
	if params.Has("api_key") {
		t.Fatal("buildURL should not mutate the caller's query parameters")
	}
}

func TestBuildURL_NoAuth(t *testing.T) {
	c := NewClient("my-secret-key")
	u := c.buildURLNoAuth("/status", nil)
	if strings.Contains(u, "api_key") {
		t.Errorf("URL should not contain api_key, got: %s", u)
	}
}

func TestConcurrentSafe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"start_height":1,"end_height":2,"block_count":2}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			var result BlockchainStatus
			_ = c.doRequest(context.Background(), "GET", "/bitcoin/status", nil, &result)
			done <- struct{}{}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestIsRetryable(t *testing.T) {
	if !isRetryable(http.MethodGet) {
		t.Error("GET should be retryable")
	}
	if isRetryable(http.MethodPost) {
		t.Error("POST should not be retryable")
	}
	if isRetryable(http.MethodPut) {
		t.Error("PUT should not be retryable")
	}
}

func TestShouldRetryStatus(t *testing.T) {
	if !shouldRetryStatus(http.StatusTooManyRequests) {
		t.Error("429 should be retryable")
	}
	if !shouldRetryStatus(http.StatusInternalServerError) {
		t.Error("500 should be retryable")
	}
	if !shouldRetryStatus(http.StatusBadGateway) {
		t.Error("502 should be retryable")
	}
	if shouldRetryStatus(http.StatusBadRequest) {
		t.Error("400 should not be retryable")
	}
	if shouldRetryStatus(http.StatusUnauthorized) {
		t.Error("401 should not be retryable")
	}
}

func TestBackoff(t *testing.T) {
	c := NewClient("k", WithRetry(5, 100*time.Millisecond, 1*time.Second))
	if d := c.backoff(0); d != 100*time.Millisecond {
		t.Errorf("backoff(0) = %v, want 100ms", d)
	}
	if d := c.backoff(1); d != 200*time.Millisecond {
		t.Errorf("backoff(1) = %v, want 200ms", d)
	}
	if d := c.backoff(2); d != 400*time.Millisecond {
		t.Errorf("backoff(2) = %v, want 400ms", d)
	}
	if d := c.backoff(10); d != 1*time.Second {
		t.Errorf("backoff(10) = %v, want 1s (capped)", d)
	}
}

func TestParseRetryAfter_Seconds(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "30")
	d, ok := parseRetryAfterHeader(h)
	if !ok {
		t.Fatal("expected ok, got false")
	}
	if d != 30*time.Second {
		t.Errorf("duration = %v, want 30s", d)
	}
}

func TestParseRetryAfter_Absent(t *testing.T) {
	h := http.Header{}
	_, ok := parseRetryAfterHeader(h)
	if ok {
		t.Error("expected false for absent header")
	}
}

func TestAPIError_As(t *testing.T) {
	original := &APIError{StatusCode: 429, Message: "rate limited"}
	var target *APIError
	if !errors.As(original, &target) {
		t.Fatal("errors.As should succeed for *APIError")
	}
	if target.StatusCode != 429 || target.Message != "rate limited" {
		t.Errorf("target = %+v, want {429 rate limited}", target)
	}
}

func TestNewAPIError_ParsesErrorField(t *testing.T) {
	h := http.Header{}
	err := newAPIError(http.StatusUnauthorized, []byte(`{"error":"invalid key"}`), h)
	if err.Message != "invalid key" {
		t.Errorf("Message = %q, want %q", err.Message, "invalid key")
	}
	if err.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, want %d", err.StatusCode, http.StatusUnauthorized)
	}
}

func TestNewAPIError_ParsesMessageField(t *testing.T) {
	h := http.Header{}
	err := newAPIError(http.StatusBadRequest, []byte(`{"message":"bad input"}`), h)
	if err.Message != "bad input" {
		t.Errorf("Message = %q, want %q", err.Message, "bad input")
	}
}

func TestNewAPIError_FallsBackToStatusText(t *testing.T) {
	h := http.Header{}
	err := newAPIError(http.StatusNotFound, []byte(`{}`), h)
	if err.Message != http.StatusText(http.StatusNotFound) {
		t.Errorf("Message = %q, want %q", err.Message, http.StatusText(http.StatusNotFound))
	}
}

func TestNewAPIError_RetryAfter(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "60")
	err := newAPIError(http.StatusTooManyRequests, []byte(`{"error":"slow down"}`), h)
	if err.RetryAfter == nil || *err.RetryAfter != 60*time.Second {
		t.Errorf("RetryAfter = %v, want 60s", err.RetryAfter)
	}
}

func TestSanitizeBody_RedactsAPIKey(t *testing.T) {
	input := []byte(`{"url":"https://example.com?api_key=my-secret-key&foo=bar"}`)
	result := sanitizeBody(input, 512)
	if strings.Contains(result, "my-secret-key") {
		t.Errorf("body should not contain raw api_key value, got: %s", result)
	}
}

func TestSanitizeBody_Capped(t *testing.T) {
	long := strings.Repeat("a", 1000)
	result := sanitizeBody([]byte(long), 100)
	if len(result) > 100 {
		t.Errorf("result length = %d, want <= 100", len(result))
	}
}

func TestDecodeJSON_EmptyBody(t *testing.T) {
	var v BlockchainStatus
	err := decodeJSON([]byte{}, &v)
	if err == nil {
		t.Fatal("expected error for empty body")
	}
}

func TestDecodeJSON_Valid(t *testing.T) {
	var v BlockchainStatus
	err := decodeJSON([]byte(`{"start_height":1,"end_height":2,"block_count":2}`), &v)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.StartHeight != 1 {
		t.Errorf("StartHeight = %d, want 1", v.StartHeight)
	}
}

func TestModels_TransactionJSON(t *testing.T) {
	txJSON := `{
		"height": 17616182,
		"index_in_block": 6,
		"timestamp": 1688420591,
		"hash": "0xabc",
		"fee": "0.00238487557",
		"fee_symbol": "ETH",
		"fee_symbol_price": 1957.0,
		"sub_transactions": [
			{
				"symbol": "ETH",
				"transaction_type": "transfer",
				"inputs": [{"amount": "0.00238", "address": "0xffec", "owner": "nexo"}],
				"outputs": [{"amount": "0", "address": "0xdac1"}]
			}
		]
	}`
	var tx Transaction
	if err := json.Unmarshal([]byte(txJSON), &tx); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if tx.Height != 17616182 {
		t.Errorf("Height = %d, want 17616182", tx.Height)
	}
	if tx.Fee != "0.00238487557" {
		t.Errorf("Fee = %q, want %q", tx.Fee, "0.00238487557")
	}
	if tx.FeeSymbol != "ETH" {
		t.Errorf("FeeSymbol = %q, want ETH", tx.FeeSymbol)
	}
	if len(tx.SubTransactions) != 1 {
		t.Fatalf("SubTransactions len = %d, want 1", len(tx.SubTransactions))
	}
	st := tx.SubTransactions[0]
	if st.Symbol != "ETH" {
		t.Errorf("SubTx Symbol = %q, want ETH", st.Symbol)
	}
	if len(st.Inputs) != 1 || st.Inputs[0].Amount != "0.00238" {
		t.Errorf("Input amount = %q, want 0.00238", st.Inputs[0].Amount)
	}
	if st.Inputs[0].Owner != "nexo" {
		t.Errorf("Input owner = %q, want nexo", st.Inputs[0].Owner)
	}
}

func TestModels_TransactionPageJSON(t *testing.T) {
	pageJSON := `{
		"transactions": [],
		"next": "https://leviathan.whale-alert.io/bitcoin/transactions?start_height=100&start_index=100&limit=100"
	}`
	var page TransactionPage
	if err := json.Unmarshal([]byte(pageJSON), &page); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if page.Next == "" {
		t.Error("Next should not be empty")
	}
}

func TestModels_BlockchainJSON(t *testing.T) {
	raw := `[{"name":"bitcoin","symbols":["BTC","USDT","EURT"]},{"name":"dogecoin","symbols":["DOGE"]}]`
	var chains []Blockchain
	if err := json.Unmarshal([]byte(raw), &chains); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(chains) != 2 {
		t.Fatalf("len = %d, want 2", len(chains))
	}
	if chains[0].Name != "bitcoin" {
		t.Errorf("Name = %q, want bitcoin", chains[0].Name)
	}
	if len(chains[0].Symbols) != 3 {
		t.Errorf("Symbols len = %d, want 3", len(chains[0].Symbols))
	}
}

func TestModels_BlockJSON(t *testing.T) {
	raw := `{
		"timestamp": 1688420591,
		"hash": "0xblockhash",
		"transactions": []
	}`
	var block Block
	if err := json.Unmarshal([]byte(raw), &block); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if block.Timestamp != 1688420591 {
		t.Errorf("Timestamp = %d, want 1688420591", block.Timestamp)
	}
	if block.Hash != "0xblockhash" {
		t.Errorf("Hash = %q, want 0xblockhash", block.Hash)
	}
}

func TestModels_BlockchainStatusJSON(t *testing.T) {
	raw := `{"start_height":770789,"end_height":776799,"block_count":6011}`
	var status BlockchainStatus
	if err := json.Unmarshal([]byte(raw), &status); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if status.StartHeight != 770789 || status.EndHeight != 776799 || status.BlockCount != 6011 {
		t.Errorf("status = %+v, want {770789 776799 6011}", status)
	}
}

func TestDoRequestRaw_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"raw":"data"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	data, err := c.doRequestRaw(context.Background(), "GET", "/status", nil)
	if err != nil {
		t.Fatalf("doRequestRaw error: %v", err)
	}
	if string(data) != `{"raw":"data"}` {
		t.Errorf("data = %q, want {\"raw\":\"data\"}", string(data))
	}
}

func TestDoRequestRaw_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.doRequestRaw(context.Background(), "GET", "/status", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) = false, want true")
	}
}

func TestFmtErrorDoesNotLeakKey(t *testing.T) {
	c := NewClient("super-secret-key")
	u := c.buildURL("/status", nil)
	if strings.Contains(u, "super-secret-key") {
		// This is expected in the actual URL; what we test is that redactURL removes it
	}
	redacted := redactURL(u)
	if strings.Contains(redacted, "super-secret-key") {
		t.Errorf("redacted URL should not contain api key value: %s", redacted)
	}
}

// Ensure fmt is used
var _ = fmt.Sprintf
