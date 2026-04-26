package orchestrator

import (
	"context"
	"sync"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/registry"
)

// mockUIMediator captures RenderMessage calls for testing.
// Used by multiple test files in the orchestrator package.
type mockUIMediator struct {
	mu       sync.Mutex
	messages []string
	events   []registry.UIEvent

	// promptResponse is the response to return from PromptDecision.
	// If nil, returns the first option.
	promptResponse *registry.PromptResponse
	// promptRequests captures all prompt requests for testing.
	promptRequests []registry.PromptRequest
	// blockOnPrompt if true, blocks until context is cancelled.
	blockOnPrompt bool
}

func (m *mockUIMediator) RenderMessage(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, msg)
}

func (m *mockUIMediator) ClearScreen() {}

func (m *mockUIMediator) NotifyAll(event registry.UIEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
}

func (m *mockUIMediator) PromptDecision(ctx context.Context, req registry.PromptRequest) registry.PromptResponse {
	m.mu.Lock()
	m.promptRequests = append(m.promptRequests, req)
	response := m.promptResponse
	shouldBlock := m.blockOnPrompt
	m.mu.Unlock()

	// If shouldBlock is true, wait for context to be done (simulate user delay/timeout)
	if shouldBlock {
		<-ctx.Done()
		return registry.PromptResponse{Error: ctx.Err()}
	}

	// Check if context is already cancelled
	select {
	case <-ctx.Done():
		return registry.PromptResponse{Error: ctx.Err()}
	default:
	}

	if response != nil {
		return *response
	}

	// Return first option by default (legacy behavior)
	if len(req.Options) > 0 {
		return registry.PromptResponse{SelectedOption: req.Options[0]}
	}
	if len(req.PromptOptions) > 0 {
		return registry.PromptResponse{
			SelectedOption:   req.PromptOptions[0].Label,
			SelectedOptionID: req.PromptOptions[0].ID,
		}
	}
	return registry.PromptResponse{}
}

func (m *mockUIMediator) Messages() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.messages...)
}

func (m *mockUIMediator) Events() []registry.UIEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]registry.UIEvent(nil), m.events...)
}

func (m *mockUIMediator) PromptRequests() []registry.PromptRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]registry.PromptRequest(nil), m.promptRequests...)
}

func (m *mockUIMediator) SetPromptResponse(resp *registry.PromptResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.promptResponse = resp
}

func (m *mockUIMediator) SetBlockOnPrompt(block bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blockOnPrompt = block
}

// interceptorMockPermissionRepo implements ProjectPermissionRepository for testing.
type interceptorMockPermissionRepo struct {
	perm *domain.ProjectPermission
	err  error
}

func (m *interceptorMockPermissionRepo) FindByProjectID(ctx context.Context, projectID string) (*domain.ProjectPermission, error) {
	return m.perm, m.err
}

func (m *interceptorMockPermissionRepo) Upsert(ctx context.Context, perm *domain.ProjectPermission) error {
	m.perm = perm
	return nil
}

// interceptorMockTaskRepo implements minimal TaskRepository for testing.
type interceptorMockTaskRepo struct {
	mu              sync.Mutex
	tasks           []*domain.Task
	statusUpdates   []statusUpdate
	statusUpdateErr error
}

type statusUpdate struct {
	TaskID string
	Status domain.TaskStatus
}

func (m *interceptorMockTaskRepo) Create(ctx context.Context, task *domain.Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks = append(m.tasks, task)
	return nil
}

func (m *interceptorMockTaskRepo) FindByID(ctx context.Context, id string) (*domain.Task, error) {
	return nil, nil
}

func (m *interceptorMockTaskRepo) Next(ctx context.Context, sessionID string) (*domain.Task, error) {
	return nil, nil
}

func (m *interceptorMockTaskRepo) Claim(ctx context.Context, id string) error {
	return nil
}

func (m *interceptorMockTaskRepo) UpdateStatus(ctx context.Context, id string, status domain.TaskStatus, output string) error {
	return nil
}

func (m *interceptorMockTaskRepo) UpdateTaskStatus(ctx context.Context, id string, status domain.TaskStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusUpdates = append(m.statusUpdates, statusUpdate{TaskID: id, Status: status})
	return m.statusUpdateErr
}

func (m *interceptorMockTaskRepo) UpdateAssignee(ctx context.Context, id string, assigneeID string) error {
	return nil
}

func (m *interceptorMockTaskRepo) FindInterrupted(ctx context.Context, sessionID string) ([]*domain.Task, error) {
	return nil, nil
}

func (m *interceptorMockTaskRepo) ListCoordinationAfter(ctx context.Context, sessionID, afterTaskID string) ([]*domain.Task, error) {
	return nil, nil
}

func (m *interceptorMockTaskRepo) StatusUpdates() []statusUpdate {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]statusUpdate(nil), m.statusUpdates...)
}

// interceptorMockAgentRepo implements minimal AgentRepository for testing.
type interceptorMockAgentRepo struct {
	agents map[string]*domain.Agent
}

func (m *interceptorMockAgentRepo) FindByName(ctx context.Context, name string) (*domain.Agent, error) {
	if m.agents == nil {
		return nil, nil
	}
	return m.agents[name], nil
}

func (m *interceptorMockAgentRepo) EnsureSystemAgents(ctx context.Context) error {
	return nil
}
