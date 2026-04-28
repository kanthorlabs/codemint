package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"codemint.kanthorlabs.com/internal/repository"
)

// lockWorkflowGoalOutput is the expected JSON structure from the goal-capture skill.
type lockWorkflowGoalOutput struct {
	GoalText        string   `json:"goal_text"`
	SuccessCriteria []string `json:"success_criteria"`
}

// LockWorkflowGoalHandler returns a HandlerFunc that parses skill output
// and calls WorkflowRepo.LockGoal to persist the goal and success criteria.
//
// Expected output format:
//
//	{"goal_text":"<sentence>","success_criteria":["<criterion>","<criterion>"]}
//
// Validation:
//   - goal_text must be non-empty
//   - success_criteria must have at least 1 item
//
// Returns error on invalid JSON, empty goal, or empty criteria - these become task Failure.
func LockWorkflowGoalHandler(repo repository.WorkflowRepository) HandlerFunc {
	return func(ctx context.Context, args HandlerArgs) error {
		if repo == nil {
			return errors.New("lock_workflow_goal: workflow repository is nil")
		}

		if args.WorkflowID == "" {
			return errors.New("lock_workflow_goal: workflow ID is required")
		}

		if args.Output == "" {
			return errors.New("lock_workflow_goal: output is empty")
		}

		// Parse the JSON output.
		var parsed lockWorkflowGoalOutput
		if err := json.Unmarshal([]byte(args.Output), &parsed); err != nil {
			return fmt.Errorf("lock_workflow_goal: invalid JSON: %w", err)
		}

		// Validate goal_text.
		if strings.TrimSpace(parsed.GoalText) == "" {
			return errors.New("lock_workflow_goal: goal_text is required")
		}

		// Validate success_criteria.
		if len(parsed.SuccessCriteria) == 0 {
			return errors.New("lock_workflow_goal: at least one success criterion required")
		}

		// Filter out empty criteria.
		var validCriteria []string
		for _, c := range parsed.SuccessCriteria {
			if strings.TrimSpace(c) != "" {
				validCriteria = append(validCriteria, strings.TrimSpace(c))
			}
		}
		if len(validCriteria) == 0 {
			return errors.New("lock_workflow_goal: all success criteria are empty")
		}

		// Marshal criteria back to JSON for storage.
		criteriaJSON, err := json.Marshal(validCriteria)
		if err != nil {
			return fmt.Errorf("lock_workflow_goal: failed to marshal criteria: %w", err)
		}

		// Persist to the database.
		if err := repo.LockGoal(ctx, args.WorkflowID, strings.TrimSpace(parsed.GoalText), string(criteriaJSON)); err != nil {
			return fmt.Errorf("lock_workflow_goal: %w", err)
		}

		return nil
	}
}

// RegisterBuiltinHandlersDeps holds dependencies for registering built-in handlers.
type RegisterBuiltinHandlersDeps struct {
	WorkflowRepo repository.WorkflowRepository
	TaskRepo     repository.TaskRepository
	SessionRepo  repository.SessionRepository
	AgentRepo    repository.AgentRepository
	FileRegistry *FileRegistry
}

// RegisterBuiltinHandlers registers all built-in output handlers.
// Call this during application startup to make handlers available to the orchestrator.
func RegisterBuiltinHandlers(registry *HandlerRegistry, deps RegisterBuiltinHandlersDeps) error {
	handlers := map[string]HandlerFunc{
		"lock_workflow_goal":          LockWorkflowGoalHandler(deps.WorkflowRepo),
		"append_targeted_context":     AppendTargetedContextHandler(),
		"lock_chosen_option":          LockChosenOptionHandler(deps.WorkflowRepo, deps.TaskRepo, deps.FileRegistry),
		"create_implementation_tasks": CreateImplementationTasksHandler(PlanGenerationDeps(deps)),
	}

	for name, fn := range handlers {
		if err := registry.Register(name, fn); err != nil {
			return err
		}
	}

	return nil
}
