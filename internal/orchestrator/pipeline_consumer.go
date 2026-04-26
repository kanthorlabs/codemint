package orchestrator

import (
	"context"
	"log/slog"

	"codemint.kanthorlabs.com/internal/acp"
)

// TaskIDProvider provides the current task ID.
// This interface exists for testability - in production, *acp.Worker implements this.
type TaskIDProvider interface {
	CurrentTaskID() string
}

// PipelineConsumer consumes events from the ACP Pipeline and applies the StatusMapper.
// It handles both the Events channel (turn-start/turn-end) and the Halted channel
// (permission requests that were not auto-approved).
//
// This implements Task 3.7.2: Drive the Mapper From the Pipeline.
// It also pushes events to the BufferRegistry for the /summary command (Task 3.10.2).
type PipelineConsumer struct {
	mapper         *StatusMapper
	interceptor    *Interceptor
	fanout         *Fanout
	taskIDProvider TaskIDProvider
	bufferRegistry *acp.BufferRegistry
	logger         *slog.Logger
}

// PipelineConsumerConfig holds the dependencies for creating a PipelineConsumer.
type PipelineConsumerConfig struct {
	Mapper         *StatusMapper
	Interceptor    *Interceptor
	Fanout         *Fanout
	Worker         TaskIDProvider // *acp.Worker in production
	BufferRegistry *acp.BufferRegistry
	Logger         *slog.Logger
}

// NewPipelineConsumer creates a new PipelineConsumer with the provided dependencies.
func NewPipelineConsumer(cfg PipelineConsumerConfig) *PipelineConsumer {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &PipelineConsumer{
		mapper:         cfg.Mapper,
		interceptor:    cfg.Interceptor,
		fanout:         cfg.Fanout,
		taskIDProvider: cfg.Worker,
		bufferRegistry: cfg.BufferRegistry,
		logger:         logger,
	}
}

// Run starts consuming events from the pipeline channels.
// It blocks until the context is cancelled or both channels are closed.
// The sessionID is used for interceptor context and buffer registry.
func (c *PipelineConsumer) Run(ctx context.Context, pipeline *acp.Pipeline, sessionID string) {
	// Start goroutines to consume both channels.
	eventsDone := make(chan struct{})
	haltedDone := make(chan struct{})

	go func() {
		defer close(eventsDone)
		c.consumeEvents(ctx, pipeline.Events, sessionID)
	}()

	go func() {
		defer close(haltedDone)
		c.consumeHalted(ctx, pipeline.Halted, sessionID)
	}()

	// Wait for both to complete.
	<-eventsDone
	<-haltedDone
}

// consumeEvents processes events from the Pipeline.Events channel.
// It applies StatusMapper for turn-start/turn-end events and forwards
// all events to Fanout for UI rendering.
func (c *PipelineConsumer) consumeEvents(ctx context.Context, events <-chan acp.Event, sessionID string) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}

			// Get the current task ID from the worker.
			taskID := ""
			if c.taskIDProvider != nil {
				taskID = c.taskIDProvider.CurrentTaskID()
			}

			// Push to BufferRegistry for /summary command (Task 3.10.2).
			// This captures the event before any processing for debugging purposes.
			if c.bufferRegistry != nil {
				c.bufferRegistry.Push(sessionID, taskID, ev)
			}

			// Check for memory-override tag in agent messages (Task 3.11.4).
			// This logs when the agent overrides project preferences from memory.
			if acp.ContainsMemoryOverrideTag(ev) {
				c.logger.Info("acp.memory.override",
					"session_id", sessionID,
					"task_id", taskID,
					"event_kind", ev.Kind.String(),
				)
			}

			// Apply StatusMapper for lifecycle events (turn-start/turn-end).
			if c.mapper != nil && (ev.Kind == acp.EventTurnStart || ev.Kind == acp.EventTurnEnd) {
				if err := c.mapper.Apply(ctx, taskID, ev); err != nil {
					c.logger.Error("pipeline_consumer: failed to apply status transition",
						"kind", ev.Kind.String(),
						"task_id", taskID,
						"error", err,
					)
				}
			}

			// Forward to Fanout for UI rendering.
			if c.fanout != nil {
				c.fanout.broadcast(ev, taskID)
			}
		}
	}
}

// consumeHalted processes events from the Pipeline.Halted channel.
// For permission requests from the Block/Unknown path (Story 3.6), it applies
// StatusMapper to transition the task to Awaiting. For auto-allow path (Story 3.5),
// the status stays Processing.
func (c *PipelineConsumer) consumeHalted(ctx context.Context, halted <-chan acp.Event, sessionID string) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-halted:
			if !ok {
				return
			}

			// Get the current task ID from the worker.
			taskID := ""
			if c.taskIDProvider != nil {
				taskID = c.taskIDProvider.CurrentTaskID()
			}

			// Push to BufferRegistry for /summary command (Task 3.10.2).
			// This captures tool calls and permission requests for debugging.
			if c.bufferRegistry != nil {
				c.bufferRegistry.Push(sessionID, taskID, ev)
			}

			// Forward to interceptor to evaluate against permissions.
			// The interceptor handles the auto-allow vs Block/Unknown decision.
			// If Block/Unknown, the interceptor will call taskRepo.UpdateTaskStatus
			// to transition to Awaiting (this is already implemented in interceptor.go).
			if c.interceptor != nil {
				hctx := HandleContext{
					SessionID: sessionID,
					TaskID:    taskID,
				}
				c.interceptor.Handle(ctx, ev, hctx)
			}

			// Note: We don't apply StatusMapper here for permission requests because:
			// 1. For auto-allow path: status stays Processing (no transition needed)
			// 2. For Block/Unknown path: interceptor already handles the Awaiting transition
		}
	}
}
