package repl

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/registry"
	"codemint.kanthorlabs.com/internal/repository"
	"codemint.kanthorlabs.com/internal/workflow"
)

// WorkflowSessionInfo provides access to session state needed for workflow commands.
// This interface breaks the import cycle between repl and orchestrator.
type WorkflowSessionInfo interface {
	registry.MutableSessionInfo
	// Wakeup signals the scheduler to check for new tasks.
	Wakeup()
}

// WorkflowCommandDeps holds dependencies for workflow-related commands.
type WorkflowCommandDeps struct {
	FileRegistry  *workflow.FileRegistry
	TaskGenerator *workflow.TaskGenerator
	TaskRepo      repository.TaskRepository
	WorkflowRepo  repository.WorkflowRepository
	ActiveSession WorkflowSessionInfo
}

// RegisterWorkflowCommands registers the /workflow command for starting
// and listing workflow files (Story 2.0.4).
func RegisterWorkflowCommands(r *registry.CommandRegistry, deps *WorkflowCommandDeps) error {
	commands := []registry.Command{
		{
			Name:           "workflow",
			Description:    "Start a workflow or list available workflows.",
			Usage:          "/workflow [name]",
			SupportedModes: []registry.ClientMode{registry.ClientModeCLI, registry.ClientModeDaemon},
			Handler:        workflowHandler(deps),
		},
	}

	for _, c := range commands {
		if err := r.Register(c); err != nil {
			return fmt.Errorf("repl: register workflow command %q: %w", c.Name, err)
		}
	}
	return nil
}

// workflowHandler handles the /workflow command.
// Without args: lists all available workflows.
// With name: starts the named workflow.
func workflowHandler(deps *WorkflowCommandDeps) registry.Handler {
	return func(ctx context.Context, active registry.ActiveSessionInfo, args []string, _ string) (registry.CommandResult, error) {
		// Validate dependencies.
		if deps.FileRegistry == nil {
			return registry.CommandResult{
				Message: "Workflow file registry not available.",
				Action:  registry.ActionNone,
			}, nil
		}

		// No args: list available workflows.
		if len(args) == 0 {
			return listWorkflows(deps)
		}

		// With name: start the workflow.
		name := args[0]
		return startWorkflow(ctx, deps, name)
	}
}

// listWorkflows returns a formatted list of all available workflow files.
func listWorkflows(deps *WorkflowCommandDeps) (registry.CommandResult, error) {
	workflows := deps.FileRegistry.All()

	if len(workflows) == 0 {
		return registry.CommandResult{
			Message: "No workflows available.\n\nWorkflows are loaded from:\n- ~/.local/share/codemint/workflows/\n- Built-in embedded workflows",
			Action:  registry.ActionNone,
		}, nil
	}

	var sb strings.Builder
	sb.WriteString("Available workflows:\n\n")

	for _, wf := range workflows {
		// Format: name    description
		desc := wf.Description
		if desc == "" {
			desc = "(no description)"
		}
		fmt.Fprintf(&sb, "  %-20s %s\n", wf.Name, desc)
	}

	sb.WriteString("\nUsage: /workflow <name> to start a workflow")

	return registry.CommandResult{
		Message: sb.String(),
		Action:  registry.ActionNone,
	}, nil
}

