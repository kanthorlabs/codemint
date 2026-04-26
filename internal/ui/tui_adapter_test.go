package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/registry"
)

func TestNewTUIAdapter(t *testing.T) {
	var buf bytes.Buffer
	adapter := NewTUIAdapter(TUIAdapterConfig{
		Writer:    &buf,
		Verbosity: VerbosityTask,
	})
	defer adapter.Stop()

	if adapter == nil {
		t.Fatal("NewTUIAdapter returned nil")
	}

	if adapter.GetVerbosity() != VerbosityTask {
		t.Errorf("expected verbosity %d, got %d", VerbosityTask, adapter.GetVerbosity())
	}
}

func TestTUIAdapter_VerbosityGetterOverride(t *testing.T) {
	var buf bytes.Buffer
	level := VerbosityStory

	adapter := NewTUIAdapter(TUIAdapterConfig{
		Writer:    &buf,
		Verbosity: VerbosityTask, // Default
		VerbosityGetter: func() VerbosityLevel {
			return level
		},
	})
	defer adapter.Stop()

	// Getter should override stored value
	if adapter.GetVerbosity() != VerbosityStory {
		t.Errorf("expected verbosity %d from getter, got %d", VerbosityStory, adapter.GetVerbosity())
	}

	// Change the level through the closure
	level = VerbosityEpic
	if adapter.GetVerbosity() != VerbosityEpic {
		t.Errorf("expected verbosity %d after change, got %d", VerbosityEpic, adapter.GetVerbosity())
	}
}

func TestTUIAdapter_SetVerbosity(t *testing.T) {
	var buf bytes.Buffer
	adapter := NewTUIAdapter(TUIAdapterConfig{
		Writer:    &buf,
		Verbosity: VerbosityTask,
	})
	defer adapter.Stop()

	adapter.SetVerbosity(VerbosityEpic)
	if adapter.GetVerbosity() != VerbosityEpic {
		t.Errorf("expected verbosity %d, got %d", VerbosityEpic, adapter.GetVerbosity())
	}
}

func TestTUIAdapter_NotifyEvent_ThinkingCoalescing(t *testing.T) {
	var buf bytes.Buffer
	adapter := NewTUIAdapter(TUIAdapterConfig{
		Writer:    &buf,
		Verbosity: VerbosityTask,
	})

	// Send multiple thinking chunks
	for i := 0; i < 3; i++ {
		rawJSON, _ := json.Marshal(map[string]any{
			"thought": "chunk ",
		})
		adapter.NotifyEvent(registry.UIEvent{
			Type: registry.EventACPStream,
			Payload: acp.Event{
				Kind: acp.EventThinking,
				Raw:  rawJSON,
			},
		})
	}

	// Wait for processing
	time.Sleep(50 * time.Millisecond)

	// Send a message to flush thinking buffer
	msgJSON, _ := json.Marshal(map[string]any{
		"content": "Hello",
	})
	adapter.NotifyEvent(registry.UIEvent{
		Type: registry.EventACPStream,
		Payload: acp.Event{
			Kind: acp.EventMessage,
			Raw:  msgJSON,
		},
	})

	// Wait for flush
	time.Sleep(50 * time.Millisecond)
	adapter.Stop()

	output := buf.String()
	// Thinking should be coalesced into single line with marker
	if !bytes.Contains(buf.Bytes(), []byte("thinking")) {
		t.Errorf("expected thinking marker in output, got: %s", output)
	}
}

func TestTUIAdapter_NotifyEvent_MessageStreaming(t *testing.T) {
	var buf bytes.Buffer
	adapter := NewTUIAdapter(TUIAdapterConfig{
		Writer:    &buf,
		Verbosity: VerbosityTask,
	})

	// Send message chunks
	chunks := []string{"Hello", " ", "World"}
	for _, chunk := range chunks {
		rawJSON, _ := json.Marshal(map[string]any{
			"content": chunk,
		})
		adapter.NotifyEvent(registry.UIEvent{
			Type: registry.EventACPStream,
			Payload: acp.Event{
				Kind: acp.EventMessage,
				Raw:  rawJSON,
			},
		})
	}

	// Wait for processing
	time.Sleep(50 * time.Millisecond)
	adapter.Stop()

	output := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("Hello")) || !bytes.Contains(buf.Bytes(), []byte("World")) {
		t.Errorf("expected message content in output, got: %s", output)
	}
}

