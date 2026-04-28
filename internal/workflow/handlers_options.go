package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/repository"
	"codemint.kanthorlabs.com/internal/util/idgen"
)

// optionsProposerOutput is the expected JSON structure from the options-proposer skill.
type optionsProposerOutput struct {
	Options          []json.RawMessage `json:"options"`
	ReasonForSingle  *string           `json:"reason_for_single"`
}

// optionMeta extracts just the ID for matching.
type optionMeta struct {
	ID string `json:"id"`
}

// LockChosenOptionHandler returns a HandlerFunc that parses skill output,
// finds the picked option, and calls WorkflowRepo.LockChosenOption.
//
// This handler is invoked when the exit command is "/pick-option <id>".
// If the exit command is "/modify", this handler delegates to ResetWorkflowToGoalHandler.
//
// Expected output format:
//
//	{"options":[{"id":"A",...},{"id":"B",...}],"reason_for_single":null}
//
// Validation:
//   - Exit command must be "/pick-option <id>" or "/modify"
//   - For /pick-option: the option ID must exist in the options array
//
// Returns error on invalid JSON, unknown option ID, etc. - these become task Failure.
func LockChosenOptionHandler(
	workflowRepo repository.WorkflowRepository,
	taskRepo repository.TaskRepository,
	fileRegistry *FileRegistry,
) HandlerFunc {
	// Create the reset handler for delegation.
	resetHandler := ResetWorkflowToGoalHandler(workflowRepo, taskRepo, fileRegistry)

	return func(ctx context.Context, args HandlerArgs) error {
		// Dispatch based on exit command.
		if strings.HasPrefix(args.ExitCmd, "/modify") {
			return resetHandler(ctx, args)
		}

		// Must be /pick-option <id>
		return lockChosenOption(ctx, args, workflowRepo)
	}
}

// lockChosenOption handles the /pick-option <id> case.
func lockChosenOption(ctx context.Context, args HandlerArgs, repo repository.WorkflowRepository) error {
	if repo == nil {
		return errors.New("lock_chosen_option: workflow repository is nil")
	}

	if args.WorkflowID == "" {
		return errors.New("lock_chosen_option: workflow ID is required")
	}

	// Parse the exit command: "/pick-option B" → "B"
	parts := strings.Fields(args.ExitCmd)
	if len(parts) != 2 || parts[0] != "/pick-option" {
		return fmt.Errorf("lock_chosen_option: expected '/pick-option <id>', got %q", args.ExitCmd)
	}
	pickedID := strings.ToUpper(parts[1])

	if args.Output == "" {
		return errors.New("lock_chosen_option: output is empty")
	}

	// Parse the JSON output.
	var parsed optionsProposerOutput
	if err := json.Unmarshal([]byte(args.Output), &parsed); err != nil {
		return fmt.Errorf("lock_chosen_option: invalid JSON: %w", err)
	}

	// Validate we have options.
	if len(parsed.Options) == 0 {
		return errors.New("lock_chosen_option: no options in output")
	}

	// Find the option with the matching ID.
	for _, raw := range parsed.Options {
		var meta optionMeta
		if err := json.Unmarshal(raw, &meta); err != nil {
			continue
		}
		if strings.ToUpper(meta.ID) == pickedID {
			// Found it! Store the full option JSON.
			if err := repo.LockChosenOption(ctx, args.WorkflowID, string(raw)); err != nil {
				return fmt.Errorf("lock_chosen_option: %w", err)
			}
			return nil
		}
	}

	return fmt.Errorf("lock_chosen_option: option %q not found in proposed options", pickedID)
}

