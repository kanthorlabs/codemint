package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"codemint.kanthorlabs.com/internal/domain"
)

// mockPlanTaskRepo implements repository.TaskRepository for plan handler tests.
type mockPlanTaskRepo struct {
	bulkInsertCalled  bool
	bulkInsertTasks   []*domain.Task
	bulkInsertErr     error
}

func (m *mockPlanTaskRepo) Create(ctx context.Context, t *domain.Task) error { return nil }
func (m *mockPlanTaskRepo) Next(ctx context.Context, sessionID string) (*domain.Task, error) {
	return nil, nil
}
func (m *mockPlanTaskRepo) Claim(ctx context.Context, taskID string) error { return nil }
func (m *mockPlanTaskRepo) UpdateStatus(ctx context.Context, taskID string, status domain.TaskStatus, output string) error {
	return nil
}
func (m *mockPlanTaskRepo) FindInterrupted(ctx context.Context, sessionID string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockPlanTaskRepo) FindByID(ctx context.Context, taskID string) (*domain.Task, error) {
	return nil, nil
}
func (m *mockPlanTaskRepo) UpdateTaskStatus(ctx context.Context, taskID string, status domain.TaskStatus) error {
	return nil
}
func (m *mockPlanTaskRepo) UpdateAssignee(ctx context.Context, taskID string, assigneeID string) error {
	return nil
}
func (m *mockPlanTaskRepo) ListCoordinationAfter(ctx context.Context, sessionID string, afterTaskID string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockPlanTaskRepo) ListBySession(ctx context.Context, sessionID string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockPlanTaskRepo) ListPending(ctx context.Context, sessionID string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockPlanTaskRepo) MostRecentActive(ctx context.Context, sessionID string) (*domain.Task, error) {
	return nil, nil
}
func (m *mockPlanTaskRepo) CancelByWorkflowAndStoryIDs(ctx context.Context, workflowID string, storyIDs []string) error {
	return nil
}
func (m *mockPlanTaskRepo) GetMaxSeqTask(ctx context.Context, workflowID string) (int, error) {
	return 0, nil
}
func (m *mockPlanTaskRepo) ListByWorkflowAndStoryIDs(ctx context.Context, workflowID string, storyIDs []string) ([]*domain.Task, error) {
	return nil, nil
}
func (m *mockPlanTaskRepo) BulkInsert(ctx context.Context, tasks []*domain.Task) error {
	m.bulkInsertCalled = true
	m.bulkInsertTasks = tasks
	return m.bulkInsertErr
}

// mockPlanWorkflowRepo implements repository.WorkflowRepository for plan handler tests.
type mockPlanWorkflowRepo struct {
	workflow *domain.Workflow
	err      error
}

func (m *mockPlanWorkflowRepo) Create(ctx context.Context, w *domain.Workflow) error { return nil }
func (m *mockPlanWorkflowRepo) FindByID(ctx context.Context, id string) (*domain.Workflow, error) {
	return m.workflow, m.err
}
func (m *mockPlanWorkflowRepo) GetActiveForSession(ctx context.Context, sessionID string) (*domain.Workflow, error) {
	return nil, nil
}
func (m *mockPlanWorkflowRepo) UpdateProgress(ctx context.Context, id, epicID, storyID string) error {
	return nil
}
func (m *mockPlanWorkflowRepo) MarkCompleted(ctx context.Context, id string) error { return nil }
func (m *mockPlanWorkflowRepo) MarkCancelled(ctx context.Context, id string) error { return nil }
func (m *mockPlanWorkflowRepo) ListByFilePath(ctx context.Context, filePath string) ([]*domain.Workflow, error) {
	return nil, nil
}
func (m *mockPlanWorkflowRepo) ListBySession(ctx context.Context, sessionID string) ([]*domain.Workflow, error) {
	return nil, nil
}
func (m *mockPlanWorkflowRepo) LockGoal(ctx context.Context, workflowID, goalText, criteriaJSON string) error {
	return nil
}
func (m *mockPlanWorkflowRepo) LockChosenOption(ctx context.Context, workflowID, optionJSON string) error {
	return nil
}
func (m *mockPlanWorkflowRepo) ResetGOROW(ctx context.Context, workflowID string) error {
	return nil
}

