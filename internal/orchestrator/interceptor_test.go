package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/registry"
)

func TestInterceptor_Handle_ToolCall_NoPermissions(t *testing.T) {
	ui := &mockUIMediator{}
	interceptor := NewInterceptor(InterceptorConfig{
		UI: ui,
	})

	ev := acp.Event{
		Kind:         acp.EventToolCall,
		ACPSessionID: "sess-1",
		ToolName:     "bash",
		Command:      "ls -la",
		Raw:          json.RawMessage(`{}`),
	}

	hctx := HandleContext{
		SessionID: "session-1",
		TaskID:    "task-123",
	}

	interceptor.Handle(context.Background(), ev, hctx)

	messages := ui.Messages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	expected := "[ACP] Tool call halted: bash `ls -la`"
	if messages[0] != expected {
		t.Errorf("message = %q, want %q", messages[0], expected)
	}
}

func TestInterceptor_Handle_ToolCallNoCommand(t *testing.T) {
	ui := &mockUIMediator{}
	interceptor := NewInterceptor(InterceptorConfig{
		UI: ui,
	})

	ev := acp.Event{
		Kind:         acp.EventToolCall,
		ACPSessionID: "sess-1",
		ToolName:     "read",
		Raw:          json.RawMessage(`{}`),
	}

	hctx := HandleContext{TaskID: "task-123"}

	interceptor.Handle(context.Background(), ev, hctx)

	messages := ui.Messages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	expected := "[ACP] Tool call halted: read"
	if messages[0] != expected {
		t.Errorf("message = %q, want %q", messages[0], expected)
	}
}

func TestInterceptor_Handle_PermissionRequest_NoPermissions(t *testing.T) {
	ui := &mockUIMediator{}
	interceptor := NewInterceptor(InterceptorConfig{
		UI: ui,
	})

	ev := acp.Event{
		Kind:         acp.EventPermissionRequest,
		ACPSessionID: "sess-2",
		RequestID:    "req-001",
		ToolName:     "bash",
		Command:      "rm -rf /tmp/test",
		Raw:          json.RawMessage(`{}`),
	}

	hctx := HandleContext{TaskID: "task-456"}

	interceptor.Handle(context.Background(), ev, hctx)

	messages := ui.Messages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	if !strings.Contains(messages[0], "bash") || !strings.Contains(messages[0], "rm -rf /tmp/test") {
		t.Errorf("message should contain tool and command: %q", messages[0])
	}
}

func TestInterceptor_Handle_AllowedCommand(t *testing.T) {
	ui := &mockUIMediator{}
	permRepo := &interceptorMockPermissionRepo{
		perm: &domain.ProjectPermission{
			AllowedCommands:    mustJSON([]string{"echo"}),
			AllowedDirectories: mustJSON([]string{"/tmp"}),
		},
	}
	taskRepo := &interceptorMockTaskRepo{}
	agentRepo := &interceptorMockAgentRepo{
		agents: map[string]*domain.Agent{
			"sys-auto-approve": {ID: "agent-1", Name: "sys-auto-approve"},
		},
	}

	interceptor := NewInterceptor(InterceptorConfig{
		UI:         ui,
		PermRepo:   permRepo,
		TaskRepo:   taskRepo,
		AgentRepo:  agentRepo,
		ProjectID:  "project-1",
		WorkingDir: "/tmp",
	})

	ev := acp.Event{
		Kind:         acp.EventToolCall,
		ACPSessionID: "sess-1",
		ToolName:     "bash",
		Command:      "echo hello",
		Cwd:          "/tmp",
		Raw:          json.RawMessage(`{}`),
	}

	hctx := HandleContext{
		SessionID: "session-1",
		TaskID:    "task-123",
	}

	interceptor.Handle(context.Background(), ev, hctx)

	// Should NOT have rendered a halt message
	messages := ui.Messages()
	if len(messages) != 0 {
		t.Errorf("expected 0 halt messages for allowed command, got %d: %v", len(messages), messages)
	}

	// Should have sent an auto-approval notification
	events := ui.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != registry.EventACPAutoApproved {
		t.Errorf("event type = %s, want %s", events[0].Type, registry.EventACPAutoApproved)
	}

	// Should have created an audit task
	if len(taskRepo.tasks) != 1 {
		t.Fatalf("expected 1 audit task, got %d", len(taskRepo.tasks))
	}
	task := taskRepo.tasks[0]
	if task.Type != domain.TaskTypeCoordination {
		t.Errorf("task type = %d, want %d", task.Type, domain.TaskTypeCoordination)
	}
	if task.Status != domain.TaskStatusCompleted {
		t.Errorf("task status = %d, want %d", task.Status, domain.TaskStatusCompleted)
	}
}

