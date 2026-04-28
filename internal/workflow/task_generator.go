// Package workflow provides the WorkflowRegistry for managing workflow
// definitions loaded from configuration.
package workflow

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/skills"
	"codemint.kanthorlabs.com/internal/util/idgen"
)

// TaskGenerator generates domain.Task entities from a WorkflowFile.
// It translates story routes into depends_on and condition fields
// for conditional execution (Story 2.0.2).
//
// Guardrail auto-injection (Story 2.0.4):
//   - Verification tasks are auto-injected after Coding tasks
//   - Confirmation tasks are auto-injected at the end of each story
//   - Retrospective tasks are auto-injected at the end of each epic
type TaskGenerator struct {
	// humanAgentID is the ID of the human agent for Confirmation/Retrospective tasks.
	humanAgentID string
	// assistantAgentID is the ID of the assistant agent for Coding/Verification tasks.
	assistantAgentID string
	// yoloAgentID is the ID of the sys-auto-approve agent for YOLO mode tasks.
	yoloAgentID string
	// skills is the skill resolver for L2 validation and skill ID lookup.
	skills skills.SkillResolver
}

// NewTaskGenerator creates a new TaskGenerator with agent IDs for task assignment.
// The agent IDs are used to assign appropriate agents based on task type:
//   - humanAgentID: for Confirmation and Retrospective tasks (require human approval)
//   - assistantAgentID: for Coding and Verification tasks (AI-executed)
//   - yoloAgentID: for auto-approved tasks in YOLO mode
//   - skillResolver: for L2 skill validation (may be nil to skip validation)
func NewTaskGenerator(humanAgentID, assistantAgentID, yoloAgentID string, skillResolver skills.SkillResolver) *TaskGenerator {
	return &TaskGenerator{
		humanAgentID:     humanAgentID,
		assistantAgentID: assistantAgentID,
		yoloAgentID:      yoloAgentID,
		skills:           skillResolver,
	}
}

// GenerateConfig holds configuration for task generation.
type GenerateConfig struct {
	ProjectID  string
	SessionID  string
	WorkflowID string
	AssigneeID string // Default assignee if agent IDs not set on generator
}

// GeneratedTask wraps a domain.Task with additional metadata for routing.
type GeneratedTask struct {
	*domain.Task
	StoryID string // Original story ID for route resolution
}

// GenerateTasks creates domain.Task entities from a WorkflowFile.
// It processes epics and stories in order, setting up depends_on and condition
// based on story routes for conditional execution.
//
// Route handling:
//   - If a story has routes, successor tasks will have depends_on pointing to
//     the routed story's task, with condition set to the route's status.
//   - If a story has depends_on without routes, the task will wait for any
//     terminal status of the predecessor.
func (g *TaskGenerator) GenerateTasks(wf *domain.WorkflowFile, cfg GenerateConfig) ([]*domain.Task, error) {
	var tasks []*domain.Task

	// Map story ID to generated task for route resolution.
	storyTaskMap := make(map[string]*domain.Task)

	// Process epics in order.
	for epicIdx, epic := range wf.Epics {
		// Process stories in order within each epic.
		for storyIdx, story := range epic.Stories {
			task, err := g.createTask(cfg, epicIdx+1, storyIdx+1, 1, story)
			if err != nil {
				return nil, err
			}
			tasks = append(tasks, task)
			storyTaskMap[story.ID] = task
		}
	}

	// Second pass: resolve depends_on and condition from routes.
	for _, epic := range wf.Epics {
		for _, story := range epic.Stories {
			task := storyTaskMap[story.ID]
			if task == nil {
				continue
			}

			// Handle explicit depends_on (story-level dependency).
			if story.DependsOn != "" {
				if predTask, ok := storyTaskMap[story.DependsOn]; ok {
					task.DependsOn = sql.NullString{String: predTask.ID, Valid: true}

					// Handle condition if specified.
					if story.Condition != nil {
						task.Condition = sql.NullInt64{Int64: int64(*story.Condition), Valid: true}
					}
				}
			}
		}
	}

	return tasks, nil
}

