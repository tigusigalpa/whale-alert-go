package websocket

import (
	"errors"
	"fmt"
)

// Sentinel errors for WebSocket operations.
var (
	ErrConnectionClosed   = errors.New("whalealert: websocket connection closed")
	ErrNotConnected       = errors.New("whalealert: websocket not connected")
	ErrSubscriptionFailed = errors.New("whalealert: subscription failed")
	ErrInvalidFilter      = errors.New("whalealert: invalid subscription filter")
	ErrMaxReconnects      = errors.New("whalealert: maximum reconnection attempts exceeded")
)

// WSError wraps a provider error message.
type WSError struct {
	Message string
}

func (e *WSError) Error() string {
	return fmt.Sprintf("whalealert: websocket error: %s", e.Message)
}

// Is reports whether the WebSocket error represents a subscription failure.
func (e *WSError) Is(target error) bool {
	return target == ErrSubscriptionFailed
}
