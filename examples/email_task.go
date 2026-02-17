package examples

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type EmailPayload struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// HandleEmailSend is an example task handler for sending emails.
func HandleEmailSend(ctx context.Context, payload json.RawMessage) (json.RawMessage, error) {
	var p EmailPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("invalid email payload: %w", err)
	}

	// Simulate sending an email
	fmt.Printf("[EMAIL] Sending to %s: %s\n", p.To, p.Subject)
	time.Sleep(500 * time.Millisecond)

	return json.Marshal(map[string]string{
		"status":    "sent",
		"to":        p.To,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}
