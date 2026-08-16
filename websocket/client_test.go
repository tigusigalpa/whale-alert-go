package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// startWSTestServer starts a local WebSocket test server that echoes
// subscription confirmations and optional alert messages.
func startWSTestServer(t *testing.T, handler func(conn *websocket.Conn)) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade error: %v", err)
		}
		defer conn.Close()
		handler(conn)
	}))
	return srv
}

func wsURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func TestAlertSubscription_Validate(t *testing.T) {
	cases := []struct {
		name    string
		sub     AlertSubscription
		wantErr bool
	}{
		{
			name:    "valid subscription",
			sub:     AlertSubscription{Blockchains: []string{"ethereum"}, MinValueUSD: 100000},
			wantErr: false,
		},
		{
			name:    "min_value below 100000",
			sub:     AlertSubscription{Blockchains: []string{"ethereum"}, MinValueUSD: 50000},
			wantErr: true,
		},
		{
			name:    "no filters",
			sub:     AlertSubscription{MinValueUSD: 100000},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.sub.Validate()
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestAlertSubscription_MarshalJSON(t *testing.T) {
	sub := AlertSubscription{
		ID:          "test-id",
		Blockchains: []string{"ethereum"},
		Symbols:     []string{"eth"},
		MinValueUSD: 100000,
	}
	data, err := json.Marshal(&sub)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if raw["type"] != "subscribe_alerts" {
		t.Errorf("type = %v, want subscribe_alerts", raw["type"])
	}
}

func TestSocialSubscription_MarshalJSON(t *testing.T) {
	sub := SocialSubscription{ID: "test-id"}
	data, err := json.Marshal(&sub)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if raw["type"] != "subscribe_socials" {
		t.Errorf("type = %v, want subscribe_socials", raw["type"])
	}
}

func TestDecodeMessage_SubscribedAlerts(t *testing.T) {
	raw := `{"id":"8QFdN74g","type":"subscribed_alerts","blockchains":["ethereum"],"symbols":["eth","weth"],"tx_types":["transfer"],"min_value_usd":1000000}`
	msg := decodeMessage([]byte(raw))
	if msg.EventType != EventTypeSubscribedAlerts {
		t.Errorf("EventType = %v, want %v", msg.EventType, EventTypeSubscribedAlerts)
	}
	if msg.AlertConfirm == nil {
		t.Fatal("AlertConfirm should not be nil")
	}
	if msg.AlertConfirm.ID != "8QFdN74g" {
		t.Errorf("ID = %q, want 8QFdN74g", msg.AlertConfirm.ID)
	}
}

func TestDecodeMessage_SubscribedSocials(t *testing.T) {
	raw := `{"id":"8QFdN74g","type":"subscribed_socials"}`
	msg := decodeMessage([]byte(raw))
	if msg.EventType != EventTypeSubscribedSocials {
		t.Errorf("EventType = %v, want %v", msg.EventType, EventTypeSubscribedSocials)
	}
	if msg.SocialConfirm == nil {
		t.Fatal("SocialConfirm should not be nil")
	}
	if msg.SocialConfirm.ID != "8QFdN74g" {
		t.Errorf("ID = %q, want 8QFdN74g", msg.SocialConfirm.ID)
	}
}

func TestDecodeMessage_Alert(t *testing.T) {
	raw := `{"channel_id":"xlLZ7tJq","timestamp":1687389431,"blockchain":"ethereum","transaction_type":"transfer","from":"unknown wallet","to":"unknown wallet","amounts":[{"symbol":"USDC","amount":20006425.31,"value_usd":20008425.95}],"text":"transferred"}`
	msg := decodeMessage([]byte(raw))
	if msg.EventType != EventTypeAlert {
		t.Errorf("EventType = %v, want %v", msg.EventType, EventTypeAlert)
	}
	if msg.Alert == nil {
		t.Fatal("Alert should not be nil")
	}
	if msg.Alert.Blockchain != "ethereum" {
		t.Errorf("Blockchain = %q, want ethereum", msg.Alert.Blockchain)
	}
	if len(msg.Alert.Amounts) != 1 {
		t.Fatalf("Amounts len = %d, want 1", len(msg.Alert.Amounts))
	}
	if msg.Alert.Amounts[0].Symbol != "USDC" {
		t.Errorf("Symbol = %q, want USDC", msg.Alert.Amounts[0].Symbol)
	}
}

func TestDecodeMessage_Social(t *testing.T) {
	raw := `{"channel_id":"xlLZ7tJq","timestamp":1692724660,"blockchain":"tron","text":"1,200,000,000 USDT burned","urls":["https://twitter.com/whale_alert/status/1694036126422450598"]}`
	msg := decodeMessage([]byte(raw))
	if msg.EventType != EventTypeSocial {
		t.Errorf("EventType = %v, want %v", msg.EventType, EventTypeSocial)
	}
	if msg.Social == nil {
		t.Fatal("Social should not be nil")
	}
	if msg.Social.Blockchain != "tron" {
		t.Errorf("Blockchain = %q, want tron", msg.Social.Blockchain)
	}
	if len(msg.Social.URLs) != 1 {
		t.Fatalf("URLs len = %d, want 1", len(msg.Social.URLs))
	}
}

func TestDecodeMessage_Error(t *testing.T) {
	raw := `{"error":"Invalid API key"}`
	msg := decodeMessage([]byte(raw))
	if msg.EventType != EventTypeError {
		t.Errorf("EventType = %v, want %v", msg.EventType, EventTypeError)
	}
	if msg.Error == nil {
		t.Fatal("Error should not be nil")
	}
	if msg.Error.Error != "Invalid API key" {
		t.Errorf("Error = %q, want 'Invalid API key'", msg.Error.Error)
	}
}

func TestDecodeMessage_Unknown(t *testing.T) {
	raw := `{"some_unknown_field": true}`
	msg := decodeMessage([]byte(raw))
	if msg.EventType != EventTypeUnknown {
		t.Errorf("EventType = %v, want %v", msg.EventType, EventTypeUnknown)
	}
}

func TestDecodeMessage_InvalidJSON(t *testing.T) {
	msg := decodeMessage([]byte(`{invalid`))
	if msg.EventType != EventTypeUnknown {
		t.Errorf("EventType = %v, want %v", msg.EventType, EventTypeUnknown)
	}
}

func TestClient_ConnectAndClose(t *testing.T) {
	srv := startWSTestServer(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
	})
	defer srv.Close()

	c := NewClient(Config{URL: wsURL(srv)})
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	if !c.IsConnected() {
		t.Error("should be connected")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
	if c.IsConnected() {
		t.Error("should not be connected after Close")
	}
}

func TestClient_SubscribeAlerts(t *testing.T) {
	type result struct {
		msgType string
	}
	resultCh := make(chan result, 1)
	srv := startWSTestServer(t, func(conn *websocket.Conn) {
		_, data, err := conn.ReadMessage()
		if err != nil {
			resultCh <- result{msgType: ""}
			return
		}
		var msg map[string]interface{}
		if json.Unmarshal(data, &msg) == nil {
			if tp, ok := msg["type"].(string); ok {
				resultCh <- result{msgType: tp}
			} else {
				resultCh <- result{msgType: ""}
			}
		} else {
			resultCh <- result{msgType: ""}
		}
		conn.WriteMessage(websocket.TextMessage, []byte(`{"id":"test-id","type":"subscribed_alerts","blockchains":["ethereum"],"symbols":["eth"],"tx_types":["transfer"],"min_value_usd":100000}`))
		time.Sleep(100 * time.Millisecond)
	})
	defer srv.Close()

	c := NewClient(Config{URL: wsURL(srv)})
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer c.Close()

	err := c.SubscribeAlerts(context.Background(), AlertSubscription{
		ID:          "test-id",
		Blockchains: []string{"ethereum"},
		Symbols:     []string{"eth"},
		TxTypes:     []string{"transfer"},
		MinValueUSD: 100000,
	})
	if err != nil {
		t.Fatalf("SubscribeAlerts error: %v", err)
	}

	select {
	case r := <-resultCh:
		if r.msgType != "subscribe_alerts" {
			t.Errorf("received type = %q, want subscribe_alerts", r.msgType)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for server to receive subscription")
	}
}

func TestClient_SubscribeAlerts_InvalidFilter(t *testing.T) {
	srv := startWSTestServer(t, func(conn *websocket.Conn) {})
	defer srv.Close()

	c := NewClient(Config{URL: wsURL(srv)})
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer c.Close()

	err := c.SubscribeAlerts(context.Background(), AlertSubscription{
		Blockchains: []string{"ethereum"},
		MinValueUSD: 50000,
	})
	if err == nil {
		t.Fatal("expected error for min_value below 100000")
	}
}

func TestClient_SubscribeSocials(t *testing.T) {
	srv := startWSTestServer(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
		conn.WriteMessage(websocket.TextMessage, []byte(`{"id":"test-id","type":"subscribed_socials"}`))
		time.Sleep(100 * time.Millisecond)
	})
	defer srv.Close()

	c := NewClient(Config{URL: wsURL(srv)})
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer c.Close()

	err := c.SubscribeSocials(context.Background(), SocialSubscription{ID: "test-id"})
	if err != nil {
		t.Fatalf("SubscribeSocials error: %v", err)
	}
}

func TestClient_OnMessage(t *testing.T) {
	srv := startWSTestServer(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
		conn.WriteMessage(websocket.TextMessage, []byte(`{"channel_id":"test","timestamp":1687389431,"blockchain":"ethereum","transaction_type":"transfer","from":"wallet","to":"wallet","amounts":[],"text":"test alert"}`))
		time.Sleep(200 * time.Millisecond)
	})
	defer srv.Close()

	c := NewClient(Config{URL: wsURL(srv)})
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer c.Close()

	if err := c.SubscribeAlerts(context.Background(), AlertSubscription{
		Blockchains: []string{"ethereum"},
		MinValueUSD: 100000,
	}); err != nil {
		t.Fatalf("Subscribe error: %v", err)
	}

	received := make(chan Message, 1)
	c.OnMessage(func(msg Message) {
		received <- msg
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		_ = c.Listen(ctx)
	}()

	select {
	case msg := <-received:
		if msg.EventType != EventTypeAlert {
			t.Errorf("EventType = %v, want %v", msg.EventType, EventTypeAlert)
		}
		if msg.Alert == nil {
			t.Fatal("Alert should not be nil")
		}
		if msg.Alert.Text != "test alert" {
			t.Errorf("Text = %q, want 'test alert'", msg.Alert.Text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestClient_OnError(t *testing.T) {
	srv := startWSTestServer(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
		conn.WriteMessage(websocket.TextMessage, []byte(`{"error":"something went wrong"}`))
		time.Sleep(200 * time.Millisecond)
	})
	defer srv.Close()

	c := NewClient(Config{URL: wsURL(srv)})
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer c.Close()

	if err := c.SubscribeAlerts(context.Background(), AlertSubscription{
		Blockchains: []string{"ethereum"},
		MinValueUSD: 100000,
	}); err != nil {
		t.Fatalf("Subscribe error: %v", err)
	}

	errReceived := make(chan error, 1)
	c.OnError(func(err error) {
		errReceived <- err
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		_ = c.Listen(ctx)
	}()

	select {
	case err := <-errReceived:
		wsErr, ok := err.(*WSError)
		if !ok {
			t.Fatalf("expected *WSError, got %T", err)
		}
		if wsErr.Message != "something went wrong" {
			t.Errorf("Message = %q, want 'something went wrong'", wsErr.Message)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for error")
	}
}

func TestClient_GetSubscriptionID(t *testing.T) {
	srv := startWSTestServer(t, func(conn *websocket.Conn) {})
	defer srv.Close()

	c := NewClient(Config{URL: wsURL(srv)})
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer c.Close()

	if err := c.SubscribeAlerts(context.Background(), AlertSubscription{
		ID:          "my-sub-id",
		Blockchains: []string{"ethereum"},
		MinValueUSD: 100000,
	}); err != nil {
		t.Fatalf("Subscribe error: %v", err)
	}

	if got := c.GetSubscriptionID(); got != "my-sub-id" {
		t.Errorf("SubscriptionID = %q, want my-sub-id", got)
	}
}

func TestClient_CloseWithoutConnect(t *testing.T) {
	c := NewClient(Config{URL: "ws://localhost"})
	if err := c.Close(); err != nil {
		t.Errorf("Close should not error when not connected: %v", err)
	}
}

func TestClient_ListenReturnsCleanlyOnClose(t *testing.T) {
	srv := startWSTestServer(t, func(conn *websocket.Conn) {
		// Block reading until the client closes the connection.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer srv.Close()

	c := NewClient(Config{URL: wsURL(srv)})
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect error: %v", err)
	}

	listenErrCh := make(chan error, 1)
	go func() {
		listenErrCh <- c.Listen(context.Background())
	}()

	// Give the read loop a moment to start blocking on ReadMessage.
	time.Sleep(100 * time.Millisecond)

	if err := c.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	select {
	case err := <-listenErrCh:
		if err != nil {
			t.Errorf("Listen should return nil on caller-initiated Close, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for Listen to return after Close")
	}
}

func TestClient_PingLoop_SendsPing(t *testing.T) {
	pingReceived := make(chan struct{}, 1)
	srv := startWSTestServer(t, func(conn *websocket.Conn) {
		conn.SetPingHandler(func(appData string) error {
			select {
			case pingReceived <- struct{}{}:
			default:
			}
			return conn.WriteMessage(websocket.PongMessage, []byte(appData))
		})
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer srv.Close()

	c := NewClient(Config{URL: wsURL(srv), PingInterval: 50 * time.Millisecond})
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer c.Close()

	select {
	case <-pingReceived:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for ping frame")
	}
}

func TestClient_ConnectTwice(t *testing.T) {
	srv := startWSTestServer(t, func(conn *websocket.Conn) {})
	defer srv.Close()

	c := NewClient(Config{URL: wsURL(srv)})
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer c.Close()

	err := c.Connect(context.Background())
	if err == nil {
		t.Fatal("expected error for double connect")
	}
}

func TestClient_ReconnectsAfterConnectionCloses(t *testing.T) {
	secondConnection := make(chan struct{}, 1)
	var connections int
	var connectionsMu sync.Mutex

	srv := startWSTestServer(t, func(conn *websocket.Conn) {
		connectionsMu.Lock()
		connections++
		connectionNumber := connections
		connectionsMu.Unlock()

		if connectionNumber == 1 {
			_ = conn.Close()
			return
		}

		secondConnection <- struct{}{}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer srv.Close()

	c := NewClient(Config{
		URL: wsURL(srv),
		Reconnect: ReconnectConfig{
			MaxAttempts:  1,
			InitialDelay: time.Millisecond,
			MaxDelay:     time.Millisecond,
		},
	})
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect error: %v", err)
	}

	listenErr := make(chan error, 1)
	go func() { listenErr <- c.Listen(context.Background()) }()

	select {
	case <-secondConnection:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for reconnection")
	}

	if err := c.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
	select {
	case err := <-listenErr:
		if err != nil {
			t.Errorf("Listen error after Close = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for Listen to return")
	}
}