func TestTUIAdapter_NotifyEvent_ToolCall(t *testing.T) {
	var buf bytes.Buffer
	adapter := NewTUIAdapter(TUIAdapterConfig{
		Writer:    &buf,
		Verbosity: VerbosityTask,
	})

	adapter.NotifyEvent(registry.UIEvent{
		Type: registry.EventACPStream,
		Payload: acp.Event{
			Kind:     acp.EventToolCall,
			ToolName: "read_file",
		},
	})

	time.Sleep(50 * time.Millisecond)
	adapter.Stop()

	output := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("tool: read_file")) {
		t.Errorf("expected tool call in output, got: %s", output)
	}
}

func TestTUIAdapter_VerbosityFiltering_Story(t *testing.T) {
	var buf bytes.Buffer
	adapter := NewTUIAdapter(TUIAdapterConfig{
		Writer:    &buf,
		Verbosity: VerbosityStory, // Should filter thinking and tool_update
	})

	// Thinking should be filtered
	adapter.NotifyEvent(registry.UIEvent{
		Type: registry.EventACPStream,
		Payload: acp.Event{
			Kind: acp.EventThinking,
			Raw:  json.RawMessage(`{"thought":"test"}`),
		},
	})

	// Tool update should be filtered
	adapter.NotifyEvent(registry.UIEvent{
		Type: registry.EventACPStream,
		Payload: acp.Event{
			Kind:     acp.EventToolUpdate,
			ToolName: "bash",
		},
	})

	// Tool call should NOT be filtered
	adapter.NotifyEvent(registry.UIEvent{
		Type: registry.EventACPStream,
		Payload: acp.Event{
			Kind:     acp.EventToolCall,
			ToolName: "read",
		},
	})

	// Message should NOT be filtered
	adapter.NotifyEvent(registry.UIEvent{
		Type: registry.EventACPStream,
		Payload: acp.Event{
			Kind: acp.EventMessage,
			Raw:  json.RawMessage(`{"content":"visible"}`),
		},
	})

	time.Sleep(50 * time.Millisecond)
	adapter.Stop()

	output := buf.String()

	// Should NOT contain thinking
	if bytes.Contains(buf.Bytes(), []byte("thinking")) {
		t.Errorf("thinking should be filtered at story verbosity, got: %s", output)
	}

	// Should contain tool call
	if !bytes.Contains(buf.Bytes(), []byte("tool: read")) {
		t.Errorf("tool call should not be filtered at story verbosity, got: %s", output)
	}

	// Should contain message
	if !bytes.Contains(buf.Bytes(), []byte("visible")) {
		t.Errorf("message should not be filtered at story verbosity, got: %s", output)
	}
}