// mockPlanSessionRepo implements repository.SessionRepository for plan handler tests.
type mockPlanSessionRepo struct {
	session *domain.Session
	err     error
}

func (m *mockPlanSessionRepo) Create(ctx context.Context, s *domain.Session) error { return nil }
func (m *mockPlanSessionRepo) FindByID(ctx context.Context, id string) (*domain.Session, error) {
	return m.session, m.err
}
func (m *mockPlanSessionRepo) FindActiveByProjectID(ctx context.Context, projectID string) (*domain.Session, error) {
	return nil, nil
}
func (m *mockPlanSessionRepo) Archive(ctx context.Context, id string) error { return nil }
func (m *mockPlanSessionRepo) ListByProjectID(ctx context.Context, projectID string) ([]*domain.Session, error) {
	return nil, nil
}
func (m *mockPlanSessionRepo) SaveState(ctx context.Context, sessionID, activeClient string, lastActivityAt int64) error {
	return nil
}
func (m *mockPlanSessionRepo) GetMostRecentActive(ctx context.Context) (*domain.Session, error) {
	return nil, nil
}
func (m *mockPlanSessionRepo) ClearOwnership(ctx context.Context, sessionID string) error {
	return nil
}
func (m *mockPlanSessionRepo) ListActive(ctx context.Context) ([]*domain.Session, error) {
	return nil, nil
}
func (m *mockPlanSessionRepo) CountActiveByProjectID(ctx context.Context, projectID string) (int, error) {
	return 0, nil
}

// mockPlanAgentRepo implements repository.AgentRepository for plan handler tests.
type mockPlanAgentRepo struct {
	humanAgent *domain.Agent
	err        error
}

func (m *mockPlanAgentRepo) EnsureSystemAgents(ctx context.Context) error { return nil }
func (m *mockPlanAgentRepo) FindByName(ctx context.Context, name string) (*domain.Agent, error) {
	if name == "human" && m.humanAgent != nil {
		return m.humanAgent, nil
	}
	return nil, m.err
}

func TestCreateImplementationTasksHandler_HappyPath(t *testing.T) {
	t.Parallel()

	taskRepo := &mockPlanTaskRepo{}
	workflowRepo := &mockPlanWorkflowRepo{
		workflow: &domain.Workflow{
			ID:        "wf-plan-test",
			SessionID: "session-plan-test",
		},
	}
	sessionRepo := &mockPlanSessionRepo{
		session: &domain.Session{
			ID:        "session-plan-test",
			ProjectID: "project-plan-test",
		},
	}
	agentRepo := &mockPlanAgentRepo{
		humanAgent: &domain.Agent{ID: "human-agent-1"},
	}

	// Create a minimal file registry for testing.
	fileRegistry := NewFileRegistry()

	handler := CreateImplementationTasksHandler(PlanGenerationDeps{
		WorkflowRepo: workflowRepo,
		TaskRepo:     taskRepo,
		SessionRepo:  sessionRepo,
		AgentRepo:    agentRepo,
		FileRegistry: fileRegistry,
	})

	// Valid plan with 1 epic, 1 story, 2 tasks.
	planJSON := `{
		"epics": [{
			"id": "epic-1",
			"name": "Authentication",
			"stories": [{
				"id": "story-1-1",
				"name": "Add Login",
				"tasks": [
					{"id": "task-1-1-1", "name": "Create login form"},
					{"id": "task-1-1-2", "name": "Add validation"}
				]
			}]
		}]
	}`

	args := HandlerArgs{
		WorkflowID: "wf-plan-test",
		Output:     planJSON,
	}

	err := handler(context.Background(), args)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	// Verify BulkInsert was called.
	if !taskRepo.bulkInsertCalled {
		t.Error("BulkInsert was not called")
	}

	// Should have 2 coding tasks + 1 verification + 1 confirmation = 4 tasks.
	if len(taskRepo.bulkInsertTasks) != 4 {
		t.Errorf("Expected 4 tasks, got %d", len(taskRepo.bulkInsertTasks))
	}

	// Verify task types.
	codingCount := 0
	verificationCount := 0
	confirmationCount := 0
	for _, task := range taskRepo.bulkInsertTasks {
		switch task.Type {
		case domain.TaskTypeCoding:
			codingCount++
		case domain.TaskTypeVerification:
			verificationCount++
		case domain.TaskTypeConfirmation:
			confirmationCount++
		}
	}

	if codingCount != 2 {
		t.Errorf("Expected 2 Coding tasks, got %d", codingCount)
	}
	if verificationCount != 1 {
		t.Errorf("Expected 1 Verification task, got %d", verificationCount)
	}
	if confirmationCount != 1 {
		t.Errorf("Expected 1 Confirmation task, got %d", confirmationCount)
	}
}

