package agent

import (
	"context"
	"testing"

	"codemint.kanthorlabs.com/internal/domain"
)

// TestNullCodingAgent_Accept asserts that NullCodingAgent.Accept is a no-op.
func TestNullCodingAgent_Accept(t *testing.T) {
	agent := NewNullCodingAgent()

	task := &domain.Task{
		ID:        "task-123",
		SessionID: "session-456",
		Status:    domain.TaskStatusAwaiting,
	}

	if err := agent.Accept(context.Background(), task); err != nil {
		t.Fatalf("Accept returned unexpected error: %v", err)
	}
}

// TestNullCodingAgent_Revert asserts that NullCodingAgent.Revert is a no-op.
func TestNullCodingAgent_Revert(t *testing.T) {
	agent := NewNullCodingAgent()

	task := &domain.Task{
		ID:        "task-123",
		SessionID: "session-456",
		Status:    domain.TaskStatusAwaiting,
	}

	if err := agent.Revert(context.Background(), task); err != nil {
		t.Fatalf("Revert returned unexpected error: %v", err)
	}
}

// TestACPCodingAgent_NewACPCodingAgent_RequiresAttacher asserts that attacher is required.
func TestACPCodingAgent_NewACPCodingAgent_RequiresAttacher(t *testing.T) {
	_, err := NewACPCodingAgent(ACPCodingAgentConfig{
		Attacher: nil,
		Provider: &Provider{Name: "test", Command: "/bin/true"},
	})
	if err == nil {
		t.Error("expected error when attacher is nil")
	}
}

// TestACPCodingAgent_NewACPCodingAgent_RequiresProvider asserts that provider is required.
func TestACPCodingAgent_NewACPCodingAgent_RequiresProvider(t *testing.T) {
	_, err := NewACPCodingAgent(ACPCodingAgentConfig{
		Attacher: &mockAttacher{},
		Provider: nil,
	})
	if err == nil {
		t.Error("expected error when provider is nil")
	}
}

// TestACPCodingAgent_NewACPCodingAgent_MissingBinary asserts error when binary not found.
func TestACPCodingAgent_NewACPCodingAgent_MissingBinary(t *testing.T) {
	_, err := NewACPCodingAgent(ACPCodingAgentConfig{
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