func TestInterceptor_Handle_BlockedCommand(t *testing.T) {
	ui := &mockUIMediator{}
	permRepo := &interceptorMockPermissionRepo{
		perm: &domain.ProjectPermission{
			AllowedCommands:    mustJSON([]string{"echo"}),
			AllowedDirectories: mustJSON([]string{"/tmp"}),
			BlockedCommands:    mustJSON([]string{"rm -rf"}),
		},
	}

	interceptor := NewInterceptor(InterceptorConfig{
		UI:         ui,
		PermRepo:   permRepo,
		ProjectID:  "project-1",
		WorkingDir: "/tmp",
	})

	ev := acp.Event{
		Kind:         acp.EventToolCall,
		ACPSessionID: "sess-1",
		ToolName:     "bash",
		Command:      "rm -rf /",
		Cwd:          "/tmp",
		Raw:          json.RawMessage(`{}`),
	}

	hctx := HandleContext{TaskID: "task-123"}

	interceptor.Handle(context.Background(), ev, hctx)

	// Should have rendered a halt message for blocked command
	messages := ui.Messages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message for blocked command, got %d", len(messages))
	}
	if !strings.Contains(messages[0], "rm -rf /") {
		t.Errorf("message should contain blocked command: %q", messages[0])
	}

	// Should NOT have sent an auto-approval notification
	events := ui.Events()
	if len(events) != 0 {
		t.Errorf("expected 0 events for blocked command, got %d", len(events))
	}
}

func TestInterceptor_Handle_UnexpectedKind(t *testing.T) {
	ui := &mockUIMediator{}
	interceptor := NewInterceptor(InterceptorConfig{
		UI: ui,
	})

	// EventThinking should not normally reach the interceptor
	ev := acp.Event{
		Kind:         acp.EventThinking,
		ACPSessionID: "sess-3",
	}

	hctx := HandleContext{TaskID: "task-789"}

	// Should not panic
	interceptor.Handle(context.Background(), ev, hctx)

	// No messages should be rendered for unexpected kinds
	messages := ui.Messages()
	if len(messages) != 0 {
		t.Errorf("expected 0 messages for unexpected kind, got %d", len(messages))
	}
}

func TestInterceptor_Run(t *testing.T) {
	ui := &mockUIMediator{}
	interceptor := NewInterceptor(InterceptorConfig{
		UI: ui,
	})

	halted := make(chan acp.Event, 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hctx := HandleContext{TaskID: "task-run"}

	// Start interceptor in background
	done := make(chan struct{})
	go func() {
		interceptor.Run(ctx, halted, hctx)
		close(done)
	}()

	// Send events
	halted <- acp.Event{
		Kind:     acp.EventToolCall,
		ToolName: "bash",
		Command:  "echo hello",
	}
	halted <- acp.Event{
		Kind:      acp.EventPermissionRequest,
		ToolName:  "shell",
		Command:   "npm install",
		RequestID: "req-1",
	}

	// Close channel to signal completion
	close(halted)

	// Wait for interceptor to finish
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("interceptor did not finish")
	}

	messages := ui.Messages()
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
}

func TestInterceptor_Run_ContextCancel(t *testing.T) {
	ui := &mockUIMediator{}
	interceptor := NewInterceptor(InterceptorConfig{
		UI: ui,
	})

	halted := make(chan acp.Event)
	ctx, cancel := context.WithCancel(context.Background())

	hctx := HandleContext{TaskID: "task-cancel"}

	done := make(chan struct{})
	go func() {
		interceptor.Run(ctx, halted, hctx)
		close(done)
	}()

	// Cancel context
	cancel()

	// Wait for interceptor to finish
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("interceptor did not finish on context cancel")
	}
}

