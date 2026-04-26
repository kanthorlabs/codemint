package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	"codemint.kanthorlabs.com/internal/registry"
)

// TestCUIAdapter_InterfaceSatisfaction verifies that CUIAdapter implements UIAdapter.
func TestCUIAdapter_InterfaceSatisfaction(t *testing.T) {
	// This is a compile-time check, but we include it in a test for clarity.
	var _ UIAdapter = (*CUIAdapter)(nil)
	
	// Also verify we can create one.
	adapter := NewCUIAdapter()
	if adapter == nil {
		t.Error("NewCUIAdapter returned nil")
	}
}

// TestCUIAdapter_NotifyEvent verifies NotifyEvent doesn't panic.
func TestCUIAdapter_NotifyEvent(t *testing.T) {
	adapter := NewCUIAdapter()
	
	// Should not panic.
	adapter.NotifyEvent(registry.UIEvent{
		Type:    registry.EventTaskStarted,
		TaskID:  "task-123",
		Message: "Test task started",
	})
	
	adapter.NotifyEvent(registry.UIEvent{
		Type:    registry.EventSessionTakeover,
		Message: "Session taken over by daemon:xyz",
		Payload: "daemon:xyz123",
	})
}

// TestCUIAdapter_PromptDecision verifies PromptDecision waits for cancellation.
func TestCUIAdapter_PromptDecision(t *testing.T) {
	adapter := NewCUIAdapter()
	
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	
	req := registry.PromptRequest{
		TaskID:  "task-123",
		Message: "Accept changes?",
		Options: []string{"Accept", "Revert"},
	}
	
	resp := adapter.PromptDecision(ctx, req)
	
	// Should return ErrPromptCanceled after context timeout.
	if !errors.Is(resp.Error, ErrPromptCanceled) {
		t.Errorf("expected ErrPromptCanceled, got %v", resp.Error)
	}
}
