package ui

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"codemint.kanthorlabs.com/internal/registry"
)

// FastMockAdapter returns a response quickly (10ms).
type FastMockAdapter struct {
	response         registry.PromptResponse
	called           atomic.Bool
	canceledPromptID atomic.Value // stores string
	// events stores received UIEvent notifications for test assertions.
	events   []registry.UIEvent
	eventsMu sync.Mutex
}

func (a *FastMockAdapter) NotifyEvent(event registry.UIEvent) {
	a.eventsMu.Lock()
	defer a.eventsMu.Unlock()
	a.events = append(a.events, event)
}

func (a *FastMockAdapter) Events() []registry.UIEvent {
	a.eventsMu.Lock()
	defer a.eventsMu.Unlock()
	result := make([]registry.UIEvent, len(a.events))
	copy(result, a.events)
	return result
}

func (a *FastMockAdapter) PromptDecision(ctx context.Context, req registry.PromptRequest) registry.PromptResponse {
	a.called.Store(true)
	select {
	case <-time.After(10 * time.Millisecond):
		return a.response
	case <-ctx.Done():
		return registry.PromptResponse{Error: ctx.Err()}
	}
}

func (a *FastMockAdapter) CancelPrompt(promptID string) {
	a.canceledPromptID.Store(promptID)
}

func (a *FastMockAdapter) CanceledPromptID() string {
	v := a.canceledPromptID.Load()
	if v == nil {
		return ""
	}
	return v.(string)
}

// SlowMockAdapter blocks until context is canceled or returns after 100ms.
type SlowMockAdapter struct {
	response         registry.PromptResponse
	called           atomic.Bool
	canceled         atomic.Bool
	cancelErr        error
	canceledPromptID atomic.Value // stores string
	// events stores received UIEvent notifications for test assertions.
	events   []registry.UIEvent
	eventsMu sync.Mutex
}

func (a *SlowMockAdapter) NotifyEvent(event registry.UIEvent) {
	a.eventsMu.Lock()
	defer a.eventsMu.Unlock()
	a.events = append(a.events, event)
}

func (a *SlowMockAdapter) Events() []registry.UIEvent {
	a.eventsMu.Lock()
	defer a.eventsMu.Unlock()
	result := make([]registry.UIEvent, len(a.events))
	copy(result, a.events)
	return result
}

func (a *SlowMockAdapter) PromptDecision(ctx context.Context, req registry.PromptRequest) registry.PromptResponse {
	a.called.Store(true)
	select {
	case <-time.After(100 * time.Millisecond):
		return a.response
	case <-ctx.Done():
		a.canceled.Store(true)
		a.cancelErr = ctx.Err()
		return registry.PromptResponse{Error: ctx.Err()}
	}
}

func (a *SlowMockAdapter) CancelPrompt(promptID string) {
	a.canceledPromptID.Store(promptID)
}

func (a *SlowMockAdapter) CanceledPromptID() string {
	v := a.canceledPromptID.Load()
	if v == nil {
		return ""
	}
	return v.(string)
}

func TestNewUIMediator(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)
	if m == nil {
		t.Fatal("NewUIMediator returned nil")
	}
	if len(m.Adapters()) != 0 {
		t.Errorf("expected 0 adapters, got %d", len(m.Adapters()))
	}
}

func TestUIMediator_RegisterAdapter(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)
	fast := &FastMockAdapter{response: registry.PromptResponse{SelectedOption: "Accept"}}
	slow := &SlowMockAdapter{response: registry.PromptResponse{SelectedOption: "Revert"}}

	m.RegisterAdapter(fast)
	m.RegisterAdapter(slow)

	adapters := m.Adapters()
	if len(adapters) != 2 {
		t.Errorf("expected 2 adapters, got %d", len(adapters))
	}
}

func TestUIMediator_PromptDecision_NoAdapters(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	resp := m.PromptDecision(context.Background(), registry.PromptRequest{
		TaskID:  "task-1",
		Message: "Review changes?",
		Options: []string{"Accept", "Revert"},
	})

	if resp.Error != ErrNoAdapters {
		t.Errorf("expected ErrNoAdapters, got %v", resp.Error)
	}
}

