package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/repository"
	"codemint.kanthorlabs.com/internal/util/idgen"
)

// planEpic represents an epic in the task-generator skill output.
type planEpic struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Stories     []planStory `json:"stories"`
}

// planStory represents a story in the task-generator skill output.
type planStory struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Description  string     `json:"description,omitempty"`
	Verification string     `json:"verification,omitempty"`
	Tasks        []planTask `json:"tasks"`
}

// planTask represents a task in the task-generator skill output.
type planTask struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Files       []string `json:"files,omitempty"`
}

// planOutput represents the full output from the task-generator skill.
type planOutput struct {
	Error string      `json:"error,omitempty"`
	Epics []planEpic  `json:"epics,omitempty"`
}

// PlanGenerationDeps holds dependencies for the plan generation handler.
type PlanGenerationDeps struct {
	WorkflowRepo repository.WorkflowRepository
	TaskRepo     repository.TaskRepository
	SessionRepo  repository.SessionRepository
	AgentRepo    repository.AgentRepository
	FileRegistry *FileRegistry
}

// CreateImplementationTasksHandler returns a HandlerFunc that parses the task-generator
// skill output and inserts implementation tasks into the database.
//
// The handler:
// 1. Validates the skill output JSON structure
// 2. Checks for skill-side error response
// 3. Creates Coding tasks for each task in the plan
// 4. Auto-injects Verification tasks after each story's coding tasks (if guardrails.verification is true)
// 5. Auto-injects Confirmation tasks after verification (if guardrails.confirmation is true)
// 6. Inserts all tasks in a single transaction
//
// Task assignment:
//   - Coding/Verification tasks: assigned to assistant agent (from session)
//   - Confirmation tasks: assigned to human agent
//
// Sequencing:
//   - Tasks are ordered by (seq_epic, seq_story, seq_task)
//   - Verification depends_on the last coding task of its story
//   - Confirmation depends_on verification (or last coding task if verification is disabled)
func CreateImplementationTasksHandler(deps PlanGenerationDeps) HandlerFunc {
	return func(ctx context.Context, args HandlerArgs) error {
		// Validate dependencies.
		if deps.WorkflowRepo == nil {
			return fmt.Errorf("create_implementation_tasks: workflow repository is nil")
		}
		if deps.TaskRepo == nil {
			return fmt.Errorf("create_implementation_tasks: task repository is nil")
		}
		if deps.SessionRepo == nil {
			return fmt.Errorf("create_implementation_tasks: session repository is nil")
		}
		if deps.AgentRepo == nil {
			return fmt.Errorf("create_implementation_tasks: agent repository is nil")
		}
		if deps.FileRegistry == nil {
			return fmt.Errorf("create_implementation_tasks: file registry is nil")
		}

		if args.WorkflowID == "" {
			return fmt.Errorf("create_implementation_tasks: workflow ID is required")
		}

		if args.Output == "" {
			return fmt.Errorf("create_implementation_tasks: output is empty")
		}

		// Parse the skill output.
		var plan planOutput
		if err := json.Unmarshal([]byte(args.Output), &plan); err != nil {
			return fmt.Errorf("create_implementation_tasks: invalid JSON: %w", err)
		}

		// Check for skill-side error.
		if plan.Error != "" {
			return fmt.Errorf("create_implementation_tasks: skill aborted: %s", plan.Error)
		}

		// Validate we have epics.
		if len(plan.Epics) == 0 {
			return fmt.Errorf("create_implementation_tasks: no epics in plan")
		}

		// Get the workflow to find session and settings.
		wf, err := deps.WorkflowRepo.FindByID(ctx, args.WorkflowID)
		if err != nil {
			return fmt.Errorf("create_implementation_tasks: find workflow: %w", err)
		}
		if wf == nil {
			return fmt.Errorf("create_implementation_tasks: workflow %q not found", args.WorkflowID)
		}

		// Get the session for project ID.
		sess, err := deps.SessionRepo.FindByID(ctx, wf.SessionID)
		if err != nil {
			return fmt.Errorf("create_implementation_tasks: find session: %w", err)
		}
		if sess == nil {
			return fmt.Errorf("create_implementation_tasks: session %q not found", wf.SessionID)
		}

		// Get agent IDs for task assignment.
		humanAgent, err := deps.AgentRepo.FindByName(ctx, "human")
		if err != nil {
			return fmt.Errorf("create_implementation_tasks: find human agent: %w", err)
		}
		var humanAgentID string
		if humanAgent != nil {
			humanAgentID = humanAgent.ID
		}

		// For assistant agent, we use an empty string as placeholder.
		// The scheduler will assign the actual assistant based on session configuration.
		assistantAgentID := ""

		// Get the workflow spec to check guardrail settings.
		var guardrails domain.GuardrailSettings
		if wf.FilePath.Valid {
			spec, ok := deps.FileRegistry.GetBySourcePath(wf.FilePath.String)
			if ok {
				guardrails = spec.Settings.Guardrails
			} else {
				// Default guardrails if spec not found.
				guardrails = domain.DefaultGuardrailSettings()
			}
		} else {
			guardrails = domain.DefaultGuardrailSettings()
		}

		// Build the task list.
		var tasks []*domain.Task

		for epicIdx, epic := range plan.Epics {
			for storyIdx, story := range epic.Stories {
				seqTask := 0 // 0-based within story
				var lastCodingTaskID string

				// Create coding tasks.
				for _, t := range story.Tasks {
					task := buildCodingTask(
						args.WorkflowID,
						wf.SessionID,
						sess.ProjectID,
						assistantAgentID,
						epicIdx,
						storyIdx,
						seqTask,
						&epic,
						&story,
						&t,
					)
					tasks = append(tasks, task)
					lastCodingTaskID = task.ID
					seqTask++
				}

				// Auto-inject Verification task after coding tasks.
				var lastTaskID = lastCodingTaskID
				if guardrails.Verification && len(story.Tasks) > 0 {
					verifyTask := buildVerificationTask(
						args.WorkflowID,
						wf.SessionID,
						sess.ProjectID,
						assistantAgentID,
						epicIdx,
						storyIdx,
						seqTask,
						&epic,
						&story,
						lastCodingTaskID,
					)
					tasks = append(tasks, verifyTask)
					lastTaskID = verifyTask.ID
					seqTask++
				}

				// Auto-inject Confirmation task after verification.
				if guardrails.Confirmation && len(story.Tasks) > 0 {
					confirmTask := buildConfirmationTask(
						args.WorkflowID,
						wf.SessionID,
						sess.ProjectID,
						humanAgentID,
						epicIdx,
						storyIdx,
						seqTask,
						&epic,
						&story,
						lastTaskID,
					)
					tasks = append(tasks, confirmTask)
					seqTask++
				}
			}
		}

		// Bulk insert all tasks in a single transaction.
		if err := deps.TaskRepo.BulkInsert(ctx, tasks); err != nil {
			return fmt.Errorf("create_implementation_tasks: bulk insert: %w", err)
		}

		return nil
	}
}

