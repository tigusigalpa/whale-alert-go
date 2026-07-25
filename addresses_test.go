package whalealert

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetAddressTransactions_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bitcoin/address/17BkMzRJt3XjEqp3TTmjmoj4dCZdjB9NEk/transactions" {
			t.Errorf("path = %q, want /bitcoin/address/17BkMzRJt3XjEqp3TTmjmoj4dCZdjB9NEk/transactions", r.URL.Path)
		}
		if got := r.URL.Query().Get("api_key"); got != "test-api-key" {
			t.Errorf("api_key = %q, want test-api-key", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"transactions": [{"height":100,"hash":"0xabc","fee":"0.001","fee_symbol":"BTC","fee_symbol_price":50000,"sub_transactions":[]}],
			"next": ""
		}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	page, err := c.Addresses.GetAddressTransactions(context.Background(), "bitcoin", "17BkMzRJt3XjEqp3TTmjmoj4dCZdjB9NEk", AddressTransactionOptions{
		Limit: 50,
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(page.Transactions) != 1 {
		t.Fatalf("transactions len = %d, want 1", len(page.Transactions))
	}
}

func TestGetAddressTransactions_MissingAPIKey(t *testing.T) {
	c := NewClient("")
	_, err := c.Addresses.GetAddressTransactions(context.Background(), "bitcoin", "addr", AddressTransactionOptions{})
	if !errors.Is(err, ErrMissingAPIKey) {
		t.Errorf("error should be ErrMissingAPIKey, got: %v", err)
	}
}

func TestGetAddressTransactions_EmptyBlockchain(t *testing.T) {
	c := NewClient("key")
	_, err := c.Addresses.GetAddressTransactions(context.Background(), "", "addr", AddressTransactionOptions{})
	if err == nil {
		t.Fatal("expected error for empty blockchain")
	}
}

func TestGetAddressTransactions_EmptyAddress(t *testing.T) {
	c := NewClient("key")
	_, err := c.Addresses.GetAddressTransactions(context.Background(), "bitcoin", "", AddressTransactionOptions{})
	if err == nil {
		t.Fatal("expected error for empty address")
	}
}

func TestGetAddressTransactions_WithFilters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("symbol") != "BTC" {
			t.Errorf("symbol = %q, want BTC", q.Get("symbol"))
		}
		if q.Get("limit") != "10" {
			t.Errorf("limit = %q, want 10", q.Get("limit"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"transactions":[],"next":""}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Addresses.GetAddressTransactions(context.Background(), "bitcoin", "addr", AddressTransactionOptions{
		Symbol: "BTC",
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
}

func TestGetAddressTransactionsNext_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"transactions":[],"next":""}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, WithBaseURL(srv.URL))
	nextURL := srv.URL + "/bitcoin/address/abc/transactions?start_index=100"
	page, err := c.Addresses.GetAddressTransactionsNext(context.Background(), nextURL)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if page == nil {
		t.Fatal("page should not be nil")
	}
}

func TestGetAddressTransactionsNext_UnsafeURL(t *testing.T) {
	c := NewClient("key", WithBaseURL("https://leviathan.whale-alert.io"))
	_, err := c.Addresses.GetAddressTransactionsNext(context.Background(), "https://evil.example.com/transactions")
	if err == nil {
		t.Fatal("expected error for unsafe URL")
	}
}

func TestGetAddressTransactionsNext_EmptyURL(t *testing.T) {
	c := NewClient("key")
	_, err := c.Addresses.GetAddressTransactionsNext(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
}
