// Package repl provides the REPL loop and command handlers for CodeMint.
package repl

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/registry"
	"codemint.kanthorlabs.com/internal/repository"
)

// ProjectCommandDeps holds the dependencies needed for project-related commands.
type ProjectCommandDeps struct {
	ProjectRepo      repository.ProjectRepository
	SessionRepo      repository.SessionRepository
	PermissionRepo   repository.ProjectPermissionRepository
	ActiveSession    registry.MutableSessionInfo
	ProviderRegistry ProviderLister // Optional: for validating provider names.
}

// ProviderLister is a minimal interface for listing available providers.
// Defined here to avoid importing the agent package directly.
type ProviderLister interface {
	Names() []string
}

// RegisterProjectCommands registers project management commands (/project-open, /project-list, /project-assistant).
func RegisterProjectCommands(r *registry.CommandRegistry, deps *ProjectCommandDeps) error {
	commands := []registry.Command{
		{
			Name:           "project-open",
			Description:    "Open or create a Coding project from a git directory and switch to it.",
			Usage:          "/project-open <path>",
			SupportedModes: []registry.ClientMode{registry.ClientModeCLI, registry.ClientModeDaemon},
			Handler:        projectOpenHandler(deps),
		},
		{
			Name:           "project-list",
			Description:    "List all registered projects.",
			Usage:          "/project-list",
			SupportedModes: []registry.ClientMode{registry.ClientModeCLI, registry.ClientModeDaemon},
			Handler:        projectListHandler(deps),
		},
		{
			Name:           "project-assistant",
			Description:    "View or set the assistant provider override for the current project.",
			Usage:          "/project-assistant [provider|reset]",
			SupportedModes: []registry.ClientMode{registry.ClientModeCLI, registry.ClientModeDaemon},
			Handler:        projectAssistantHandler(deps),
		},
	}

	for _, c := range commands {
		if err := r.Register(c); err != nil {
			return fmt.Errorf("repl: register project command %q: %w", c.Name, err)
		}
	}
	return nil
}

// projectOpenHandler handles the /project-open <path> command.
// It creates or retrieves a Coding project for the given directory,
// creates an active session, and switches to it.
func projectOpenHandler(deps *ProjectCommandDeps) registry.Handler {
	return func(ctx context.Context, active registry.ActiveSessionInfo, args []string, _ string) (registry.CommandResult, error) {
		if len(args) == 0 {
			return registry.CommandResult{
				Message: "Usage: /project-open <path>\n\nPath must be a git-initialized directory.",
				Action:  registry.ActionNone,
			}, nil
		}

		path := args[0]

		// Resolve to absolute path.
		absPath, err := filepath.Abs(path)
		if err != nil {
			return registry.CommandResult{}, fmt.Errorf("resolve path: %w", err)
		}

		// Verify directory exists.
		info, err := os.Stat(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				return registry.CommandResult{
					Message: fmt.Sprintf("Directory not found: %s", absPath),
					Action:  registry.ActionNone,
				}, nil
			}
			return registry.CommandResult{}, fmt.Errorf("stat path: %w", err)
		}
		if !info.IsDir() {
			return registry.CommandResult{
				Message: fmt.Sprintf("Path is not a directory: %s", absPath),
				Action:  registry.ActionNone,
			}, nil
		}

		// Verify git repository.
		if !isGitRepository(absPath) {
			return registry.CommandResult{
				Message: fmt.Sprintf("Not a git repository: %s\n\nCoding projects require git. Run 'git init' first.", absPath),
				Action:  registry.ActionNone,
			}, nil
		}

		// Derive project name from directory basename.
		projectName := filepath.Base(absPath)

		// Check if project already exists for this path.
		existing, err := deps.ProjectRepo.FindByName(ctx, projectName)
		if err != nil {
			return registry.CommandResult{}, fmt.Errorf("lookup project: %w", err)
		}

		var project *domain.Project
		if existing != nil {
			// Project exists - verify path matches.
			if existing.WorkingDir != absPath {
				// Different path - generate unique name.
				projectName = generateUniqueName(projectName, absPath)
				existing = nil // Force create new project.
			}
		}

		if existing != nil {
			project = existing
		} else {
			// Create new project.
			project = domain.NewProject(projectName, absPath, domain.ProjectKindCoding)
			if err := deps.ProjectRepo.Create(ctx, project); err != nil {
				// Check for duplicate name (race condition).
				if strings.Contains(err.Error(), "UNIQUE constraint") {
					projectName = generateUniqueName(projectName, absPath)
					project = domain.NewProject(projectName, absPath, domain.ProjectKindCoding)
					if err := deps.ProjectRepo.Create(ctx, project); err != nil {
						return registry.CommandResult{}, fmt.Errorf("create project: %w", err)
					}
				} else {
					return registry.CommandResult{}, fmt.Errorf("create project: %w", err)
				}
			}

			// Create default permission row (all NULL = no restrictions).
			perm := &domain.ProjectPermission{ProjectID: project.ID}
			// Non-fatal: permission will be permissive by default if upsert fails.
			_ = deps.PermissionRepo.Upsert(ctx, perm)
		}

		// Find or create active session for this project.
		session, err := deps.SessionRepo.FindActiveByProjectID(ctx, project.ID)
		if err != nil {
			return registry.CommandResult{}, fmt.Errorf("find active session: %w", err)
		}

		if session == nil {
			// Create new session.
			session = domain.NewSession(project.ID)
			if err := deps.SessionRepo.Create(ctx, session); err != nil {
				return registry.CommandResult{}, fmt.Errorf("create session: %w", err)
			}
		}

		// Switch active session using the interface.
		// YoloMode defaults to off for new Coding projects.
		yoloEnabled := project.YoloMode == int(domain.YoloModeOn)
		deps.ActiveSession.SetSession(session, project, yoloEnabled)

		var sb strings.Builder
		if existing != nil {
			sb.WriteString(fmt.Sprintf("Switched to project: %s\n", project.Name))
		} else {
			sb.WriteString(fmt.Sprintf("Created project: %s\n", project.Name))
		}
		sb.WriteString(fmt.Sprintf("Working directory: %s\n", project.WorkingDir))
		sb.WriteString(fmt.Sprintf("Session: %s", session.ID))

		return registry.CommandResult{
			Message: sb.String(),
			Action:  registry.ActionNone,
		}, nil
	}
}