// buildCodingTask creates a Coding task from the plan.
func buildCodingTask(
	workflowID, sessionID, projectID, assistantAgentID string,
	seqEpic, seqStory, seqTask int,
	epic *planEpic,
	story *planStory,
	t *planTask,
) *domain.Task {
	// Build task input JSON with metadata.
	input := CodingTaskInput{
		EpicID:      epic.ID,
		EpicName:    epic.Name,
		StoryID:     story.ID,
		StoryName:   story.Name,
		TaskID:      t.ID,
		TaskName:    t.Name,
		Description: t.Description,
		Files:       t.Files,
	}
	inputJSON, _ := json.Marshal(input)

	return &domain.Task{
		ID:         idgen.MustNew(),
		ProjectID:  projectID,
		SessionID:  sessionID,
		WorkflowID: sql.NullString{String: workflowID, Valid: true},
		AssigneeID: assistantAgentID,
		SeqEpic:    seqEpic,
		SeqStory:   seqStory,
		SeqTask:    seqTask,
		Type:       domain.TaskTypeCoding,
		Status:     domain.TaskStatusPending,
		Timeout:    domain.DefaultTaskTimeout,
		Input:      sql.NullString{String: string(inputJSON), Valid: true},
	}
}

// buildVerificationTask creates a Verification task that depends on the last coding task.
func buildVerificationTask(
	workflowID, sessionID, projectID, assistantAgentID string,
	seqEpic, seqStory, seqTask int,
	epic *planEpic,
	story *planStory,
	dependsOnTaskID string,
) *domain.Task {
	// Determine verification command: story-level override or default.
	verifyCmd := story.Verification
	if verifyCmd == "" {
		verifyCmd = "go test ./..."
	}

	// Build task input JSON.
	input := VerificationTaskInput{
		EpicID:    epic.ID,
		EpicName:  epic.Name,
		StoryID:   story.ID,
		StoryName: story.Name,
		Command:   verifyCmd,
	}
	inputJSON, _ := json.Marshal(input)

	return &domain.Task{
		ID:         idgen.MustNew(),
		ProjectID:  projectID,
		SessionID:  sessionID,
		WorkflowID: sql.NullString{String: workflowID, Valid: true},
		AssigneeID: assistantAgentID,
		SeqEpic:    seqEpic,
		SeqStory:   seqStory,
		SeqTask:    seqTask,
		Type:       domain.TaskTypeVerification,
		Status:     domain.TaskStatusPending,
		Timeout:    domain.DefaultTaskTimeout,
		Input:      sql.NullString{String: string(inputJSON), Valid: true},
		DependsOn:  sql.NullString{String: dependsOnTaskID, Valid: true},
		Condition:  sql.NullInt64{Int64: int64(domain.TaskStatusSuccess), Valid: true},
	}
}