func TestTUIAdapter_VerbosityFiltering_Epic(t *testing.T) {
	var buf bytes.Buffer
	adapter := NewTUIAdapter(TUIAdapterConfig{
		Writer:    &buf,
		Verbosity: VerbosityEpic, // Only turn_end and task_status
	})

	// Message should be filtered
	adapter.NotifyEvent(registry.UIEvent{
		Type: registry.EventACPStream,
		Payload: acp.Event{
			Kind: acp.EventMessage,
			Raw:  json.RawMessage(`{"content":"hidden"}`),
		},
	})

	// Tool call should be filtered
	adapter.NotifyEvent(registry.UIEvent{
		Type: registry.EventACPStream,
		Payload: acp.Event{
			Kind:     acp.EventToolCall,
			ToolName: "hidden_tool",
		},
	})

	// Turn end should NOT be filtered
	adapter.NotifyEvent(registry.UIEvent{
		Type: registry.EventACPStream,
		Payload: acp.Event{
			Kind: acp.EventTurnEnd,
		},
	})

	// Task status changed should NOT be filtered
	adapter.NotifyEvent(registry.UIEvent{
		Type:    registry.EventTaskStatusChanged,
		Message: "Task completed",
	})

	time.Sleep(50 * time.Millisecond)
	adapter.Stop()

	output := buf.String()

	// Should NOT contain hidden content
	if bytes.Contains(buf.Bytes(), []byte("hidden")) {
		t.Errorf("message/tool should be filtered at epic verbosity, got: %s", output)
	}

	// Should contain turn end
	if !bytes.Contains(buf.Bytes(), []byte("turn end")) {
		t.Errorf("turn end should not be filtered at epic verbosity, got: %s", output)
	}

	// Should contain task status
	if !bytes.Contains(buf.Bytes(), []byte("Task completed")) {
		t.Errorf("task status should not be filtered at epic verbosity, got: %s", output)
	}
}

func TestTUIAdapter_Backpressure(t *testing.T) {
	var buf bytes.Buffer
	adapter := NewTUIAdapter(TUIAdapterConfig{
		Writer:    &buf,
		Verbosity: VerbosityTask,
	})

	// Send more events than the channel capacity (256)
	for i := 0; i < 300; i++ {
		adapter.NotifyEvent(registry.UIEvent{
			Type: registry.EventACPStream,
			Payload: acp.Event{
				Kind: acp.EventMessage,
				Raw:  json.RawMessage(`{"content":"x"}`),
			},
		})
	}

	// Wait a bit then check dropped events
	time.Sleep(100 * time.Millisecond)
	adapter.Stop()

	dropped := adapter.DroppedEvents()
	// With 300 events and 256 capacity, some should be dropped
	// unless the consumer is fast enough
	t.Logf("Dropped events: %d", dropped)
}

func TestTUIAdapter_Backpressure_HighVolume(t *testing.T) {
	var buf bytes.Buffer
	adapter := NewTUIAdapter(TUIAdapterConfig{
		Writer:    &buf,
		Verbosity: VerbosityTask,
	})

	// Simulate stress test: 10k thinking chunks
	var wg sync.WaitGroup
	for i := 0; i < 10000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			adapter.NotifyEvent(registry.UIEvent{
				Type: registry.EventACPStream,
				Payload: acp.Event{
					Kind: acp.EventThinking,
					Raw:  json.RawMessage(`{"thought":"x"}`),
				},
			})
		}()
	}

	wg.Wait()
	time.Sleep(200 * time.Millisecond)
	adapter.Stop()

	dropped := adapter.DroppedEvents()
	t.Logf("Stress test: 10k events, %d dropped", dropped)

	// The adapter should remain responsive (not hang)
	// This test passes if it completes within timeout
}

func TestTUIAdapter_InputFocused(t *testing.T) {
	var buf bytes.Buffer
	adapter := NewTUIAdapter(TUIAdapterConfig{
		Writer:    &buf,
		Verbosity: VerbosityTask,
	})

	// Set input focused - renders should be buffered
	adapter.SetInputFocused(true)

	adapter.NotifyEvent(registry.UIEvent{
		Type: registry.EventACPStream,
		Payload: acp.Event{
			Kind: acp.EventMessage,
			Raw:  json.RawMessage(`{"content":"buffered"}`),
		},
	})

	time.Sleep(50 * time.Millisecond)

	// Output should be empty while input is focused
	if buf.Len() > 0 {
		t.Errorf("expected empty output while input focused, got: %s", buf.String())
	}

	// Release input focus and flush
	adapter.SetInputFocused(false)
	adapter.FlushPendingOutput()

	time.Sleep(50 * time.Millisecond)
	adapter.Stop()

	// Now output should contain the message
	if !bytes.Contains(buf.Bytes(), []byte("buffered")) {
		t.Errorf("expected buffered content after flush, got: %s", buf.String())
	}
}

