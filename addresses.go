package whalealert

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// AddressesService provides access to address-related endpoints.
type AddressesService struct {
	client *Client
}

// AddressTransactionOptions controls the query parameters for listing
// address transactions.
type AddressTransactionOptions struct {
	Symbol          string // Optional: filter by symbol
	TransactionType string // Optional: filter by transaction type
	Limit           int    // Optional: max results per page
	StartIndex      int    // Optional: pagination offset within the page
	Order           string // Optional: "asc" or "desc"
}

// GetAddressTransactions returns transactions for an address from the last
// 30 days.
//
// GET /{blockchain}/address/{hash}/transactions
// https://developer.whale-alert.io/api-account/documentation#v2-address
func (s *AddressesService) GetAddressTransactions(ctx context.Context, blockchain, address string, opts AddressTransactionOptions) (*AddressTransactionPage, error) {
	if s.client.apiKey == "" {
		return nil, ErrMissingAPIKey
	}
	if blockchain == "" {
		return nil, fmt.Errorf("whalealert: blockchain is required")
	}
	if address == "" {
		return nil, fmt.Errorf("whalealert: address is required")
	}

	params := url.Values{}
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

	path := fmt.Sprintf("/%s/address/%s/transactions", url.PathEscape(blockchain), url.PathEscape(address))
	var result AddressTransactionPage
	if err := s.client.doRequest(ctx, "GET", path, params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetAddressTransactionsNext fetches the next page of address transactions
// using the provider-supplied next URL.
func (s *AddressesService) GetAddressTransactionsNext(ctx context.Context, nextURL string) (*AddressTransactionPage, error) {
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

	result, err := decodeResponse[AddressTransactionPage](resp)
	if err != nil {
		return nil, err
	}
	return &result, nil
}
