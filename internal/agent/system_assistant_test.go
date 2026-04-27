package agent

import (
	"context"
	"testing"
	"time"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/domain"
)

// TestNullAssistant_Ask_StreamsChunks asserts that NullAssistant returns a canned
// response followed by a Done signal.
func TestNullAssistant_Ask_StreamsChunks(t *testing.T) {
	assistant := NewNullAssistant("Hello, I'm the test assistant!")

	sess := AssistantSession{
		Session:  nil,
		Project:  nil,
		IsGlobal: true,
	}

	chunks, err := assistant.Ask(context.Background(), sess, "What's a goroutine?")
	if err != nil {
		t.Fatalf("Ask returned unexpected error: %v", err)
	}

	var receivedText string
	var gotDone bool

	for chunk := range chunks {
		if chunk.Err != nil {
			t.Fatalf("Unexpected error in chunk: %v", chunk.Err)
		}
		if chunk.Text != "" {
			receivedText += chunk.Text
		}
		if chunk.Done {
			gotDone = true
		}
	}

	if receivedText != "Hello, I'm the test assistant!" {
		t.Errorf("expected %q, got %q", "Hello, I'm the test assistant!", receivedText)
	}
	if !gotDone {
		t.Error("expected Done=true signal, but never received it")
	}
}

// TestNullAssistant_AgentID_ReturnsNullAssistant asserts the agent ID is correct.
func TestNullAssistant_AgentID_ReturnsNullAssistant(t *testing.T) {
	assistant := NewNullAssistant("test")
	if got := assistant.AgentID(); got != "null-assistant" {
		t.Errorf("AgentID() = %q, want %q", got, "null-assistant")
	}
}

// TestNullAssistant_Provider_ReturnsNullProvider asserts the provider name is correct.
func TestNullAssistant_Provider_ReturnsNullProvider(t *testing.T) {
	assistant := NewNullAssistant("test")
	provider := assistant.Provider()
	if provider == nil {
		t.Fatal("Provider() returned nil")
	}
	if provider.Name != "null" {
		t.Errorf("Provider().Name = %q, want %q", provider.Name, "null")
	}
}

// TestNullAssistant_Ask_WithContext_RespectsCancellation asserts that context
// cancellation stops the assistant gracefully.
func TestNullAssistant_Ask_WithContext_RespectsCancellation(t *testing.T) {
	assistant := NewNullAssistant("response")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	sess := AssistantSession{IsGlobal: true}
	chunks, err := assistant.Ask(ctx, sess, "test")
	if err != nil {
		t.Fatalf("Ask returned unexpected error: %v", err)
	}

	// Should still receive chunks even with cancelled context
	// (NullAssistant doesn't check context in goroutine)
	timeout := time.After(100 * time.Millisecond)
	for {
		select {
		case _, ok := <-chunks:
			if !ok {
				return // Channel closed, test passed
			}
		case <-timeout:
			t.Fatal("test timed out waiting for channel close")
		}
	}
}

// TestAssistantSession_NoProject_StillWorks asserts that assistant session works
// without a project (global mode).
func TestAssistantSession_NoProject_StillWorks(t *testing.T) {
	assistant := NewNullAssistant("Works without project")

	sess := AssistantSession{
		Session:  nil, // No session
		Project:  nil, // No project
		IsGlobal: true,
	}

	chunks, err := assistant.Ask(context.Background(), sess, "hello")
	if err != nil {
		t.Fatalf("Ask failed: %v", err)
	}

	for chunk := range chunks {
		if chunk.Err != nil {
			t.Fatalf("Unexpected error: %v", chunk.Err)
		}
	}
}

// TestAssistantSession_WithSession_Works asserts that assistant session works
// with an actual session.
func TestAssistantSession_WithSession_Works(t *testing.T) {
	assistant := NewNullAssistant("Works with session")

	sess := AssistantSession{
		Session: &domain.Session{
			ID:     "test-session",
			Status: domain.SessionStatusActive,
		},
		Project:  nil,
		IsGlobal: false,
	}

	chunks, err := assistant.Ask(context.Background(), sess, "hello")
	if err != nil {
		t.Fatalf("Ask failed: %v", err)
	}

	for chunk := range chunks {
		if chunk.Err != nil {
			t.Fatalf("Unexpected error: %v", chunk.Err)
		}
	}
}

// TestACPAssistant_NewACPAssistant_RequiresAttacher asserts that attacher is required.
func TestACPAssistant_NewACPAssistant_RequiresAttacher(t *testing.T) {
	_, err := NewACPAssistant(ACPAssistantConfig{
		Attacher: nil,
		Provider: &Provider{Name: "test", Command: "/bin/true"},
	})
	if err == nil {
		t.Error("expected error when attacher is nil")
	}
}

// TestACPAssistant_NewACPAssistant_RequiresProvider asserts that provider is required.
func TestACPAssistant_NewACPAssistant_RequiresProvider(t *testing.T) {
	_, err := NewACPAssistant(ACPAssistantConfig{
		Attacher: &mockAttacher{},
		Provider: nil,
	})
	if err == nil {
		t.Error("expected error when provider is nil")
	}
}

// TestACPAssistant_NewACPAssistant_MissingBinary asserts error when binary not found.
func TestACPAssistant_NewACPAssistant_MissingBinary(t *testing.T) {
	_, err := NewACPAssistant(ACPAssistantConfig{
		Attacher: &mockAttacher{},
		Provider: &Provider{Name: "test", Command: "/nonexistent/binary/path"},
	})
	if err == nil {
		t.Error("expected error for missing binary")
	}
	if !contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
}

// mockAttacher is a test double for WorkerAttacher.
type mockAttacher struct{}

func (m *mockAttacher) AttachWorker(ctx context.Context, sess *domain.Session, project *domain.Project) (*acp.Worker, error) {
	return nil, nil
}

// contains is a helper to check if s contains substr.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