// buildConfirmationTask creates a Confirmation task that depends on verification (or coding).
func buildConfirmationTask(
	workflowID, sessionID, projectID, humanAgentID string,
	seqEpic, seqStory, seqTask int,
	epic *planEpic,
	story *planStory,
	dependsOnTaskID string,
) *domain.Task {
	// Build task input JSON with confirmation prompt.
	input := ConfirmationTaskInput{
		EpicID:    epic.ID,
		EpicName:  epic.Name,
		StoryID:   story.ID,
		StoryName: story.Name,
		Prompt:    fmt.Sprintf("Approve story '%s'?", story.Name),
	}
	inputJSON, _ := json.Marshal(input)

	return &domain.Task{
		ID:         idgen.MustNew(),
		ProjectID:  projectID,
		SessionID:  sessionID,
		WorkflowID: sql.NullString{String: workflowID, Valid: true},
		AssigneeID: humanAgentID,
		SeqEpic:    seqEpic,
		SeqStory:   seqStory,
		SeqTask:    seqTask,
		Type:       domain.TaskTypeConfirmation,
		Status:     domain.TaskStatusPending,
		Timeout:    domain.DefaultTaskTimeout,
		Input:      sql.NullString{String: string(inputJSON), Valid: true},
		DependsOn:  sql.NullString{String: dependsOnTaskID, Valid: true},
	}
}

// CodingTaskInput is the JSON structure stored in task.Input for Coding tasks.
type CodingTaskInput struct {
	EpicID      string   `json:"epic_id"`
	EpicName    string   `json:"epic_name"`
	StoryID     string   `json:"story_id"`
	StoryName   string   `json:"story_name"`
	TaskID      string   `json:"task_id"`
	TaskName    string   `json:"task_name"`
	Description string   `json:"description,omitempty"`
	Files       []string `json:"files,omitempty"`
}

// VerificationTaskInput is the JSON structure stored in task.Input for Verification tasks.
type VerificationTaskInput struct {
	EpicID    string `json:"epic_id"`
	EpicName  string `json:"epic_name"`
	StoryID   string `json:"story_id"`
	StoryName string `json:"story_name"`
	Command   string `json:"command"`
}

// ConfirmationTaskInput is the JSON structure stored in task.Input for Confirmation tasks.
type ConfirmationTaskInput struct {
	EpicID    string `json:"epic_id"`
	EpicName  string `json:"epic_name"`
	StoryID   string `json:"story_id"`
	StoryName string `json:"story_name"`
	Prompt    string `json:"prompt"`
}