// ResetWorkflowToGoalHandler returns a HandlerFunc that handles the /modify command.
// It clears goal_text, success_criteria, and chosen_option, then cancels existing
// Goal/Reality/Options tasks and re-creates them with incremented seq_task.
//
// This handler is invoked when the exit command is "/modify" during the Options step.
func ResetWorkflowToGoalHandler(
	workflowRepo repository.WorkflowRepository,
	taskRepo repository.TaskRepository,
	fileRegistry *FileRegistry,
) HandlerFunc {
	return func(ctx context.Context, args HandlerArgs) error {
		if workflowRepo == nil {
			return errors.New("reset_workflow_to_goal: workflow repository is nil")
		}
		if taskRepo == nil {
			return errors.New("reset_workflow_to_goal: task repository is nil")
		}
		if fileRegistry == nil {
			return errors.New("reset_workflow_to_goal: file registry is nil")
		}
		if args.WorkflowID == "" {
			return errors.New("reset_workflow_to_goal: workflow ID is required")
		}

		// Step 1: Clear the GOROW columns (goal_text, success_criteria, chosen_option).
		if err := workflowRepo.ResetGOROW(ctx, args.WorkflowID); err != nil {
			return fmt.Errorf("reset_workflow_to_goal: %w", err)
		}

		// Step 2: Cancel existing Goal/Reality/Options tasks.
		// These are the story IDs we need to cancel and recreate.
		storyIDsToReset := []string{"capture_goal", "gather_targeted", "propose_options"}

		if err := taskRepo.CancelByWorkflowAndStoryIDs(ctx, args.WorkflowID, storyIDsToReset); err != nil {
			return fmt.Errorf("reset_workflow_to_goal: cancel tasks: %w", err)
		}

		// Step 3: Get the workflow to find its spec file and session.
		wf, err := workflowRepo.FindByID(ctx, args.WorkflowID)
		if err != nil {
			return fmt.Errorf("reset_workflow_to_goal: find workflow: %w", err)
		}
		if wf == nil {
			return fmt.Errorf("reset_workflow_to_goal: workflow %q not found", args.WorkflowID)
		}

		// Step 4: Get the workflow spec to find the story definitions.
		// The file_path in workflow stores the source path (e.g., "/path/to/brainstorming/WORKFLOW.yaml").
		spec, ok := fileRegistry.GetBySourcePath(wf.FilePath.String)
		if !ok {
			return fmt.Errorf("reset_workflow_to_goal: workflow spec %q not found in registry", wf.FilePath.String)
		}

		// Step 5: Get the current max seq_task so we can increment from there.
		maxSeq, err := taskRepo.GetMaxSeqTask(ctx, args.WorkflowID)
		if err != nil {
			return fmt.Errorf("reset_workflow_to_goal: get max seq_task: %w", err)
		}

		// Step 6: Find the stories in the spec and create new tasks.
		// We need to find capture_goal, gather_targeted, and propose_options stories.
		for _, epic := range spec.Epics {
			for storyIdx, story := range epic.Stories {
				// Check if this story is one we need to recreate.
				shouldRecreate := false
				for _, sid := range storyIDsToReset {
					if story.ID == sid {
						shouldRecreate = true
						break
					}
				}
				if !shouldRecreate {
					continue
				}

				maxSeq++

				// Create the task input JSON.
				input := TaskInput{
					EpicID:    epic.ID,
					EpicName:  epic.Name,
					StoryID:   story.ID,
					StoryName: story.Name,
					Skill:     story.Skill,
				}
				inputJSON, err := json.Marshal(input)
				if err != nil {
					return fmt.Errorf("reset_workflow_to_goal: marshal input: %w", err)
				}

				// Create the new task.
				task := &domain.Task{
					ID:         idgen.MustNew(),
					ProjectID:  args.Task.ProjectID, // Use the project ID from the current task
					SessionID:  wf.SessionID,
					WorkflowID: domain.NewNullString(args.WorkflowID),
					AssigneeID: "", // Will be assigned by scheduler
					SeqEpic:    0,  // First epic
					SeqStory:   storyIdx,
					SeqTask:    maxSeq,
					Type:       story.Type,
					Status:     domain.TaskStatusPending,
					Timeout:    spec.Settings.DefaultTimeout,
					Input:      domain.NewNullString(string(inputJSON)),
					DependsOn:  domain.NewNullString(story.DependsOn),
				}

				if story.Condition != nil {
					task.Condition = sql.NullInt64{Int64: int64(*story.Condition), Valid: true}
				}

				if err := taskRepo.Create(ctx, task); err != nil {
					return fmt.Errorf("reset_workflow_to_goal: create task for story %q: %w", story.ID, err)
				}
			}
		}

		return nil
	}
}

// TaskInput is the JSON structure stored in task.Input.
// This duplicates the structure from orchestrator to avoid circular imports.
type TaskInput struct {
	EpicID    string `json:"epic_id"`
	EpicName  string `json:"epic_name"`
	StoryID   string `json:"story_id"`
	StoryName string `json:"story_name"`
	Skill     string `json:"skill,omitempty"`
}
