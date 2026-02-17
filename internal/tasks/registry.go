package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// HandlerFunc is the function signature for task handlers.
type HandlerFunc func(ctx context.Context, payload json.RawMessage) (json.RawMessage, error)

// Registry maps task types to their handler functions.
type Registry struct {
	mu       sync.RWMutex
	handlers map[string]HandlerFunc
}

// NewRegistry creates a new task registry.
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[string]HandlerFunc),
	}
}

// Register adds a handler for a task type.
func (r *Registry) Register(taskType string, handler HandlerFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[taskType] = handler
}

// Get retrieves the handler for a task type.
func (r *Registry) Get(taskType string) (HandlerFunc, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	handler, ok := r.handlers[taskType]
	if !ok {
		return nil, fmt.Errorf("no handler registered for task type: %s", taskType)
	}
	return handler, nil
}

// Has checks if a handler is registered for a task type.
func (r *Registry) Has(taskType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.handlers[taskType]
	return ok
}

// Types returns all registered task types.
func (r *Registry) Types() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	types := make([]string, 0, len(r.handlers))
	for t := range r.handlers {
		types = append(types, t)
	}
	return types
}
