package orchestrator

import (
	"context"
	"log/slog"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/registry"
)

// Fanout consumes classified events from the Pipeline's Events channel and
// broadcasts them to the UI mediator. This allows Story 3.8/3.9 adapters to
// subscribe and render streaming content.
type Fanout struct {
	ui     registry.UIMediator
	logger *slog.Logger
}

// FanoutConfig holds configuration for creating a Fanout.
type FanoutConfig struct {
	UI     registry.UIMediator
	Logger *slog.Logger
}

// NewFanout creates a new Fanout that broadcasts events to the UI mediator.
func NewFanout(cfg FanoutConfig) *Fanout {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Fanout{
		ui:     cfg.UI,
		logger: logger,
	}
}

// Run starts the fanout, consuming events from the events channel and
// broadcasting them to the UI mediator. It blocks until the channel is
// closed or the context is cancelled.
func (f *Fanout) Run(ctx context.Context, events <-chan acp.Event, taskID string) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			f.broadcast(ev, taskID)
		}
	}
}

// broadcast sends an event to the UI mediator as an EventACPStream event.
func (f *Fanout) broadcast(ev acp.Event, taskID string) {
	if f.ui == nil {
		return
	}

	uiEvent := registry.UIEvent{
		Type:    registry.EventACPStream,
		TaskID:  taskID,
		Message: ev.Kind.String(),
		Payload: ev,
	}

	f.ui.NotifyAll(uiEvent)
}
