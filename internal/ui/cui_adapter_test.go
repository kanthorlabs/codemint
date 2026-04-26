package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/registry"
)

// TestCUIAdapter_InterfaceSatisfaction verifies that CUIAdapter implements UIAdapter.
func TestCUIAdapter_InterfaceSatisfaction(t *testing.T) {
	// This is a compile-time check, but we include it in a test for clarity.
	var _ UIAdapter = (*CUIAdapter)(nil)

	// Also verify we can create one.
	adapter := NewCUIAdapter(CUIAdapterConfig{})
	if adapter == nil {
		t.Error("NewCUIAdapter returned nil")
	}
	defer adapter.Close()
}

// TestCUIAdapter_NotifyEvent verifies NotifyEvent filters events correctly.
func TestCUIAdapter_NotifyEvent(t *testing.T) {
	adapter := NewCUIAdapter(CUIAdapterConfig{})
	defer adapter.Close()

	// These should be dropped (non-terminal events).
	adapter.NotifyEvent(registry.UIEvent{
		Type:    registry.EventTaskStarted,
		TaskID:  "task-123",
		Message: "Test task started",
	})

	adapter.NotifyEvent(registry.UIEvent{
		Type:    registry.EventACPStream,
		Message: "Thinking...",
	})

	adapter.NotifyEvent(registry.UIEvent{
		Type:    registry.EventProgress,
		Message: "50%",
	})

	// These should be forwarded.
	adapter.NotifyEvent(registry.UIEvent{
		Type:    registry.EventSessionTakeover,
		Message: "Session taken over by daemon:xyz",
		Payload: "daemon:xyz123",
	})

	adapter.NotifyEvent(registry.UIEvent{
		Type:    registry.EventTaskStatusChanged,
		TaskID:  "task-123",
		Message: "Task status changed",
		Payload: registry.TaskStatusChangedPayload{
			TaskID: "task-123",
			From:   int(domain.TaskStatusProcessing),
			To:     int(domain.TaskStatusSuccess),
		},
	})
}

