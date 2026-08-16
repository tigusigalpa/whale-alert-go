package whalealert

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// validateNextURL ensures the given next URL resolves to the configured
// Whale Alert base origin. This prevents following unsafe external URLs.
func (c *Client) validateNextURL(nextURL string) (string, error) {
	parsed, err := url.Parse(nextURL)
	if err != nil {
		return "", fmt.Errorf("whalealert: invalid next URL: %w", err)
	}

	baseParsed, baseErr := url.Parse(c.baseURL)
	if baseErr != nil {
		return "", fmt.Errorf("whalealert: invalid base URL configuration: %w", baseErr)
	}

	if parsed.Host != baseParsed.Host {
		return "", fmt.Errorf("whalealert: next URL host %q does not match base URL host %q", parsed.Host, baseParsed.Host)
	}

	if !strings.HasPrefix(parsed.Scheme+"://", baseParsed.Scheme+"://") {
		return "", fmt.Errorf("whalealert: next URL scheme %q does not match base URL scheme %q", parsed.Scheme, baseParsed.Scheme)
	}

	if c.apiKey != "" {
		q := parsed.Query()
		q.Set("api_key", c.apiKey)
		parsed.RawQuery = q.Encode()
	}

	return parsed.String(), nil
}

// TransactionIterator provides lazy iteration over paginated transaction
// results. It fetches the next page only when the consumer advances it.
// Do not retain every page in memory; process items as they arrive.
type TransactionIterator struct {
	client    *Client
	ctx       context.Context
	nextURL   string
	page      *TransactionPage
	index     int
	exhausted bool
}

// NewTransactionIterator creates an iterator from an initial page.
func NewTransactionIterator(ctx context.Context, client *Client, page *TransactionPage) *TransactionIterator {
	return &TransactionIterator{
		client:  client,
		ctx:     ctx,
		page:    page,
		nextURL: page.Next,
		index:   0,
	}
}

// Next advances the iterator and returns the next transaction. It returns
// io.EOF when all pages are exhausted. It fetches the next page lazily.
func (it *TransactionIterator) Next() (*Transaction, error) {
	if it.index < len(it.page.Transactions) {
		tx := &it.page.Transactions[it.index]
		it.index++
		return tx, nil
	}

	if it.exhausted || it.nextURL == "" {
		it.exhausted = true
		return nil, io.EOF
	}

	nextPage, err := it.client.Transactions.ListTransactionsNext(it.ctx, it.nextURL)
	if err != nil {
		return nil, err
	}

	it.page = nextPage
	it.nextURL = nextPage.Next
	it.index = 0

	if len(it.page.Transactions) == 0 {
		it.exhausted = true
		return nil, io.EOF
	}

	tx := &it.page.Transactions[it.index]
	it.index++
	return tx, nil
}

// HasNext returns true if there are more transactions to iterate,
// either in the current page or via the next URL.
func (it *TransactionIterator) HasNext() bool {
	return it.index < len(it.page.Transactions) || (!it.exhausted && it.nextURL != "")
}
