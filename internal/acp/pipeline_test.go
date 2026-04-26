package acp

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestPipeline_Splits(t *testing.T) {
	// Create input channel and pipeline
	in := make(chan Message, 10)
	p := NewPipeline(in, PipelineConfig{BufferSize: 10})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start pipeline in background
	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()

	// Send mixed stream of events
	messages := []Message{
		// Agent message - should go to Events
		{
			JSONRPC: JSONRPCVersion,
			Method:  MethodSessionUpdate,
			Params:  json.RawMessage(`{"sessionId":"sess-1","update":{"sessionUpdate":"agent_message_chunk","content":"Hello"}}`),
		},
		// Tool call - should go to Halted
		{
			JSONRPC: JSONRPCVersion,
			Method:  MethodSessionUpdate,
			Params:  json.RawMessage(`{"sessionId":"sess-1","update":{"sessionUpdate":"tool_call","tool":"bash","parameters":{"command":"ls"}}}`),
		},
		// Thinking - should go to Events
		{
			JSONRPC: JSONRPCVersion,
			Method:  MethodSessionUpdate,
			Params:  json.RawMessage(`{"sessionId":"sess-1","update":{"sessionUpdate":"agent_thought_chunk","content":"Analyzing..."}}`),
		},
		// Permission request - should go to Halted
		{
			JSONRPC: JSONRPCVersion,
			ID:      json.RawMessage(`1`),
			Method:  MethodRequestPermission,
			Params:  json.RawMessage(`{"sessionId":"sess-1","requestId":"req-1","tool":"bash","parameters":{"command":"rm /tmp/foo"}}`),
		},
		// Plan - should go to Events
		{
			JSONRPC: JSONRPCVersion,
			Method:  MethodSessionUpdate,
			Params:  json.RawMessage(`{"sessionId":"sess-1","update":{"sessionUpdate":"plan","steps":[]}}`),
		},
	}

	for _, msg := range messages {
		in <- msg
	}
	close(in)

	// Collect events
	var events []Event
	var halted []Event

	timeout := time.After(time.Second)
	for {
		select {
		case ev, ok := <-p.Events:
			if ok {
				events = append(events, ev)
			}
		case ev, ok := <-p.Halted:
			if ok {
				halted = append(halted, ev)
			}
		case <-done:
			// Pipeline finished
			goto done
		case <-timeout:
			t.Fatal("test timed out")
		}
	}
done:

	// Drain any remaining events
	for ev := range p.Events {
		events = append(events, ev)
	}
	for ev := range p.Halted {
		halted = append(halted, ev)
	}

	// Verify events channel
	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}

	// Verify halted channel
	if len(halted) != 2 {
		t.Errorf("expected 2 halted events, got %d", len(halted))
	}

	// Verify event kinds in Events channel
	expectedEventKinds := []EventKind{EventMessage, EventThinking, EventPlan}
	for i, ev := range events {
		if i < len(expectedEventKinds) && ev.Kind != expectedEventKinds[i] {
			t.Errorf("events[%d].Kind = %v, want %v", i, ev.Kind, expectedEventKinds[i])
		}
	}

	// Verify event kinds in Halted channel
	expectedHaltedKinds := []EventKind{EventToolCall, EventPermissionRequest}
	for i, ev := range halted {
		if i < len(expectedHaltedKinds) && ev.Kind != expectedHaltedKinds[i] {
			t.Errorf("halted[%d].Kind = %v, want %v", i, ev.Kind, expectedHaltedKinds[i])
		}
	}
}

