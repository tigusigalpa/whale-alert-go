package whalealert

import (
	"encoding/json"
	"fmt"
)

// decodeJSON decodes a JSON byte slice into the provided target.
func decodeJSON(data []byte, v interface{}) error {
	if len(data) == 0 {
		return fmt.Errorf("empty response body")
	}
	return json.Unmarshal(data, v)
}

// Blockchain represents a supported blockchain and its symbols.
type Blockchain struct {
	Name    string   `json:"name"`
	Symbols []string `json:"symbols"`
}

// BlockchainStatus represents the availability window of a blockchain.
type BlockchainStatus struct {
	StartHeight int64 `json:"start_height"`
	EndHeight   int64 `json:"end_height"`
	BlockCount  int64 `json:"block_count"`
}

// Address represents an input or output address in a sub-transaction.
// Amount is kept as a string to preserve provider-provided precision.
type Address struct {
	Amount  string `json:"amount"`
	Address string `json:"address"`
	Owner   string `json:"owner,omitempty"`
}

// SubTransaction represents a single currency/type split within a transaction.
type SubTransaction struct {
	Symbol          string    `json:"symbol"`
	TransactionType string    `json:"transaction_type"`
	Inputs          []Address `json:"inputs"`
	Outputs         []Address `json:"outputs"`
}

// Transaction represents a normalized blockchain transaction.
// Fee and fee_symbol_price may be strings or numbers from the provider;
// fee is always kept as a string to preserve precision.
type Transaction struct {
	Height          int64            `json:"height"`
	IndexInBlock    int64            `json:"index_in_block"`
	Timestamp       int64            `json:"timestamp"`
	Hash            string           `json:"hash"`
	Fee             string           `json:"fee"`
	FeeSymbol       string           `json:"fee_symbol"`
	FeeSymbolPrice  json.Number      `json:"fee_symbol_price"`
	SubTransactions []SubTransaction `json:"sub_transactions"`
}

// Block represents a block at a specific height.
type Block struct {
	Timestamp    int64         `json:"timestamp"`
	Hash         string        `json:"hash"`
	Transactions []Transaction `json:"transactions"`
}

// TransactionPage represents a page of transactions with a next URL.
type TransactionPage struct {
	Transactions []Transaction `json:"transactions"`
	Next         string        `json:"next"`
}

// AddressTransactionPage represents a page of address transactions with a next URL.
type AddressTransactionPage struct {
	Transactions []Transaction `json:"transactions"`
	Next         string        `json:"next"`
}

// BlockPage represents a block response that includes transactions and a next URL.
type BlockPage struct {
	Timestamp    int64         `json:"timestamp"`
	Hash         string        `json:"hash"`
	Transactions []Transaction `json:"transactions"`
	Next         string        `json:"next"`
}