// GenerateTasksWithGuardrails creates domain.Task entities from a WorkflowFile
// with automatic guardrail injection based on workflow/epic/story settings.
//
// Guardrail injection rules (Story 2.0.4):
//   - Verification: Injected after Coding tasks if guardrails.verification is true
//   - Confirmation: Injected at the end of each story if guardrails.confirmation is true
//   - Retrospective: Injected at the end of each epic if guardrails.retrospective is true
//
// Task assignment:
//   - Coding/Verification tasks: assigned to assistantAgentID
//   - Confirmation/Retrospective tasks: assigned to humanAgentID
func (g *TaskGenerator) GenerateTasksWithGuardrails(wf *domain.WorkflowFile, cfg GenerateConfig) ([]*domain.Task, error) {
	var tasks []*domain.Task

	// Map story ID to the last generated task for that story (for dependency resolution).
	storyLastTaskMap := make(map[string]*domain.Task)
	// Map story ID to the main story task for route resolution.
	storyTaskMap := make(map[string]*domain.Task)

	// Process epics in order.
	for epicIdx, epic := range wf.Epics {
		var lastEpicTask *domain.Task

		// Process stories in order within each epic.
		for storyIdx, story := range epic.Stories {
			seqTask := 1

			// Resolve guardrails for this story.
			guardrails := g.resolveGuardrails(wf, &epic, &story)

			// Create the main story task.
			mainTask := g.createTaskWithAssignee(cfg, epicIdx+1, storyIdx+1, seqTask, story.Type)
			tasks = append(tasks, mainTask)
			storyTaskMap[story.ID] = mainTask
			lastTask := mainTask
			seqTask++

			// Auto-inject Verification after Coding tasks.
			if guardrails.Verification && story.Type == domain.TaskTypeCoding {
				verifyTask := g.createVerificationTask(cfg, epicIdx+1, storyIdx+1, seqTask, lastTask)
				tasks = append(tasks, verifyTask)
				lastTask = verifyTask
				seqTask++
			}

			// Auto-inject Confirmation at end of story.
			if guardrails.Confirmation {
				confirmTask := g.createConfirmationTask(cfg, epicIdx+1, storyIdx+1, seqTask, lastTask)
				tasks = append(tasks, confirmTask)
				lastTask = confirmTask
				seqTask++
			}

			storyLastTaskMap[story.ID] = lastTask
			lastEpicTask = lastTask
		}

		// Auto-inject Retrospective at end of epic.
		retrospective := g.resolveRetrospective(wf, &epic)
		if retrospective && lastEpicTask != nil {
			// Retrospective is story 0 (after all stories in the epic).
			retroTask := g.createRetrospectiveTask(cfg, epicIdx+1, lastEpicTask)
			tasks = append(tasks, retroTask)
		}
	}

	// Second pass: resolve depends_on and condition from routes using main story tasks.
	for _, epic := range wf.Epics {
		for _, story := range epic.Stories {
			mainTask := storyTaskMap[story.ID]
			if mainTask == nil {
				continue
			}

			// Handle explicit depends_on (story-level dependency).
			// The main task depends on the LAST task of the predecessor story.
			if story.DependsOn != "" {
				if predLastTask, ok := storyLastTaskMap[story.DependsOn]; ok {
					mainTask.DependsOn = sql.NullString{String: predLastTask.ID, Valid: true}

					// Handle condition if specified.
					if story.Condition != nil {
						mainTask.Condition = sql.NullInt64{Int64: int64(*story.Condition), Valid: true}
					}
				}
			}

			// Handle routes: set depends_on and condition on target story's main task.
			for status, targetStoryID := range story.Routes {
				if targetStoryID == "" {
					continue
				}
				targetTask, ok := storyTaskMap[targetStoryID]
				if !ok {
					continue
				}
				// Target depends on the LAST task of this story.
				if predLastTask, ok := storyLastTaskMap[story.ID]; ok {
					targetTask.DependsOn = sql.NullString{String: predLastTask.ID, Valid: true}
					targetTask.Condition = sql.NullInt64{Int64: int64(status), Valid: true}
				}
			}
		}
	}

	return tasks, nil
}

