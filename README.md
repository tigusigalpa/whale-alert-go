# Whale Alert Golang SDK

![Whale Alert Golang SDK](https://i.postimg.cc/nhJLL6Jr/whale-alert-golang.jpg)

[![Go Reference](https://pkg.go.dev/badge/github.com/tigusigalpa/whale-alert-go.svg)](https://pkg.go.dev/github.com/tigusigalpa/whale-alert-go)
[![CI](https://github.com/tigusigalpa/whale-alert-go/actions/workflows/ci.yml/badge.svg)](https://github.com/tigusigalpa/whale-alert-go/actions/workflows/ci.yml)
[![CodeQL](https://github.com/tigusigalpa/whale-alert-go/actions/workflows/codeql.yml/badge.svg)](https://github.com/tigusigalpa/whale-alert-go/actions/workflows/codeql.yml)
[![Codecov](https://codecov.io/gh/tigusigalpa/whale-alert-go/graph/badge.svg)](https://codecov.io/gh/tigusigalpa/whale-alert-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/tigusigalpa/whale-alert-go)](https://goreportcard.com/report/github.com/tigusigalpa/whale-alert-go)
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

An unofficial Go client library for the [Whale Alert Enterprise API](https://developer.whale-alert.io/api-account/documentation).

> **Disclaimer:** This is an unofficial SDK and is not affiliated with, endorsed by, or sponsored by Whale Alert. All product names, logos, and brands are property of their respective owners.

## What is this?

This library makes it easy to talk to the Whale Alert Enterprise API from Go. Whether you need to check which blockchains are supported, query transactions and blocks, or listen to real-time whale movements over WebSocket, the client gives you typed, idiomatic Go methods with sane defaults and robust error handling.

It is built around a few ideas that should feel familiar to Go developers:

- Everything accepts `context.Context` for timeouts and cancellation.
- The client is safe to share across goroutines.
- Monetary values stay as strings so you never lose precision.
- Retries, pagination, and connection recovery are opt-in but easy to enable.

## Features

- **REST API**: Full coverage of the documented endpoints — status, blockchain status, transactions, blocks, and address transactions.
- **WebSocket API**: Real-time alerts and socials with subscription management, automatic reconnection, ping/pong keep-alive, and event decoding.
- **Typed models**: Strongly-typed structs for every API response, so your editor can help you explore the data.
- **Financial precision**: Amounts and fees are kept as `string` to avoid the rounding issues that come from `float64`.
- **Retry policy**: Configurable exponential backoff with jitter for idempotent GET requests when the API returns 429 or 5xx errors.
- **Pagination**: Typed page objects with a lazy iterator and safe next-URL following. The library validates next URLs against the configured base origin so you cannot accidentally follow a malicious link.
- **Error handling**: A typed `APIError` plus sentinel errors (`ErrUnauthorized`, `ErrRateLimited`, and others) that work with `errors.Is` and `errors.As`.
- **Context support**: Every REST call accepts `context.Context` for deadlines, cancellation, and request-scoped values.
- **Concurrency safe**: Create one client and reuse it across multiple goroutines without extra synchronization.
- **API key security**: Your API key is never logged. Request hooks receive URLs with the key redacted.

## Installation

Add the module to your project with `go get`:

```bash
go get github.com/tigusigalpa/whale-alert-go
```

Then import the package in your Go files:

```go
import whalealert "github.com/tigusigalpa/whale-alert-go"
```

## Quick Start

### REST API

This example shows how to create a client, call a public endpoint that does not require an API key, and then call an authenticated endpoint that does:

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "time"

    whalealert "github.com/tigusigalpa/whale-alert-go"
)

func main() {
    // Read the API key from the environment. Keep secrets out of source code.
    apiKey := os.Getenv("WHALE_ALERT_API_KEY")

    // Create a client with retries enabled: up to 3 attempts, starting at
    // 500ms and capped at 10s. Retries only apply to idempotent GET requests.
    client := whalealert.NewClient(apiKey,
        whalealert.WithRetry(3, 500*time.Millisecond, 10*time.Second),
    )

    // Public endpoint — no API key required.
    // Returns the list of blockchains Whale Alert supports.
    chains, err := client.Status.GetSupportedBlockchains(context.Background())
    if err != nil {
        log.Fatal(err)
    }
    for _, c := range chains {
        fmt.Printf("%s: %v\n", c.Name, c.Symbols)
    }

    // Authenticated endpoint — requires a valid API key.
    // Returns the current sync status for Ethereum.
    status, err := client.Status.GetBlockchainStatus(context.Background(), "ethereum")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Ethereum: %d-%d (%d blocks)\n", status.StartHeight, status.EndHeight, status.BlockCount)
}
```

### WebSocket API

The WebSocket client streams real-time alerts. You register message and error handlers, connect, subscribe, and then call `Listen` to enter the read loop. Automatic reconnection is opt-in: set `MaxAttempts` greater than zero.

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "time"

    "github.com/tigusigalpa/whale-alert-go/websocket"
)

func main() {
    apiKey := os.Getenv("WHALE_ALERT_API_KEY")
    wsURL := fmt.Sprintf("wss://leviathan.whale-alert.io/ws?api_key=%s", apiKey)

    // Configure the WebSocket client. Reconnection starts at 1 second
    // and doubles each attempt up to the 30 second cap.
    client := websocket.NewClient(websocket.Config{
        URL: wsURL,
        Reconnect: websocket.ReconnectConfig{
            MaxAttempts:  5,
            InitialDelay: 1 * time.Second,
            MaxDelay:     30 * time.Second,
        },
    })

    // Called for every decoded message.
    client.OnMessage(func(msg websocket.Message) {
        if msg.EventType == websocket.EventTypeAlert && msg.Alert != nil {
            fmt.Printf("[ALERT] %s: %s\n", msg.Alert.Blockchain, msg.Alert.Text)
        }
    })

    // Called when a non-fatal error happens, such as a temporary disconnect.
    client.OnError(func(err error) {
        log.Printf("Error: %v", err)
    })

    ctx := context.Background()
    if err := client.Connect(ctx); err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Subscribe to Ethereum whale alerts worth at least $500,000.
    if err := client.SubscribeAlerts(ctx, websocket.AlertSubscription{
        ID:           "my-sub",
        Blockchains:  []string{"ethereum"},
        MinValueUSD:  500000,
    }); err != nil {
        log.Fatal(err)
    }

    // Listen blocks until the connection closes or the context is cancelled.
    if err := client.Listen(ctx); err != nil {
        log.Fatal(err)
    }
}
```

## Configuration

### Client Options

You can customize the REST client through functional options passed to `NewClient`:

| Option | Description | Default |
|--------|-------------|---------|
| `WithBaseURL(url)` | Override the API base URL | `https://leviathan.whale-alert.io` |
| `WithHTTPClient(hc)` | Use a custom `*http.Client` | Default client with a 30s timeout |
| `WithTimeout(d)` | Set the HTTP client timeout | 30s |
| `WithUserAgent(ua)` | Override the User-Agent header | `whale-alert-go/1.0.0` |
| `WithRetry(n, init, max)` | Enable retries with exponential backoff | 0 retries (disabled) |
| `WithRequestHook(h)` | Add a hook called before each request | none |

### Retry Policy

Retries are **disabled by default**. Enable them with `WithRetry`:

```go
client := whalealert.NewClient(apiKey,
    whalealert.WithRetry(3, 500*time.Millisecond, 10*time.Second),
)
```

The retry policy is designed to be safe and predictable:

- Only idempotent GET requests are retried. State-changing operations are never retried automatically.
- Retries happen on HTTP 429 (rate limited) and 5xx server errors.
- The 429 response can include a `Retry-After` header. When present, the client waits at least that long before the next attempt.
- Backoff is exponential: `initialDelay * 2^attempt`, capped at `maxDelay`.
- Context cancellation is respected during the backoff sleep, so a cancelled request stops immediately.

## Error Handling

Every API error is wrapped in an `APIError` value. You can inspect the HTTP status code and message, or use sentinel errors for common cases.

```go
status, err := client.Status.GetBlockchainStatus(ctx, "ethereum")
if err != nil {
    var apiErr *whalealert.APIError
    if errors.As(err, &apiErr) {
        fmt.Printf("Status: %d, Message: %s\n", apiErr.StatusCode, apiErr.Message)
    }

    if errors.Is(err, whalealert.ErrUnauthorized) {
        // Handle invalid API key — check that WHALE_ALERT_API_KEY is set correctly.
    }
    if errors.Is(err, whalealert.ErrRateLimited) {
        // Handle rate limiting — you may want to back off or inspect apiErr.RetryAfter.
    }
}
```

### Sentinel Errors

| Error | HTTP Status | Typical cause |
|-------|-------------|---------------|
| `ErrBadRequest` | 400 | Malformed request parameters |
| `ErrUnauthorized` | 401 | Missing or invalid API key |
| `ErrForbidden` | 403 | Insufficient permissions |
| `ErrNotFound` | 404 | Unknown blockchain, transaction, or block |
| `ErrValidation` | 422 | Parameter validation failure |
| `ErrRateLimited` | 429 | Too many requests |
| `ErrProviderAPI` | Any 4xx/5xx | Catch-all for other provider errors |

## Pagination

List endpoints return a `TransactionPage` that includes the current slice of transactions and an optional `Next` URL. You have two ways to move through pages.

### Lazy iterator

The iterator handles page fetching for you, stopping when there are no more results or when an error occurs:

```go
page, err := client.Transactions.ListTransactions(ctx, "ethereum", whalealert.TransactionOptions{
    StartHeight: 768801,
    Limit:       100,
})
if err != nil {
    log.Fatal(err)
}

iter := whalealert.NewTransactionIterator(ctx, client, page)
for iter.HasNext() {
    tx, err := iter.Next()
    if err != nil {
        log.Printf("pagination error: %v", err)
        break
    }
    fmt.Printf("tx: %s\n", tx.Hash)
}
```

### Manual next-page fetching

If you prefer to control pagination yourself, use the `Next` URL directly. The client validates that the URL shares the same origin as the configured base URL before sending the request.

```go
if page.Next != "" {
    nextPage, err := client.Transactions.ListTransactionsNext(ctx, page.Next)
    if err != nil {
        log.Fatal(err)
    }
    // process nextPage...
}
```

Address transaction pagination is available through `GetAddressTransactionsNext`.

## Financial Precision

Cryptocurrency amounts can be very small or very large, and `float64` cannot represent them exactly. For that reason, all monetary fields in this library (`fee`, `amount` in addresses, and similar) are kept as `string`.

Keep them as strings for display or pass them to a decimal-arithmetic package such as `shopspring/decimal`. Only convert to `float64` if you fully understand the precision implications.

## API Reference

The client is organized into services that mirror the API endpoints.

- **Status**
  - `GetSupportedBlockchains()` — `GET /status` (public, no key needed)
  - `GetBlockchainStatus(blockchain)` — `GET /{blockchain}/status`
- **Transactions**
  - `GetTransaction(blockchain, hash)` — `GET /{blockchain}/transaction/{hash}`
  - `ListTransactions(blockchain, opts)` — `GET /{blockchain}/transactions`
  - `ListTransactionsNext(nextURL)` — Follow a pagination URL returned by a previous list call
- **Blocks**
  - `GetBlock(blockchain, height)` — `GET /{blockchain}/block/{height}`
- **Addresses**
  - `GetAddressTransactions(blockchain, address, opts)` — `GET /{blockchain}/address/{hash}/transactions`
  - `GetAddressTransactionsNext(nextURL)` — Follow a pagination URL for address transactions

For the official API documentation, visit https://developer.whale-alert.io/api-account/documentation.

## Examples

Runnable examples are in the `examples/` directory:

- `examples/rest/` — REST API usage
- `examples/websocket/` — WebSocket alerts subscription

Run them from the repository root:

```bash
WHALE_ALERT_API_KEY=your-key go run examples/rest/main.go
WHALE_ALERT_API_KEY=your-key go run examples/websocket/main.go
```

## Testing

The project includes unit tests for the REST client, WebSocket client, pagination helpers, and error handling. Run the full suite with:

```bash
go test ./...
go test -race ./...
```

You can also run the standard Go quality checks:

```bash
go vet ./...
gofmt -l .
```

## License

MIT — see [LICENSE](LICENSE)

## Author

Igor Sazonov — [github.com/tigusigalpa](https://github.com/tigusigalpa)