func TestUIMediator_PromptDecision_FirstResponseWins(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	fast := &FastMockAdapter{response: registry.PromptResponse{SelectedOption: "Accept"}}
	slow := &SlowMockAdapter{response: registry.PromptResponse{SelectedOption: "Revert"}}

	m.RegisterAdapter(fast)
	m.RegisterAdapter(slow)

	req := registry.PromptRequest{
		TaskID:  "task-1",
		Message: "Review changes?",
		Options: []string{"Accept", "Revert"},
	}

	resp := m.PromptDecision(context.Background(), req)

	// Fast adapter should win.
	if resp.SelectedOption != "Accept" {
		t.Errorf("expected 'Accept', got %q", resp.SelectedOption)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}

	// Both adapters should have been called.
	if !fast.called.Load() {
		t.Error("fast adapter was not called")
	}
	if !slow.called.Load() {
		t.Error("slow adapter was not called")
	}

	// Give a moment for the slow adapter to receive cancellation.
	time.Sleep(20 * time.Millisecond)

	// Slow adapter should have received cancellation.
	if !slow.canceled.Load() {
		t.Error("slow adapter did not receive cancellation")
	}
}

func TestUIMediator_PromptDecision_ParentContextCanceled(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	// Use only slow adapter so we can cancel before it responds.
	slow := &SlowMockAdapter{response: registry.PromptResponse{SelectedOption: "Revert"}}
	m.RegisterAdapter(slow)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel almost immediately.
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	req := registry.PromptRequest{
		TaskID:  "task-1",
		Message: "Review changes?",
		Options: []string{"Accept", "Revert"},
	}

	resp := m.PromptDecision(ctx, req)

	if resp.Error != ErrPromptCanceled {
		t.Errorf("expected ErrPromptCanceled, got %v", resp.Error)
	}
}

func TestUIMediator_RenderMessage(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	m.RenderMessage("Hello, World!")

	expected := "Hello, World!\n"
	if buf.String() != expected {
		t.Errorf("expected %q, got %q", expected, buf.String())
	}
}

func TestUIMediator_ClearScreen(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	m.ClearScreen()

	// ANSI escape sequence for clear screen + move cursor to top-left.
	expected := "\033[2J\033[H"
	if buf.String() != expected {
		t.Errorf("expected %q, got %q", expected, buf.String())
	}
}

func TestUIMediator_ConcurrentRegistration(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	// Register adapters concurrently.
	done := make(chan struct{})
	for i := range 10 {
		go func(idx int) {
			defer func() { done <- struct{}{} }()
			adapter := &FastMockAdapter{
				response: registry.PromptResponse{SelectedOption: "Accept"},
			}
			m.RegisterAdapter(adapter)
		}(i)
	}

	// Wait for all goroutines to complete.
	for range 10 {
		<-done
	}

	adapters := m.Adapters()
	if len(adapters) != 10 {
		t.Errorf("expected 10 adapters, got %d", len(adapters))
	}
}

func TestUIMediator_ImplementsRegistryUIMediator(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	// This assignment will fail to compile if UIMediator doesn't implement
	// registry.UIMediator.
	var _ registry.UIMediator = m
}

// Compile-time check that mock adapters implement UIAdapter.
var (
	_ UIAdapter = (*FastMockAdapter)(nil)
	_ UIAdapter = (*SlowMockAdapter)(nil)
)

func TestUIMediator_NotifyAll_BroadcastsToAllAdapters(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	fast := &FastMockAdapter{response: registry.PromptResponse{SelectedOption: "Accept"}}
	slow := &SlowMockAdapter{response: registry.PromptResponse{SelectedOption: "Revert"}}

	m.RegisterAdapter(fast)
	m.RegisterAdapter(slow)

	event := registry.UIEvent{
		Type:    registry.EventTaskStarted,
		TaskID:  "task-123",
		Message: "Task execution started",
		Payload: map[string]string{"priority": "high"},
	}

	m.NotifyAll(event)

	// Give goroutines time to process the event.
	time.Sleep(20 * time.Millisecond)

	// Both adapters should have received the event.
	fastEvents := fast.Events()
	if len(fastEvents) != 1 {
		t.Errorf("fast adapter: expected 1 event, got %d", len(fastEvents))
	} else {
		if fastEvents[0].Type != registry.EventTaskStarted {
			t.Errorf("fast adapter: expected EventTaskStarted, got %v", fastEvents[0].Type)
		}
		if fastEvents[0].TaskID != "task-123" {
			t.Errorf("fast adapter: expected task-123, got %q", fastEvents[0].TaskID)
		}
	}

	slowEvents := slow.Events()
	if len(slowEvents) != 1 {
		t.Errorf("slow adapter: expected 1 event, got %d", len(slowEvents))
	} else {
		if slowEvents[0].Type != registry.EventTaskStarted {
			t.Errorf("slow adapter: expected EventTaskStarted, got %v", slowEvents[0].Type)
		}
	}
}