// startWorkflow initiates execution of the named workflow.
func startWorkflow(ctx context.Context, deps *WorkflowCommandDeps, name string) (registry.CommandResult, error) {
	// Validate active session.
	if deps.ActiveSession == nil {
		return registry.CommandResult{
			Message: "No active session. Use /project-open to start.",
			Action:  registry.ActionNone,
		}, nil
	}

	sessionID := deps.ActiveSession.GetSessionID()
	if sessionID == "" {
		return registry.CommandResult{
			Message: "No active session. Use /project-open to start.",
			Action:  registry.ActionNone,
		}, nil
	}

	projectID := deps.ActiveSession.GetProjectID()
	if projectID == "" {
		return registry.CommandResult{
			Message: "No active project. Use /project-open to select a project.",
			Action:  registry.ActionNone,
		}, nil
	}

	// Look up the workflow.
	wf, ok := deps.FileRegistry.Get(name)
	if !ok {
		// Try prefix match for autocomplete.
		wf = findWorkflowByPrefix(deps.FileRegistry, name)
		if wf == nil {
			return registry.CommandResult{
				Message: fmt.Sprintf("Workflow %q not found. Use /workflow to list available workflows.", name),
				Action:  registry.ActionNone,
			}, nil
		}
	}

	// Check if there's already an active workflow for this session.
	if deps.WorkflowRepo != nil {
		activeWf, err := deps.WorkflowRepo.GetActiveForSession(ctx, sessionID)
		if err != nil {
			return registry.CommandResult{}, fmt.Errorf("check active workflow: %w", err)
		}
		if activeWf != nil {
			return registry.CommandResult{
				Message: fmt.Sprintf("A workflow is already active for this session (ID: %s).\nUse /workflow-cancel to cancel it first.", activeWf.ID[:8]),
				Action:  registry.ActionNone,
			}, nil
		}
	}

	// Create workflow execution record.
	workflowExec := domain.NewWorkflow(sessionID, int(domain.WorkflowTypeProjectCoding))
	workflowExec.FilePath = sql.NullString{String: wf.SourcePath, Valid: true}
	workflowExec.StartedAt = sql.NullInt64{Int64: time.Now().Unix(), Valid: true}

	if deps.WorkflowRepo != nil {
		if err := deps.WorkflowRepo.Create(ctx, workflowExec); err != nil {
			return registry.CommandResult{}, fmt.Errorf("create workflow record: %w", err)
		}
	}

	// Generate tasks from the workflow file.
	if deps.TaskGenerator == nil || deps.TaskRepo == nil {
		return registry.CommandResult{
			Message: fmt.Sprintf("Starting workflow: %s (v%s)\n\nNote: Task generation not available (missing dependencies).", wf.Name, wf.Version),
			Action:  registry.ActionNone,
		}, nil
	}

	cfg := workflow.GenerateConfig{
		ProjectID:  projectID,
		SessionID:  sessionID,
		WorkflowID: workflowExec.ID,
	}

	tasks, err := deps.TaskGenerator.GenerateTasksWithGuardrails(wf, cfg)
	if err != nil {
		return registry.CommandResult{}, fmt.Errorf("generate tasks: %w", err)
	}

	// Insert tasks into the database.
	for _, task := range tasks {
		if err := deps.TaskRepo.Create(ctx, task); err != nil {
			return registry.CommandResult{}, fmt.Errorf("insert task: %w", err)
		}
	}

	// Signal the scheduler to pick up the new tasks.
	deps.ActiveSession.Wakeup()

	// Build response message.
	var sb strings.Builder
	fmt.Fprintf(&sb, "Starting workflow: %s (v%s)\n\n", wf.Name, wf.Version)
	fmt.Fprintf(&sb, "Generated %d task(s) across %d epic(s).\n", len(tasks), len(wf.Epics))
	fmt.Fprintf(&sb, "Workflow ID: %s\n", workflowExec.ID[:8])

	if len(wf.Epics) > 0 {
		sb.WriteString("\nEpics:\n")
		for i, epic := range wf.Epics {
			fmt.Fprintf(&sb, "  %d. %s (%d stories)\n", i+1, epic.Name, len(epic.Stories))
		}
	}

	sb.WriteString("\nThe scheduler will automatically process tasks in order.")

	return registry.CommandResult{
		Message: sb.String(),
		Action:  registry.ActionNone,
	}, nil
}

// findWorkflowByPrefix attempts to find a workflow by name prefix.
// Returns nil if no match or multiple matches.
func findWorkflowByPrefix(reg *workflow.FileRegistry, prefix string) *domain.WorkflowFile {
	prefix = strings.ToLower(prefix)
	var match *domain.WorkflowFile
	matchCount := 0

	for _, wf := range reg.All() {
		if strings.HasPrefix(strings.ToLower(wf.Name), prefix) {
			match = wf
			matchCount++
		}
	}

	// Only return if exactly one match.
	if matchCount == 1 {
		return match
	}
	return nil
}

// WorkflowCompleter returns a function that provides tab completion for workflow names.
// This can be used by the REPL's line editor for autocomplete support.
func WorkflowCompleter(reg *workflow.FileRegistry) func(prefix string) []string {
	return func(prefix string) []string {
		if reg == nil {
			return nil
		}

		prefix = strings.ToLower(prefix)
		var completions []string

		for _, name := range reg.Names() {
			if strings.HasPrefix(strings.ToLower(name), prefix) {
				completions = append(completions, name)
			}
		}

		return completions
	}
}