// projectListHandler handles the /project-list command.
func projectListHandler(deps *ProjectCommandDeps) registry.Handler {
	return func(ctx context.Context, active registry.ActiveSessionInfo, _ []string, _ string) (registry.CommandResult, error) {
		projects, err := deps.ProjectRepo.List(ctx)
		if err != nil {
			return registry.CommandResult{}, fmt.Errorf("list projects: %w", err)
		}

		if len(projects) == 0 {
			return registry.CommandResult{
				Message: "No projects registered. Use /project-open <path> to create one.",
				Action:  registry.ActionNone,
			}, nil
		}

		var sb strings.Builder
		sb.WriteString("Registered projects:\n\n")

		for _, p := range projects {
			kindMarker := ""
			if p.Kind == domain.ProjectKindCodeMint {
				kindMarker = " [codemint]"
			}

			shortID := p.ID
			if len(shortID) > 12 {
				shortID = shortID[:12]
			}

			fmt.Fprintf(&sb, "  %s - %s%s\n", shortID, p.Name, kindMarker)
			fmt.Fprintf(&sb, "    %s\n", p.WorkingDir)
		}

		return registry.CommandResult{
			Message: sb.String(),
			Action:  registry.ActionNone,
		}, nil
	}
}

// isGitRepository checks if the directory is a git repository.
func isGitRepository(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "true"
}

// generateUniqueName generates a unique project name by appending a hash suffix.
func generateUniqueName(baseName, path string) string {
	// Use last 6 chars of path hash for uniqueness.
	hash := fmt.Sprintf("%x", path)
	if len(hash) > 6 {
		hash = hash[len(hash)-6:]
	}
	return fmt.Sprintf("%s-%s", baseName, hash)
}

