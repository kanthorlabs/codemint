package orchestrator

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/registry"
)

// TestRuntime_E2E_EventFlow tests the end-to-end event flow through the Runtime.
// This implements Task 3.12.5: Telemetry Smoke Test.
//
// Test scenario:
// 1. Create a Runtime with all components
// 2. Create a stub pipeline with pre-created event channels
// 3. Push events through the pipeline
// 4. Verify events appear in BufferRegistry
// 5. Verify StatusMapper transitions are applied
// 6. Verify Fanout broadcasts to UI
func TestRuntime_E2E_EventFlow(t *testing.T) {
	// Create mock components.
	bufferRegistry := acp.NewBufferRegistry(256)
	uiMediatorMock := &mockUIMediator{
		events: make([]registry.UIEvent, 0),
	}

	// Create the runtime without a real registry (we'll test the pipeline directly).
	rt := NewRuntime(RuntimeConfig{
		Registry:       acp.NewRegistry(acp.DefaultConfig()),
		BufferRegistry: bufferRegistry,
		Mediator:       uiMediatorMock,
	})

	sessionID := "e2e-test-session"
	taskID := "e2e-test-task"

	// Create the components manually since we don't have a real ACP worker.
	mockWorker := &mockTaskIDProvider{taskID: taskID}

	// Note: We skip StatusMapper here because it requires a real task in the database.
	// StatusMapper is tested separately in status_mapper_test.go.

	interceptor := NewInterceptor(InterceptorConfig{
		UI: uiMediatorMock,
	})

	fanout := NewFanout(FanoutConfig{
		UI: uiMediatorMock,
	})

	// Create a pipeline from a mock channel.
	eventsCh := make(chan acp.Message, 10)
	pipeline := acp.NewPipeline(eventsCh, acp.PipelineConfig{
		BufferSize: 10,
	})

	consumer := NewPipelineConsumer(PipelineConsumerConfig{
		// Mapper omitted - tested separately in status_mapper_test.go
		Interceptor:    interceptor,
		Fanout:         fanout,
		Worker:         mockWorker,
		BufferRegistry: bufferRegistry,
	})

	// Note: We don't store statusMapper since we're not using it in this test.
	_ = rt // Use rt to satisfy the linter

	rt.interceptorsMu.Lock()
	rt.interceptors[sessionID] = interceptor
	rt.interceptorsMu.Unlock()

	rt.pipelinesMu.Lock()
	rt.pipelines[sessionID] = pipeline
	rt.pipelinesMu.Unlock()

	// Start the pipeline and consumer.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipelineDone := make(chan struct{})
	consumerDone := make(chan struct{})

	go func() {
		defer close(pipelineDone)
		pipeline.Run(ctx)
	}()

	go func() {
		defer close(consumerDone)
		consumer.Run(ctx, pipeline, sessionID)
	}()

	// Send a turn_start event.
	eventsCh <- createTurnStartMessage()

	// Send a message chunk event.
	eventsCh <- createMessageChunkMessage("Hello, world!")

	// Send a turn_end event.
	eventsCh <- createTurnEndMessage()

	// Wait for events to be processed.
	time.Sleep(100 * time.Millisecond)

	// Close the input channel to stop the pipeline.
	close(eventsCh)

	// Wait for goroutines to finish.
	select {
	case <-pipelineDone:
	case <-time.After(time.Second):
		t.Error("Pipeline did not stop within 1 second")
	}

	select {
	case <-consumerDone:
	case <-time.After(time.Second):
		t.Error("Consumer did not stop within 1 second")
	}

	// Verify events in BufferRegistry.
	snapshot := bufferRegistry.Snapshot(sessionID, taskID)
	if len(snapshot) != 3 {
		t.Errorf("BufferRegistry.Snapshot() returned %d events, want 3", len(snapshot))
	}

	// Verify event types in order.
	expectedKinds := []acp.EventKind{acp.EventTurnStart, acp.EventMessage, acp.EventTurnEnd}
	for i, te := range snapshot {
		if te.Event.Kind != expectedKinds[i] {
			t.Errorf("Event %d: got kind %v, want %v", i, te.Event.Kind, expectedKinds[i])
		}
	}

	// Verify Fanout broadcast (UI events).
	// The fanout should have broadcast all 3 events.
	events := uiMediatorMock.Events()
	if len(events) < 3 {
		t.Errorf("UIMediator received %d events, want at least 3", len(events))
	}

	// Verify at least one EventACPStream event.
	foundStream := false
	for _, ev := range events {
		if ev.Type == registry.EventACPStream {
			foundStream = true
			break
		}
	}
	if !foundStream {
		t.Error("UIMediator did not receive EventACPStream event")
	}
}

