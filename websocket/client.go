package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultWSTimeout    = 30 * time.Second
	defaultPingInterval = 30 * time.Second
	defaultWriteTimeout = 10 * time.Second
	defaultReadTimeout  = 60 * time.Second
)

// MessageHandler is called for each decoded WebSocket message.
type MessageHandler func(msg Message)

// ErrorHandler is called when a terminal or provider error occurs.
type ErrorHandler func(err error)

// ReconnectConfig controls automatic reconnection behavior.
// Reconnection is opt-in: set MaxAttempts > 0 to enable.
type ReconnectConfig struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
}

// Config holds all WebSocket client configuration.
type Config struct {
	URL          string
	Timeout      time.Duration
	PingInterval time.Duration
	Reconnect    ReconnectConfig
}

// Client manages a WebSocket connection to the Whale Alert alerts API.
// It is safe for concurrent use. Automatic reconnection is opt-in.
type Client struct {
	config Config
	conn   *websocket.Conn
	mu     sync.Mutex
	// writeMu serializes all writes. gorilla/websocket permits one concurrent
	// reader and one concurrent writer, but does not permit concurrent writers.
	writeMu    sync.Mutex
	done       chan struct{}
	running    bool
	handler    MessageHandler
	errHandler ErrorHandler

	// subscription state for reconnection
	subscriptionID    string
	lastAlertSub      *AlertSubscription
	lastSocialSub     *SocialSubscription
	reconnectAttempts int
}

// NewClient creates a new WebSocket client. The URL should include the
// api_key query parameter.
func NewClient(cfg Config) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultWSTimeout
	}
	if cfg.PingInterval == 0 {
		cfg.PingInterval = defaultPingInterval
	}
	return &Client{
		config: cfg,
		done:   make(chan struct{}),
	}
}

// Connect establishes the WebSocket connection.
func (c *Client) Connect(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return fmt.Errorf("whalealert: already connected")
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: c.config.Timeout,
	}

	conn, _, err := dialer.DialContext(ctx, c.config.URL, nil)
	if err != nil {
		return fmt.Errorf("whalealert: websocket dial: %w", err)
	}

	conn.SetReadDeadline(time.Now().Add(defaultReadTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(defaultReadTimeout))
		return nil
	})

	c.conn = conn
	c.running = true
	c.done = make(chan struct{})

	go c.pingLoop(conn, c.done)

	return nil
}

// pingLoop periodically sends ping control frames to detect dead connections
// and keep the connection alive. It stops when done is closed.
func (c *Client) pingLoop(conn *websocket.Conn, done chan struct{}) {
	ticker := time.NewTicker(c.config.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			// Serialize the connection check and write with Close and send. This
			// prevents a ping from racing a close frame or application message.
			c.writeMu.Lock()
			c.mu.Lock()
			active := c.conn == conn
			c.mu.Unlock()
			if !active {
				c.writeMu.Unlock()
				return
			}
			conn.SetWriteDeadline(time.Now().Add(defaultWriteTimeout))
			err := conn.WriteMessage(websocket.PingMessage, nil)
			c.writeMu.Unlock()
			if err != nil {
				c.fireError(fmt.Errorf("whalealert: ping: %w", err))
				return
			}
		}
	}
}

// SubscribeAlerts sends a subscribe_alerts message and validates the filter.
func (c *Client) SubscribeAlerts(ctx context.Context, sub AlertSubscription) error {
	if err := sub.Validate(); err != nil {
		return err
	}

	c.mu.Lock()
	c.subscriptionID = sub.ID
	c.lastAlertSub = &sub
	c.mu.Unlock()

	return c.send(ctx, &sub)
}

// SubscribeSocials sends a subscribe_socials message.
func (c *Client) SubscribeSocials(ctx context.Context, sub SocialSubscription) error {
	c.mu.Lock()
	c.subscriptionID = sub.ID
	c.lastSocialSub = &sub
	c.mu.Unlock()

	return c.send(ctx, &sub)
}

// OnMessage registers the handler called for each decoded message.
func (c *Client) OnMessage(handler MessageHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handler = handler
}

// OnError registers the handler called for terminal or provider errors.
func (c *Client) OnError(handler ErrorHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.errHandler = handler
}

// Listen starts the read loop. It blocks until the connection is closed
// or the context is cancelled. If reconnection is enabled, it will
// attempt to reconnect and resubscribe with the same subscription ID.
func (c *Client) Listen(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	for {
		if err := c.readLoop(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			c.fireError(err)

			if !c.shouldReconnect() {
				return err
			}

			if err := c.reconnect(ctx); err != nil {
				return err
			}
			continue
		}
		return nil
	}
}