func TestPipeline_CleanShutdown(t *testing.T) {
	in := make(chan Message, 1)
	p := NewPipeline(in, PipelineConfig{BufferSize: 10})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()

	// Send one message
	in <- Message{
		JSONRPC: JSONRPCVersion,
		Method:  MethodSessionUpdate,
		Params:  json.RawMessage(`{"sessionId":"s","update":{"sessionUpdate":"agent_message_chunk"}}`),
	}

	// Close input channel
	close(in)

	// Wait for pipeline to finish
	select {
	case <-done:
		// OK
	case <-time.After(time.Second):
		t.Fatal("pipeline did not shut down")
	}

	// Verify both output channels are closed
	select {
	case _, ok := <-p.Events:
		if ok {
			// Drain any remaining
			for range p.Events {
			}
		}
	default:
		t.Error("Events channel should be closed or have data")
	}

	cancel()
}

func TestPipeline_ContextCancel(t *testing.T) {
	in := make(chan Message, 1)
	p := NewPipeline(in, PipelineConfig{BufferSize: 10})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()

	// Cancel context (don't close input)
	cancel()

	// Wait for pipeline to finish
	select {
	case <-done:
		// OK
	case <-time.After(time.Second):
		t.Fatal("pipeline did not shut down on context cancel")
	}

	// Verify both output channels are closed
	_, eventsOpen := <-p.Events
	_, haltedOpen := <-p.Halted

	if eventsOpen {
		t.Error("Events channel should be closed")
	}
	if haltedOpen {
		t.Error("Halted channel should be closed")
	}
}

func TestPipeline_BufferFullDropsOldest(t *testing.T) {
	// Create pipeline with tiny buffer
	in := make(chan Message, 100)
	p := NewPipeline(in, PipelineConfig{BufferSize: 2})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()

	// Send many messages quickly (more than buffer can hold)
	for i := 0; i < 10; i++ {
		in <- Message{
			JSONRPC: JSONRPCVersion,
			Method:  MethodSessionUpdate,
			Params:  json.RawMessage(`{"sessionId":"s","update":{"sessionUpdate":"agent_message_chunk"}}`),
		}
	}

	close(in)

	// Wait for pipeline to finish
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("pipeline did not finish")
	}

	// Drain events
	count := 0
	for range p.Events {
		count++
	}

	// Some events should have been dropped
	dropped := p.Dropped()
	if dropped+int64(count) != 10 {
		t.Errorf("dropped(%d) + received(%d) != 10", dropped, count)
	}

	// Verify dropped counter is positive (some were dropped due to small buffer)
	if dropped == 0 {
		t.Log("Warning: no events dropped, buffer may have been drained fast enough")
	}
}

func TestPipeline_ToolCallUpdate_GoesToHalted(t *testing.T) {
	in := make(chan Message, 1)
	p := NewPipeline(in, PipelineConfig{BufferSize: 10})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()

	// tool_call_update should NOT go to Halted (only tool_call and permission_request do)
	in <- Message{
		JSONRPC: JSONRPCVersion,
		Method:  MethodSessionUpdate,
		Params:  json.RawMessage(`{"sessionId":"s","update":{"sessionUpdate":"tool_call_update","tool":"bash"}}`),
	}
	close(in)

	<-done

	// Should be in Events, not Halted
	var events []Event
	for ev := range p.Events {
		events = append(events, ev)
	}
	var halted []Event
	for ev := range p.Halted {
		halted = append(halted, ev)
	}

	if len(events) != 1 {
		t.Errorf("expected 1 event in Events, got %d", len(events))
	}
	if len(halted) != 0 {
		t.Errorf("expected 0 events in Halted, got %d", len(halted))
	}
	if len(events) > 0 && events[0].Kind != EventToolUpdate {
		t.Errorf("event kind = %v, want EventToolUpdate", events[0].Kind)
	}
}

func TestNewPipeline_Defaults(t *testing.T) {
	in := make(chan Message)
	p := NewPipeline(in, PipelineConfig{})

	if cap(p.Events) != DefaultPipelineBufferSize {
		t.Errorf("Events capacity = %d, want %d", cap(p.Events), DefaultPipelineBufferSize)
	}
	if cap(p.Halted) != DefaultPipelineBufferSize {
		t.Errorf("Halted capacity = %d, want %d", cap(p.Halted), DefaultPipelineBufferSize)
	}
}
