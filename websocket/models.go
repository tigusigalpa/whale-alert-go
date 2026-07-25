package websocket

// EventType identifies the kind of WebSocket message received.
type EventType string

const (
	EventTypeSubscribedAlerts  EventType = "subscribed_alerts"
	EventTypeSubscribedSocials EventType = "subscribed_socials"
	EventTypeAlert             EventType = "alert"
	EventTypeSocial            EventType = "social"
	EventTypeError             EventType = "error"
	EventTypeUnknown           EventType = "unknown"
)

// AlertAmount represents a single currency amount within an alert.
type AlertAmount struct {
	Symbol   string  `json:"symbol"`
	Amount   float64 `json:"amount"`
	ValueUSD float64 `json:"value_usd"`
}

// AlertEvent represents a whale alert delivered via WebSocket.
type AlertEvent struct {
	ChannelID       string        `json:"channel_id"`
	Timestamp       int64         `json:"timestamp"`
	Blockchain      string        `json:"blockchain"`
	TransactionType string        `json:"transaction_type"`
	From            string        `json:"from"`
	To              string        `json:"to"`
	Amounts         []AlertAmount `json:"amounts"`
	Text            string        `json:"text"`
}

// SocialEvent represents a social media post alert.
type SocialEvent struct {
	ChannelID  string   `json:"channel_id"`
	Timestamp  int64    `json:"timestamp"`
	Blockchain string   `json:"blockchain"`
	Text       string   `json:"text"`
	URLs       []string `json:"urls"`
}

// SubscribedAlertsConfirmation is the server response to a subscribe_alerts message.
type SubscribedAlertsConfirmation struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Blockchains []string `json:"blockchains"`
	Symbols     []string `json:"symbols"`
	TxTypes     []string `json:"tx_types"`
	MinValueUSD float64  `json:"min_value_usd"`
}

// SubscribedSocialsConfirmation is the server response to a subscribe_socials message.
type SubscribedSocialsConfirmation struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// ErrorEvent represents a provider error message.
type ErrorEvent struct {
	Error string `json:"error"`
}

// Message is a typed wrapper around any WebSocket message. The EventType
// field indicates which concrete payload to inspect.
type Message struct {
	EventType     EventType
	Raw           []byte
	Alert         *AlertEvent
	Social        *SocialEvent
	AlertConfirm  *SubscribedAlertsConfirmation
	SocialConfirm *SubscribedSocialsConfirmation
	Error         *ErrorEvent
}
