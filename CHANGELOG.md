# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- `websocket.Client` now actually sends periodic ping control frames at
  `Config.PingInterval` and installs a pong handler that refreshes the read
  deadline, so dead connections are detected as documented. Previously
  `PingInterval` was accepted but never used.
- `websocket.Client.Listen` no longer surfaces a spurious transport error
  when the caller calls `Close` while a read is in progress; the read loop
  now recognizes the closed `done` channel and returns `nil` for a clean
  shutdown.
- The read deadline is now refreshed after every successfully read message
  (previously it was only set once at the start of the read loop).

### Removed

- Removed the unused `jsonNumber` helper type from `models.go` (dead code).

## [1.0.0] - 2024-01-01

### Added

- REST API client with full endpoint coverage:
  - `GET /status` — supported blockchains (public, no API key)
  - `GET /{blockchain}/status` — blockchain availability window
  - `GET /{blockchain}/transaction/{hash}` — single transaction lookup
  - `GET /{blockchain}/transactions` — paginated transaction listing
  - `GET /{blockchain}/block/{height}` — block at specific height
  - `GET /{blockchain}/address/{hash}/transactions` — address transactions (last 30 days)
- WebSocket client with subscription management:
  - `subscribe_alerts` with filter validation (min_value_usd >= 100,000)
  - `subscribe_socials` for social media alerts
  - Event decoding: alerts, socials, subscription confirmations, errors
  - Automatic reconnection with same subscription ID for 5-minute recovery
  - Configurable exponential backoff for reconnection
  - Graceful shutdown with close message
- Typed `APIError` with sentinel errors (`ErrUnauthorized`, `ErrRateLimited`, etc.)
- `errors.Is` and `errors.As` compatibility for programmatic error handling
- Configurable retry policy for idempotent GET requests (429, 5xx)
- `Retry-After` header parsing (seconds and HTTP-date formats)
- Pagination with typed page objects and lazy `TransactionIterator`
- Safe next-URL following with base origin validation
- Financial precision: amounts and fees as strings, `json.Number` for prices
- API key security: redacted URLs in request hooks, no key in error messages
- `context.Context` support for all operations
- Concurrency-safe client (safe for concurrent goroutines)
- Functional options pattern for client configuration
- Runnable examples for REST and WebSocket APIs
- Comprehensive test suite with `httptest.Server` and WebSocket test server
- MIT license