// projectAssistantHandler handles the /project-assistant command.
// Without args: shows current assistant binding for the project.
// With provider name: sets the assistant override for the project.
// With "reset": clears the assistant override.
func projectAssistantHandler(deps *ProjectCommandDeps) registry.Handler {
	return func(ctx context.Context, active registry.ActiveSessionInfo, args []string, _ string) (registry.CommandResult, error) {
		// Get current project ID from session.
		projectID := deps.ActiveSession.GetProjectID()
		if projectID == "" {
			return registry.CommandResult{
				Message: "No active project. Use /project-open to open a project first.",
				Action:  registry.ActionNone,
			}, nil
		}

		// Load project from database.
		project, err := deps.ProjectRepo.FindByID(ctx, projectID)
		if err != nil {
			return registry.CommandResult{}, fmt.Errorf("find project: %w", err)
		}
		if project == nil {
			return registry.CommandResult{
				Message: "Project not found. Use /project-open to open a project.",
				Action:  registry.ActionNone,
			}, nil
		}

		// CodeMint project cannot have assistant override changed.
		if project.Kind == domain.ProjectKindCodeMint {
			return registry.CommandResult{
				Message: "CodeMint project uses the system assistant. Override not supported.",
				Action:  registry.ActionNone,
			}, nil
		}

		// No args: show current binding.
		if len(args) == 0 {
			return showAssistantBinding(project, deps)
		}

		arg := strings.ToLower(args[0])

		// Reset: clear the override.
		if arg == "reset" || arg == "clear" {
			return clearAssistantOverride(ctx, project, deps)
		}

		// Set provider override.
		return setAssistantOverride(ctx, project, arg, deps)
	}
}

// showAssistantBinding displays the current assistant binding for the project.
func showAssistantBinding(project *domain.Project, deps *ProjectCommandDeps) (registry.CommandResult, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Project: %s\n", project.Name))

	if !project.AssistantProvider.Valid || project.AssistantProvider.String == "" {
		sb.WriteString("Assistant: (using global default)\n")
	} else {
		sb.WriteString(fmt.Sprintf("Assistant: %s", project.AssistantProvider.String))
		if project.AssistantModel.Valid && project.AssistantModel.String != "" {
			sb.WriteString(fmt.Sprintf(" (model: %s)", project.AssistantModel.String))
		}
		sb.WriteString("\n")
	}

	// Show available providers if registry is available.
	if deps.ProviderRegistry != nil {
		names := deps.ProviderRegistry.Names()
		if len(names) > 0 {
			sb.WriteString("\nAvailable providers:\n")
			for _, name := range names {
				sb.WriteString(fmt.Sprintf("  - %s\n", name))
			}
		}
	}

	sb.WriteString("\nUsage:\n")
	sb.WriteString("  /project-assistant <provider>  - Set assistant provider\n")
	sb.WriteString("  /project-assistant reset       - Clear override (use global default)")

	return registry.CommandResult{
		Message: sb.String(),
		Action:  registry.ActionNone,
	}, nil
}

// clearAssistantOverride removes the assistant override for the project.
func clearAssistantOverride(ctx context.Context, project *domain.Project, deps *ProjectCommandDeps) (registry.CommandResult, error) {
	if err := deps.ProjectRepo.UpdateAssistantBinding(ctx, project.ID, "", ""); err != nil {
		return registry.CommandResult{}, fmt.Errorf("clear assistant binding: %w", err)
	}

	return registry.CommandResult{
		Message: fmt.Sprintf("Cleared assistant override for project %q.\nNow using global default.", project.Name),
		Action:  registry.ActionNone,
	}, nil
}

// setAssistantOverride sets the assistant provider for the project.
func setAssistantOverride(ctx context.Context, project *domain.Project, provider string, deps *ProjectCommandDeps) (registry.CommandResult, error) {
	// Validate provider if registry is available.
	if deps.ProviderRegistry != nil {
		names := deps.ProviderRegistry.Names()
		found := false
		for _, name := range names {
			if strings.EqualFold(name, provider) {
				provider = name // Use canonical name.
				found = true
				break
			}
		}
		if !found {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Unknown provider: %s\n\n", provider))
			sb.WriteString("Available providers:\n")
			for _, name := range names {
				sb.WriteString(fmt.Sprintf("  - %s\n", name))
			}
			return registry.CommandResult{
				Message: sb.String(),
				Action:  registry.ActionNone,
			}, nil
		}
	}

	// Update project in database.
	if err := deps.ProjectRepo.UpdateAssistantBinding(ctx, project.ID, provider, ""); err != nil {
		return registry.CommandResult{}, fmt.Errorf("set assistant binding: %w", err)
	}

	return registry.CommandResult{
		Message: fmt.Sprintf("Set assistant for project %q to: %s", project.Name, provider),
		Action:  registry.ActionNone,
	}, nil
}
