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
	return registry.PromptResponse{SelectedOption: req.Options[0]}
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
	tasks []*domain.Task
}

func (m *interceptorMockTaskRepo) Create(ctx context.Context, task *domain.Task) error {
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
	return nil
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
