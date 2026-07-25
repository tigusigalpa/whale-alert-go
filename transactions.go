package whalealert

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// TransactionsService provides access to transaction-related endpoints.
type TransactionsService struct {
	client *Client
}

// TransactionOptions controls the query parameters for listing transactions.
type TransactionOptions struct {
	StartHeight     int64  // Required: starting block height
	Symbol          string // Optional: filter by symbol (e.g. "BTC")
	TransactionType string // Optional: filter by transaction type (e.g. "transfer")
	Limit           int    // Optional: max results per page
	StartIndex      int    // Optional: pagination offset within the page
	Order           string // Optional: "asc" or "desc"
	Format          string // Optional: response format
}

// GetTransaction returns a single transaction by its hash.
//
// GET /{blockchain}/transaction/{hash}
// https://developer.whale-alert.io/api-account/documentation#v2-transaction
func (s *TransactionsService) GetTransaction(ctx context.Context, blockchain, hash string) (*Transaction, error) {
	if s.client.apiKey == "" {
		return nil, ErrMissingAPIKey
	}
	if blockchain == "" {
		return nil, fmt.Errorf("whalealert: blockchain is required")
	}
	if hash == "" {
		return nil, fmt.Errorf("whalealert: hash is required")
	}

	path := fmt.Sprintf("/%s/transaction/%s", url.PathEscape(blockchain), url.PathEscape(hash))
	var result Transaction
	if err := s.client.doRequest(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListTransactions returns a page of transactions starting at the given height.
//
// GET /{blockchain}/transactions
// https://developer.whale-alert.io/api-account/documentation#v2-transactions
func (s *TransactionsService) ListTransactions(ctx context.Context, blockchain string, opts TransactionOptions) (*TransactionPage, error) {
	if s.client.apiKey == "" {
		return nil, ErrMissingAPIKey
	}
	if blockchain == "" {
		return nil, fmt.Errorf("whalealert: blockchain is required")
	}
	if opts.StartHeight == 0 {
		return nil, fmt.Errorf("whalealert: start_height is required")
	}

	params := url.Values{}
	params.Set("start_height", strconv.FormatInt(opts.StartHeight, 10))
	if opts.Symbol != "" {
		params.Set("symbol", opts.Symbol)
	}
	if opts.TransactionType != "" {
		params.Set("transaction_type", opts.TransactionType)
	}
	if opts.Limit > 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.StartIndex > 0 {
		params.Set("start_index", strconv.Itoa(opts.StartIndex))
	}
	if opts.Order != "" {
		params.Set("order", opts.Order)
	}
	if opts.Format != "" {
		params.Set("format", opts.Format)
	}

	path := fmt.Sprintf("/%s/transactions", url.PathEscape(blockchain))
	var result TransactionPage
	if err := s.client.doRequest(ctx, "GET", path, params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListTransactionsNext fetches the next page of transactions using the
// provider-supplied next URL. The URL is validated against the configured
// base URL to prevent following unsafe external URLs.
func (s *TransactionsService) ListTransactionsNext(ctx context.Context, nextURL string) (*TransactionPage, error) {
	if s.client.apiKey == "" {
		return nil, ErrMissingAPIKey
	}
	if nextURL == "" {
		return nil, fmt.Errorf("whalealert: next URL is required")
	}

	safeURL, err := s.client.validateNextURL(nextURL)
	if err != nil {
		return nil, err
	}

	req, err := newGetRequest(ctx, s.client, safeURL)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("whalealert: request failed: %w", err)
	}
	defer resp.Body.Close()

	result, err := decodeResponse[TransactionPage](resp)
	if err != nil {
		return nil, err
	}
	return &result, nil
}