func TestCreateImplementationTasksHandler_SkillError(t *testing.T) {
	t.Parallel()

	taskRepo := &mockPlanTaskRepo{}
	workflowRepo := &mockPlanWorkflowRepo{
		workflow: &domain.Workflow{ID: "wf-1", SessionID: "session-1"},
	}
	sessionRepo := &mockPlanSessionRepo{
		session: &domain.Session{ID: "session-1", ProjectID: "project-1"},
	}
	agentRepo := &mockPlanAgentRepo{}
	fileRegistry := NewFileRegistry()

	handler := CreateImplementationTasksHandler(PlanGenerationDeps{
		WorkflowRepo: workflowRepo,
		TaskRepo:     taskRepo,
		SessionRepo:  sessionRepo,
		AgentRepo:    agentRepo,
		FileRegistry: fileRegistry,
	})

	// Skill returns error.
	errorJSON := `{"error": "Option B is impossible to implement"}`

	args := HandlerArgs{
		WorkflowID: "wf-1",
		Output:     errorJSON,
	}

	err := handler(context.Background(), args)
	if err == nil {
		t.Fatal("Expected error for skill error response")
	}

	if !strContains(err.Error(), "skill aborted") {
		t.Errorf("Error = %q, want to contain 'skill aborted'", err.Error())
	}

	// BulkInsert should NOT be called.
	if taskRepo.bulkInsertCalled {
		t.Error("BulkInsert should not be called on skill error")
	}
}

func TestCreateImplementationTasksHandler_InvalidJSON(t *testing.T) {
	t.Parallel()

	taskRepo := &mockPlanTaskRepo{}
	workflowRepo := &mockPlanWorkflowRepo{
		workflow: &domain.Workflow{ID: "wf-1", SessionID: "session-1"},
	}
	sessionRepo := &mockPlanSessionRepo{
		session: &domain.Session{ID: "session-1", ProjectID: "project-1"},
	}
	agentRepo := &mockPlanAgentRepo{}
	fileRegistry := NewFileRegistry()

	handler := CreateImplementationTasksHandler(PlanGenerationDeps{
		WorkflowRepo: workflowRepo,
		TaskRepo:     taskRepo,
		SessionRepo:  sessionRepo,
		AgentRepo:    agentRepo,
		FileRegistry: fileRegistry,
	})

	args := HandlerArgs{
		WorkflowID: "wf-1",
		Output:     "{invalid json",
	}

	err := handler(context.Background(), args)
	if err == nil {
		t.Fatal("Expected error for invalid JSON")
	}

	if !strContains(err.Error(), "invalid JSON") {
		t.Errorf("Error = %q, want to contain 'invalid JSON'", err.Error())
	}
}