// resolveGuardrails determines the effective guardrail settings for a story.
// Priority: story > epic > workflow settings.
func (g *TaskGenerator) resolveGuardrails(wf *domain.WorkflowFile, epic *domain.EpicDefinition, story *domain.StoryDefinition) domain.GuardrailSettings {
	// Start with workflow-level defaults.
	result := wf.Settings.Guardrails

	// Story-level overrides take precedence.
	if story.Guardrails != nil {
		return *story.Guardrails
	}

	return result
}

// resolveRetrospective determines if a retrospective should be injected for an epic.
func (g *TaskGenerator) resolveRetrospective(wf *domain.WorkflowFile, epic *domain.EpicDefinition) bool {
	// Epic-level override takes precedence.
	if epic.Retrospective != nil {
		return *epic.Retrospective
	}
	// Fall back to workflow-level setting.
	return wf.Settings.Guardrails.Retrospective
}

// createTaskWithAssignee creates a task with appropriate agent assignment based on type.
func (g *TaskGenerator) createTaskWithAssignee(cfg GenerateConfig, seqEpic, seqStory, seqTask int, taskType domain.TaskType) *domain.Task {
	assignee := cfg.AssigneeID
	if g.assistantAgentID != "" {
		// Coding and Verification tasks go to the assistant.
		if taskType == domain.TaskTypeCoding || taskType == domain.TaskTypeVerification {
			assignee = g.assistantAgentID
		}
	}
	if g.humanAgentID != "" {
		// Confirmation and Retrospective tasks go to human.
		if taskType == domain.TaskTypeConfirmation || taskType == domain.TaskTypeRetrospective {
			assignee = g.humanAgentID
		}
	}

	return &domain.Task{
		ID:         idgen.MustNew(),
		ProjectID:  cfg.ProjectID,
		SessionID:  cfg.SessionID,
		WorkflowID: sql.NullString{String: cfg.WorkflowID, Valid: cfg.WorkflowID != ""},
		AssigneeID: assignee,
		SeqEpic:    seqEpic,
		SeqStory:   seqStory,
		SeqTask:    seqTask,
		Type:       taskType,
		Status:     domain.TaskStatusPending,
		Timeout:    domain.DefaultTaskTimeout,
	}
}

// createVerificationTask creates a Verification task that depends on the parent task.
func (g *TaskGenerator) createVerificationTask(cfg GenerateConfig, seqEpic, seqStory, seqTask int, parent *domain.Task) *domain.Task {
	task := g.createTaskWithAssignee(cfg, seqEpic, seqStory, seqTask, domain.TaskTypeVerification)
	// Verification depends on the parent (Coding) task succeeding.
	task.DependsOn = sql.NullString{String: parent.ID, Valid: true}
	task.Condition = sql.NullInt64{Int64: int64(domain.TaskStatusSuccess), Valid: true}
	return task
}

// createConfirmationTask creates a Confirmation task that depends on the parent task.
func (g *TaskGenerator) createConfirmationTask(cfg GenerateConfig, seqEpic, seqStory, seqTask int, parent *domain.Task) *domain.Task {
	task := g.createTaskWithAssignee(cfg, seqEpic, seqStory, seqTask, domain.TaskTypeConfirmation)
	// Confirmation depends on the parent task completing (any terminal status).
	task.DependsOn = sql.NullString{String: parent.ID, Valid: true}
	return task
}

// createRetrospectiveTask creates a Retrospective task at the end of an epic.
func (g *TaskGenerator) createRetrospectiveTask(cfg GenerateConfig, seqEpic int, lastEpicTask *domain.Task) *domain.Task {
	// Retrospective is placed at story position 0 with seqTask=0 to indicate it's an epic-level task.
	task := g.createTaskWithAssignee(cfg, seqEpic, 0, 0, domain.TaskTypeRetrospective)
	// Retrospective depends on the last task of the epic completing.
	task.DependsOn = sql.NullString{String: lastEpicTask.ID, Valid: true}
	return task
}