func TestInterceptor_NilUI(t *testing.T) {
	// Test that interceptor works without a UI (doesn't panic)
	interceptor := NewInterceptor(InterceptorConfig{
		// No UI provided
	})

	ev := acp.Event{
		Kind:     acp.EventToolCall,
		ToolName: "bash",
		Command:  "ls",
	}

	hctx := HandleContext{TaskID: "task-nil"}

	// Should not panic
	interceptor.Handle(context.Background(), ev, hctx)
}

func TestFormatHaltMessage(t *testing.T) {
	tests := []struct {
		name     string
		ev       acp.Event
		expected string
	}{
		{
			name: "with command",
			ev: acp.Event{
				ToolName: "bash",
				Command:  "ls -la",
			},
			expected: "[ACP] Tool call halted: bash `ls -la`",
		},
		{
			name: "without command",
			ev: acp.Event{
				ToolName: "read",
			},
			expected: "[ACP] Tool call halted: read",
		},
		{
			name: "empty tool name",
			ev: acp.Event{
				Command: "something",
			},
			expected: "[ACP] Tool call halted:  `something`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatHaltMessage(tt.ev)
			if got != tt.expected {
				t.Errorf("formatHaltMessage() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestInterceptor_EvaluateCommand_UsesWorkingDirFallback(t *testing.T) {
	permRepo := &interceptorMockPermissionRepo{
		perm: &domain.ProjectPermission{
			AllowedCommands:    mustJSON([]string{"pwd"}),
			AllowedDirectories: mustJSON([]string{"/project"}),
		},
	}

	interceptor := NewInterceptor(InterceptorConfig{
		PermRepo:   permRepo,
		ProjectID:  "project-1",
		WorkingDir: "/project", // Should be used when ev.Cwd is empty
	})

	// When cwd is empty, should use workingDir
	decision := interceptor.evaluateCommand(context.Background(), "pwd", "")
	if decision != DecisionAllow {
		t.Errorf("decision = %s, want %s (should use workingDir fallback)", decision, DecisionAllow)
	}

	// When cwd is outside allowed directory
	decision = interceptor.evaluateCommand(context.Background(), "pwd", "/other")
	if decision != DecisionUnknown {
		t.Errorf("decision = %s, want %s (cwd outside allowed dirs)", decision, DecisionUnknown)
	}
}

func TestInterceptor_AuditTrail_FallbackToHuman(t *testing.T) {
	ui := &mockUIMediator{}
	permRepo := &interceptorMockPermissionRepo{
		perm: &domain.ProjectPermission{
			AllowedCommands:    mustJSON([]string{"echo"}),
			AllowedDirectories: mustJSON([]string{"/tmp"}),
		},
	}
	taskRepo := &interceptorMockTaskRepo{}
	// No sys-auto-approve agent, should fall back to human
	agentRepo := &interceptorMockAgentRepo{
		agents: map[string]*domain.Agent{
			"human": {ID: "human-agent", Name: "human"},
		},
	}

	interceptor := NewInterceptor(InterceptorConfig{
		UI:         ui,
		PermRepo:   permRepo,
		TaskRepo:   taskRepo,
		AgentRepo:  agentRepo,
		ProjectID:  "project-1",
		WorkingDir: "/tmp",
	})

	ev := acp.Event{
		Kind:     acp.EventToolCall,
		ToolName: "bash",
		Command:  "echo test",
		Cwd:      "/tmp",
	}

	hctx := HandleContext{
		SessionID: "session-1",
		TaskID:    "task-1",
	}

	interceptor.Handle(context.Background(), ev, hctx)

	// Should still create audit task using human agent
	if len(taskRepo.tasks) != 1 {
		t.Fatalf("expected 1 audit task, got %d", len(taskRepo.tasks))
	}
	if taskRepo.tasks[0].AssigneeID != "human-agent" {
		t.Errorf("assignee = %s, want human-agent (fallback)", taskRepo.tasks[0].AssigneeID)
	}
}
