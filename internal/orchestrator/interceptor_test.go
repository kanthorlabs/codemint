package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/registry"
)

// mockUIMediator captures RenderMessage calls for testing.
type mockUIMediator struct {
	mu       sync.Mutex
	messages []string
	events   []registry.UIEvent
}

func (m *mockUIMediator) RenderMessage(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, msg)
}

func (m *mockUIMediator) ClearScreen() {}

func (m *mockUIMediator) NotifyAll(event registry.UIEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
}

func (m *mockUIMediator) PromptDecision(ctx context.Context, req registry.PromptRequest) registry.PromptResponse {
	return registry.PromptResponse{SelectedOption: req.Options[0]}
}

func (m *mockUIMediator) Messages() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.messages...)
}

func (m *mockUIMediator) Events() []registry.UIEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]registry.UIEvent(nil), m.events...)
}

func TestInterceptor_Handle_ToolCall(t *testing.T) {
	ui := &mockUIMediator{}
	interceptor := NewInterceptor(InterceptorConfig{
		UI: ui,
	})

	ev := acp.Event{
		Kind:         acp.EventToolCall,
		ACPSessionID: "sess-1",
		ToolName:     "bash",
		Command:      "ls -la",
		Raw:          json.RawMessage(`{}`),
	}

	interceptor.Handle(context.Background(), ev, "task-123")

	messages := ui.Messages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	expected := "[ACP] Tool call halted: bash `ls -la`"
	if messages[0] != expected {
		t.Errorf("message = %q, want %q", messages[0], expected)
	}
}

func TestInterceptor_Handle_ToolCallNoCommand(t *testing.T) {
	ui := &mockUIMediator{}
	interceptor := NewInterceptor(InterceptorConfig{
		UI: ui,
	})

	ev := acp.Event{
		Kind:         acp.EventToolCall,
		ACPSessionID: "sess-1",
		ToolName:     "read",
		Raw:          json.RawMessage(`{}`),
	}

	interceptor.Handle(context.Background(), ev, "task-123")

	messages := ui.Messages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	expected := "[ACP] Tool call halted: read"
	if messages[0] != expected {
		t.Errorf("message = %q, want %q", messages[0], expected)
	}
}

func TestInterceptor_Handle_PermissionRequest(t *testing.T) {
	ui := &mockUIMediator{}
	interceptor := NewInterceptor(InterceptorConfig{
		UI: ui,
	})

	ev := acp.Event{
		Kind:         acp.EventPermissionRequest,
		ACPSessionID: "sess-2",
		RequestID:    "req-001",
		ToolName:     "bash",
		Command:      "rm -rf /tmp/test",
		Raw:          json.RawMessage(`{}`),
	}

	interceptor.Handle(context.Background(), ev, "task-456")

	messages := ui.Messages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	if !strings.Contains(messages[0], "bash") || !strings.Contains(messages[0], "rm -rf /tmp/test") {
		t.Errorf("message should contain tool and command: %q", messages[0])
	}
}

func TestInterceptor_Handle_UnexpectedKind(t *testing.T) {
	ui := &mockUIMediator{}
	interceptor := NewInterceptor(InterceptorConfig{
		UI: ui,
	})

	// EventThinking should not normally reach the interceptor
	ev := acp.Event{
		Kind:         acp.EventThinking,
		ACPSessionID: "sess-3",
	}

	// Should not panic
	interceptor.Handle(context.Background(), ev, "task-789")

	// No messages should be rendered for unexpected kinds
	messages := ui.Messages()
	if len(messages) != 0 {
		t.Errorf("expected 0 messages for unexpected kind, got %d", len(messages))
	}
}

func TestInterceptor_Run(t *testing.T) {
	ui := &mockUIMediator{}
	interceptor := NewInterceptor(InterceptorConfig{
		UI: ui,
	})

	halted := make(chan acp.Event, 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start interceptor in background
	done := make(chan struct{})
	go func() {
		interceptor.Run(ctx, halted, "task-run")
		close(done)
	}()

	// Send events
	halted <- acp.Event{
		Kind:     acp.EventToolCall,
		ToolName: "bash",
		Command:  "echo hello",
	}
	halted <- acp.Event{
		Kind:      acp.EventPermissionRequest,
		ToolName:  "shell",
		Command:   "npm install",
		RequestID: "req-1",
	}

	// Close channel to signal completion
	close(halted)

	// Wait for interceptor to finish
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("interceptor did not finish")
	}

	messages := ui.Messages()
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
}

func TestInterceptor_Run_ContextCancel(t *testing.T) {
	ui := &mockUIMediator{}
	interceptor := NewInterceptor(InterceptorConfig{
		UI: ui,
	})

	halted := make(chan acp.Event)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		interceptor.Run(ctx, halted, "task-cancel")
		close(done)
	}()

	// Cancel context
	cancel()

	// Wait for interceptor to finish
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("interceptor did not finish on context cancel")
	}
}

func TestInterceptor_NilUI(t *testing.T) {
	// Test that interceptor works without a UI (doesn't panic)
	interceptor := NewInterceptor(InterceptorConfig{
		// No UI provided
	})

	ev := acp.Event{
		Kind:     acp.EventToolCall,
		ToolName: "bash",
		Command:  "ls",
	}

	// Should not panic
	interceptor.Handle(context.Background(), ev, "task-nil")
}

func TestFormatHaltMessage(t *testing.T) {
	tests := []struct {
		name     string
		ev       acp.Event
		expected string
	}{
		{
			name: "with command",
			ev: acp.Event{
				ToolName: "bash",
				Command:  "ls -la",
			},
			expected: "[ACP] Tool call halted: bash `ls -la`",
		},
		{
			name: "without command",
			ev: acp.Event{
				ToolName: "read",
			},
			expected: "[ACP] Tool call halted: read",
		},
		{
			name: "empty tool name",
			ev: acp.Event{
				Command: "something",
			},
			expected: "[ACP] Tool call halted:  `something`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatHaltMessage(tt.ev)
			if got != tt.expected {
				t.Errorf("formatHaltMessage() = %q, want %q", got, tt.expected)
			}
		})
	}
}
