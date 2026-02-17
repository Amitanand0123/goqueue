package examples

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type ImageResizePayload struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// HandleImageResize is an example task handler for image resizing.
func HandleImageResize(ctx context.Context, payload json.RawMessage) (json.RawMessage, error) {
	var p ImageResizePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("invalid image resize payload: %w", err)
	}

	// Simulate image processing
	fmt.Printf("[IMAGE] Resizing %s to %dx%d\n", p.URL, p.Width, p.Height)
	time.Sleep(1 * time.Second)

	return json.Marshal(map[string]interface{}{
		"status":    "resized",
		"url":       p.URL,
		"width":     p.Width,
		"height":    p.Height,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}