// readLoop reads messages until an error or close occurs.
func (c *Client) readLoop(ctx context.Context) error {
	c.mu.Lock()
	conn := c.conn
	done := c.done
	c.mu.Unlock()

	if conn == nil {
		return ErrNotConnected
	}

	for {
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			select {
			case <-done:
				// Close() was called; treat as a clean shutdown.
				return nil
			default:
			}
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				c.disconnect(conn, done)
				return ErrConnectionClosed
			}
			c.disconnect(conn, done)
			return fmt.Errorf("whalealert: read: %w", err)
		}

		conn.SetReadDeadline(time.Now().Add(defaultReadTimeout))

		msg := decodeMessage(data)

		if msg.EventType == EventTypeError && msg.Error != nil {
			c.fireError(&WSError{Message: msg.Error.Error})
			continue
		}

		c.fireMessage(msg)
	}
}

// disconnect marks conn as inactive and stops its ping loop. It only changes
// state when conn is still the client's current connection.
func (c *Client) disconnect(conn *websocket.Conn, done chan struct{}) {
	c.mu.Lock()
	if c.conn != conn || c.done != done {
		c.mu.Unlock()
		return
	}
	c.running = false
	c.conn = nil
	close(done)
	c.mu.Unlock()

	c.writeMu.Lock()
	_ = conn.Close()
	c.writeMu.Unlock()
}

// shouldReconnect returns true if automatic reconnection is enabled
// and the maximum attempts have not been exhausted.
func (c *Client) shouldReconnect() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.config.Reconnect.MaxAttempts > 0 && c.reconnectAttempts < c.config.Reconnect.MaxAttempts
}

// reconnect attempts to reconnect and resubscribe with the same subscription ID.
func (c *Client) reconnect(ctx context.Context) error {
	c.mu.Lock()
	c.reconnectAttempts++
	attempt := c.reconnectAttempts
	c.mu.Unlock()

	delay := c.backoff(attempt)
	select {
	case <-time.After(delay):
	case <-ctx.Done():
		return ctx.Err()
	}

	if err := c.Connect(ctx); err != nil {
		if attempt >= c.config.Reconnect.MaxAttempts {
			return ErrMaxReconnects
		}
		return c.reconnect(ctx)
	}

	// Resubscribe with the same ID for 5-minute recovery
	c.mu.Lock()
	alertSub := c.lastAlertSub
	socialSub := c.lastSocialSub
	c.mu.Unlock()

	if alertSub != nil {
		if err := c.send(ctx, alertSub); err != nil {
			return fmt.Errorf("whalealert: resubscribe alerts: %w", err)
		}
	}
	if socialSub != nil {
		if err := c.send(ctx, socialSub); err != nil {
			return fmt.Errorf("whalealert: resubscribe socials: %w", err)
		}
	}

	c.mu.Lock()
	c.reconnectAttempts = 0
	c.mu.Unlock()

	return nil
}

func (c *Client) backoff(attempt int) time.Duration {
	delay := c.config.Reconnect.InitialDelay * (1 << (attempt - 1))
	if delay > c.config.Reconnect.MaxDelay || delay <= 0 {
		return c.config.Reconnect.MaxDelay
	}
	return delay
}

// send marshals and sends a JSON message over the WebSocket.
func (c *Client) send(ctx context.Context, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("whalealert: marshal: %w", err)
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return ErrNotConnected
	}

	conn.SetWriteDeadline(time.Now().Add(defaultWriteTimeout))
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("whalealert: write: %w", err)
	}

	return nil
}

// fireMessage calls the message handler if registered.
func (c *Client) fireMessage(msg Message) {
	c.mu.Lock()
	handler := c.handler
	c.mu.Unlock()
	if handler != nil {
		handler(msg)
	}
}

// fireError calls the error handler if registered.
func (c *Client) fireError(err error) {
	c.mu.Lock()
	handler := c.errHandler
	c.mu.Unlock()
	if handler != nil {
		handler(err)
	}
}

// Close gracefully shuts down the WebSocket connection.
func (c *Client) Close() error {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return nil
	}

	c.running = false
	close(c.done)
	conn := c.conn
	c.conn = nil
	c.mu.Unlock()

	if conn != nil {
		c.writeMu.Lock()
		err := conn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		)
		closeErr := conn.Close()
		c.writeMu.Unlock()
		if err != nil {
			return err
		}
		return closeErr
	}

	return nil
}

// IsConnected returns whether the client currently has an active connection.
func (c *Client) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running && c.conn != nil
}

// GetSubscriptionID returns the current subscription ID used for reconnection.
func (c *Client) GetSubscriptionID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.subscriptionID
}