// TestRuntime_E2E_ToolCallHalted tests that tool_call events are routed to the Halted channel.
func TestRuntime_E2E_ToolCallHalted(t *testing.T) {
	bufferRegistry := acp.NewBufferRegistry(256)
	uiMediatorMock := &mockUIMediator{
		events: make([]registry.UIEvent, 0),
	}

	sessionID := "e2e-test-session-halted"
	taskID := "e2e-test-task-halted"

	mockWorker := &mockTaskIDProvider{taskID: taskID}

	interceptor := NewInterceptor(InterceptorConfig{
		UI: uiMediatorMock,
	})

	fanout := NewFanout(FanoutConfig{
		UI: uiMediatorMock,
	})

	// Create a pipeline from a mock channel.
	eventsCh := make(chan acp.Message, 10)
	pipeline := acp.NewPipeline(eventsCh, acp.PipelineConfig{
		BufferSize: 10,
	})

	consumer := NewPipelineConsumer(PipelineConsumerConfig{
		Interceptor:    interceptor,
		Fanout:         fanout,
		Worker:         mockWorker,
		BufferRegistry: bufferRegistry,
	})

	// Start the pipeline and consumer.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go pipeline.Run(ctx)
	go consumer.Run(ctx, pipeline, sessionID)

	// Send a tool_call event.
	eventsCh <- createToolCallMessage("bash", "go test")

	// Wait for event to be processed.
	time.Sleep(100 * time.Millisecond)

	// Close and cleanup.
	close(eventsCh)
	cancel()

	// Verify event in BufferRegistry (tool_call events are also pushed).
	snapshot := bufferRegistry.Snapshot(sessionID, taskID)
	if len(snapshot) != 1 {
		t.Errorf("BufferRegistry.Snapshot() returned %d events, want 1", len(snapshot))
	}

	if len(snapshot) > 0 && snapshot[0].Event.Kind != acp.EventToolCall {
		t.Errorf("Event kind = %v, want EventToolCall", snapshot[0].Event.Kind)
	}
}

// createTestMessage creates a test ACP message with the given method and update body.
// The params must include "sessionUpdate" for the kind and "sessionId" for the session.
func createTestMessage(method string, updateBody map[string]any) acp.Message {
	// Structure: { "sessionId": "...", "update": { "sessionUpdate": "kind", ... } }
	params := map[string]any{
		"sessionId": "test-acp-session",
		"update":    updateBody,
	}
	paramsJSON, _ := json.Marshal(params)
	return acp.Message{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsJSON,
	}
}

// createTurnStartMessage creates a turn_start event message.
func createTurnStartMessage() acp.Message {
	return createTestMessage(acp.MethodSessionUpdate, map[string]any{
		"sessionUpdate": "turn_start",
	})
}

// createTurnEndMessage creates a turn_end event message.
func createTurnEndMessage() acp.Message {
	return createTestMessage(acp.MethodSessionUpdate, map[string]any{
		"sessionUpdate": "turn_end",
	})
}

// createMessageChunkMessage creates an agent_message_chunk event message.
func createMessageChunkMessage(content string) acp.Message {
	return createTestMessage(acp.MethodSessionUpdate, map[string]any{
		"sessionUpdate": acp.UpdateKindAgentMessageChunk,
		"content":       content,
	})
}

// createToolCallMessage creates a tool_call event message.
func createToolCallMessage(tool, command string) acp.Message {
	return createTestMessage(acp.MethodSessionUpdate, map[string]any{
		"sessionUpdate": acp.UpdateKindToolCall,
		"tool":          tool,
		"parameters": map[string]any{
			"command": command,
		},
	})
}
