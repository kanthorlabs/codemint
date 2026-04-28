package workflow

import (
	"context"
	"errors"
	"testing"

	"codemint.kanthorlabs.com/internal/domain"
)

func TestHandlerRegistry_Register(t *testing.T) {
	t.Parallel()

	r := NewHandlerRegistry()

	// Register a handler.
	err := r.Register("test-handler", func(ctx context.Context, args HandlerArgs) error {
		return nil
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Verify it's registered.
	if !r.Has("test-handler") {
		t.Error("Has returned false for registered handler")
	}
}

func TestHandlerRegistry_RegisterDuplicate(t *testing.T) {
	t.Parallel()

	r := NewHandlerRegistry()

	// Register first handler.
	err := r.Register("dup-handler", func(ctx context.Context, args HandlerArgs) error {
		return nil
	})
	if err != nil {
		t.Fatalf("First Register failed: %v", err)
	}

	// Attempt duplicate registration.
	err = r.Register("dup-handler", func(ctx context.Context, args HandlerArgs) error {
		return nil
	})
	if err == nil {
		t.Fatal("Expected error for duplicate registration, got nil")
	}

	// Verify error message.
	if err.Error() != `workflow: handler "dup-handler" already registered` {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestHandlerRegistry_Invoke(t *testing.T) {
	t.Parallel()

	r := NewHandlerRegistry()

	// Track invocation.
	var invokedWith HandlerArgs
	err := r.Register("invoke-test", func(ctx context.Context, args HandlerArgs) error {
		invokedWith = args
		return nil
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Invoke the handler.
	args := HandlerArgs{
		WorkflowID: "wf-123",
		Task:       &domain.Task{ID: "task-456"},
		Output:     `{"goal_text":"test"}`,
		ExitCmd:    "/lock-goal",
	}

	err = r.Invoke(context.Background(), "invoke-test", args)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}

	// Verify args were passed correctly.
	if invokedWith.WorkflowID != "wf-123" {
		t.Errorf("WorkflowID = %q, want %q", invokedWith.WorkflowID, "wf-123")
	}
	if invokedWith.Task.ID != "task-456" {
		t.Errorf("Task.ID = %q, want %q", invokedWith.Task.ID, "task-456")
	}
	if invokedWith.Output != `{"goal_text":"test"}` {
		t.Errorf("Output = %q, want %q", invokedWith.Output, `{"goal_text":"test"}`)
	}
	if invokedWith.ExitCmd != "/lock-goal" {
		t.Errorf("ExitCmd = %q, want %q", invokedWith.ExitCmd, "/lock-goal")
	}
}

func TestHandlerRegistry_InvokeUnknown(t *testing.T) {
	t.Parallel()

	r := NewHandlerRegistry()

	// Invoke unknown handler.
	err := r.Invoke(context.Background(), "unknown-handler", HandlerArgs{})
	if err == nil {
		t.Fatal("Expected error for unknown handler, got nil")
	}

	// Verify error message.
	if err.Error() != `workflow: handler "unknown-handler" not registered` {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestHandlerRegistry_InvokeReturnsHandlerError(t *testing.T) {
	t.Parallel()

	r := NewHandlerRegistry()

	// Register handler that returns error.
	handlerErr := errors.New("handler error")
	err := r.Register("error-handler", func(ctx context.Context, args HandlerArgs) error {
		return handlerErr
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Invoke and verify error is returned.
	err = r.Invoke(context.Background(), "error-handler", HandlerArgs{})
	if err != handlerErr {
		t.Errorf("Invoke error = %v, want %v", err, handlerErr)
	}
}

func TestHandlerRegistry_Has(t *testing.T) {
	t.Parallel()

	r := NewHandlerRegistry()

	// Unregistered handler.
	if r.Has("nonexistent") {
		t.Error("Has returned true for unregistered handler")
	}

	// Register and check again.
	_ = r.Register("exists", func(ctx context.Context, args HandlerArgs) error {
		return nil
	})

	if !r.Has("exists") {
		t.Error("Has returned false for registered handler")
	}
}

func TestHandlerRegistry_Names(t *testing.T) {
	t.Parallel()

	r := NewHandlerRegistry()

	// Empty registry.
	names := r.Names()
	if len(names) != 0 {
		t.Errorf("Names() = %v, want empty", names)
	}

	// Add handlers.
	_ = r.Register("handler-a", func(ctx context.Context, args HandlerArgs) error { return nil })
	_ = r.Register("handler-b", func(ctx context.Context, args HandlerArgs) error { return nil })

	names = r.Names()
	if len(names) != 2 {
		t.Errorf("len(Names()) = %d, want 2", len(names))
	}

	// Check both names are present (order not guaranteed).
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	if !nameSet["handler-a"] || !nameSet["handler-b"] {
		t.Errorf("Names() = %v, missing expected handlers", names)
	}
}

func TestHandlerRegistry_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	r := NewHandlerRegistry()

	// Register initial handler.
	_ = r.Register("concurrent", func(ctx context.Context, args HandlerArgs) error {
		return nil
	})

	// Concurrent reads and invokes.
	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func() {
			_ = r.Has("concurrent")
			_ = r.Invoke(context.Background(), "concurrent", HandlerArgs{})
			_ = r.Names()
			done <- true
		}()
	}

	// Wait for all goroutines.
	for i := 0; i < 100; i++ {
		<-done
	}
}
