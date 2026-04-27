package workflow

import (
	"testing"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/util/idgen"
)

func TestTaskGenerator_GenerateTasks_Basic(t *testing.T) {
	gen := NewTaskGenerator("", "", "")

	wf := &domain.WorkflowFile{
		Name:    "test-workflow",
		Version: "1.0",
		Epics: []domain.EpicDefinition{
			{
				ID:   "epic-1",
				Name: "Epic 1",
				Stories: []domain.StoryDefinition{
					{ID: "story-1", Name: "Story 1", Type: domain.TaskTypeCoding},
					{ID: "story-2", Name: "Story 2", Type: domain.TaskTypeVerification},
				},
			},
		},
	}

	cfg := GenerateConfig{
		ProjectID:  idgen.MustNew(),
		SessionID:  idgen.MustNew(),
		WorkflowID: idgen.MustNew(),
		AssigneeID: idgen.MustNew(),
	}

	tasks, err := gen.GenerateTasks(wf, cfg)
	if err != nil {
		t.Fatalf("GenerateTasks failed: %v", err)
	}

	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}

	// Verify task properties.
	task1 := tasks[0]
	if task1.SeqEpic != 1 || task1.SeqStory != 1 || task1.SeqTask != 1 {
		t.Errorf("task1 seq mismatch: got (%d,%d,%d)", task1.SeqEpic, task1.SeqStory, task1.SeqTask)
	}
	if task1.Type != domain.TaskTypeCoding {
		t.Errorf("task1 type mismatch: got %d", task1.Type)
	}
	if task1.DependsOn.Valid {
		t.Error("task1 should not have depends_on")
	}

	task2 := tasks[1]
	if task2.SeqEpic != 1 || task2.SeqStory != 2 || task2.SeqTask != 1 {
		t.Errorf("task2 seq mismatch: got (%d,%d,%d)", task2.SeqEpic, task2.SeqStory, task2.SeqTask)
	}
	if task2.Type != domain.TaskTypeVerification {
		t.Errorf("task2 type mismatch: got %d", task2.Type)
	}
}

func TestTaskGenerator_GenerateTasks_WithDependsOn(t *testing.T) {
	gen := NewTaskGenerator("", "", "")

	wf := &domain.WorkflowFile{
		Name:    "test-workflow",
		Version: "1.0",
		Epics: []domain.EpicDefinition{
			{
				ID:   "epic-1",
				Name: "Epic 1",
				Stories: []domain.StoryDefinition{
					{ID: "review", Name: "Review", Type: domain.TaskTypeConfirmation},
					{
						ID:        "execute",
						Name:      "Execute",
						Type:      domain.TaskTypeCoding,
						DependsOn: "review",
					},
				},
			},
		},
	}

	cfg := GenerateConfig{
		ProjectID:  idgen.MustNew(),
		SessionID:  idgen.MustNew(),
		WorkflowID: idgen.MustNew(),
		AssigneeID: idgen.MustNew(),
	}

	tasks, err := gen.GenerateTasks(wf, cfg)
	if err != nil {
		t.Fatalf("GenerateTasks failed: %v", err)
	}

	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}

	reviewTask := tasks[0]
	executeTask := tasks[1]

	// Execute task should depend on review task.
	if !executeTask.DependsOn.Valid {
		t.Error("execute task should have depends_on")
	}
	if executeTask.DependsOn.String != reviewTask.ID {
		t.Errorf("execute depends_on mismatch: got %s, want %s",
			executeTask.DependsOn.String, reviewTask.ID)
	}

	// No specific condition (any terminal state is OK).
	if executeTask.Condition.Valid {
		t.Error("execute task should not have condition (any terminal state)")
	}
}

