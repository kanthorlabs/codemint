package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"codemint.kanthorlabs.com/internal/agent"
	"codemint.kanthorlabs.com/internal/input"
	"codemint.kanthorlabs.com/internal/registry"
	"codemint.kanthorlabs.com/internal/ui"
)

// echoAssistant is a mock assistant that echoes input with a prefix.
type echoAssistant struct {
	prefix string
}

func (e *echoAssistant) Ask(ctx context.Context, sess agent.AssistantSession, text string) (<-chan agent.ChatChunk, error) {
	ch := make(chan agent.ChatChunk, 2)
	go func() {
		defer close(ch)
		response := e.prefix + text
		ch <- agent.ChatChunk{Text: response}
		ch <- agent.ChatChunk{Done: true}
	}()
	return ch, nil
}

func (e *echoAssistant) AgentID() string           { return "echo-assistant" }
func (e *echoAssistant) Provider() *agent.Provider { return &agent.Provider{Name: "echo"} }

// TestInputMultiplexer_CrossInterfaceLoop verifies that messages from
// both TUI and CUI sources are properly routed through the dispatcher
// with correct source attribution.
func TestInputMultiplexer_CrossInterfaceLoop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create the multiplexer.
	mux := input.NewMultiplexer()

	// Create mock UI mediator (from mocks_test.go).
	mockUI := &mockUIMediator{}

	// Create a mock assistant that echoes with a prefix.
	assistant := &echoAssistant{prefix: "Echo: "}

	// Create command registry with core commands.
	cmdRegistry := registry.NewCommandRegistry()

	// Create dispatcher with the mock assistant.
	dispatcher := NewDispatcher(cmdRegistry, mockUI, assistant, nil)

	// Create active session.
	active := &ActiveSession{
		ClientMode: registry.ClientModeHybrid,
		IsGlobal:   true,
	}

	// Register both TUI and CUI sources.
	tuiCh := mux.RegisterSource("tui", 16)

	// Create stub inbound for CUI.
	stub := ui.NewStubInbound(mux)

	// Start a goroutine to process messages from the multiplexer.
	results := make(chan struct {
		source string
		text   string
	}, 10)

	dispatchDone := make(chan struct{})
	go func() {
		defer close(dispatchDone)
		for msg := range mux.Recv() {
			// Track the message source.
			results <- struct {
				source string
				text   string
			}{source: msg.Source, text: msg.Text}

			// Dispatch via the dispatcher.
			_ = dispatcher.DispatchInbound(ctx, active, msg)
		}
	}()

	// Send message via TUI source.
	tuiCh <- input.InboundMessage{
		Source: "tui",
		Text:   "What's a goroutine?",
		At:     time.Now(),
	}

	// Send message via CUI stub.
	stub.Inject("Explain channels too.")

	// Wait for results.
	time.Sleep(100 * time.Millisecond)

	// Verify messages were received with correct sources.
	var tuiCount, cuiCount int
loop:
	for {
		select {
		case r := <-results:
			if r.source == "tui" {
				tuiCount++
				if !strings.Contains(r.text, "goroutine") {
					t.Errorf("TUI message should contain 'goroutine', got: %q", r.text)
				}
			} else if r.source == "cui-stub" {
				cuiCount++
				if !strings.Contains(r.text, "channels") {
					t.Errorf("CUI message should contain 'channels', got: %q", r.text)
				}
			}
		case <-time.After(200 * time.Millisecond):
			break loop
		}
	}

	// Close the multiplexer to stop processing.
	mux.Close()
	<-dispatchDone

	if tuiCount != 1 {
		t.Errorf("expected 1 TUI message, got %d", tuiCount)
	}
	if cuiCount != 1 {
		t.Errorf("expected 1 CUI message, got %d", cuiCount)
	}

	// Verify the assistant was called (events should be emitted).
	events := mockUI.Events()
	if len(events) == 0 {
		t.Error("expected chat events to be emitted")
	}

	// Verify that events contain echo responses.
	hasEchoResponse := false
	for _, event := range events {
		if event.Type == registry.EventChatChunk {
			if payload, ok := event.Payload.(registry.ChatChunkPayload); ok {
				if strings.Contains(payload.Text, "Echo:") {
					hasEchoResponse = true
					break
				}
			}
		}
	}
	if !hasEchoResponse {
		t.Error("expected echo response in events")
	}
}