// TestCUIAdapter_shouldForwardEvent tests the event filtering logic.
func TestCUIAdapter_shouldForwardEvent(t *testing.T) {
	adapter := NewCUIAdapter(CUIAdapterConfig{})
	defer adapter.Close()

	tests := []struct {
		name     string
		event    registry.UIEvent
		expected bool
	}{
		{
			name: "ACPStream should be dropped",
			event: registry.UIEvent{
				Type: registry.EventACPStream,
			},
			expected: false,
		},
		{
			name: "Progress should be dropped",
			event: registry.UIEvent{
				Type: registry.EventProgress,
			},
			expected: false,
		},
		{
			name: "AutoApproved should be dropped",
			event: registry.UIEvent{
				Type: registry.EventACPAutoApproved,
			},
			expected: false,
		},
		{
			name: "SessionTakeover should be forwarded",
			event: registry.UIEvent{
				Type: registry.EventSessionTakeover,
			},
			expected: true,
		},
		{
			name: "SessionReclaimed should be forwarded",
			event: registry.UIEvent{
				Type: registry.EventSessionReclaimed,
			},
			expected: true,
		},
		{
			name: "AwaitingApproval should be forwarded",
			event: registry.UIEvent{
				Type: registry.EventACPAwaitingApproval,
			},
			expected: true,
		},
		{
			name: "ApprovalResolved should be forwarded",
			event: registry.UIEvent{
				Type: registry.EventACPApprovalResolved,
			},
			expected: true,
		},
		{
			name: "StatusChanged to Processing should be dropped",
			event: registry.UIEvent{
				Type: registry.EventTaskStatusChanged,
				Payload: registry.TaskStatusChangedPayload{
					To: int(domain.TaskStatusProcessing),
				},
			},
			expected: false,
		},
		{
			name: "StatusChanged to Awaiting should be forwarded",
			event: registry.UIEvent{
				Type: registry.EventTaskStatusChanged,
				Payload: registry.TaskStatusChangedPayload{
					To: int(domain.TaskStatusAwaiting),
				},
			},
			expected: true,
		},
		{
			name: "StatusChanged to Success should be forwarded",
			event: registry.UIEvent{
				Type: registry.EventTaskStatusChanged,
				Payload: registry.TaskStatusChangedPayload{
					To: int(domain.TaskStatusSuccess),
				},
			},
			expected: true,
		},
		{
			name: "StatusChanged to Failure should be forwarded",
			event: registry.UIEvent{
				Type: registry.EventTaskStatusChanged,
				Payload: registry.TaskStatusChangedPayload{
					To: int(domain.TaskStatusFailure),
				},
			},
			expected: true,
		},
		{
			name: "StatusChanged to Reverted should be forwarded",
			event: registry.UIEvent{
				Type: registry.EventTaskStatusChanged,
				Payload: registry.TaskStatusChangedPayload{
					To: int(domain.TaskStatusReverted),
				},
			},
			expected: true,
		},
		{
			name: "StatusChanged with invalid payload should be dropped",
			event: registry.UIEvent{
				Type:    registry.EventTaskStatusChanged,
				Payload: "invalid",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adapter.shouldForwardEvent(tt.event)
			if got != tt.expected {
				t.Errorf("shouldForwardEvent() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestCUIAdapter_PromptDecision verifies PromptDecision waits for cancellation.
func TestCUIAdapter_PromptDecision(t *testing.T) {
	adapter := NewCUIAdapter(CUIAdapterConfig{})
	defer adapter.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req := registry.PromptRequest{
		TaskID:  "task-123",
		Title:   "Accept changes?",
		Message: "The agent made changes to foo.go",
		Options: []string{"Accept", "Revert"},
	}

	resp := adapter.PromptDecision(ctx, req)

	// Should return ErrPromptCanceled after context timeout.
	if !errors.Is(resp.Error, ErrPromptCanceled) {
		t.Errorf("expected ErrPromptCanceled, got %v", resp.Error)
	}
}

// TestCUIAdapter_ResolvePrompt tests the approval flow.
func TestCUIAdapter_ResolvePrompt(t *testing.T) {
	adapter := NewCUIAdapter(CUIAdapterConfig{})
	defer adapter.Close()

	req := registry.PromptRequest{
		TaskID: "task-123",
		Title:  "Approve?",
		PromptOptions: []registry.PromptOption{
			{ID: "allow_once", Label: "Allow Once"},
			{ID: "allow_session", Label: "Allow Session"},
		},
	}

	// Start prompt in goroutine.
	respCh := make(chan registry.PromptResponse, 1)
	go func() {
		ctx := context.Background()
		respCh <- adapter.PromptDecision(ctx, req)
	}()

	// Give the goroutine time to register the prompt.
	time.Sleep(50 * time.Millisecond)

	// Resolve the prompt.
	err := adapter.ResolvePrompt(1, "allow_once")
	if err != nil {
		t.Fatalf("ResolvePrompt failed: %v", err)
	}

	// Check response.
	select {
	case resp := <-respCh:
		if resp.Error != nil {
			t.Errorf("unexpected error: %v", resp.Error)
		}
		if resp.SelectedOptionID != "allow_once" {
			t.Errorf("expected SelectedOptionID 'allow_once', got %q", resp.SelectedOptionID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response")
	}
}

// TestCUIAdapter_DenyPrompt tests the deny flow.
func TestCUIAdapter_DenyPrompt(t *testing.T) {
	adapter := NewCUIAdapter(CUIAdapterConfig{})
	defer adapter.Close()

	req := registry.PromptRequest{
		TaskID: "task-123",
		Title:  "Approve?",
	}

	// Start prompt in goroutine.
	respCh := make(chan registry.PromptResponse, 1)
	go func() {
		ctx := context.Background()
		respCh <- adapter.PromptDecision(ctx, req)
	}()

	// Give the goroutine time to register the prompt.
	time.Sleep(50 * time.Millisecond)

	// Deny the prompt.
	err := adapter.DenyPrompt(1)
	if err != nil {
		t.Fatalf("DenyPrompt failed: %v", err)
	}

	// Check response.
	select {
	case resp := <-respCh:
		if !errors.Is(resp.Error, ErrPromptCanceled) {
			t.Errorf("expected ErrPromptCanceled, got %v", resp.Error)
		}
		if resp.SelectedOptionID != "deny" {
			t.Errorf("expected SelectedOptionID 'deny', got %q", resp.SelectedOptionID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response")
	}
}

// TestCUIAdapter_ResolvePrompt_NotFound tests resolving a non-existent prompt.
func TestCUIAdapter_ResolvePrompt_NotFound(t *testing.T) {
	adapter := NewCUIAdapter(CUIAdapterConfig{})
	defer adapter.Close()

	err := adapter.ResolvePrompt(999, "allow")
	if err == nil {
		t.Error("expected error for non-existent prompt")
	}
}

// TestCUIAdapter_ListPendingPrompts tests listing pending prompts.
func TestCUIAdapter_ListPendingPrompts(t *testing.T) {
	adapter := NewCUIAdapter(CUIAdapterConfig{})
	defer adapter.Close()

	// Initially empty.
	prompts := adapter.ListPendingPrompts()
	if len(prompts) != 0 {
		t.Errorf("expected 0 pending prompts, got %d", len(prompts))
	}

	// Register a prompt.
	req := registry.PromptRequest{
		Title: "Test Prompt",
		Kind:  registry.PromptKindACPCommandApproval,
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		adapter.PromptDecision(ctx, req)
	}()

	// Give the goroutine time to register.
	time.Sleep(50 * time.Millisecond)

	prompts = adapter.ListPendingPrompts()
	if len(prompts) != 1 {
		t.Errorf("expected 1 pending prompt, got %d", len(prompts))
	}

	if prompts[0].Title != "Test Prompt" {
		t.Errorf("expected title 'Test Prompt', got %q", prompts[0].Title)
	}

	// Cleanup.
	cancel()
	time.Sleep(50 * time.Millisecond)

	prompts = adapter.ListPendingPrompts()
	if len(prompts) != 0 {
		t.Errorf("expected 0 pending prompts after cancel, got %d", len(prompts))
	}
}

// mockPusher is a test pusher that records messages.
type mockPusher struct {
	messages []string
	err      error
}

func (m *mockPusher) Push(ctx context.Context, msg string) error {
	if m.err != nil {
		return m.err
	}
	m.messages = append(m.messages, msg)
	return nil
}

// TestCUIAdapter_DefaultPusher_LogsOnly verifies that the default pusher does not panic.
func TestCUIAdapter_DefaultPusher_LogsOnly(t *testing.T) {
	adapter := NewCUIAdapter(CUIAdapterConfig{})
	defer adapter.Close()

	// This should not panic - events go to log file only (default noopPusher).
	adapter.NotifyEvent(registry.UIEvent{
		Type:    registry.EventSessionTakeover,
		Message: "Test takeover event",
	})

	// Give time for async processing.
	time.Sleep(50 * time.Millisecond)
}

// TestCUIAdapter_SetPusher verifies that SetPusher changes the pusher.
func TestCUIAdapter_SetPusher(t *testing.T) {
	adapter := NewCUIAdapter(CUIAdapterConfig{})
	defer adapter.Close()

	pusher := &mockPusher{}
	adapter.SetPusher(pusher)

	// Send an event that should be forwarded.
	adapter.NotifyEvent(registry.UIEvent{
		Type:    registry.EventSessionTakeover,
		Message: "Test takeover",
	})

	// Give time for async processing.
	time.Sleep(50 * time.Millisecond)

	if len(pusher.messages) != 1 {
		t.Errorf("expected 1 pushed message, got %d", len(pusher.messages))
	}
}

// TestCUIAdapter_SetPusher_Nil verifies that setting nil pusher resets to noop.
func TestCUIAdapter_SetPusher_Nil(t *testing.T) {
	adapter := NewCUIAdapter(CUIAdapterConfig{})
	defer adapter.Close()

	pusher := &mockPusher{}
	adapter.SetPusher(pusher)
	adapter.SetPusher(nil) // Reset to noop.

	// Send an event that should be forwarded.
	adapter.NotifyEvent(registry.UIEvent{
		Type:    registry.EventSessionTakeover,
		Message: "Test takeover",
	})

	// Give time for async processing.
	time.Sleep(50 * time.Millisecond)

	// The mock pusher should not receive messages since we reset to noop.
	if len(pusher.messages) != 0 {
		t.Errorf("expected 0 pushed messages after reset, got %d", len(pusher.messages))
	}
}

// TestNoopPusher_SatisfiesInterface is a compile-time check.
func TestNoopPusher_SatisfiesInterface(t *testing.T) {
	var _ NotificationPusher = noopPusher{}
}
