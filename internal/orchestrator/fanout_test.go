package orchestrator

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/registry"
)

func TestFanout_Run(t *testing.T) {
	ui := &mockUIMediator{}
	fanout := NewFanout(FanoutConfig{
		UI: ui,
	})

	events := make(chan acp.Event, 3)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		fanout.Run(ctx, events, "task-fanout")
		close(done)
	}()

	// Send events
	events <- acp.Event{
		Kind:         acp.EventMessage,
		ACPSessionID: "sess-1",
		Raw:          json.RawMessage(`{"content":"Hello"}`),
	}
	events <- acp.Event{
		Kind:         acp.EventThinking,
		ACPSessionID: "sess-1",
		Raw:          json.RawMessage(`{"content":"Analyzing..."}`),
	}
	events <- acp.Event{
		Kind:         acp.EventPlan,
		ACPSessionID: "sess-1",
		Raw:          json.RawMessage(`{"steps":[]}`),
	}

	close(events)

	// Wait for fanout to finish
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("fanout did not finish")
	}

	uiEvents := ui.Events()
	if len(uiEvents) != 3 {
		t.Fatalf("expected 3 UI events, got %d", len(uiEvents))
	}

	// Verify all events have correct type
	for i, ev := range uiEvents {
		if ev.Type != registry.EventACPStream {
			t.Errorf("event[%d].Type = %v, want %v", i, ev.Type, registry.EventACPStream)
		}
		if ev.TaskID != "task-fanout" {
			t.Errorf("event[%d].TaskID = %v, want %v", i, ev.TaskID, "task-fanout")
		}
	}

	// Verify event kinds in order
	expectedKinds := []string{"message", "thinking", "plan"}
	for i, ev := range uiEvents {
		if ev.Message != expectedKinds[i] {
			t.Errorf("event[%d].Message = %v, want %v", i, ev.Message, expectedKinds[i])
		}
	}
}

func TestFanout_ToolEventsNotReceived(t *testing.T) {
	// This test verifies that tool events are NOT in the events channel
	// (they should be in the Halted channel, consumed by Interceptor)
	ui := &mockUIMediator{}
	fanout := NewFanout(FanoutConfig{
		UI: ui,
	})

	events := make(chan acp.Event, 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		fanout.Run(ctx, events, "task-no-tools")
		close(done)
	}()

	// Send only non-tool events (tool events go to Halted, not Events)
	events <- acp.Event{
		Kind:         acp.EventMessage,
		ACPSessionID: "sess-1",
	}
	events <- acp.Event{
		Kind:         acp.EventToolUpdate, // tool_update goes to Events, not Halted
		ACPSessionID: "sess-1",
		ToolName:     "bash",
	}

	close(events)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("fanout did not finish")
	}

	uiEvents := ui.Events()
	if len(uiEvents) != 2 {
		t.Fatalf("expected 2 UI events, got %d", len(uiEvents))
	}

	// Verify payloads are acp.Event
	for i, uiEvent := range uiEvents {
		_, ok := uiEvent.Payload.(acp.Event)
		if !ok {
			t.Errorf("event[%d].Payload is not acp.Event", i)
		}
	}
}

func TestFanout_ContextCancel(t *testing.T) {
	ui := &mockUIMediator{}
	fanout := NewFanout(FanoutConfig{
		UI: ui,
	})

	events := make(chan acp.Event)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		fanout.Run(ctx, events, "task-cancel")
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("fanout did not finish on context cancel")
	}
}

func TestFanout_NilUI(t *testing.T) {
	fanout := NewFanout(FanoutConfig{
		// No UI
	})

	events := make(chan acp.Event, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		fanout.Run(ctx, events, "task-nil")
		close(done)
	}()

	// Should not panic with nil UI
	events <- acp.Event{
		Kind: acp.EventMessage,
	}

	close(events)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("fanout did not finish")
	}
}

func TestFanout_Broadcast(t *testing.T) {
	ui := &mockUIMediator{}
	fanout := NewFanout(FanoutConfig{
		UI: ui,
	})

	ev := acp.Event{
		Kind:         acp.EventThinking,
		ACPSessionID: "sess-test",
		Raw:          json.RawMessage(`{"content":"test"}`),
	}

	fanout.broadcast(ev, "task-broadcast")

	uiEvents := ui.Events()
	if len(uiEvents) != 1 {
		t.Fatalf("expected 1 UI event, got %d", len(uiEvents))
	}

	uiEvent := uiEvents[0]
	if uiEvent.Type != registry.EventACPStream {
		t.Errorf("Type = %v, want %v", uiEvent.Type, registry.EventACPStream)
	}
	if uiEvent.TaskID != "task-broadcast" {
		t.Errorf("TaskID = %v, want %v", uiEvent.TaskID, "task-broadcast")
	}
	if uiEvent.Message != "thinking" {
		t.Errorf("Message = %v, want %v", uiEvent.Message, "thinking")
	}

	payload, ok := uiEvent.Payload.(acp.Event)
	if !ok {
		t.Fatal("Payload is not acp.Event")
	}
	if payload.Kind != acp.EventThinking {
		t.Errorf("payload.Kind = %v, want %v", payload.Kind, acp.EventThinking)
	}
	if payload.ACPSessionID != "sess-test" {
		t.Errorf("payload.ACPSessionID = %v, want %v", payload.ACPSessionID, "sess-test")
	}
}
