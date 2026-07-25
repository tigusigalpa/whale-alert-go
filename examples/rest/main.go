package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	whalealert "github.com/tigusigalpa/whale-alert-go"
)

func main() {
	apiKey := os.Getenv("WHALE_ALERT_API_KEY")
	if apiKey == "" {
		log.Fatal("WHALE_ALERT_API_KEY environment variable is required")
	}

	client := whalealert.NewClient(apiKey)

	// Get supported blockchains (public endpoint, no API key needed)
	chains, err := client.Status.GetSupportedBlockchains(context.Background())
	if err != nil {
		log.Fatalf("GetSupportedBlockchains: %v", err)
	}
	for _, c := range chains {
		fmt.Printf("  %s: %v\n", c.Name, c.Symbols)
	}

	// Get blockchain status
	status, err := client.Status.GetBlockchainStatus(context.Background(), "ethereum")
	if err != nil {
		var apiErr *whalealert.APIError
		if errors.As(err, &apiErr) {
			log.Printf("API error %d: %s", apiErr.StatusCode, apiErr.Message)
		} else {
			log.Printf("Error: %v", err)
		}
	} else {
		fmt.Printf("Ethereum: height %d-%d (%d blocks)\n", status.StartHeight, status.EndHeight, status.BlockCount)
	}

	// List transactions
	page, err := client.Transactions.ListTransactions(context.Background(), "ethereum", whalealert.TransactionOptions{
		StartHeight: status.StartHeight,
		Limit:       100,
	})
	if err != nil {
		log.Fatalf("ListTransactions: %v", err)
	}

	for _, tx := range page.Transactions {
		fmt.Printf("  tx %s: fee=%s %s\n", tx.Hash, tx.Fee, tx.FeeSymbol)
	}

	if page.Next != "" {
		fmt.Println("  Next page available")
	}
}
