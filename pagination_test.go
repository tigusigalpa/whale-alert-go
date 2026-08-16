package whalealert

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateNextURL_SafeURL(t *testing.T) {
	c := NewClient("key", WithBaseURL("https://leviathan.whale-alert.io"))
	safe, err := c.validateNextURL("https://leviathan.whale-alert.io/bitcoin/transactions?start_height=100")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if safe == "" {
		t.Error("safe URL should not be empty")
	}
}

func TestValidateNextURL_UnsafeHost(t *testing.T) {
	c := NewClient("key", WithBaseURL("https://leviathan.whale-alert.io"))
	_, err := c.validateNextURL("https://evil.example.com/transactions")
	if err == nil {
		t.Fatal("expected error for unsafe host")
	}
}

func TestValidateNextURL_UnsafeScheme(t *testing.T) {
	c := NewClient("key", WithBaseURL("https://leviathan.whale-alert.io"))
	_, err := c.validateNextURL("http://leviathan.whale-alert.io/transactions")
	if err == nil {
		t.Fatal("expected error for unsafe scheme")
	}
}

func TestValidateNextURL_AppendsAPIKey(t *testing.T) {
	c := NewClient("my-key", WithBaseURL("https://leviathan.whale-alert.io"))
	safe, err := c.validateNextURL("https://leviathan.whale-alert.io/bitcoin/transactions?start_height=100")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !contains(safe, "api_key=my-key") {
		t.Errorf("safe URL should contain api_key, got: %s", safe)
	}
}

func TestValidateNextURL_InvalidURL(t *testing.T) {
	c := NewClient("key", WithBaseURL("https://leviathan.whale-alert.io"))
	_, err := c.validateNextURL("://invalid")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestTransactionIterator_SinglePage(t *testing.T) {
	page := &TransactionPage{
		Transactions: []Transaction{
			{Hash: "tx1"},
			{Hash: "tx2"},
		},
		Next: "",
	}

	iter := NewTransactionIterator(context.Background(), NewClient("key"), page)

	if !iter.HasNext() {
		t.Fatal("HasNext should be true")
	}

	tx1, err := iter.Next()
	if err != nil {
		t.Fatalf("Next error: %v", err)
	}
	if tx1.Hash != "tx1" {
		t.Errorf("Hash = %q, want tx1", tx1.Hash)
	}

	tx2, err := iter.Next()
	if err != nil {
		t.Fatalf("Next error: %v", err)
	}
	if tx2.Hash != "tx2" {
		t.Errorf("Hash = %q, want tx2", tx2.Hash)
	}

	_, err = iter.Next()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("Next error = %v, want io.EOF", err)
	}
}

func TestTransactionIterator_MultiplePages(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"transactions": [{"height":1,"hash":"tx3","fee":"0","fee_symbol":"BTC","fee_symbol_price":0,"sub_transactions":[]}],
				"next": ""
			}`))
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv)

	page := &TransactionPage{
		Transactions: []Transaction{
			{Hash: "tx1"},
			{Hash: "tx2"},
		},
		Next: srv.URL + "/bitcoin/transactions?start_height=100&start_index=2",
	}

	iter := NewTransactionIterator(context.Background(), c, page)

	tx1, _ := iter.Next()
	if tx1.Hash != "tx1" {
		t.Errorf("Hash = %q, want tx1", tx1.Hash)
	}

	tx2, _ := iter.Next()
	if tx2.Hash != "tx2" {
		t.Errorf("Hash = %q, want tx2", tx2.Hash)
	}

	tx3, err := iter.Next()
	if err != nil {
		t.Fatalf("Next error: %v", err)
	}
	if tx3.Hash != "tx3" {
		t.Errorf("Hash = %q, want tx3", tx3.Hash)
	}

	_, err = iter.Next()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("Next error = %v, want io.EOF", err)
	}
}

func TestTransactionIterator_HasNext(t *testing.T) {
	page := &TransactionPage{
		Transactions: []Transaction{{Hash: "tx1"}},
		Next:         "some-url",
	}
	iter := NewTransactionIterator(context.Background(), NewClient("key"), page)

	if !iter.HasNext() {
		t.Error("HasNext should be true with items in page")
	}

	_, _ = iter.Next()

	if !iter.HasNext() {
		t.Error("HasNext should be true with next URL available")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
