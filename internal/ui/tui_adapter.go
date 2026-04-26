package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/registry"
)

// VerbosityLevel controls the amount of detail shown in TUI output.
// Lower levels show more detail; higher levels filter more aggressively.
type VerbosityLevel int

const (
	// VerbosityTask shows everything including thinking, tool updates, messages.
	VerbosityTask VerbosityLevel = 0
	// VerbosityStory drops thinking and tool_update, keeps tool_call announcements + final messages.
	VerbosityStory VerbosityLevel = 1
	// VerbosityEpic drops everything except turn_end and task_status_changed.
	VerbosityEpic VerbosityLevel = 2
)

// String returns a human-readable name for the verbosity level.
func (v VerbosityLevel) String() string {
	switch v {
	case VerbosityTask:
		return "task"
	case VerbosityStory:
		return "story"
	case VerbosityEpic:
		return "epic"
	default:
		return "unknown"
	}
}

// ParseVerbosityLevel parses a string or integer into a VerbosityLevel.
func ParseVerbosityLevel(s string) (VerbosityLevel, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "0", "task":
		return VerbosityTask, nil
	case "1", "story":
		return VerbosityStory, nil
	case "2", "epic":
		return VerbosityEpic, nil
	default:
		return VerbosityTask, fmt.Errorf("invalid verbosity level: %q (use 0/task, 1/story, 2/epic)", s)
	}
}

// eventChannelCapacity is the buffer size for the event channel.
// Overflow drops oldest non-state events.
const eventChannelCapacity = 256

// VerbosityGetter is a function that returns the current verbosity level.
// This allows the TUI adapter to read verbosity from the session dynamically.
type VerbosityGetter func() VerbosityLevel

// TUIAdapter implements UIAdapter for terminal-based rendering with streaming
// support, backpressure handling, and verbosity filtering.
type TUIAdapter struct {
	writer          io.Writer
	logger          *slog.Logger
	verbosity       atomic.Int32    // Stores VerbosityLevel as int32 for atomic access
	verbosityGetter VerbosityGetter // Optional dynamic getter

	// Event processing
	eventCh       chan registry.UIEvent
	stopCh        chan struct{}
	wg            sync.WaitGroup
	droppedEvents atomic.Int64

	// State tracking for coalescing consecutive thinking chunks
	mu              sync.Mutex
	lastEventKind   acp.EventKind
	thinkingBuffer  strings.Builder
	thinkingStarted bool

	// Input coordination - when true, buffer renders until newline boundary
	inputFocused atomic.Bool
	renderBuffer strings.Builder
}

// TUIAdapterConfig holds configuration for creating a TUIAdapter.
type TUIAdapterConfig struct {
	Writer          io.Writer
	Logger          *slog.Logger
	Verbosity       VerbosityLevel
	VerbosityGetter VerbosityGetter // Optional: dynamic verbosity from session
}

// NewTUIAdapter creates a new TUIAdapter with the given configuration.
func NewTUIAdapter(cfg TUIAdapterConfig) *TUIAdapter {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	t := &TUIAdapter{
		writer:          cfg.Writer,
		logger:          logger,
		verbosityGetter: cfg.VerbosityGetter,
		eventCh:         make(chan registry.UIEvent, eventChannelCapacity),
		stopCh:          make(chan struct{}),
	}
	t.verbosity.Store(int32(cfg.Verbosity))

	// Start the render goroutine
	t.wg.Add(1)
	go t.renderLoop()

	return t
}

// Compile-time check that TUIAdapter implements UIAdapter.
var _ UIAdapter = (*TUIAdapter)(nil)

// SetVerbosity updates the verbosity level. Thread-safe.
func (t *TUIAdapter) SetVerbosity(level VerbosityLevel) {
	t.verbosity.Store(int32(level))
}

// GetVerbosity returns the current verbosity level. Thread-safe.
// If a VerbosityGetter was configured, it takes precedence.
func (t *TUIAdapter) GetVerbosity() VerbosityLevel {
	if t.verbosityGetter != nil {
		return t.verbosityGetter()
	}
	return VerbosityLevel(t.verbosity.Load())
}

