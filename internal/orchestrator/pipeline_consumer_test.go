package orchestrator

import (
	"context"
	"testing"
	"time"

	"codemint.kanthorlabs.com/internal/acp"
)

func TestPipelineConsumer_BufferHookup(t *testing.T) {
	// Create a BufferRegistry.
	bufReg := acp.NewBufferRegistry(256)

	// Create a mock worker with a known task ID.
	worker := &mockWorkerWithTaskID{taskID: "task-123"}

	// Create the PipelineConsumer with the buffer registry.
	consumer := NewPipelineConsumer(PipelineConsumerConfig{
		BufferRegistry: bufReg,
		Worker:         worker,
	})

	// Create input channel and pipeline.
	input := make(chan acp.Message, 10)
	pipeline := acp.NewPipeline(input, acp.PipelineConfig{BufferSize: 10})

	// Start the pipeline in a goroutine.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go pipeline.Run(ctx)

	// Start the consumer in a goroutine.
	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		consumer.Run(ctx, pipeline, "sess-abc")
	}()

	// Send some events through the pipeline.
	input <- acp.Message{
		JSONRPC: "2.0",
		Method:  "session/update",
		Params:  []byte(`{"sessionId":"acp-sess","update":{"sessionUpdate":"agent_thought_chunk","content":"thinking..."}}`),
	}
	input <- acp.Message{
		JSONRPC: "2.0",
		Method:  "session/update",
		Params:  []byte(`{"sessionId":"acp-sess","update":{"sessionUpdate":"agent_message_chunk","content":"Hello!"}}`),
	}

	// Give time for events to be processed.
	time.Sleep(50 * time.Millisecond)

	// Verify events are in the buffer.
	snapshot := bufReg.Snapshot("sess-abc", "task-123")
	if len(snapshot) != 2 {
		t.Errorf("Snapshot len = %d, want 2", len(snapshot))
	}

	// Verify event kinds.
	if snapshot[0].Event.Kind != acp.EventThinking {
		t.Errorf("snapshot[0].Kind = %v, want EventThinking", snapshot[0].Event.Kind)
	}
	if snapshot[1].Event.Kind != acp.EventMessage {
		t.Errorf("snapshot[1].Kind = %v, want EventMessage", snapshot[1].Event.Kind)
	}

	cancel()
	<-consumerDone
}

func TestPipelineConsumer_BufferHookup_HaltedChannel(t *testing.T) {
	// Create a BufferRegistry.
	bufReg := acp.NewBufferRegistry(256)

	// Create a mock worker with a known task ID.
	worker := &mockWorkerWithTaskID{taskID: "task-456"}

	// Create the PipelineConsumer with the buffer registry.
	consumer := NewPipelineConsumer(PipelineConsumerConfig{
		BufferRegistry: bufReg,
		Worker:         worker,
	})

	// Create input channel and pipeline.
	input := make(chan acp.Message, 10)
	pipeline := acp.NewPipeline(input, acp.PipelineConfig{BufferSize: 10})

	// Start the pipeline in a goroutine.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go pipeline.Run(ctx)

	// Start the consumer in a goroutine.
	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		consumer.Run(ctx, pipeline, "sess-xyz")
	}()

	// Send a tool_call event (goes to Halted channel).
	input <- acp.Message{
		JSONRPC: "2.0",
		Method:  "session/update",
		Params:  []byte(`{"sessionId":"acp-sess","update":{"sessionUpdate":"tool_call","tool":"bash","parameters":{"command":"ls"}}}`),
	}

	// Give time for events to be processed.
	time.Sleep(50 * time.Millisecond)

	// Verify event is in the buffer.
	snapshot := bufReg.Snapshot("sess-xyz", "task-456")
	if len(snapshot) != 1 {
		t.Errorf("Snapshot len = %d, want 1", len(snapshot))
	}

	if snapshot[0].Event.Kind != acp.EventToolCall {
		t.Errorf("snapshot[0].Kind = %v, want EventToolCall", snapshot[0].Event.Kind)
	}
	if snapshot[0].Event.ToolName != "bash" {
		t.Errorf("snapshot[0].ToolName = %v, want bash", snapshot[0].Event.ToolName)
	}

	cancel()
	<-consumerDone
}

func TestPipelineConsumer_BufferHookup_SessionDefault(t *testing.T) {
	// Test the session-default buffer (empty taskID).
	bufReg := acp.NewBufferRegistry(256)

	// Create a mock worker with empty task ID (ad-hoc prompt).
	worker := &mockWorkerWithTaskID{taskID: ""}

	consumer := NewPipelineConsumer(PipelineConsumerConfig{
		BufferRegistry: bufReg,
		Worker:         worker,
	})

	input := make(chan acp.Message, 10)
	pipeline := acp.NewPipeline(input, acp.PipelineConfig{BufferSize: 10})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go pipeline.Run(ctx)

	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		consumer.Run(ctx, pipeline, "sess-adhoc")
	}()

	// Send an event.
	input <- acp.Message{
		JSONRPC: "2.0",
		Method:  "session/update",
		Params:  []byte(`{"sessionId":"acp-sess","update":{"sessionUpdate":"agent_message_chunk","content":"Response"}}`),
	}

	time.Sleep(50 * time.Millisecond)

	// Verify event is in the session-default buffer (empty taskID).
	snapshot := bufReg.Snapshot("sess-adhoc", "")
	if len(snapshot) != 1 {
		t.Errorf("Session-default Snapshot len = %d, want 1", len(snapshot))
	}

	cancel()
	<-consumerDone
}

func TestPipelineConsumer_BufferHookup_NoWorker(t *testing.T) {
	// Test that buffer works even when worker is nil.
	bufReg := acp.NewBufferRegistry(256)

	consumer := NewPipelineConsumer(PipelineConsumerConfig{
		BufferRegistry: bufReg,
		Worker:         nil, // No worker
	})

	input := make(chan acp.Message, 10)
	pipeline := acp.NewPipeline(input, acp.PipelineConfig{BufferSize: 10})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go pipeline.Run(ctx)

	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		consumer.Run(ctx, pipeline, "sess-noworker")
	}()

	input <- acp.Message{
		JSONRPC: "2.0",
		Method:  "session/update",
		Params:  []byte(`{"sessionId":"acp-sess","update":{"sessionUpdate":"agent_thought_chunk","content":"test"}}`),
	}

	time.Sleep(50 * time.Millisecond)

	// Events should go to session-default buffer (empty taskID).
	snapshot := bufReg.Snapshot("sess-noworker", "")
	if len(snapshot) != 1 {
		t.Errorf("No-worker Snapshot len = %d, want 1", len(snapshot))
	}

	cancel()
	<-consumerDone
}

// mockWorkerWithTaskID implements just the CurrentTaskID method for testing.
type mockWorkerWithTaskID struct {
	taskID string
}

func (m *mockWorkerWithTaskID) CurrentTaskID() string {
	return m.taskID
}