func TestTUIAdapter_PromptDecision(t *testing.T) {
	var buf bytes.Buffer
	adapter := NewTUIAdapter(TUIAdapterConfig{
		Writer:    &buf,
		Verbosity: VerbosityTask,
	})
	defer adapter.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req := registry.PromptRequest{
		Title:   "Test Prompt",
		Body:    "Do you approve?",
		Options: []string{"Yes", "No"},
	}

	// PromptDecision should block until context is canceled
	resp := adapter.PromptDecision(ctx, req)

	if resp.Error != ErrPromptCanceled {
		t.Errorf("expected ErrPromptCanceled, got: %v", resp.Error)
	}

	// Should have rendered the prompt
	output := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("Test Prompt")) {
		t.Errorf("expected prompt title in output, got: %s", output)
	}
}

func TestParseVerbosityLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected VerbosityLevel
		wantErr  bool
	}{
		{"0", VerbosityTask, false},
		{"task", VerbosityTask, false},
		{"TASK", VerbosityTask, false},
		{"1", VerbosityStory, false},
		{"story", VerbosityStory, false},
		{"2", VerbosityEpic, false},
		{"epic", VerbosityEpic, false},
		{"invalid", VerbosityTask, true},
		{"3", VerbosityTask, true},
		{"-1", VerbosityTask, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseVerbosityLevel(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseVerbosityLevel(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.expected {
				t.Errorf("ParseVerbosityLevel(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestVerbosityLevel_String(t *testing.T) {
	tests := []struct {
		level    VerbosityLevel
		expected string
	}{
		{VerbosityTask, "task"},
		{VerbosityStory, "story"},
		{VerbosityEpic, "epic"},
		{VerbosityLevel(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.level.String(); got != tt.expected {
				t.Errorf("VerbosityLevel(%d).String() = %q, want %q", tt.level, got, tt.expected)
			}
		})
	}
}

func TestTUIAdapter_TaskStatusChanged(t *testing.T) {
	var buf bytes.Buffer
	adapter := NewTUIAdapter(TUIAdapterConfig{
		Writer:    &buf,
		Verbosity: VerbosityTask,
	})

	adapter.NotifyEvent(registry.UIEvent{
		Type:    registry.EventTaskStatusChanged,
		Message: "Task running → completed",
	})

	time.Sleep(50 * time.Millisecond)
	adapter.Stop()

	output := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("Task running")) {
		t.Errorf("expected task status message in output, got: %s", output)
	}
}

func TestTUIAdapter_AutoApproved(t *testing.T) {
	var buf bytes.Buffer
	adapter := NewTUIAdapter(TUIAdapterConfig{
		Writer:    &buf,
		Verbosity: VerbosityTask,
	})

	adapter.NotifyEvent(registry.UIEvent{
		Type:    registry.EventACPAutoApproved,
		Message: "ls -la",
	})

	time.Sleep(50 * time.Millisecond)
	adapter.Stop()

	output := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("Auto-approved")) {
		t.Errorf("expected auto-approved message in output, got: %s", output)
	}
}

func TestTUIAdapter_AutoApproved_FilteredAtEpic(t *testing.T) {
	var buf bytes.Buffer
	adapter := NewTUIAdapter(TUIAdapterConfig{
		Writer:    &buf,
		Verbosity: VerbosityEpic,
	})

	adapter.NotifyEvent(registry.UIEvent{
		Type:    registry.EventACPAutoApproved,
		Message: "ls -la",
	})

	time.Sleep(50 * time.Millisecond)
	adapter.Stop()

	output := buf.String()
	if bytes.Contains(buf.Bytes(), []byte("Auto-approved")) {
		t.Errorf("auto-approved should be filtered at epic verbosity, got: %s", output)
	}
}
