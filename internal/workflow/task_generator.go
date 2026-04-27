// Package workflow provides the WorkflowRegistry for managing workflow
// definitions loaded from configuration.
package workflow

import (
	"database/sql"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/util/idgen"
)

// TaskGenerator generates domain.Task entities from a WorkflowFile.
// It translates story routes into depends_on and condition fields
// for conditional execution (Story 2.0.2).
type TaskGenerator struct{}

// NewTaskGenerator creates a new TaskGenerator.
func NewTaskGenerator() *TaskGenerator {
	return &TaskGenerator{}
}

// GenerateConfig holds configuration for task generation.
type GenerateConfig struct {
	ProjectID  string
	SessionID  string
	WorkflowID string
	AssigneeID string
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
			task := g.createTask(cfg, epicIdx+1, storyIdx+1, 1, story)
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

// createTask creates a single domain.Task from a StoryDefinition.
func (g *TaskGenerator) createTask(cfg GenerateConfig, seqEpic, seqStory, seqTask int, story domain.StoryDefinition) *domain.Task {
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

	return task
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
			task := g.createTask(cfg, epicIdx+1, storyIdx+1, 1, story)
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