// TestInputMultiplexer_SourceAttribution verifies that the input source
// is properly tracked in the active session during dispatch.
func TestInputMultiplexer_SourceAttribution(t *testing.T) {
	ctx := context.Background()

	// Create active session.
	active := &ActiveSession{
		ClientMode: registry.ClientModeHybrid,
		IsGlobal:   true,
	}

	// Test SetInputSource and GetInputSource.
	active.SetInputSource("cui-telegram", "user123")
	source, userID := active.GetInputSource()
	if source != "cui-telegram" {
		t.Errorf("expected source 'cui-telegram', got %q", source)
	}
	if userID != "user123" {
		t.Errorf("expected userID 'user123', got %q", userID)
	}

	// Test ClearInputSource.
	active.ClearInputSource()
	source, userID = active.GetInputSource()
	if source != "" || userID != "" {
		t.Errorf("expected empty source/userID after clear, got %q/%q", source, userID)
	}

	// Test DispatchInbound properly sets/clears source.
	mockUI := &mockUIMediator{}
	assistant := &echoAssistant{prefix: "Test: "}
	cmdRegistry := registry.NewCommandRegistry()
	dispatcher := NewDispatcher(cmdRegistry, mockUI, assistant, nil)

	msg := input.InboundMessage{
		Source: "test-source",
		UserID: "test-user",
		Text:   "hello",
		At:     time.Now(),
	}

	// Dispatch should set source during execution.
	err := dispatcher.DispatchInbound(ctx, active, msg)
	if err != nil {
		t.Fatalf("DispatchInbound error: %v", err)
	}

	// After dispatch, source should be cleared.
	source, userID = active.GetInputSource()
	if source != "" || userID != "" {
		t.Errorf("source should be cleared after dispatch, got %q/%q", source, userID)
	}
}

// TestStubInbound_IntegrationWithMultiplexer verifies that the stub inbound
// backend correctly injects messages into the multiplexer.
func TestStubInbound_IntegrationWithMultiplexer(t *testing.T) {
	mux := input.NewMultiplexer()
	defer mux.Close()

	// Create stub with custom source name.
	stub := ui.NewStubInboundWithSource(mux, "telegram-test")

	// Inject messages.
	if !stub.Inject("message 1") {
		t.Error("Inject should succeed")
	}
	if !stub.InjectWithUserID("message 2", "user456") {
		t.Error("InjectWithUserID should succeed")
	}

	// Collect messages.
	var messages []input.InboundMessage
	timeout := time.After(200 * time.Millisecond)
collect:
	for {
		select {
		case msg := <-mux.Recv():
			messages = append(messages, msg)
			if len(messages) >= 2 {
				break collect
			}
		case <-timeout:
			break collect
		}
	}

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	// Verify first message.
	if messages[0].Source != "telegram-test" {
		t.Errorf("expected source 'telegram-test', got %q", messages[0].Source)
	}
	if messages[0].Text != "message 1" {
		t.Errorf("expected text 'message 1', got %q", messages[0].Text)
	}
	if messages[0].UserID != "" {
		t.Errorf("expected empty userID, got %q", messages[0].UserID)
	}

	// Verify second message.
	if messages[1].UserID != "user456" {
		t.Errorf("expected userID 'user456', got %q", messages[1].UserID)
	}
}

// TestMultiSourceFIFO verifies per-source FIFO ordering.
func TestMultiSourceFIFO(t *testing.T) {
	mux := input.NewMultiplexer()
	defer mux.Close()

	stub1 := ui.NewStubInboundWithSource(mux, "source-A")
	stub2 := ui.NewStubInboundWithSource(mux, "source-B")

	// Send interleaved messages.
	stub1.Inject("A1")
	stub2.Inject("B1")
	stub1.Inject("A2")
	stub2.Inject("B2")
	stub1.Inject("A3")

	// Collect and verify ordering.
	var sourceA, sourceB []string
	timeout := time.After(500 * time.Millisecond)

collect:
	for {
		select {
		case msg := <-mux.Recv():
			if msg.Source == "source-A" {
				sourceA = append(sourceA, msg.Text)
			} else {
				sourceB = append(sourceB, msg.Text)
			}
			if len(sourceA)+len(sourceB) >= 5 {
				break collect
			}
		case <-timeout:
			break collect
		}
	}

	// Verify per-source FIFO.
	if len(sourceA) != 3 {
		t.Errorf("expected 3 source-A messages, got %d", len(sourceA))
	}
	if len(sourceB) != 2 {
		t.Errorf("expected 2 source-B messages, got %d", len(sourceB))
	}

	expectedA := []string{"A1", "A2", "A3"}
	for i, want := range expectedA {
		if i < len(sourceA) && sourceA[i] != want {
			t.Errorf("sourceA[%d]: got %q, want %q", i, sourceA[i], want)
		}
	}

	expectedB := []string{"B1", "B2"}
	for i, want := range expectedB {
		if i < len(sourceB) && sourceB[i] != want {
			t.Errorf("sourceB[%d]: got %q, want %q", i, sourceB[i], want)
		}
	}
}