func TestUIMediator_NotifyAll_NoAdapters(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	// Should not panic with no adapters.
	event := registry.UIEvent{
		Type:    registry.EventTaskCompleted,
		TaskID:  "task-456",
		Message: "Task completed",
	}

	m.NotifyAll(event) // Should complete without error.
}

func TestUIMediator_NotifyAll_MultipleEvents(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	fast := &FastMockAdapter{response: registry.PromptResponse{SelectedOption: "Accept"}}
	m.RegisterAdapter(fast)

	events := []registry.UIEvent{
		{Type: registry.EventTaskStarted, TaskID: "task-1", Message: "Started"},
		{Type: registry.EventProgress, TaskID: "task-1", Message: "50% complete"},
		{Type: registry.EventTaskCompleted, TaskID: "task-1", Message: "Done"},
	}

	for _, e := range events {
		m.NotifyAll(e)
	}

	// Give goroutines time to process all events.
	time.Sleep(30 * time.Millisecond)

	received := fast.Events()
	if len(received) != 3 {
		t.Errorf("expected 3 events, got %d", len(received))
	}
}

func TestUIMediator_NotifyAll_FireAndForget(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	// Create a slow adapter that takes time to process events.
	slow := &SlowMockAdapter{response: registry.PromptResponse{SelectedOption: "Revert"}}
	m.RegisterAdapter(slow)

	event := registry.UIEvent{
		Type:    registry.EventAgentCrashed,
		TaskID:  "task-error",
		Message: "Agent crashed unexpectedly",
	}

	start := time.Now()
	m.NotifyAll(event)
	elapsed := time.Since(start)

	// NotifyAll should return immediately (fire-and-forget).
	// It should not block waiting for the adapter to process.
	if elapsed > 10*time.Millisecond {
		t.Errorf("NotifyAll blocked for %v, expected immediate return", elapsed)
	}

	// Give slow adapter time to receive.
	time.Sleep(20 * time.Millisecond)

	received := slow.Events()
	if len(received) != 1 {
		t.Errorf("expected 1 event, got %d", len(received))
	}
}

// --- Task 3.18.1: Baseline Legacy Test ---
// This test documents the current mediator behavior before 3.18 changes.
// It verifies that the mediator already broadcasts to all adapters concurrently
// and returns the first response.
func TestMediator_Legacy_SingleAdapter(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	// Single adapter case: should return its response.
	adapter := &FastMockAdapter{response: registry.PromptResponse{SelectedOption: "Accept"}}
	m.RegisterAdapter(adapter)

	req := registry.PromptRequest{
		TaskID:  "task-legacy",
		Message: "Legacy test",
		Options: []string{"Accept", "Deny"},
	}

	resp := m.PromptDecision(context.Background(), req)

	if resp.SelectedOption != "Accept" {
		t.Errorf("expected 'Accept', got %q", resp.SelectedOption)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}
	if !adapter.called.Load() {
		t.Error("adapter was not called")
	}
}

// --- Task 3.18.2: First-In-Wins with Cancel-the-Losers Tests ---

// FailingMockAdapter always returns an error after a delay.
type FailingMockAdapter struct {
	delay  time.Duration
	err    error
	called atomic.Bool
}

func (a *FailingMockAdapter) NotifyEvent(event registry.UIEvent) {}

func (a *FailingMockAdapter) PromptDecision(ctx context.Context, req registry.PromptRequest) registry.PromptResponse {
	a.called.Store(true)
	select {
	case <-time.After(a.delay):
		return registry.PromptResponse{Error: a.err}
	case <-ctx.Done():
		return registry.PromptResponse{Error: ctx.Err()}
	}
}

