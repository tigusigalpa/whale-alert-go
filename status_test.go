package whalealert

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetSupportedBlockchains_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			t.Errorf("path = %q, want /status", r.URL.Path)
		}
		if got := r.URL.Query().Get("api_key"); got != "" {
			t.Errorf("api_key should not be sent for public status, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"name":"bitcoin","symbols":["BTC","USDT","EURT"]},{"name":"ethereum","symbols":["ETH","USDT","USDC"]}]`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	chains, err := c.Status.GetSupportedBlockchains(context.Background())
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(chains) != 2 {
		t.Fatalf("len = %d, want 2", len(chains))
	}
	if chains[0].Name != "bitcoin" {
		t.Errorf("name = %q, want bitcoin", chains[0].Name)
	}
	if len(chains[0].Symbols) != 3 {
		t.Errorf("symbols len = %d, want 3", len(chains[0].Symbols))
	}
}

func TestGetSupportedBlockchains_NoAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"name":"bitcoin","symbols":["BTC"]}]`))
	}))
	defer srv.Close()

	c := NewClient("", WithBaseURL(srv.URL))
	chains, err := c.Status.GetSupportedBlockchains(context.Background())
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(chains) != 1 {
		t.Fatalf("len = %d, want 1", len(chains))
	}
}

func TestGetSupportedBlockchains_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Status.GetSupportedBlockchains(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetSupportedBlockchains_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{broken`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Status.GetSupportedBlockchains(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestGetBlockchainStatus_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bitcoin/status" {
			t.Errorf("path = %q, want /bitcoin/status", r.URL.Path)
		}
		if got := r.URL.Query().Get("api_key"); got != "test-api-key" {
			t.Errorf("api_key = %q, want test-api-key", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"start_height":770789,"end_height":776799,"block_count":6011}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	status, err := c.Status.GetBlockchainStatus(context.Background(), "bitcoin")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if status.StartHeight != 770789 {
		t.Errorf("StartHeight = %d, want 770789", status.StartHeight)
	}
	if status.EndHeight != 776799 {
		t.Errorf("EndHeight = %d, want 776799", status.EndHeight)
	}
	if status.BlockCount != 6011 {
		t.Errorf("BlockCount = %d, want 6011", status.BlockCount)
	}
}

func TestGetBlockchainStatus_MissingAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not make a request without API key")
	}))
	defer srv.Close()

	c := NewClient("", WithBaseURL(srv.URL))
	_, err := c.Status.GetBlockchainStatus(context.Background(), "bitcoin")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrMissingAPIKey) {
		t.Errorf("error should be ErrMissingAPIKey, got: %v", err)
	}
}

func TestGetBlockchainStatus_EmptyBlockchain(t *testing.T) {
	c := NewClient("key")
	_, err := c.Status.GetBlockchainStatus(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty blockchain")
	}
}

func TestGetBlockchainStatus_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"blockchain not found"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Status.GetBlockchainStatus(context.Background(), "unknown")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) = false, want true")
	}
}

func TestGetBlockchainStatus_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Status.GetBlockchainStatus(context.Background(), "bitcoin")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("errors.Is(err, ErrUnauthorized) = false, want true")
	}
}

func TestGetBlockchainStatus_DecodesCorrectly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"start_height":0,"end_height":0,"block_count":0}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	status, err := c.Status.GetBlockchainStatus(context.Background(), "bitcoin")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if status == nil {
		t.Fatal("status should not be nil")
	}
}

// Ensure json import is used
var _ = json.Marshal
