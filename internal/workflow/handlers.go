// Package workflow provides workflow parsing, task generation, and output handler registration.
package workflow

import (
	"context"
	"fmt"
	"sync"

	"codemint.kanthorlabs.com/internal/domain"
)

// HandlerArgs is the payload a handler sees when invoked.
type HandlerArgs struct {
	// WorkflowID is the ID of the workflow execution.
	WorkflowID string
	// Task is the task that triggered the handler.
	Task *domain.Task
	// Output is the raw skill output (task.Output.String).
	Output string
	// ExitCmd is the slash command that triggered exit (e.g., "/lock-goal").
	ExitCmd string
}

// HandlerFunc is the signature for output handlers.
// Handlers are invoked when a story task with Output.Handler set transitions to Success.
type HandlerFunc func(ctx context.Context, args HandlerArgs) error

// HandlerRegistry maps handler names to their implementations.
// It is safe for concurrent use.
type HandlerRegistry struct {
	mu sync.RWMutex
	m  map[string]HandlerFunc
}

// NewHandlerRegistry creates a new empty HandlerRegistry.
func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{m: make(map[string]HandlerFunc)}
}

// Register adds a handler to the registry.
// Returns an error if a handler with the same name is already registered.
func (r *HandlerRegistry) Register(name string, fn HandlerFunc) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.m[name]; exists {
		return fmt.Errorf("workflow: handler %q already registered", name)
	}

	r.m[name] = fn
	return nil
}

// Invoke calls the named handler with the provided arguments.
// Returns an error if the handler is not registered or if the handler returns an error.
func (r *HandlerRegistry) Invoke(ctx context.Context, name string, args HandlerArgs) error {
	r.mu.RLock()
	fn, ok := r.m[name]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf("workflow: handler %q not registered", name)
	}

	return fn(ctx, args)
}

// Has returns true if a handler with the given name is registered.
func (r *HandlerRegistry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.m[name]
	return ok
}

// Names returns a list of all registered handler names.
func (r *HandlerRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.m))
	for name := range r.m {
		names = append(names, name)
	}
	return names
}