func TestTaskGenerator_GenerateTasks_WithCondition(t *testing.T) {
	gen := NewTaskGenerator("", "", "")

	successCondition := domain.TaskStatusSuccess
	wf := &domain.WorkflowFile{
		Name:    "test-workflow",
		Version: "1.0",
		Epics: []domain.EpicDefinition{
			{
				ID:   "epic-1",
				Name: "Epic 1",
				Stories: []domain.StoryDefinition{
					{ID: "review", Name: "Review", Type: domain.TaskTypeConfirmation},
					{
						ID:        "execute",
						Name:      "Execute",
						Type:      domain.TaskTypeCoding,
						DependsOn: "review",
						Condition: &successCondition,
					},
				},
			},
		},
	}

	cfg := GenerateConfig{
		ProjectID:  idgen.MustNew(),
		SessionID:  idgen.MustNew(),
		WorkflowID: idgen.MustNew(),
		AssigneeID: idgen.MustNew(),
	}

	tasks, err := gen.GenerateTasks(wf, cfg)
	if err != nil {
		t.Fatalf("GenerateTasks failed: %v", err)
	}

	executeTask := tasks[1]

	// Execute task should have condition=Success.
	if !executeTask.Condition.Valid {
		t.Error("execute task should have condition")
	}
	if domain.TaskStatus(executeTask.Condition.Int64) != domain.TaskStatusSuccess {
		t.Errorf("execute condition mismatch: got %d, want %d",
			executeTask.Condition.Int64, domain.TaskStatusSuccess)
	}
}

func TestTaskGenerator_GenerateRoutedTasks_WithRoutes(t *testing.T) {
	gen := NewTaskGenerator("", "", "")

	wf := &domain.WorkflowFile{
		Name:    "test-workflow",
		Version: "1.0",
		Epics: []domain.EpicDefinition{
			{
				ID:   "epic-1",
				Name: "Epic 1",
				Stories: []domain.StoryDefinition{
					{
						ID:   "review",
						Name: "Review",
						Type: domain.TaskTypeConfirmation,
						Routes: map[domain.TaskStatus]string{
							domain.TaskStatusSuccess: "execute",
							domain.TaskStatusFailure: "clarify",
						},
					},
					{ID: "execute", Name: "Execute", Type: domain.TaskTypeCoding},
					{ID: "clarify", Name: "Clarify", Type: domain.TaskTypeCoding},
				},
			},
		},
	}

	cfg := GenerateConfig{
		ProjectID:  idgen.MustNew(),
		SessionID:  idgen.MustNew(),
		WorkflowID: idgen.MustNew(),
		AssigneeID: idgen.MustNew(),
	}

	tasks, err := gen.GenerateRoutedTasks(wf, cfg)
	if err != nil {
		t.Fatalf("GenerateRoutedTasks failed: %v", err)
	}

	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}

	// Find tasks by story ID.
	taskMap := make(map[string]*GeneratedTask)
	for _, task := range tasks {
		taskMap[task.StoryID] = task
	}

	reviewTask := taskMap["review"]
	executeTask := taskMap["execute"]
	clarifyTask := taskMap["clarify"]

	// Review task should have no dependencies.
	if reviewTask.DependsOn.Valid {
		t.Error("review task should not have depends_on")
	}

	// Execute task should depend on review with condition=Success.
	if !executeTask.DependsOn.Valid {
		t.Error("execute task should have depends_on")
	}
	if executeTask.DependsOn.String != reviewTask.ID {
		t.Errorf("execute depends_on mismatch: got %s, want %s",
			executeTask.DependsOn.String, reviewTask.ID)
	}
	if !executeTask.Condition.Valid || domain.TaskStatus(executeTask.Condition.Int64) != domain.TaskStatusSuccess {
		t.Errorf("execute condition mismatch: got %d, want %d",
			executeTask.Condition.Int64, domain.TaskStatusSuccess)
	}

	// Clarify task should depend on review with condition=Failure.
	if !clarifyTask.DependsOn.Valid {
		t.Error("clarify task should have depends_on")
	}
	if clarifyTask.DependsOn.String != reviewTask.ID {
		t.Errorf("clarify depends_on mismatch: got %s, want %s",
			clarifyTask.DependsOn.String, reviewTask.ID)
	}
	if !clarifyTask.Condition.Valid || domain.TaskStatus(clarifyTask.Condition.Int64) != domain.TaskStatusFailure {
		t.Errorf("clarify condition mismatch: got %d, want %d",
			clarifyTask.Condition.Int64, domain.TaskStatusFailure)
	}
}

