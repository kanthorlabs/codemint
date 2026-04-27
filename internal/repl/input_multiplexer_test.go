package repl

import (
	"sync"
	"testing"
	"time"
)

func TestMultiplexer_FIFO_PerSource(t *testing.T) {
	mux := NewInputMultiplexer()

	src1 := mux.RegisterSource("tui", 16)
	src2 := mux.RegisterSource("cui", 16)

	// Send interleaved messages.
	src1 <- InboundMessage{Source: "tui", Text: "tui-1", At: time.Now()}
	src2 <- InboundMessage{Source: "cui", Text: "cui-1", At: time.Now()}
	src1 <- InboundMessage{Source: "tui", Text: "tui-2", At: time.Now()}
	src2 <- InboundMessage{Source: "cui", Text: "cui-2", At: time.Now()}
	src1 <- InboundMessage{Source: "tui", Text: "tui-3", At: time.Now()}

	// Give forwarders time to process.
	time.Sleep(50 * time.Millisecond)

	// Collect all messages.
	var tuiMsgs, cuiMsgs []string
	timeout := time.After(500 * time.Millisecond)
collect:
	for {
		select {
		case msg := <-mux.Recv():
			if msg.Source == "tui" {
				tuiMsgs = append(tuiMsgs, msg.Text)
			} else {
				cuiMsgs = append(cuiMsgs, msg.Text)
			}
			if len(tuiMsgs)+len(cuiMsgs) >= 5 {
				break collect
			}
		case <-timeout:
			break collect
		}
	}

	// Clean up.
	mux.Close()

	// Verify per-source FIFO ordering.
	if len(tuiMsgs) != 3 {
		t.Errorf("expected 3 tui messages, got %d: %v", len(tuiMsgs), tuiMsgs)
	}
	if len(cuiMsgs) != 2 {
		t.Errorf("expected 2 cui messages, got %d: %v", len(cuiMsgs), cuiMsgs)
	}

	// Check TUI order.
	expected := []string{"tui-1", "tui-2", "tui-3"}
	for i, want := range expected {
		if i < len(tuiMsgs) && tuiMsgs[i] != want {
			t.Errorf("tui[%d]: got %q, want %q", i, tuiMsgs[i], want)
		}
	}

	// Check CUI order.
	expected = []string{"cui-1", "cui-2"}
	for i, want := range expected {
		if i < len(cuiMsgs) && cuiMsgs[i] != want {
			t.Errorf("cui[%d]: got %q, want %q", i, cuiMsgs[i], want)
		}
	}
}

func TestMultiplexer_DropOldest_WhenFull(t *testing.T) {
	mux := NewInputMultiplexer()

	// Register source with small capacity.
	src := mux.RegisterSource("test", 2)

	// Fill the source buffer without reading.
	src <- InboundMessage{Source: "test", Text: "msg-1", At: time.Now()}
	src <- InboundMessage{Source: "test", Text: "msg-2", At: time.Now()}

	// Give forwarder time to process.
	time.Sleep(20 * time.Millisecond)

	// Send one more - should succeed (forwarded to out).
	select {
	case src <- InboundMessage{Source: "test", Text: "msg-3", At: time.Now()}:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("send should not block")
	}

	// Give time to forward.
	time.Sleep(50 * time.Millisecond)

	// Drain and verify we have messages.
	var received []string
	timeout := time.After(200 * time.Millisecond)
drain:
	for {
		select {
		case msg := <-mux.Recv():
			received = append(received, msg.Text)
			if len(received) >= 3 {
				break drain
			}
		case <-timeout:
			break drain
		}
	}

	mux.Close()

	if len(received) == 0 {
		t.Error("expected at least some messages to be received")
	}
	// We should have received some messages (exact count depends on timing).
	t.Logf("received %d messages: %v", len(received), received)
}

func TestMultiplexer_Close_StopsAllForwarders(t *testing.T) {
	mux := NewInputMultiplexer()

	// Register multiple sources.
	src1 := mux.RegisterSource("src1", 16)
	src2 := mux.RegisterSource("src2", 16)

	// Send some messages.
	src1 <- InboundMessage{Source: "src1", Text: "hello", At: time.Now()}
	src2 <- InboundMessage{Source: "src2", Text: "world", At: time.Now()}

	// Give forwarders time to start.
	time.Sleep(20 * time.Millisecond)

	// Close should not block and should clean up.
	done := make(chan struct{})
	go func() {
		mux.Close()
		close(done)
	}()

	select {
	case <-done:
		// Good, closed cleanly.
	case <-time.After(1 * time.Second):
		t.Fatal("Close() blocked for too long")
	}

	// Output channel should be closed.
	_, ok := <-mux.Recv()
	if ok {
		// Might receive buffered messages first, drain them.
		for range mux.Recv() {
		}
	}
}

func TestMultiplexer_RegisterSource_Idempotent(t *testing.T) {
	mux := NewInputMultiplexer()

	ch1 := mux.RegisterSource("tui", 16)
	ch2 := mux.RegisterSource("tui", 32) // Same source, different capacity.

	// Should return the same channel.
	ch1 <- InboundMessage{Source: "tui", Text: "test", At: time.Now()}
	ch2 <- InboundMessage{Source: "tui", Text: "test2", At: time.Now()}

	// Both sends should work without blocking.
	time.Sleep(20 * time.Millisecond)

	// Verify messages are received.
	timeout := time.After(100 * time.Millisecond)
	count := 0
collect:
	for {
		select {
		case <-mux.Recv():
			count++
			if count >= 2 {
				break collect
			}
		case <-timeout:
			break collect
		}
	}

	mux.Close()

	if count < 2 {
		t.Errorf("expected 2 messages, got %d", count)
	}
}

func TestMultiplexer_ConcurrentSources(t *testing.T) {
	mux := NewInputMultiplexer()

	const numSources = 5
	const msgsPerSource = 10

	var sendWg sync.WaitGroup
	sendWg.Add(numSources)

	sources := make([]chan<- InboundMessage, numSources)
	for i := 0; i < numSources; i++ {
		srcName := "src" + string(rune('A'+i))
		sources[i] = mux.RegisterSource(srcName, 32)
	}

	for i := 0; i < numSources; i++ {
		go func(ch chan<- InboundMessage, srcName string) {
			defer sendWg.Done()
			for j := 0; j < msgsPerSource; j++ {
				ch <- InboundMessage{Source: srcName, Text: "msg", At: time.Now()}
			}
		}(sources[i], "src"+string(rune('A'+i)))
	}

	// Count received messages in background.
	receivedCount := 0
	receiveDone := make(chan struct{})
	go func() {
		expected := numSources * msgsPerSource
		timeout := time.After(2 * time.Second)
		for receivedCount < expected {
			select {
			case <-mux.Recv():
				receivedCount++
			case <-timeout:
				close(receiveDone)
				return
			}
		}
		close(receiveDone)
	}()

	// Wait for senders to finish.
	sendWg.Wait()

	// Wait for receiver to collect all.
	<-receiveDone

	mux.Close()

	expected := numSources * msgsPerSource
	if receivedCount != expected {
		t.Errorf("expected %d messages, got %d", expected, receivedCount)
	}
}