func TestCreateImplementationTasksHandler_EmptyOutput(t *testing.T) {
	t.Parallel()

	handler := CreateImplementationTasksHandler(PlanGenerationDeps{
		WorkflowRepo: &mockPlanWorkflowRepo{},
		TaskRepo:     &mockPlanTaskRepo{},
		SessionRepo:  &mockPlanSessionRepo{},
		AgentRepo:    &mockPlanAgentRepo{},
		FileRegistry: NewFileRegistry(),
	})

	args := HandlerArgs{
		WorkflowID: "wf-1",
		Output:     "",
	}

	err := handler(context.Background(), args)
	if err == nil {
		t.Fatal("Expected error for empty output")
	}

	if !strContains(err.Error(), "output is empty") {
		t.Errorf("Error = %q, want to contain 'output is empty'", err.Error())
	}
}

func TestCreateImplementationTasksHandler_MissingWorkflowID(t *testing.T) {
	t.Parallel()

	handler := CreateImplementationTasksHandler(PlanGenerationDeps{
		WorkflowRepo: &mockPlanWorkflowRepo{},
		TaskRepo:     &mockPlanTaskRepo{},
		SessionRepo:  &mockPlanSessionRepo{},
		AgentRepo:    &mockPlanAgentRepo{},
		FileRegistry: NewFileRegistry(),
	})

	args := HandlerArgs{
		WorkflowID: "",
		Output:     `{"epics":[]}`,
	}

	err := handler(context.Background(), args)
	if err == nil {
		t.Fatal("Expected error for missing workflow ID")
	}

	if !strContains(err.Error(), "workflow ID is required") {
		t.Errorf("Error = %q, want to contain 'workflow ID is required'", err.Error())
	}
}

func TestCreateImplementationTasksHandler_NoEpics(t *testing.T) {
	t.Parallel()

	workflowRepo := &mockPlanWorkflowRepo{
		workflow: &domain.Workflow{ID: "wf-1", SessionID: "session-1"},
	}
	sessionRepo := &mockPlanSessionRepo{
		session: &domain.Session{ID: "session-1", ProjectID: "project-1"},
	}

	handler := CreateImplementationTasksHandler(PlanGenerationDeps{
		WorkflowRepo: workflowRepo,
		TaskRepo:     &mockPlanTaskRepo{},
		SessionRepo:  sessionRepo,
		AgentRepo:    &mockPlanAgentRepo{},
		FileRegistry: NewFileRegistry(),
	})

	args := HandlerArgs{
		WorkflowID: "wf-1",
		Output:     `{"epics":[]}`,
	}

	err := handler(context.Background(), args)
	if err == nil {
		t.Fatal("Expected error for empty epics")
	}

	if !strContains(err.Error(), "no epics") {
		t.Errorf("Error = %q, want to contain 'no epics'", err.Error())
	}
}

func TestCreateImplementationTasksHandler_BulkInsertError(t *testing.T) {
	t.Parallel()

	taskRepo := &mockPlanTaskRepo{
		bulkInsertErr: errors.New("database connection lost"),
	}
	workflowRepo := &mockPlanWorkflowRepo{
		workflow: &domain.Workflow{ID: "wf-1", SessionID: "session-1"},
	}
	sessionRepo := &mockPlanSessionRepo{
		session: &domain.Session{ID: "session-1", ProjectID: "project-1"},
	}
	agentRepo := &mockPlanAgentRepo{
		humanAgent: &domain.Agent{ID: "human-1"},
	}

	handler := CreateImplementationTasksHandler(PlanGenerationDeps{
		WorkflowRepo: workflowRepo,
		TaskRepo:     taskRepo,
		SessionRepo:  sessionRepo,
		AgentRepo:    agentRepo,
		FileRegistry: NewFileRegistry(),
	})

	planJSON := `{
		"epics": [{
			"id": "epic-1",
			"name": "Auth",
			"stories": [{
				"id": "story-1-1",
				"name": "Login",
				"tasks": [{"id": "task-1", "name": "Add form"}]
			}]
		}]
	}`

	args := HandlerArgs{
		WorkflowID: "wf-1",
		Output:     planJSON,
	}

	err := handler(context.Background(), args)
	if err == nil {
		t.Fatal("Expected error from BulkInsert failure")
	}

	if !strContains(err.Error(), "database connection lost") {
		t.Errorf("Error = %q, want to contain 'database connection lost'", err.Error())
	}
}

