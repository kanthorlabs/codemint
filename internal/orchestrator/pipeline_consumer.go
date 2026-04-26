package orchestrator

import (
	"context"
	"log/slog"

	"codemint.kanthorlabs.com/internal/acp"
)

// PipelineConsumer consumes events from the ACP Pipeline and applies the StatusMapper.
// It handles both the Events channel (turn-start/turn-end) and the Halted channel
// (permission requests that were not auto-approved).
//
// This implements Task 3.7.2: Drive the Mapper From the Pipeline.
type PipelineConsumer struct {
	mapper      *StatusMapper
	interceptor *Interceptor
	fanout      *Fanout
	worker      *acp.Worker
	logger      *slog.Logger
}

// PipelineConsumerConfig holds the dependencies for creating a PipelineConsumer.
type PipelineConsumerConfig struct {
	Mapper      *StatusMapper
	Interceptor *Interceptor
	Fanout      *Fanout
	Worker      *acp.Worker
	Logger      *slog.Logger
}

// NewPipelineConsumer creates a new PipelineConsumer with the provided dependencies.
func NewPipelineConsumer(cfg PipelineConsumerConfig) *PipelineConsumer {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &PipelineConsumer{
		mapper:      cfg.Mapper,
		interceptor: cfg.Interceptor,
		fanout:      cfg.Fanout,
		worker:      cfg.Worker,
		logger:      logger,
	}
}

// Run starts consuming events from the pipeline channels.
// It blocks until the context is cancelled or both channels are closed.
// The sessionID is used for interceptor context.
func (c *PipelineConsumer) Run(ctx context.Context, pipeline *acp.Pipeline, sessionID string) {
	// Start goroutines to consume both channels.
	eventsDone := make(chan struct{})
	haltedDone := make(chan struct{})

	go func() {
		defer close(eventsDone)
		c.consumeEvents(ctx, pipeline.Events)
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
func (c *PipelineConsumer) consumeEvents(ctx context.Context, events <-chan acp.Event) {
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
			if c.worker != nil {
				taskID = c.worker.CurrentTaskID()
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
			if c.worker != nil {
				taskID = c.worker.CurrentTaskID()
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
