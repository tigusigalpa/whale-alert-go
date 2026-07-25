package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/tigusigalpa/whale-alert-go/websocket"
)

func main() {
	apiKey := os.Getenv("WHALE_ALERT_API_KEY")
	if apiKey == "" {
		log.Fatal("WHALE_ALERT_API_KEY environment variable is required")
	}

	wsURL := fmt.Sprintf("wss://leviathan.whale-alert.io/ws?api_key=%s", apiKey)

	client := websocket.NewClient(websocket.Config{
		URL: wsURL,
		Reconnect: websocket.ReconnectConfig{
			MaxAttempts:  5,
			InitialDelay: 1 * time.Second,
			MaxDelay:     30 * time.Second,
		},
	})

	client.OnMessage(func(msg websocket.Message) {
		switch msg.EventType {
		case websocket.EventTypeAlert:
			a := msg.Alert
			fmt.Printf("[ALERT] %s %s: %s -> %s | %s\n",
				a.Blockchain, a.TransactionType, a.From, a.To, a.Text)
			for _, amt := range a.Amounts {
				fmt.Printf("  %.2f %s ($%.2f)\n", amt.Amount, amt.Symbol, amt.ValueUSD)
			}
		case websocket.EventTypeSocial:
			s := msg.Social
			fmt.Printf("[SOCIAL] %s: %s\n", s.Blockchain, s.Text)
		case websocket.EventTypeSubscribedAlerts:
			fmt.Printf("[SUBSCRIBED] alerts id=%s\n", msg.AlertConfirm.ID)
		case websocket.EventTypeSubscribedSocials:
			fmt.Printf("[SUBSCRIBED] socials id=%s\n", msg.SocialConfirm.ID)
		}
	})

	client.OnError(func(err error) {
		log.Printf("[ERROR] %v", err)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		log.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	if err := client.SubscribeAlerts(ctx, websocket.AlertSubscription{
		ID:          "my-subscription",
		Blockchains: []string{"ethereum", "bitcoin"},
		MinValueUSD: 500000,
	}); err != nil {
		log.Fatalf("SubscribeAlerts: %v", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		cancel()
		client.Close()
	}()

	if err := client.Listen(ctx); err != nil {
		log.Printf("Listen ended: %v", err)
	}
}