func TestCreateImplementationTasksHandler_VerifyDependsOnChain(t *testing.T) {
	t.Parallel()

	taskRepo := &mockPlanTaskRepo{}
	workflowRepo := &mockPlanWorkflowRepo{
		workflow: &domain.Workflow{ID: "wf-1", SessionID: "session-1"},
	}
	sessionRepo := &mockPlanSessionRepo{
		session: &domain.Session{ID: "session-1", ProjectID: "project-1"},
	}
	agentRepo := &mockPlanAgentRepo{
		humanAgent: &domain.Agent{ID: "human-1"},
	}

	handler := CreateImplementationTasksHandler(PlanGenerationDeps{
		WorkflowRepo: workflowRepo,
		TaskRepo:     taskRepo,
		SessionRepo:  sessionRepo,
		AgentRepo:    agentRepo,
		FileRegistry: NewFileRegistry(),
	})

	// Plan with 2 coding tasks in one story.
	planJSON := `{
		"epics": [{
			"id": "epic-1",
			"name": "Feature",
			"stories": [{
				"id": "story-1-1",
				"name": "Implementation",
				"tasks": [
					{"id": "task-1", "name": "First"},
					{"id": "task-2", "name": "Second"}
				]
			}]
		}]
	}`

	args := HandlerArgs{
		WorkflowID: "wf-1",
		Output:     planJSON,
	}

	err := handler(context.Background(), args)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	tasks := taskRepo.bulkInsertTasks
	if len(tasks) != 4 {
		t.Fatalf("Expected 4 tasks, got %d", len(tasks))
	}

	// Task order: coding1, coding2, verification, confirmation.
	coding1 := tasks[0]
	coding2 := tasks[1]
	verifyTask := tasks[2]
	confirmTask := tasks[3]

	// Verify types.
	if coding1.Type != domain.TaskTypeCoding {
		t.Errorf("Task 0 type = %v, want Coding", coding1.Type)
	}
	if coding2.Type != domain.TaskTypeCoding {
		t.Errorf("Task 1 type = %v, want Coding", coding2.Type)
	}
	if verifyTask.Type != domain.TaskTypeVerification {
		t.Errorf("Task 2 type = %v, want Verification", verifyTask.Type)
	}
	if confirmTask.Type != domain.TaskTypeConfirmation {
		t.Errorf("Task 3 type = %v, want Confirmation", confirmTask.Type)
	}

	// Verify depends_on chain.
	// Verification depends on last coding task (coding2).
	if !verifyTask.DependsOn.Valid || verifyTask.DependsOn.String != coding2.ID {
		t.Errorf("Verification.DependsOn = %q, want %q", verifyTask.DependsOn.String, coding2.ID)
	}

	// Confirmation depends on verification.
	if !confirmTask.DependsOn.Valid || confirmTask.DependsOn.String != verifyTask.ID {
		t.Errorf("Confirmation.DependsOn = %q, want %q", confirmTask.DependsOn.String, verifyTask.ID)
	}
}

