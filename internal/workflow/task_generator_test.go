package workflow

import (
	"testing"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/util/idgen"
)

func TestTaskGenerator_GenerateTasks_Basic(t *testing.T) {
	gen := NewTaskGenerator()

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
	gen := NewTaskGenerator()

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
	gen := NewTaskGenerator()

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
	gen := NewTaskGenerator()

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
	gen := NewTaskGenerator()

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
	gen := NewTaskGenerator()

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
