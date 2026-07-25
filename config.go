package whalealert

import "time"

// RetryConfig controls the retry behavior for idempotent GET requests.
type RetryConfig struct {
	// MaxAttempts is the maximum number of retry attempts (0 = no retries).
	MaxAttempts int
	// InitialDelay is the delay before the first retry.
	InitialDelay time.Duration
	// MaxDelay caps the exponential backoff delay.
	MaxDelay time.Duration
}
