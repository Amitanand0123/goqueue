package examples

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type WebhookPayload struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body"`
}

// HandleWebhookDeliver is an example task handler for webhook delivery.
func HandleWebhookDeliver(ctx context.Context, payload json.RawMessage) (json.RawMessage, error) {
	var p WebhookPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("invalid webhook payload: %w", err)
	}

	if p.Method == "" {
		p.Method = "POST"
	}

	// Simulate webhook delivery
	fmt.Printf("[WEBHOOK] %s %s\n", p.Method, p.URL)
	time.Sleep(300 * time.Millisecond)

	return json.Marshal(map[string]interface{}{
		"status":      "delivered",
		"url":         p.URL,
		"method":      p.Method,
		"status_code": 200,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
	})
}