func TestCreateImplementationTasksHandler_StoryVerificationCommand(t *testing.T) {
	t.Parallel()

	taskRepo := &mockPlanTaskRepo{}
	workflowRepo := &mockPlanWorkflowRepo{
		workflow: &domain.Workflow{ID: "wf-1", SessionID: "session-1"},
	}
	sessionRepo := &mockPlanSessionRepo{
		session: &domain.Session{ID: "session-1", ProjectID: "project-1"},
	}
	agentRepo := &mockPlanAgentRepo{
		humanAgent: &domain.Agent{ID: "human-1"},
	}

	handler := CreateImplementationTasksHandler(PlanGenerationDeps{
		WorkflowRepo: workflowRepo,
		TaskRepo:     taskRepo,
		SessionRepo:  sessionRepo,
		AgentRepo:    agentRepo,
		FileRegistry: NewFileRegistry(),
	})

	// Plan with custom verification command.
	planJSON := `{
		"epics": [{
			"id": "epic-1",
			"name": "Feature",
			"stories": [{
				"id": "story-1-1",
				"name": "Implementation",
				"verification": "npm run test -- --testPathPattern=login",
				"tasks": [{"id": "task-1", "name": "Add login"}]
			}]
		}]
	}`

	args := HandlerArgs{
		WorkflowID: "wf-1",
		Output:     planJSON,
	}

	err := handler(context.Background(), args)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	tasks := taskRepo.bulkInsertTasks
	
	// Find verification task.
	var verifyTask *domain.Task
	for _, task := range tasks {
		if task.Type == domain.TaskTypeVerification {
			verifyTask = task
			break
		}
	}

	if verifyTask == nil {
		t.Fatal("No verification task found")
	}

	// Parse input to check command.
	var input VerificationTaskInput
	if err := json.Unmarshal([]byte(verifyTask.Input.String), &input); err != nil {
		t.Fatalf("Failed to parse verification input: %v", err)
	}

	expectedCmd := "npm run test -- --testPathPattern=login"
	if input.Command != expectedCmd {
		t.Errorf("Verification command = %q, want %q", input.Command, expectedCmd)
	}
}

