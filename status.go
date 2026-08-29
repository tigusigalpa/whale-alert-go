package whalealert

import (
	"context"
	"fmt"
	"net/url"
)

// StatusService provides access to status-related endpoints.
type StatusService struct {
	client *Client
}

// GetSupportedBlockchains returns the list of supported blockchains and their
// symbols. This endpoint does not require an API key.
//
// GET /status
// https://developer.whale-alert.io/api-account/documentation#v2-blockchains
func (s *StatusService) GetSupportedBlockchains(ctx context.Context) ([]Blockchain, error) {
	reqURL := s.client.buildURLNoAuth("/status", nil)
	var result []Blockchain
	if err := s.client.doRequestURL(ctx, "GET", reqURL, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetBlockchainStatus returns the availability window for a specific blockchain.
// An API key is required.
//
// GET /{blockchain}/status
// https://developer.whale-alert.io/api-account/documentation#v2-blockchainstatus
func (s *StatusService) GetBlockchainStatus(ctx context.Context, blockchain string) (*BlockchainStatus, error) {
	if s.client.apiKey == "" {
		return nil, ErrMissingAPIKey
	}
	if blockchain == "" {
		return nil, fmt.Errorf("whalealert: blockchain is required")
	}

	path := fmt.Sprintf("/%s/status", url.PathEscape(blockchain))
	var result BlockchainStatus
	if err := s.client.doRequest(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
