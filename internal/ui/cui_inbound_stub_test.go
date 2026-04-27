package ui

import (
	"context"
	"testing"
	"time"

	"codemint.kanthorlabs.com/internal/input"
)

func TestStubInbound_Inject_DispatchesViaMux(t *testing.T) {
	mux := input.NewMultiplexer()
	defer mux.Close()

	stub := NewStubInbound(mux)

	// Inject a message.
	ok := stub.Inject("hello from stub")
	if !ok {
		t.Fatal("Inject should return true when message is accepted")
	}

	// Receive from multiplexer.
	select {
	case msg := <-mux.Recv():
		if msg.Source != "cui-stub" {
			t.Errorf("expected Source=cui-stub, got %q", msg.Source)
		}
		if msg.Text != "hello from stub" {
			t.Errorf("expected Text='hello from stub', got %q", msg.Text)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for message from multiplexer")
	}
}

func TestStubInbound_InjectWithUserID(t *testing.T) {
	mux := input.NewMultiplexer()
	defer mux.Close()

	stub := NewStubInbound(mux)

	// Inject a message with user ID.
	ok := stub.InjectWithUserID("hello", "user123")
	if !ok {
		t.Fatal("InjectWithUserID should return true")
	}

	// Receive from multiplexer.
	select {
	case msg := <-mux.Recv():
		if msg.UserID != "user123" {
			t.Errorf("expected UserID=user123, got %q", msg.UserID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for message")
	}
}

func TestStubInbound_Stop_PreventsInjection(t *testing.T) {
	mux := input.NewMultiplexer()
	defer mux.Close()

	stub := NewStubInbound(mux)

	// Stop the stub.
	err := stub.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop should not return error: %v", err)
	}

	// Inject should fail after stop.
	ok := stub.Inject("after stop")
	if ok {
		t.Error("Inject should return false after Stop")
	}
}

func TestStubInbound_ImplementsInboundBackend(t *testing.T) {
	mux := input.NewMultiplexer()
	defer mux.Close()

	var backend InboundBackend = NewStubInbound(mux)

	// Start should work.
	err := backend.Start(context.Background(), nil)
	if err != nil {
		t.Errorf("Start should not return error: %v", err)
	}

	// Stop should work.
	err = backend.Stop(context.Background())
	if err != nil {
		t.Errorf("Stop should not return error: %v", err)
	}
}

func TestStubInbound_CustomSource(t *testing.T) {
	mux := input.NewMultiplexer()
	defer mux.Close()

	stub := NewStubInboundWithSource(mux, "telegram-test")

	if stub.Source() != "telegram-test" {
		t.Errorf("expected source=telegram-test, got %q", stub.Source())
	}

	stub.Inject("test message")

	select {
	case msg := <-mux.Recv():
		if msg.Source != "telegram-test" {
			t.Errorf("expected Source=telegram-test, got %q", msg.Source)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout")
	}
}