func TestTaskGenerator_GenerateRoutedTasks_NullRoute(t *testing.T) {
	gen := NewTaskGenerator("", "", "")

	wf := &domain.WorkflowFile{
		Name:    "test-workflow",
		Version: "1.0",
		Epics: []domain.EpicDefinition{
			{
				ID:   "epic-1",
				Name: "Epic 1",
				Stories: []domain.StoryDefinition{
					{
						ID:   "review",
						Name: "Review",
						Type: domain.TaskTypeConfirmation,
						Routes: map[domain.TaskStatus]string{
							domain.TaskStatusSuccess:   "execute",
							domain.TaskStatusCancelled: "", // null route - workflow ends
						},
					},
					{ID: "execute", Name: "Execute", Type: domain.TaskTypeCoding},
				},
			},
		},
	}

	cfg := GenerateConfig{
		ProjectID:  idgen.MustNew(),
		SessionID:  idgen.MustNew(),
		WorkflowID: idgen.MustNew(),
		AssigneeID: idgen.MustNew(),
	}

	tasks, err := gen.GenerateRoutedTasks(wf, cfg)
	if err != nil {
		t.Fatalf("GenerateRoutedTasks failed: %v", err)
	}

	// Should still have 2 tasks (null route doesn't create a task).
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}

	// Find tasks by story ID.
	taskMap := make(map[string]*GeneratedTask)
	for _, task := range tasks {
		taskMap[task.StoryID] = task
	}

	executeTask := taskMap["execute"]

	// Execute task should depend on review with condition=Success only.
	if !executeTask.DependsOn.Valid {
		t.Error("execute task should have depends_on")
	}
	if !executeTask.Condition.Valid || domain.TaskStatus(executeTask.Condition.Int64) != domain.TaskStatusSuccess {
		t.Errorf("execute condition mismatch: got %d, want %d",
			executeTask.Condition.Int64, domain.TaskStatusSuccess)
	}
}

func TestTaskGenerator_GenerateTasks_MultipleEpics(t *testing.T) {
	gen := NewTaskGenerator("", "", "")

	wf := &domain.WorkflowFile{
		Name:    "test-workflow",
		Version: "1.0",
		Epics: []domain.EpicDefinition{
			{
				ID:   "epic-1",
				Name: "Epic 1",
				Stories: []domain.StoryDefinition{
					{ID: "story-1-1", Name: "Story 1.1", Type: domain.TaskTypeCoding},
					{ID: "story-1-2", Name: "Story 1.2", Type: domain.TaskTypeCoding},
				},
			},
			{
				ID:   "epic-2",
				Name: "Epic 2",
				Stories: []domain.StoryDefinition{
					{ID: "story-2-1", Name: "Story 2.1", Type: domain.TaskTypeVerification},
				},
			},
		},
	}

	cfg := GenerateConfig{
		ProjectID:  idgen.MustNew(),
		SessionID:  idgen.MustNew(),
		WorkflowID: idgen.MustNew(),
		AssigneeID: idgen.MustNew(),
	}

	tasks, err := gen.GenerateTasks(wf, cfg)
	if err != nil {
		t.Fatalf("GenerateTasks failed: %v", err)
	}

	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}

	// Verify epic/story sequence.
	expectedSeqs := []struct {
		epic, story int
	}{
		{1, 1}, // story-1-1
		{1, 2}, // story-1-2
		{2, 1}, // story-2-1
	}

	for i, expected := range expectedSeqs {
		task := tasks[i]
		if task.SeqEpic != expected.epic || task.SeqStory != expected.story {
			t.Errorf("task[%d] seq mismatch: got (%d,%d), want (%d,%d)",
				i, task.SeqEpic, task.SeqStory, expected.epic, expected.story)
		}
	}
}