// createTask creates a single domain.Task from a StoryDefinition.
// It validates and copies the skill ID into TaskInput if present.
func (g *TaskGenerator) createTask(cfg GenerateConfig, seqEpic, seqStory, seqTask int, story domain.StoryDefinition) (*domain.Task, error) {
	// L2 skill validation: re-check that the skill still exists in the registry.
	if story.Skill != "" && g.skills != nil {
		if _, ok := g.skills.Get(story.Skill); !ok {
			return nil, fmt.Errorf("task generator: skill %q no longer in registry", story.Skill)
		}
	}

	task := &domain.Task{
		ID:         idgen.MustNew(),
		ProjectID:  cfg.ProjectID,
		SessionID:  cfg.SessionID,
		WorkflowID: sql.NullString{String: cfg.WorkflowID, Valid: cfg.WorkflowID != ""},
		AssigneeID: cfg.AssigneeID,
		SeqEpic:    seqEpic,
		SeqStory:   seqStory,
		SeqTask:    seqTask,
		Type:       story.Type,
		Status:     domain.TaskStatusPending,
		Timeout:    domain.DefaultTaskTimeout,
	}

	// Build TaskInput with skill ID if present.
	if story.Skill != "" {
		taskInput := domain.TaskInput{
			Skill: story.Skill,
		}
		inputJSON, err := json.Marshal(taskInput)
		if err != nil {
			return nil, fmt.Errorf("task generator: marshal task input: %w", err)
		}
		task.Input = sql.NullString{String: string(inputJSON), Valid: true}
	}

	return task, nil
}

// GenerateRoutedTasks creates tasks with route-based conditional execution.
// For stories with routes, it creates multiple successor tasks that will be
// activated based on the predecessor's outcome.
//
// Example route handling:
//
//	Story "review" with routes: {Success: "execute", Failure: "clarify"}
//	- Task "execute" gets: depends_on="review_task_id", condition=Success(3)
//	- Task "clarify" gets: depends_on="review_task_id", condition=Failure(4)
func (g *TaskGenerator) GenerateRoutedTasks(wf *domain.WorkflowFile, cfg GenerateConfig) ([]*GeneratedTask, error) {
	var tasks []*GeneratedTask

	// Map story ID to generated task.
	storyTaskMap := make(map[string]*GeneratedTask)

	// First pass: create tasks for all stories.
	for epicIdx, epic := range wf.Epics {
		for storyIdx, story := range epic.Stories {
			task, err := g.createTask(cfg, epicIdx+1, storyIdx+1, 1, story)
			if err != nil {
				return nil, err
			}
			genTask := &GeneratedTask{
				Task:    task,
				StoryID: story.ID,
			}
			tasks = append(tasks, genTask)
			storyTaskMap[story.ID] = genTask
		}
	}

	// Second pass: resolve routes into depends_on/condition.
	for _, epic := range wf.Epics {
		for _, story := range epic.Stories {
			// Skip stories without routes.
			if len(story.Routes) == 0 && story.DependsOn == "" {
				continue
			}

			// Handle explicit depends_on.
			if story.DependsOn != "" {
				genTask := storyTaskMap[story.ID]
				if genTask == nil {
					continue
				}

				if predTask, ok := storyTaskMap[story.DependsOn]; ok {
					genTask.DependsOn = sql.NullString{String: predTask.ID, Valid: true}

					if story.Condition != nil {
						genTask.Condition = sql.NullInt64{Int64: int64(*story.Condition), Valid: true}
					}
				}
			}

			// Handle routes: for each route, set depends_on and condition on the target story's task.
			predTask := storyTaskMap[story.ID]
			if predTask == nil {
				continue
			}

			for status, targetStoryID := range story.Routes {
				if targetStoryID == "" {
					// Empty target means workflow ends (e.g., Cancelled → null).
					continue
				}

				targetGenTask, ok := storyTaskMap[targetStoryID]
				if !ok {
					continue
				}

				// Set the dependency and condition on the target task.
				targetGenTask.DependsOn = sql.NullString{String: predTask.ID, Valid: true}
				targetGenTask.Condition = sql.NullInt64{Int64: int64(status), Valid: true}
			}
		}
	}

	return tasks, nil
}