// SetInputFocused indicates whether the REPL has stdin focus.
// When true, renders are buffered to avoid interleaving with user input.
func (t *TUIAdapter) SetInputFocused(focused bool) {
	t.inputFocused.Store(focused)
}

// DroppedEvents returns the number of events dropped due to backpressure.
func (t *TUIAdapter) DroppedEvents() int64 {
	return t.droppedEvents.Load()
}

// Stop gracefully shuts down the adapter, flushing any pending output.
func (t *TUIAdapter) Stop() {
	close(t.stopCh)
	t.wg.Wait()
}

// NotifyEvent receives a fire-and-forget event notification.
// Events are queued for processing by the render goroutine.
// On overflow, the oldest non-state event is dropped.
func (t *TUIAdapter) NotifyEvent(event registry.UIEvent) {
	select {
	case t.eventCh <- event:
		// Event queued successfully
	default:
		// Channel full - drop event and increment counter
		t.droppedEvents.Add(1)
		t.logger.Warn("dropped event due to backpressure",
			slog.String("type", string(event.Type)),
			slog.Int64("total_dropped", t.droppedEvents.Load()))
	}
}

// PromptDecision displays a prompt to the user and blocks until response.
// For TUI mode, this reads from stdin. The implementation handles context
// cancellation to dismiss the prompt when another adapter responds first.
func (t *TUIAdapter) PromptDecision(ctx context.Context, req registry.PromptRequest) registry.PromptResponse {
	// Flush any pending thinking buffer before showing prompt
	t.flushThinking()

	// For TUI, we render the prompt and wait for context cancellation
	// The actual input handling is done by the REPL interceptor
	t.writeLine("")
	t.writeLine(fmt.Sprintf(">>> %s", req.Title))
	if req.Body != "" {
		t.writeLine(req.Body)
	} else if req.Message != "" {
		t.writeLine(req.Message)
	}

	// Show options
	if len(req.PromptOptions) > 0 {
		for _, opt := range req.PromptOptions {
			t.writeLine(fmt.Sprintf("  [%s] %s", opt.ID, opt.Label))
			if opt.Description != "" {
				t.writeLine(fmt.Sprintf("        %s", opt.Description))
			}
		}
	} else if len(req.Options) > 0 {
		for i, opt := range req.Options {
			t.writeLine(fmt.Sprintf("  [%d] %s", i+1, opt))
		}
	}

	// Wait for context cancellation (response will come from REPL interceptor)
	<-ctx.Done()
	return registry.PromptResponse{Error: ErrPromptCanceled}
}

// renderLoop processes events from the event channel and renders them.
// This single goroutine owns stdout to avoid interleaving.
func (t *TUIAdapter) renderLoop() {
	defer t.wg.Done()

	for {
		select {
		case <-t.stopCh:
			t.flushThinking()
			t.flushRenderBuffer()
			return
		case event := <-t.eventCh:
			t.processEvent(event)
		}
	}
}

// processEvent handles a single UI event with verbosity filtering.
func (t *TUIAdapter) processEvent(event registry.UIEvent) {
	switch event.Type {
	case registry.EventACPStream:
		t.processACPEvent(event)
	case registry.EventTaskStatusChanged:
		t.processTaskStatusChanged(event)
	case registry.EventACPAutoApproved:
		t.processAutoApproved(event)
	case registry.EventACPAwaitingApproval:
		t.processAwaitingApproval(event)
	case registry.EventACPApprovalResolved:
		t.processApprovalResolved(event)
	default:
		// Other event types - render message if present
		if event.Message != "" {
			t.writeLine(event.Message)
		}
	}
}