func TestTaskGenerator_GenerateTasksWithGuardrails_VerificationInjection(t *testing.T) {
	humanID := idgen.MustNew()
	assistantID := idgen.MustNew()
	gen := NewTaskGenerator(humanID, assistantID, "")

	wf := &domain.WorkflowFile{
		Name:     "test-workflow",
		Version:  "1.0",
		Settings: domain.DefaultWorkflowSettings(), // Verification=true by default
		Epics: []domain.EpicDefinition{
			{
				ID:   "epic-1",
				Name: "Epic 1",
				Stories: []domain.StoryDefinition{
					{ID: "coding-story", Name: "Coding Story", Type: domain.TaskTypeCoding},
				},
			},
		},
	}

	cfg := GenerateConfig{
		ProjectID:  idgen.MustNew(),
		SessionID:  idgen.MustNew(),
		WorkflowID: idgen.MustNew(),
	}

	tasks, err := gen.GenerateTasksWithGuardrails(wf, cfg)
	if err != nil {
		t.Fatalf("GenerateTasksWithGuardrails failed: %v", err)
	}

	// With default guardrails (all enabled):
	// 1 Coding + 1 Verification + 1 Confirmation + 1 Retrospective = 4 tasks
	if len(tasks) != 4 {
		t.Fatalf("expected 4 tasks, got %d", len(tasks))
	}

	// Task 1: Coding (assigned to assistant)
	if tasks[0].Type != domain.TaskTypeCoding {
		t.Errorf("task[0] type mismatch: got %d, want %d", tasks[0].Type, domain.TaskTypeCoding)
	}
	if tasks[0].AssigneeID != assistantID {
		t.Errorf("task[0] assignee mismatch: got %s, want %s", tasks[0].AssigneeID, assistantID)
	}

	// Task 2: Verification (assigned to assistant, depends on Coding with Success condition)
	if tasks[1].Type != domain.TaskTypeVerification {
		t.Errorf("task[1] type mismatch: got %d, want %d", tasks[1].Type, domain.TaskTypeVerification)
	}
	if tasks[1].AssigneeID != assistantID {
		t.Errorf("task[1] assignee mismatch: got %s, want %s", tasks[1].AssigneeID, assistantID)
	}
	if !tasks[1].DependsOn.Valid || tasks[1].DependsOn.String != tasks[0].ID {
		t.Error("verification task should depend on coding task")
	}
	if !tasks[1].Condition.Valid || domain.TaskStatus(tasks[1].Condition.Int64) != domain.TaskStatusSuccess {
		t.Error("verification task should have Success condition")
	}

	// Task 3: Confirmation (assigned to human, depends on Verification)
	if tasks[2].Type != domain.TaskTypeConfirmation {
		t.Errorf("task[2] type mismatch: got %d, want %d", tasks[2].Type, domain.TaskTypeConfirmation)
	}
	if tasks[2].AssigneeID != humanID {
		t.Errorf("task[2] assignee mismatch: got %s, want %s", tasks[2].AssigneeID, humanID)
	}

	// Task 4: Retrospective (assigned to human, depends on last task of epic)
	if tasks[3].Type != domain.TaskTypeRetrospective {
		t.Errorf("task[3] type mismatch: got %d, want %d", tasks[3].Type, domain.TaskTypeRetrospective)
	}
	if tasks[3].AssigneeID != humanID {
		t.Errorf("task[3] assignee mismatch: got %s, want %s", tasks[3].AssigneeID, humanID)
	}
}

func TestTaskGenerator_GenerateTasksWithGuardrails_DisabledGuardrails(t *testing.T) {
	gen := NewTaskGenerator("", "", "")

	wf := &domain.WorkflowFile{
		Name:    "test-workflow",
		Version: "1.0",
		Settings: domain.WorkflowSettings{
			DefaultTimeout: domain.DefaultTaskTimeout,
			Guardrails: domain.GuardrailSettings{
				Verification:  false,
				Confirmation:  false,
				Retrospective: false,
			},
		},
		Epics: []domain.EpicDefinition{
			{
				ID:   "epic-1",
				Name: "Epic 1",
				Stories: []domain.StoryDefinition{
					{ID: "coding-story", Name: "Coding Story", Type: domain.TaskTypeCoding},
				},
			},
		},
	}

	cfg := GenerateConfig{
		ProjectID:  idgen.MustNew(),
		SessionID:  idgen.MustNew(),
		WorkflowID: idgen.MustNew(),
		AssigneeID: idgen.MustNew(),
	}

	tasks, err := gen.GenerateTasksWithGuardrails(wf, cfg)
	if err != nil {
		t.Fatalf("GenerateTasksWithGuardrails failed: %v", err)
	}

	// With all guardrails disabled: just 1 Coding task
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}

	if tasks[0].Type != domain.TaskTypeCoding {
		t.Errorf("task[0] type mismatch: got %d, want %d", tasks[0].Type, domain.TaskTypeCoding)
	}
}