func TestCreateImplementationTasksHandler_NilDependencies(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		deps PlanGenerationDeps
		want string
	}{
		{
			name: "nil workflow repo",
			deps: PlanGenerationDeps{
				WorkflowRepo: nil,
				TaskRepo:     &mockPlanTaskRepo{},
				SessionRepo:  &mockPlanSessionRepo{},
				AgentRepo:    &mockPlanAgentRepo{},
				FileRegistry: NewFileRegistry(),
			},
			want: "workflow repository is nil",
		},
		{
			name: "nil task repo",
			deps: PlanGenerationDeps{
				WorkflowRepo: &mockPlanWorkflowRepo{},
				TaskRepo:     nil,
				SessionRepo:  &mockPlanSessionRepo{},
				AgentRepo:    &mockPlanAgentRepo{},
				FileRegistry: NewFileRegistry(),
			},
			want: "task repository is nil",
		},
		{
			name: "nil session repo",
			deps: PlanGenerationDeps{
				WorkflowRepo: &mockPlanWorkflowRepo{},
				TaskRepo:     &mockPlanTaskRepo{},
				SessionRepo:  nil,
				AgentRepo:    &mockPlanAgentRepo{},
				FileRegistry: NewFileRegistry(),
			},
			want: "session repository is nil",
		},
		{
			name: "nil agent repo",
			deps: PlanGenerationDeps{
				WorkflowRepo: &mockPlanWorkflowRepo{},
				TaskRepo:     &mockPlanTaskRepo{},
				SessionRepo:  &mockPlanSessionRepo{},
				AgentRepo:    nil,
				FileRegistry: NewFileRegistry(),
			},
			want: "agent repository is nil",
		},
		{
			name: "nil file registry",
			deps: PlanGenerationDeps{
				WorkflowRepo: &mockPlanWorkflowRepo{},
				TaskRepo:     &mockPlanTaskRepo{},
				SessionRepo:  &mockPlanSessionRepo{},
				AgentRepo:    &mockPlanAgentRepo{},
				FileRegistry: nil,
			},
			want: "file registry is nil",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler := CreateImplementationTasksHandler(tc.deps)
			args := HandlerArgs{
				WorkflowID: "wf-1",
				Output:     `{"epics":[]}`,
			}

			err := handler(context.Background(), args)
			if err == nil {
				t.Fatal("Expected error for nil dependency")
			}

			if !strContains(err.Error(), tc.want) {
				t.Errorf("Error = %q, want to contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestCreateImplementationTasksHandler_MultipleStoriesPerEpic(t *testing.T) {
	t.Parallel()

	taskRepo := &mockPlanTaskRepo{}
	workflowRepo := &mockPlanWorkflowRepo{
		workflow: &domain.Workflow{ID: "wf-1", SessionID: "session-1"},
	}
	sessionRepo := &mockPlanSessionRepo{
		session: &domain.Session{ID: "session-1", ProjectID: "project-1"},
	}
	agentRepo := &mockPlanAgentRepo{
		humanAgent: &domain.Agent{ID: "human-1"},
	}

	handler := CreateImplementationTasksHandler(PlanGenerationDeps{
		WorkflowRepo: workflowRepo,
		TaskRepo:     taskRepo,
		SessionRepo:  sessionRepo,
		AgentRepo:    agentRepo,
		FileRegistry: NewFileRegistry(),
	})

	// Plan with 1 epic, 2 stories, 1 task each.
	planJSON := `{
		"epics": [{
			"id": "epic-1",
			"name": "Feature",
			"stories": [
				{
					"id": "story-1-1",
					"name": "Part A",
					"tasks": [{"id": "task-1-1-1", "name": "Task A"}]
				},
				{
					"id": "story-1-2",
					"name": "Part B",
					"tasks": [{"id": "task-1-2-1", "name": "Task B"}]
				}
			]
		}]
	}`

	args := HandlerArgs{
		WorkflowID: "wf-1",
		Output:     planJSON,
	}

	err := handler(context.Background(), args)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	// Each story: 1 coding + 1 verification + 1 confirmation = 3 tasks.
	// 2 stories = 6 tasks.
	if len(taskRepo.bulkInsertTasks) != 6 {
		t.Errorf("Expected 6 tasks, got %d", len(taskRepo.bulkInsertTasks))
	}

	// Verify seq_story values are different.
	story1Tasks := 0
	story2Tasks := 0
	for _, task := range taskRepo.bulkInsertTasks {
		if task.SeqStory == 0 {
			story1Tasks++
		} else if task.SeqStory == 1 {
			story2Tasks++
		}
	}

	if story1Tasks != 3 {
		t.Errorf("Expected 3 tasks in story 1, got %d", story1Tasks)
	}
	if story2Tasks != 3 {
		t.Errorf("Expected 3 tasks in story 2, got %d", story2Tasks)
	}
}

func TestCodingTaskInput_JSON(t *testing.T) {
	t.Parallel()

	input := CodingTaskInput{
		EpicID:      "epic-1",
		EpicName:    "Authentication",
		StoryID:     "story-1-1",
		StoryName:   "Add Login",
		TaskID:      "task-1-1-1",
		TaskName:    "Create form",
		Description: "Create the login form component",
		Files:       []string{"src/login.tsx", "src/login.test.tsx"},
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var parsed CodingTaskInput
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if parsed.TaskID != input.TaskID {
		t.Errorf("TaskID = %q, want %q", parsed.TaskID, input.TaskID)
	}
	if len(parsed.Files) != 2 {
		t.Errorf("Files length = %d, want 2", len(parsed.Files))
	}
}

func TestVerificationTaskInput_JSON(t *testing.T) {
	t.Parallel()

	input := VerificationTaskInput{
		EpicID:    "epic-1",
		EpicName:  "Auth",
		StoryID:   "story-1-1",
		StoryName: "Login",
		Command:   "npm run test -- --testPathPattern=login",
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var parsed VerificationTaskInput
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if parsed.Command != input.Command {
		t.Errorf("Command = %q, want %q", parsed.Command, input.Command)
	}
}

func TestConfirmationTaskInput_JSON(t *testing.T) {
	t.Parallel()

	input := ConfirmationTaskInput{
		EpicID:    "epic-1",
		EpicName:  "Auth",
		StoryID:   "story-1-1",
		StoryName: "Login",
		Prompt:    "Approve story 'Login'?",
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var parsed ConfirmationTaskInput
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if parsed.Prompt != input.Prompt {
		t.Errorf("Prompt = %q, want %q", parsed.Prompt, input.Prompt)
	}
}
