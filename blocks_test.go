package whalealert

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetBlock_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bitcoin/block/771103" {
			t.Errorf("path = %q, want /bitcoin/block/771103", r.URL.Path)
		}
		if got := r.URL.Query().Get("api_key"); got != "test-api-key" {
			t.Errorf("api_key = %q, want test-api-key", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"timestamp": 1688420591,
			"hash": "0xblockhash",
			"transactions": [{"height":771103,"hash":"0xtx1","fee":"0.001","fee_symbol":"BTC","fee_symbol_price":50000,"sub_transactions":[]}]
		}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	block, err := c.Blocks.GetBlock(context.Background(), "bitcoin", 771103)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if block.Timestamp != 1688420591 {
		t.Errorf("Timestamp = %d, want 1688420591", block.Timestamp)
	}
	if block.Hash != "0xblockhash" {
		t.Errorf("Hash = %q, want 0xblockhash", block.Hash)
	}
	if len(block.Transactions) != 1 {
		t.Fatalf("transactions len = %d, want 1", len(block.Transactions))
	}
}

func TestGetBlock_MissingAPIKey(t *testing.T) {
	c := NewClient("")
	_, err := c.Blocks.GetBlock(context.Background(), "bitcoin", 100)
	if !errors.Is(err, ErrMissingAPIKey) {
		t.Errorf("error should be ErrMissingAPIKey, got: %v", err)
	}
}

func TestGetBlock_EmptyBlockchain(t *testing.T) {
	c := NewClient("key")
	_, err := c.Blocks.GetBlock(context.Background(), "", 100)
	if err == nil {
		t.Fatal("expected error for empty blockchain")
	}
}

func TestGetBlock_InvalidHeight(t *testing.T) {
	c := NewClient("key")
	_, err := c.Blocks.GetBlock(context.Background(), "bitcoin", 0)
	if err == nil {
		t.Fatal("expected error for zero height")
	}
	_, err = c.Blocks.GetBlock(context.Background(), "bitcoin", -1)
	if err == nil {
		t.Fatal("expected error for negative height")
	}
}

func TestGetBlock_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"block not found"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Blocks.GetBlock(context.Background(), "bitcoin", 999999)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) = false, want true")
	}
}
