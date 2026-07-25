package whalealert

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetTransaction_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bitcoin/transaction/abc123" {
			t.Errorf("path = %q, want /bitcoin/transaction/abc123", r.URL.Path)
		}
		if got := r.URL.Query().Get("api_key"); got != "test-api-key" {
			t.Errorf("api_key = %q, want test-api-key", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"height": 100,
			"index_in_block": 1,
			"timestamp": 1688420591,
			"hash": "abc123",
			"fee": "0.001",
			"fee_symbol": "BTC",
			"fee_symbol_price": 50000.0,
			"sub_transactions": []
		}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	tx, err := c.Transactions.GetTransaction(context.Background(), "bitcoin", "abc123")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if tx.Hash != "abc123" {
		t.Errorf("Hash = %q, want abc123", tx.Hash)
	}
	if tx.Fee != "0.001" {
		t.Errorf("Fee = %q, want 0.001", tx.Fee)
	}
	if tx.FeeSymbol != "BTC" {
		t.Errorf("FeeSymbol = %q, want BTC", tx.FeeSymbol)
	}
}

func TestGetTransaction_MissingAPIKey(t *testing.T) {
	c := NewClient("")
	_, err := c.Transactions.GetTransaction(context.Background(), "bitcoin", "abc")
	if !errors.Is(err, ErrMissingAPIKey) {
		t.Errorf("error should be ErrMissingAPIKey, got: %v", err)
	}
}

func TestGetTransaction_EmptyBlockchain(t *testing.T) {
	c := NewClient("key")
	_, err := c.Transactions.GetTransaction(context.Background(), "", "abc")
	if err == nil {
		t.Fatal("expected error for empty blockchain")
	}
}

func TestGetTransaction_EmptyHash(t *testing.T) {
	c := NewClient("key")
	_, err := c.Transactions.GetTransaction(context.Background(), "bitcoin", "")
	if err == nil {
		t.Fatal("expected error for empty hash")
	}
}

func TestGetTransaction_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"transaction not found"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Transactions.GetTransaction(context.Background(), "bitcoin", "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) = false, want true")
	}
}

func TestListTransactions_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bitcoin/transactions" {
			t.Errorf("path = %q, want /bitcoin/transactions", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("start_height") != "768801" {
			t.Errorf("start_height = %q, want 768801", q.Get("start_height"))
		}
		if q.Get("limit") != "100" {
			t.Errorf("limit = %q, want 100", q.Get("limit"))
		}
		if q.Get("api_key") != "test-api-key" {
			t.Errorf("api_key = %q, want test-api-key", q.Get("api_key"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"transactions": [{"height":768801,"hash":"0xabc","fee":"0.001","fee_symbol":"BTC","fee_symbol_price":50000,"sub_transactions":[]}],
			"next": "https://leviathan.whale-alert.io/bitcoin/transactions?start_height=768801&start_index=100&limit=100"
		}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	page, err := c.Transactions.ListTransactions(context.Background(), "bitcoin", TransactionOptions{
		StartHeight: 768801,
		Limit:       100,
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(page.Transactions) != 1 {
		t.Fatalf("transactions len = %d, want 1", len(page.Transactions))
	}
	if page.Next == "" {
		t.Error("Next should not be empty")
	}
}

func TestListTransactions_MissingStartHeight(t *testing.T) {
	c := NewClient("key")
	_, err := c.Transactions.ListTransactions(context.Background(), "bitcoin", TransactionOptions{})
	if err == nil {
		t.Fatal("expected error for missing start_height")
	}
}

func TestListTransactions_MissingAPIKey(t *testing.T) {
	c := NewClient("")
	_, err := c.Transactions.ListTransactions(context.Background(), "bitcoin", TransactionOptions{StartHeight: 100})
	if !errors.Is(err, ErrMissingAPIKey) {
		t.Errorf("error should be ErrMissingAPIKey, got: %v", err)
	}
}

func TestListTransactions_WithFilters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("symbol") != "BTC" {
			t.Errorf("symbol = %q, want BTC", q.Get("symbol"))
		}
		if q.Get("transaction_type") != "transfer" {
			t.Errorf("transaction_type = %q, want transfer", q.Get("transaction_type"))
		}
		if q.Get("order") != "desc" {
			t.Errorf("order = %q, want desc", q.Get("order"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"transactions":[],"next":""}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Transactions.ListTransactions(context.Background(), "bitcoin", TransactionOptions{
		StartHeight:     100,
		Symbol:          "BTC",
		TransactionType: "transfer",
		Order:           "desc",
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
}

func TestListTransactionsNext_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"transactions":[],"next":""}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, WithBaseURL(srv.URL))
	nextURL := srv.URL + "/bitcoin/transactions?start_height=100&start_index=200&limit=100"
	page, err := c.Transactions.ListTransactionsNext(context.Background(), nextURL)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if page == nil {
		t.Fatal("page should not be nil")
	}
}

func TestListTransactionsNext_UnsafeURL(t *testing.T) {
	c := NewClient("key", WithBaseURL("https://leviathan.whale-alert.io"))
	_, err := c.Transactions.ListTransactionsNext(context.Background(), "https://evil.example.com/transactions")
	if err == nil {
		t.Fatal("expected error for unsafe URL")
	}
}

func TestListTransactionsNext_EmptyURL(t *testing.T) {
	c := NewClient("key")
	_, err := c.Transactions.ListTransactionsNext(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
}
