package websocket

import (
	"encoding/json"
	"fmt"
)

// AlertSubscription builds a subscribe_alerts message.
type AlertSubscription struct {
	ID          string   `json:"id,omitempty"`
	Blockchains []string `json:"blockchains,omitempty"`
	Symbols     []string `json:"symbols,omitempty"`
	TxTypes     []string `json:"tx_types,omitempty"`
	MinValueUSD float64  `json:"min_value_usd"`
}

// SocialSubscription builds a subscribe_socials message.
type SocialSubscription struct {
	ID string `json:"id,omitempty"`
}

const (
	minMinValueUSD = 100000.0
)

// Validate checks that the subscription filter is sensible before sending.
// It rejects min_value_usd below the documented USD 100,000 minimum.
func (s *AlertSubscription) Validate() error {
	if s.MinValueUSD < minMinValueUSD {
		return fmt.Errorf("whalealert: min_value_usd must be at least %.0f, got %.0f", minMinValueUSD, s.MinValueUSD)
	}
	hasFilter := len(s.Blockchains) > 0 || len(s.Symbols) > 0 || len(s.TxTypes) > 0
	if !hasFilter {
		return fmt.Errorf("whalealert: at least one filter (blockchains, symbols, or tx_types) is required")
	}
	return nil
}

// Type returns the subscription message type.
func (s *AlertSubscription) Type() string {
	return "subscribe_alerts"
}

// MarshalJSON encodes the subscription with the required "type" field.
func (s *AlertSubscription) MarshalJSON() ([]byte, error) {
	type alias AlertSubscription
	return json.Marshal(struct {
		Type string `json:"type"`
		*alias
	}{
		Type:  "subscribe_alerts",
		alias: (*alias)(s),
	})
}

// Type returns the subscription message type.
func (s *SocialSubscription) Type() string {
	return "subscribe_socials"
}

// MarshalJSON encodes the subscription with the required "type" field.
func (s *SocialSubscription) MarshalJSON() ([]byte, error) {
	type alias SocialSubscription
	return json.Marshal(struct {
		Type string `json:"type"`
		*alias
	}{
		Type:  "subscribe_socials",
		alias: (*alias)(s),
	})
}

// decodeMessage classifies a raw JSON message into a typed Message.
func decodeMessage(data []byte) Message {
	msg := Message{
		EventType: EventTypeUnknown,
		Raw:       data,
	}

	var probe struct {
		Type      string        `json:"type"`
		Error     string        `json:"error"`
		ChannelID string        `json:"channel_id"`
		Amounts   []AlertAmount `json:"amounts"`
		URLs      []string      `json:"urls"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return msg
	}

	if probe.Error != "" {
		msg.EventType = EventTypeError
		msg.Error = &ErrorEvent{Error: probe.Error}
		return msg
	}

	switch probe.Type {
	case "subscribed_alerts":
		msg.EventType = EventTypeSubscribedAlerts
		var conf SubscribedAlertsConfirmation
		if json.Unmarshal(data, &conf) == nil {
			msg.AlertConfirm = &conf
		}
		return msg
	case "subscribed_socials":
		msg.EventType = EventTypeSubscribedSocials
		var conf SubscribedSocialsConfirmation
		if json.Unmarshal(data, &conf) == nil {
			msg.SocialConfirm = &conf
		}
		return msg
	}

	// Alert and social events do not have a "type" field.
	// Identify them by content: alerts have amounts, socials have urls.
	if probe.ChannelID != "" {
		if probe.Amounts != nil {
			msg.EventType = EventTypeAlert
			var alert AlertEvent
			if json.Unmarshal(data, &alert) == nil {
				msg.Alert = &alert
			}
		} else if probe.URLs != nil {
			msg.EventType = EventTypeSocial
			var social SocialEvent
			if json.Unmarshal(data, &social) == nil {
				msg.Social = &social
			}
		} else {
			var alert AlertEvent
			if json.Unmarshal(data, &alert) == nil && alert.Blockchain != "" {
				msg.EventType = EventTypeAlert
				msg.Alert = &alert
			}
		}
	}

	return msg
}
