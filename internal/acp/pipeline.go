package acp

import (
	"context"
	"log/slog"
	"sync/atomic"
)

const (
	// DefaultPipelineBufferSize is the default buffer size for pipeline channels.
	DefaultPipelineBufferSize = 256
)

// Pipeline routes classified events from a Worker's output channel to separate
// channels based on event type. Tool-call and permission-request events are
// sent to the Halted channel for interception, while all other events flow
// to the Events channel.
type Pipeline struct {
	in <-chan Message // from Worker.Out()

	// Events receives classified, non-intercepted events (thinking, message, plan, etc.)
	Events chan Event

	// Halted receives tool_call and permission_request events for the interceptor.
	Halted chan Event

	// Dropped tracks the number of events dropped due to full buffers.
	dropped atomic.Int64

	logger *slog.Logger
}

// PipelineConfig holds configuration for creating a Pipeline.
type PipelineConfig struct {
	BufferSize int
	Logger     *slog.Logger
}

// NewPipeline creates a new Pipeline that reads from the given input channel.
// The pipeline must be started with Run().
func NewPipeline(in <-chan Message, cfg PipelineConfig) *Pipeline {
	bufSize := cfg.BufferSize
	if bufSize <= 0 {
		bufSize = DefaultPipelineBufferSize
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Pipeline{
		in:     in,
		Events: make(chan Event, bufSize),
		Halted: make(chan Event, bufSize),
		logger: logger,
	}
}

// Run starts the pipeline, reading from the input channel and routing events
// to the appropriate output channels. It blocks until the input channel is
// closed or the context is cancelled. Both output channels are closed when
// Run returns.
func (p *Pipeline) Run(ctx context.Context) {
	defer func() {
		close(p.Events)
		close(p.Halted)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-p.in:
			if !ok {
				// Input channel closed, drain complete
				return
			}
			p.route(Classify(msg))
		}
	}
}

// route sends the event to the appropriate channel based on its kind.
func (p *Pipeline) route(ev Event) {
	if p.shouldHalt(ev.Kind) {
		p.trySend(p.Halted, ev, "halted")
	} else {
		p.trySend(p.Events, ev, "events")
	}
}

// shouldHalt returns true if the event kind should be sent to the Halted channel.
func (p *Pipeline) shouldHalt(kind EventKind) bool {
	return kind == EventToolCall || kind == EventPermissionRequest
}

// trySend attempts to send an event to a channel without blocking.
// If the channel is full, the event is dropped and a counter is incremented.
func (p *Pipeline) trySend(ch chan Event, ev Event, name string) {
	select {
	case ch <- ev:
		// Successfully sent
	default:
		// Channel full, drop the event
		count := p.dropped.Add(1)
		p.logger.Warn("pipeline buffer full, dropping event",
			"channel", name,
			"kind", ev.Kind.String(),
			"dropped_total", count,
		)
	}
}

// Dropped returns the total number of events dropped due to full buffers.
func (p *Pipeline) Dropped() int64 {
	return p.dropped.Load()
}