func (a *FailingMockAdapter) CancelPrompt(promptID string) {}

// Compile-time check that FailingMockAdapter implements UIAdapter.
var _ UIAdapter = (*FailingMockAdapter)(nil)

func TestMediator_FirstInWins_CancelsCalled(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	// Fast adapter wins, slow adapter should receive CancelPrompt.
	fast := &FastMockAdapter{response: registry.PromptResponse{SelectedOption: "Accept"}}
	slow := &SlowMockAdapter{response: registry.PromptResponse{SelectedOption: "Revert"}}

	m.RegisterAdapter(fast)
	m.RegisterAdapter(slow)

	req := registry.PromptRequest{
		TaskID:  "task-cancel-test",
		Message: "Test cancel",
		Options: []string{"Accept", "Revert"},
	}

	resp := m.PromptDecision(context.Background(), req)

	// Fast adapter should win.
	if resp.SelectedOption != "Accept" {
		t.Errorf("expected 'Accept', got %q", resp.SelectedOption)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}

	// Give time for CancelPrompt to be called on slow adapter.
	time.Sleep(30 * time.Millisecond)

	// Slow adapter should have received CancelPrompt.
	if slow.CanceledPromptID() != "task-cancel-test" {
		t.Errorf("slow adapter CancelPrompt not called with correct promptID; got %q", slow.CanceledPromptID())
	}

	// Fast adapter (winner) should NOT receive CancelPrompt.
	if fast.CanceledPromptID() != "" {
		t.Errorf("fast adapter (winner) should not receive CancelPrompt; got %q", fast.CanceledPromptID())
	}
}

func TestMediator_AllFail_ReturnsErrAllAdaptersFailed(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	// Both adapters return errors.
	errFirst := errors.New("adapter1 failed")
	errSecond := errors.New("adapter2 failed")

	adapter1 := &FailingMockAdapter{delay: 10 * time.Millisecond, err: errFirst}
	adapter2 := &FailingMockAdapter{delay: 20 * time.Millisecond, err: errSecond}

	m.RegisterAdapter(adapter1)
	m.RegisterAdapter(adapter2)

	req := registry.PromptRequest{
		TaskID:  "task-all-fail",
		Message: "All fail test",
		Options: []string{"Accept"},
	}

	resp := m.PromptDecision(context.Background(), req)

	// Should return ErrAllAdaptersFailed wrapping the first error.
	if resp.Error == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(resp.Error, ErrAllAdaptersFailed) {
		t.Errorf("expected ErrAllAdaptersFailed, got %v", resp.Error)
	}

	// The first error should be wrapped.
	if !strings.Contains(resp.Error.Error(), "adapter1 failed") {
		t.Errorf("expected wrapped error to contain first adapter's error; got %v", resp.Error)
	}

	// Both adapters should have been called.
	if !adapter1.called.Load() {
		t.Error("adapter1 was not called")
	}
	if !adapter2.called.Load() {
		t.Error("adapter2 was not called")
	}
}

func TestMediator_FirstNonErrorWins_IgnoresEarlierErrors(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	// First adapter fails quickly, second adapter succeeds slowly.
	failing := &FailingMockAdapter{delay: 5 * time.Millisecond, err: errors.New("failed")}
	succeeding := &SlowMockAdapter{response: registry.PromptResponse{SelectedOption: "Success"}}

	m.RegisterAdapter(failing)
	m.RegisterAdapter(succeeding)

	req := registry.PromptRequest{
		TaskID:  "task-error-then-success",
		Message: "Error then success test",
		Options: []string{"Success"},
	}

	resp := m.PromptDecision(context.Background(), req)

	// The succeeding adapter should win despite failing adapter responding first.
	if resp.SelectedOption != "Success" {
		t.Errorf("expected 'Success', got %q", resp.SelectedOption)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}
}

// --- Task 3.18.5: Context-Cancellation and Goroutine Leak Tests ---

// BlockingMockAdapter blocks indefinitely until context is canceled.
type BlockingMockAdapter struct {
	called           atomic.Bool
	canceledPromptID atomic.Value
}

func (a *BlockingMockAdapter) NotifyEvent(event registry.UIEvent) {}