// processACPEvent handles EventACPStream events with verbosity filtering.
func (t *TUIAdapter) processACPEvent(event registry.UIEvent) {
	acpEvent, ok := event.Payload.(acp.Event)
	if !ok {
		return
	}

	verbosity := t.GetVerbosity()

	// Apply verbosity filter
	if !t.shouldRenderKind(acpEvent.Kind, verbosity) {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Check if we need to flush thinking buffer due to kind change
	if t.thinkingStarted && acpEvent.Kind != acp.EventThinking {
		t.flushThinkingLocked()
	}

	t.lastEventKind = acpEvent.Kind

	switch acpEvent.Kind {
	case acp.EventThinking:
		t.handleThinking(acpEvent)
	case acp.EventMessage:
		t.handleMessage(acpEvent)
	case acp.EventToolCall:
		t.handleToolCall(acpEvent)
	case acp.EventToolUpdate:
		t.handleToolUpdate(acpEvent)
	case acp.EventPlan:
		t.handlePlan(acpEvent)
	case acp.EventTurnStart:
		t.handleTurnStart(acpEvent)
	case acp.EventTurnEnd:
		t.handleTurnEnd(acpEvent)
	}
}

// shouldRenderKind returns whether an event kind should be rendered at the given verbosity.
func (t *TUIAdapter) shouldRenderKind(kind acp.EventKind, verbosity VerbosityLevel) bool {
	switch verbosity {
	case VerbosityTask:
		// Show everything
		return true
	case VerbosityStory:
		// Drop thinking and tool_update
		switch kind {
		case acp.EventThinking, acp.EventToolUpdate:
			return false
		default:
			return true
		}
	case VerbosityEpic:
		// Only turn_end (task_status_changed handled separately)
		return kind == acp.EventTurnEnd
	default:
		return true
	}
}

// handleThinking accumulates thinking chunks for coalesced rendering.
func (t *TUIAdapter) handleThinking(ev acp.Event) {
	// Extract delta text from raw message
	delta := t.extractDelta(ev.Raw, "thought")

	if !t.thinkingStarted {
		t.thinkingStarted = true
		t.thinkingBuffer.WriteString("\x1b[90m· thinking · ")
	}
	t.thinkingBuffer.WriteString(delta)
}

// handleMessage renders message chunks character-by-character (streaming).
func (t *TUIAdapter) handleMessage(ev acp.Event) {
	delta := t.extractDelta(ev.Raw, "content")
	if delta != "" {
		t.writeRawLocked(delta)
	}
}

// handleToolCall renders tool call announcements.
func (t *TUIAdapter) handleToolCall(ev acp.Event) {
	t.writeLineLocked(fmt.Sprintf("\x1b[33m→ tool: %s [calling]\x1b[0m", ev.ToolName))
}

// handleToolUpdate renders tool update status.
func (t *TUIAdapter) handleToolUpdate(ev acp.Event) {
	status := t.extractField(ev.Raw, "status")
	if status == "" {
		status = "running"
	}
	t.writeLineLocked(fmt.Sprintf("\x1b[33m→ tool: %s [%s]\x1b[0m", ev.ToolName, status))
}

// handlePlan renders plan events as indented bullet list.
func (t *TUIAdapter) handlePlan(ev acp.Event) {
	// Extract plan items from raw
	var planData struct {
		Items []string `json:"items"`
		Text  string   `json:"text"`
	}
	if err := json.Unmarshal(ev.Raw, &planData); err == nil {
		if len(planData.Items) > 0 {
			t.writeLineLocked("\x1b[36m📋 Plan:\x1b[0m")
			for _, item := range planData.Items {
				t.writeLineLocked(fmt.Sprintf("    • %s", item))
			}
		} else if planData.Text != "" {
			t.writeLineLocked(fmt.Sprintf("\x1b[36m📋 Plan: %s\x1b[0m", planData.Text))
		}
	}
}

// handleTurnStart renders turn start indicator.
func (t *TUIAdapter) handleTurnStart(ev acp.Event) {
	t.writeLineLocked("\x1b[90m--- turn start ---\x1b[0m")
}

// handleTurnEnd renders turn end indicator.
func (t *TUIAdapter) handleTurnEnd(ev acp.Event) {
	t.writeLineLocked("\x1b[90m--- turn end ---\x1b[0m")
}

// processTaskStatusChanged handles task status change events.
func (t *TUIAdapter) processTaskStatusChanged(event registry.UIEvent) {
	// Always render status changes regardless of verbosity
	t.flushThinking()
	t.writeLine(fmt.Sprintf("\x1b[34m⬡ %s\x1b[0m", event.Message))
}

// processAutoApproved handles auto-approved command events.
func (t *TUIAdapter) processAutoApproved(event registry.UIEvent) {
	if t.GetVerbosity() >= VerbosityEpic {
		return
	}
	t.flushThinking()
	t.writeLine(fmt.Sprintf("\x1b[32m✓ Auto-approved: %s\x1b[0m", event.Message))
}

// processAwaitingApproval handles awaiting approval events.
func (t *TUIAdapter) processAwaitingApproval(event registry.UIEvent) {
	t.flushThinking()
	t.writeLine(fmt.Sprintf("\x1b[33m⏳ Awaiting approval: %s\x1b[0m", event.Message))
}

// processApprovalResolved handles resolved approval events.
func (t *TUIAdapter) processApprovalResolved(event registry.UIEvent) {
	t.flushThinking()
	t.writeLine(fmt.Sprintf("\x1b[34m✓ Approval resolved: %s\x1b[0m", event.Message))
}

// extractDelta extracts the delta/chunk text from a raw ACP message.
func (t *TUIAdapter) extractDelta(raw json.RawMessage, field string) string {
	// Try to extract from update.raw or direct field
	var wrapper struct {
		Update struct {
			Raw json.RawMessage `json:"raw"`
		} `json:"update"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil && len(wrapper.Update.Raw) > 0 {
		return t.extractFieldFromRaw(wrapper.Update.Raw, field)
	}

	// Try direct extraction
	return t.extractFieldFromRaw(raw, field)
}

// extractFieldFromRaw extracts a string field from raw JSON.
func (t *TUIAdapter) extractFieldFromRaw(raw json.RawMessage, field string) string {
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return ""
	}

	// Try the specific field
	if v, ok := data[field]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}

	// Try common alternatives
	for _, alt := range []string{"delta", "text", "chunk", "content"} {
		if v, ok := data[alt]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}

	return ""
}

// extractField extracts a string field from raw JSON.
func (t *TUIAdapter) extractField(raw json.RawMessage, field string) string {
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return ""
	}
	if v, ok := data[field]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// flushThinking outputs any accumulated thinking text and resets the buffer.
func (t *TUIAdapter) flushThinking() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.flushThinkingLocked()
}

// flushThinkingLocked outputs thinking text without acquiring lock.
func (t *TUIAdapter) flushThinkingLocked() {
	if t.thinkingStarted {
		t.writeLineLocked(t.thinkingBuffer.String() + "\x1b[0m")
		t.thinkingBuffer.Reset()
		t.thinkingStarted = false
	}
}

// writeLine writes a line of text to the output, handling input focus buffering.
func (t *TUIAdapter) writeLine(line string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.writeLineLocked(line)
}

// writeLineLocked writes a line without acquiring lock.
func (t *TUIAdapter) writeLineLocked(line string) {
	if t.inputFocused.Load() {
		t.renderBuffer.WriteString(line)
		t.renderBuffer.WriteByte('\n')
		return
	}
	if t.writer != nil {
		t.writer.Write([]byte(line + "\n"))
	}
}

// writeRaw writes raw text without newline (for streaming).
func (t *TUIAdapter) writeRaw(text string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.writeRawLocked(text)
}

// writeRawLocked writes raw text without acquiring lock.
func (t *TUIAdapter) writeRawLocked(text string) {
	if t.inputFocused.Load() {
		t.renderBuffer.WriteString(text)
		return
	}
	if t.writer != nil {
		t.writer.Write([]byte(text))
	}
}

// flushRenderBuffer outputs any buffered render content.
func (t *TUIAdapter) flushRenderBuffer() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.renderBuffer.Len() > 0 && t.writer != nil {
		t.writer.Write([]byte(t.renderBuffer.String()))
		t.renderBuffer.Reset()
	}
}

// FlushPendingOutput flushes any buffered output when input focus is released.
// Call this when the REPL prompt is ready to show output.
func (t *TUIAdapter) FlushPendingOutput() {
	t.flushThinking()
	t.flushRenderBuffer()
}
