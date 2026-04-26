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
	// With escalation, unknown commands now prompt the user.
	// Default mock response is "allow_once" which executes the command.
	ui := &mockUIMediator{}
	ui.SetPromptResponse(&registry.PromptResponse{SelectedOptionID: ApprovalOptionDeny})

	interceptor := NewInterceptor(InterceptorConfig{
		UI:              ui,
		ApprovalTimeout: 100 * time.Millisecond,
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

	// Wait for async prompt to complete
	time.Sleep(200 * time.Millisecond)

	// Should have prompted the user
	requests := ui.PromptRequests()
	if len(requests) != 1 {
		t.Fatalf("expected 1 prompt request, got %d", len(requests))
	}

	// After denial, should have forwarded to UI
	messages := ui.Messages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message after denial, got %d", len(messages))
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
	// With escalation, unknown permission requests now prompt the user.
	ui := &mockUIMediator{}
	ui.SetPromptResponse(&registry.PromptResponse{SelectedOptionID: ApprovalOptionDeny})

	interceptor := NewInterceptor(InterceptorConfig{
		UI:              ui,
		ApprovalTimeout: 100 * time.Millisecond,
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

	// Wait for async prompt to complete
	time.Sleep(200 * time.Millisecond)

	// Should have prompted the user
	requests := ui.PromptRequests()
	if len(requests) != 1 {
		t.Fatalf("expected 1 prompt request, got %d", len(requests))
	}

	// After denial, should have forwarded to UI
	messages := ui.Messages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message after denial, got %d: %v", len(messages), messages)
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
	// With escalation, blocked commands also prompt the user.
	ui := &mockUIMediator{}
	ui.SetPromptResponse(&registry.PromptResponse{SelectedOptionID: ApprovalOptionDeny})

	permRepo := &interceptorMockPermissionRepo{
		perm: &domain.ProjectPermission{
			AllowedCommands:    mustJSON([]string{"echo"}),
			AllowedDirectories: mustJSON([]string{"/tmp"}),
			BlockedCommands:    mustJSON([]string{"rm -rf"}),
		},
	}

	interceptor := NewInterceptor(InterceptorConfig{
		UI:              ui,
		PermRepo:        permRepo,
		ProjectID:       "project-1",
		WorkingDir:      "/tmp",
		ApprovalTimeout: 100 * time.Millisecond,
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

	// Wait for async prompt to complete
	time.Sleep(200 * time.Millisecond)

	// Should have prompted the user (blocked commands also escalate)
	requests := ui.PromptRequests()
	if len(requests) != 1 {
		t.Fatalf("expected 1 prompt request for blocked command, got %d", len(requests))
	}

	// After denial, should have rendered a halt message for blocked command
	messages := ui.Messages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message for blocked command after denial, got %d", len(messages))
	}
	if !strings.Contains(messages[0], "rm -rf /") {
		t.Errorf("message should contain blocked command: %q", messages[0])
	}

	// Should NOT have sent an auto-approval notification (command was denied)
	events := ui.Events()
	for _, ev := range events {
		if ev.Type == registry.EventACPAutoApproved {
			t.Error("should not have auto-approval event for blocked command")
		}
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
	// With escalation, Run() now prompts for each unknown command.
	ui := &mockUIMediator{}
	ui.SetPromptResponse(&registry.PromptResponse{SelectedOptionID: ApprovalOptionDeny})

	interceptor := NewInterceptor(InterceptorConfig{
		UI:              ui,
		ApprovalTimeout: 100 * time.Millisecond,
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
	case <-time.After(2 * time.Second):
		t.Fatal("interceptor did not finish")
	}

	// Wait for async prompts to complete
	time.Sleep(300 * time.Millisecond)

	// Should have prompted for both commands
	requests := ui.PromptRequests()
	if len(requests) != 2 {
		t.Fatalf("expected 2 prompt requests, got %d", len(requests))
	}

	// After denial, should have forwarded both to UI
	messages := ui.Messages()
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages after denials, got %d: %v", len(messages), messages)
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

// --- Story 3.6: Blocked Command Escalation Tests ---

func TestInterceptor_EscalateForApproval_TaskStatusAwaiting(t *testing.T) {
	// Task 3.6.1: Verify task status is updated to Awaiting
	ui := &mockUIMediator{}
	ui.SetPromptResponse(&registry.PromptResponse{SelectedOptionID: ApprovalOptionAllowOnce})

	taskRepo := &interceptorMockTaskRepo{}
	permRepo := &interceptorMockPermissionRepo{
		perm: &domain.ProjectPermission{
			AllowedCommands:    mustJSON([]string{"safe"}),
			AllowedDirectories: mustJSON([]string{"/tmp"}),
		},
	}

	interceptor := NewInterceptor(InterceptorConfig{
		UI:              ui,
		TaskRepo:        taskRepo,
		PermRepo:        permRepo,
		ProjectID:       "project-1",
		WorkingDir:      "/tmp",
		ApprovalTimeout: 5 * time.Second,
	})

	ev := acp.Event{
		Kind:         acp.EventToolCall,
		ACPSessionID: "sess-1",
		ToolName:     "bash",
		Command:      "unknown-command",
		Cwd:          "/tmp",
	}

	hctx := HandleContext{
		SessionID: "session-1",
		TaskID:    "task-123",
	}

	interceptor.Handle(context.Background(), ev, hctx)

	// Wait a bit for the approval goroutine to complete
	time.Sleep(100 * time.Millisecond)

	// Should have updated task status to Awaiting, then back to Processing
	updates := taskRepo.StatusUpdates()
	if len(updates) < 2 {
		t.Fatalf("expected at least 2 status updates, got %d: %+v", len(updates), updates)
	}

	// First update should be to Awaiting
	if updates[0].Status != domain.TaskStatusAwaiting {
		t.Errorf("first update status = %d, want %d (Awaiting)", updates[0].Status, domain.TaskStatusAwaiting)
	}

	// Second update should be back to Processing
	if updates[1].Status != domain.TaskStatusProcessing {
		t.Errorf("second update status = %d, want %d (Processing)", updates[1].Status, domain.TaskStatusProcessing)
	}
}

func TestInterceptor_ApprovalPrompt_Structure(t *testing.T) {
	// Task 3.6.2: Verify prompt structure matches spec
	ui := &mockUIMediator{}
	ui.SetPromptResponse(&registry.PromptResponse{SelectedOptionID: ApprovalOptionDeny})

	interceptor := NewInterceptor(InterceptorConfig{
		UI:              ui,
		ApprovalTimeout: 5 * time.Second,
	})

	ev := acp.Event{
		Kind:         acp.EventToolCall,
		ACPSessionID: "sess-1",
		ToolName:     "bash",
		Command:      "rm -rf /important",
		Cwd:          "/home/user",
	}

	hctx := HandleContext{
		SessionID: "session-1",
		TaskID:    "task-123",
	}

	interceptor.Handle(context.Background(), ev, hctx)

	// Wait for prompt to be processed
	time.Sleep(100 * time.Millisecond)

	requests := ui.PromptRequests()
	if len(requests) != 1 {
		t.Fatalf("expected 1 prompt request, got %d", len(requests))
	}

	req := requests[0]

	// Verify prompt kind
	if req.Kind != registry.PromptKindACPCommandApproval {
		t.Errorf("kind = %s, want %s", req.Kind, registry.PromptKindACPCommandApproval)
	}

	// Verify task ID
	if req.TaskID != "task-123" {
		t.Errorf("task ID = %s, want task-123", req.TaskID)
	}

	// Verify title
	if req.Title != "Allow command?" {
		t.Errorf("title = %q, want %q", req.Title, "Allow command?")
	}

	// Verify body contains command and cwd
	if !strings.Contains(req.Body, "rm -rf /important") {
		t.Errorf("body should contain command: %q", req.Body)
	}
	if !strings.Contains(req.Body, "/home/user") {
		t.Errorf("body should contain cwd: %q", req.Body)
	}

	// Verify options
	if len(req.PromptOptions) != 3 {
		t.Fatalf("expected 3 options, got %d", len(req.PromptOptions))
	}

	optionIDs := []string{req.PromptOptions[0].ID, req.PromptOptions[1].ID, req.PromptOptions[2].ID}
	expectedIDs := []string{ApprovalOptionAllowOnce, ApprovalOptionAllowSession, ApprovalOptionDeny}

	for i, id := range expectedIDs {
		if optionIDs[i] != id {
			t.Errorf("option[%d].ID = %s, want %s", i, optionIDs[i], id)
		}
	}
}

func TestInterceptor_AllowOnce_ExecutesCommand(t *testing.T) {
	// Task 3.6.3: Allow once executes the command
	ui := &mockUIMediator{}
	ui.SetPromptResponse(&registry.PromptResponse{SelectedOptionID: ApprovalOptionAllowOnce})

	taskRepo := &interceptorMockTaskRepo{}
	agentRepo := &interceptorMockAgentRepo{
		agents: map[string]*domain.Agent{
			"sys-auto-approve": {ID: "agent-1", Name: "sys-auto-approve"},
		},
	}

	interceptor := NewInterceptor(InterceptorConfig{
		UI:              ui,
		TaskRepo:        taskRepo,
		AgentRepo:       agentRepo,
		ApprovalTimeout: 5 * time.Second,
	})

	ev := acp.Event{
		Kind:         acp.EventToolCall,
		ACPSessionID: "sess-1",
		ToolName:     "bash",
		Command:      "echo hello",
		Cwd:          "/tmp",
	}

	hctx := HandleContext{
		SessionID: "session-1",
		TaskID:    "task-123",
	}

	interceptor.Handle(context.Background(), ev, hctx)

	// Wait for the async approval to complete
	time.Sleep(200 * time.Millisecond)

	// Should have created an audit task (command was executed)
	if len(taskRepo.tasks) != 1 {
		t.Fatalf("expected 1 audit task, got %d", len(taskRepo.tasks))
	}

	// Verify resolution notification was sent
	events := ui.Events()
	var resolvedEvent *registry.UIEvent
	for i := range events {
		if events[i].Type == registry.EventACPApprovalResolved {
			resolvedEvent = &events[i]
			break
		}
	}

	if resolvedEvent == nil {
		t.Fatal("expected EventACPApprovalResolved event")
	}

	payload, ok := resolvedEvent.Payload.(ApprovalResolvedPayload)
	if !ok {
		t.Fatalf("payload type = %T, want ApprovalResolvedPayload", resolvedEvent.Payload)
	}
	if payload.Resolution != "allowed_once" {
		t.Errorf("resolution = %s, want allowed_once", payload.Resolution)
	}
}

func TestInterceptor_AllowSession_AddsToWhitelist(t *testing.T) {
	// Task 3.6.3: Allow session adds command to session whitelist
	ui := &mockUIMediator{}
	ui.SetPromptResponse(&registry.PromptResponse{SelectedOptionID: ApprovalOptionAllowSession})

	taskRepo := &interceptorMockTaskRepo{}
	agentRepo := &interceptorMockAgentRepo{
		agents: map[string]*domain.Agent{
			"sys-auto-approve": {ID: "agent-1", Name: "sys-auto-approve"},
		},
	}

	interceptor := NewInterceptor(InterceptorConfig{
		UI:              ui,
		TaskRepo:        taskRepo,
		AgentRepo:       agentRepo,
		ApprovalTimeout: 5 * time.Second,
	})

	ev := acp.Event{
		Kind:         acp.EventToolCall,
		ACPSessionID: "sess-1",
		ToolName:     "bash",
		Command:      "npm install",
		Cwd:          "/project",
	}

	hctx := HandleContext{
		SessionID: "session-1",
		TaskID:    "task-123",
	}

	// First call - should prompt
	interceptor.Handle(context.Background(), ev, hctx)
	time.Sleep(200 * time.Millisecond)

	// Verify command was added to session whitelist
	if !interceptor.isSessionWhitelisted("npm install") {
		t.Error("command should be in session whitelist after allow_session")
	}

	// Second call with same command - should NOT prompt (auto-approved via whitelist)
	ui.mu.Lock()
	ui.promptRequests = nil // Clear previous requests
	ui.mu.Unlock()

	interceptor.Handle(context.Background(), ev, hctx)
	time.Sleep(100 * time.Millisecond)

	// Should not have prompted again
	requests := ui.PromptRequests()
	if len(requests) != 0 {
		t.Errorf("expected 0 prompt requests for whitelisted command, got %d", len(requests))
	}
}

func TestInterceptor_Deny_SendsDenialToAgent(t *testing.T) {
	// Task 3.6.3: Deny sends rejection to agent
	ui := &mockUIMediator{}
	ui.SetPromptResponse(&registry.PromptResponse{SelectedOptionID: ApprovalOptionDeny})

	taskRepo := &interceptorMockTaskRepo{}

	interceptor := NewInterceptor(InterceptorConfig{
		UI:              ui,
		TaskRepo:        taskRepo,
		ApprovalTimeout: 5 * time.Second,
	})

	ev := acp.Event{
		Kind:         acp.EventToolCall,
		ACPSessionID: "sess-1",
		ToolName:     "bash",
		Command:      "dangerous command",
		Cwd:          "/tmp",
	}

	hctx := HandleContext{
		SessionID: "session-1",
		TaskID:    "task-123",
	}

	interceptor.Handle(context.Background(), ev, hctx)
	time.Sleep(200 * time.Millisecond)

	// Should have forwarded to UI (RenderMessage) with denial reason
	messages := ui.Messages()
	found := false
	for _, msg := range messages {
		if strings.Contains(msg, "dangerous command") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected halt message for denied command")
	}

	// Verify resolution notification
	events := ui.Events()
	var resolvedEvent *registry.UIEvent
	for i := range events {
		if events[i].Type == registry.EventACPApprovalResolved {
			resolvedEvent = &events[i]
			break
		}
	}

	if resolvedEvent == nil {
		t.Fatal("expected EventACPApprovalResolved event")
	}

	payload, ok := resolvedEvent.Payload.(ApprovalResolvedPayload)
	if !ok {
		t.Fatalf("payload type = %T, want ApprovalResolvedPayload", resolvedEvent.Payload)
	}
	if payload.Resolution != "denied" {
		t.Errorf("resolution = %s, want denied", payload.Resolution)
	}

	// Task status should be restored to Processing
	updates := taskRepo.StatusUpdates()
	lastUpdate := updates[len(updates)-1]
	if lastUpdate.Status != domain.TaskStatusProcessing {
		t.Errorf("final status = %d, want %d (Processing)", lastUpdate.Status, domain.TaskStatusProcessing)
	}
}

func TestInterceptor_Timeout_AutoDenies(t *testing.T) {
	// Task 3.6.4: Timeout auto-denies the command
	ui := &mockUIMediator{}
	// Make the mock block until context timeout
	ui.SetBlockOnPrompt(true)

	taskRepo := &interceptorMockTaskRepo{}

	// Very short timeout for testing
	interceptor := NewInterceptor(InterceptorConfig{
		UI:              ui,
		TaskRepo:        taskRepo,
		ApprovalTimeout: 50 * time.Millisecond,
	})

	ev := acp.Event{
		Kind:         acp.EventToolCall,
		ACPSessionID: "sess-1",
		ToolName:     "bash",
		Command:      "slow command",
		Cwd:          "/tmp",
	}

	hctx := HandleContext{
		SessionID: "session-1",
		TaskID:    "task-timeout",
	}

	interceptor.Handle(context.Background(), ev, hctx)

	// Wait for timeout + processing
	time.Sleep(200 * time.Millisecond)

	// Should have a timeout message
	messages := ui.Messages()
	found := false
	for _, msg := range messages {
		if strings.Contains(msg, "timed out") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected timeout message, got: %v", messages)
	}
}

func TestInterceptor_CancelPendingApprovals(t *testing.T) {
	// Task 3.6.4: Cancel pending approvals on session shutdown
	ui := &mockUIMediator{}
	// Make the mock block to keep the approval pending
	ui.SetBlockOnPrompt(true)

	interceptor := NewInterceptor(InterceptorConfig{
		UI:              ui,
		ApprovalTimeout: 10 * time.Second,
	})

	ev := acp.Event{
		Kind:         acp.EventToolCall,
		ACPSessionID: "sess-1",
		ToolName:     "bash",
		Command:      "pending command",
		Cwd:          "/tmp",
	}

	hctx := HandleContext{
		SessionID: "session-1",
		TaskID:    "task-cancel",
	}

	// Start escalation
	interceptor.Handle(context.Background(), ev, hctx)

	// Verify there's a pending approval
	time.Sleep(50 * time.Millisecond)
	if interceptor.PendingApprovalCount() == 0 {
		t.Fatal("expected at least 1 pending approval")
	}

	// Cancel all pending approvals
	interceptor.CancelPendingApprovals()

	// Wait for cancellation to process
	time.Sleep(100 * time.Millisecond)

	// Should have no pending approvals
	if interceptor.PendingApprovalCount() != 0 {
		t.Errorf("expected 0 pending approvals after cancel, got %d", interceptor.PendingApprovalCount())
	}
}

func TestInterceptor_BlockedCommand_EscalatesForApproval(t *testing.T) {
	// Blocked commands should also escalate (not just auto-deny)
	ui := &mockUIMediator{}
	ui.SetPromptResponse(&registry.PromptResponse{SelectedOptionID: ApprovalOptionAllowOnce})

	taskRepo := &interceptorMockTaskRepo{}
	agentRepo := &interceptorMockAgentRepo{
		agents: map[string]*domain.Agent{
			"sys-auto-approve": {ID: "agent-1", Name: "sys-auto-approve"},
		},
	}
	permRepo := &interceptorMockPermissionRepo{
		perm: &domain.ProjectPermission{
			BlockedCommands: mustJSON([]string{"rm -rf"}),
		},
	}

	interceptor := NewInterceptor(InterceptorConfig{
		UI:              ui,
		TaskRepo:        taskRepo,
		AgentRepo:       agentRepo,
		PermRepo:        permRepo,
		ProjectID:       "project-1",
		WorkingDir:      "/tmp",
		ApprovalTimeout: 5 * time.Second,
	})

	ev := acp.Event{
		Kind:         acp.EventToolCall,
		ACPSessionID: "sess-1",
		ToolName:     "bash",
		Command:      "rm -rf /some/path",
		Cwd:          "/tmp",
	}

	hctx := HandleContext{
		SessionID: "session-1",
		TaskID:    "task-blocked",
	}

	interceptor.Handle(context.Background(), ev, hctx)
	time.Sleep(200 * time.Millisecond)

	// Should have prompted user (not auto-denied)
	requests := ui.PromptRequests()
	if len(requests) != 1 {
		t.Fatalf("expected 1 prompt request for blocked command, got %d", len(requests))
	}

	// If user approves, command should execute
	if len(taskRepo.tasks) != 1 {
		t.Errorf("expected 1 audit task (command executed after approval), got %d", len(taskRepo.tasks))
	}
}

func TestInterceptor_AwaitingApprovalNotification(t *testing.T) {
	// Verify awaiting approval notification is sent
	ui := &mockUIMediator{}
	ui.SetPromptResponse(&registry.PromptResponse{SelectedOptionID: ApprovalOptionDeny})

	interceptor := NewInterceptor(InterceptorConfig{
		UI:              ui,
		ApprovalTimeout: 5 * time.Second,
	})

	ev := acp.Event{
		Kind:         acp.EventToolCall,
		ACPSessionID: "sess-1",
		ToolName:     "bash",
		Command:      "some command",
		Cwd:          "/tmp",
	}

	hctx := HandleContext{
		SessionID: "session-1",
		TaskID:    "task-notify",
	}

	interceptor.Handle(context.Background(), ev, hctx)
	time.Sleep(100 * time.Millisecond)

	// Check for awaiting approval notification
	events := ui.Events()
	var awaitingEvent *registry.UIEvent
	for i := range events {
		if events[i].Type == registry.EventACPAwaitingApproval {
			awaitingEvent = &events[i]
			break
		}
	}

	if awaitingEvent == nil {
		t.Fatal("expected EventACPAwaitingApproval notification")
	}

	payload, ok := awaitingEvent.Payload.(AwaitingApprovalPayload)
	if !ok {
		t.Fatalf("payload type = %T, want AwaitingApprovalPayload", awaitingEvent.Payload)
	}

	if payload.Command != "some command" {
		t.Errorf("command = %s, want 'some command'", payload.Command)
	}
	if payload.TaskID != "task-notify" {
		t.Errorf("task ID = %s, want task-notify", payload.TaskID)
	}
}