func (a *BlockingMockAdapter) PromptDecision(ctx context.Context, req registry.PromptRequest) registry.PromptResponse {
	a.called.Store(true)
	// Block for up to 5 seconds, but respect context cancellation.
	select {
	case <-time.After(5 * time.Second):
		return registry.PromptResponse{Error: errors.New("timed out")}
	case <-ctx.Done():
		return registry.PromptResponse{Error: ctx.Err()}
	}
}

func (a *BlockingMockAdapter) CancelPrompt(promptID string) {
	a.canceledPromptID.Store(promptID)
}

// Compile-time check that BlockingMockAdapter implements UIAdapter.
var _ UIAdapter = (*BlockingMockAdapter)(nil)

func TestMediator_ParentContextCancel_AbortsAdapters(t *testing.T) {
	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	// Use a blocking adapter that will block for 5s unless canceled.
	blocking := &BlockingMockAdapter{}
	m.RegisterAdapter(blocking)

	ctx, cancel := context.WithCancel(context.Background())

	req := registry.PromptRequest{
		TaskID:  "task-cancel-test",
		Message: "Cancel test",
		Options: []string{"Accept"},
	}

	// Cancel the context after a short delay.
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	resp := m.PromptDecision(ctx, req)
	elapsed := time.Since(start)

	// Should return ErrPromptCanceled within 250ms (well before the 5s timeout).
	if elapsed > 250*time.Millisecond {
		t.Errorf("PromptDecision took %v, expected < 250ms", elapsed)
	}

	if resp.Error != ErrPromptCanceled {
		t.Errorf("expected ErrPromptCanceled, got %v", resp.Error)
	}

	// The adapter should have been called.
	if !blocking.called.Load() {
		t.Error("blocking adapter was not called")
	}
}

func TestMediator_NoGoroutineLeak_AfterCancel(t *testing.T) {
	// Count initial goroutines.
	initialGoroutines := runtime.NumGoroutine()

	var buf bytes.Buffer
	m := NewUIMediator(&buf)

	// Use multiple blocking adapters.
	adapter1 := &BlockingMockAdapter{}
	adapter2 := &BlockingMockAdapter{}
	adapter3 := &BlockingMockAdapter{}
	m.RegisterAdapter(adapter1)
	m.RegisterAdapter(adapter2)
	m.RegisterAdapter(adapter3)

	ctx, cancel := context.WithCancel(context.Background())

	req := registry.PromptRequest{
		TaskID:  "task-leak-test",
		Message: "Leak test",
		Options: []string{"Accept"},
	}

	// Cancel immediately after starting.
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	_ = m.PromptDecision(ctx, req)

	// Give goroutines time to clean up.
	time.Sleep(100 * time.Millisecond)

	// Check for goroutine leaks.
	// Allow some tolerance (e.g., +2) for runtime variations.
	finalGoroutines := runtime.NumGoroutine()
	leaked := finalGoroutines - initialGoroutines

	if leaked > 2 {
		t.Errorf("goroutine leak detected: started with %d, ended with %d (leaked %d)",
			initialGoroutines, finalGoroutines, leaked)
	}
}

// --- Task 3.18.3: Adapter CancelPrompt Tests ---

func TestTUIAdapter_CancelPrompt_RemovesPending(t *testing.T) {
	var buf bytes.Buffer
	adapter := NewTUIAdapter(TUIAdapterConfig{
		Writer:    &buf,
		Verbosity: VerbosityTask,
	})
	defer adapter.Stop()

	// Simulate a pending prompt by adding to the map directly.
	adapter.pendingPromptsMu.Lock()
	adapter.pendingPrompts["task-123"] = struct{}{}
	adapter.pendingPromptsMu.Unlock()

	// Verify it's pending.
	if !adapter.HasPendingPrompt("task-123") {
		t.Fatal("expected prompt to be pending")
	}

	// Cancel the prompt.
	adapter.CancelPrompt("task-123")

	// Give time for the write to happen.
	time.Sleep(20 * time.Millisecond)

	// Verify it's no longer pending.
	if adapter.HasPendingPrompt("task-123") {
		t.Error("expected prompt to be removed after cancel")
	}

	// Verify log message was written.
	output := buf.String()
	if !strings.Contains(output, "answered elsewhere") {
		t.Errorf("expected cancel message in output; got %q", output)
	}
}
