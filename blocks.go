package whalealert

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// BlocksService provides access to block-related endpoints.
type BlocksService struct {
	client *Client
}

// GetBlock returns a block at a specific height.
//
// GET /{blockchain}/block/{height}
// https://developer.whale-alert.io/api-account/documentation#v2-block
func (s *BlocksService) GetBlock(ctx context.Context, blockchain string, height int64) (*Block, error) {
	if s.client.apiKey == "" {
		return nil, ErrMissingAPIKey
	}
	if blockchain == "" {
		return nil, fmt.Errorf("whalealert: blockchain is required")
	}
	if height <= 0 {
		return nil, fmt.Errorf("whalealert: height must be positive")
	}

	path := fmt.Sprintf("/%s/block/%s", url.PathEscape(blockchain), strconv.FormatInt(height, 10))
	var result Block
	if err := s.client.doRequest(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