func TestTaskGenerator_GenerateTasksWithGuardrails_StoryOverride(t *testing.T) {
	gen := NewTaskGenerator("", "", "")

	// Story-level guardrails override workflow-level
	storyGuardrails := domain.GuardrailSettings{
		Verification:  false,
		Confirmation:  false,
		Retrospective: false,
	}

	wf := &domain.WorkflowFile{
		Name:     "test-workflow",
		Version:  "1.0",
		Settings: domain.DefaultWorkflowSettings(), // All guardrails enabled
		Epics: []domain.EpicDefinition{
			{
				ID:   "epic-1",
				Name: "Epic 1",
				Stories: []domain.StoryDefinition{
					{
						ID:         "coding-story",
						Name:       "Coding Story",
						Type:       domain.TaskTypeCoding,
						Guardrails: &storyGuardrails, // Override: no verification or confirmation
					},
				},
			},
		},
	}

	cfg := GenerateConfig{
		ProjectID:  idgen.MustNew(),
		SessionID:  idgen.MustNew(),
		WorkflowID: idgen.MustNew(),
		AssigneeID: idgen.MustNew(),
	}

	tasks, err := gen.GenerateTasksWithGuardrails(wf, cfg)
	if err != nil {
		t.Fatalf("GenerateTasksWithGuardrails failed: %v", err)
	}

	// Story override disables verification/confirmation, but epic still gets retrospective
	// 1 Coding + 1 Retrospective (workflow-level) = 2 tasks
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}

	if tasks[0].Type != domain.TaskTypeCoding {
		t.Errorf("task[0] type mismatch: got %d, want %d", tasks[0].Type, domain.TaskTypeCoding)
	}
	if tasks[1].Type != domain.TaskTypeRetrospective {
		t.Errorf("task[1] type mismatch: got %d, want %d", tasks[1].Type, domain.TaskTypeRetrospective)
	}
}

func TestTaskGenerator_GenerateTasksWithGuardrails_EpicOverrideRetrospective(t *testing.T) {
	gen := NewTaskGenerator("", "", "")

	// Epic-level retrospective override
	falseVal := false

	wf := &domain.WorkflowFile{
		Name:     "test-workflow",
		Version:  "1.0",
		Settings: domain.DefaultWorkflowSettings(), // Retrospective=true
		Epics: []domain.EpicDefinition{
			{
				ID:            "epic-1",
				Name:          "Epic 1",
				Retrospective: &falseVal, // Override: no retrospective for this epic
				Stories: []domain.StoryDefinition{
					{ID: "coding-story", Name: "Coding Story", Type: domain.TaskTypeCoding},
				},
			},
		},
	}

	cfg := GenerateConfig{
		ProjectID:  idgen.MustNew(),
		SessionID:  idgen.MustNew(),
		WorkflowID: idgen.MustNew(),
		AssigneeID: idgen.MustNew(),
	}

	tasks, err := gen.GenerateTasksWithGuardrails(wf, cfg)
	if err != nil {
		t.Fatalf("GenerateTasksWithGuardrails failed: %v", err)
	}

	// 1 Coding + 1 Verification + 1 Confirmation = 3 tasks (no retrospective due to epic override)
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}

	// Verify no retrospective task
	for _, task := range tasks {
		if task.Type == domain.TaskTypeRetrospective {
			t.Error("should not have retrospective task due to epic override")
		}
	}
}

func TestTaskGenerator_GenerateTasksWithGuardrails_NoVerificationForNonCodingTasks(t *testing.T) {
	gen := NewTaskGenerator("", "", "")

	wf := &domain.WorkflowFile{
		Name:     "test-workflow",
		Version:  "1.0",
		Settings: domain.DefaultWorkflowSettings(),
		Epics: []domain.EpicDefinition{
			{
				ID:   "epic-1",
				Name: "Epic 1",
				Stories: []domain.StoryDefinition{
					// Confirmation task type should NOT get Verification injected
					{ID: "confirm-story", Name: "Confirm Story", Type: domain.TaskTypeConfirmation},
				},
			},
		},
	}

	cfg := GenerateConfig{
		ProjectID:  idgen.MustNew(),
		SessionID:  idgen.MustNew(),
		WorkflowID: idgen.MustNew(),
		AssigneeID: idgen.MustNew(),
	}

	tasks, err := gen.GenerateTasksWithGuardrails(wf, cfg)
	if err != nil {
		t.Fatalf("GenerateTasksWithGuardrails failed: %v", err)
	}

	// 1 Confirmation (main) + 1 Confirmation (guardrail) + 1 Retrospective = 3 tasks
	// No Verification because main task is not Coding
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}

	// Count verification tasks
	verificationCount := 0
	for _, task := range tasks {
		if task.Type == domain.TaskTypeVerification {
			verificationCount++
		}
	}
	if verificationCount != 0 {
		t.Errorf("expected 0 verification tasks for non-coding story, got %d", verificationCount)
	}
}
