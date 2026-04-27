// Package orchestrator contains the core orchestration logic for CodeMint.
package orchestrator

import (
	"context"
	"fmt"
	"os"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/repository"
)

// CodeMintProjectName is the canonical display + lookup name for the
// CodeMint sentinel project. Other code MUST locate the project via this
// constant rather than hardcoding the string.
const CodeMintProjectName = "codemint"

// EnsureCodeMintProject idempotently ensures the CodeMint sentinel project
// exists (kind=ProjectKindCodeMint, name=CodeMintProjectName, working_dir
// pointing at workspaceDir), the workspace directory exists on disk, a
// project_permission row exists (all-NULL columns), and at least one
// session under the project has status=Active. Safe to call on every launch.
func EnsureCodeMintProject(
	ctx context.Context,
	workspaceDir string,
	projectRepo repository.ProjectRepository,
	sessionRepo repository.SessionRepository,
	permRepo repository.ProjectPermissionRepository,
) error {
	// Step 1: Ensure the CodeMint project exists.
	project, err := projectRepo.FindByName(ctx, CodeMintProjectName)
	if err != nil {
		return fmt.Errorf("find codemint project: %w", err)
	}
	if project == nil {
		project = domain.NewProject(CodeMintProjectName, workspaceDir, domain.ProjectKindCodeMint)
		if err := projectRepo.Create(ctx, project); err != nil {
			return fmt.Errorf("create codemint project: %w", err)
		}
	}

	// Step 2: Ensure the workspace directory exists on disk (no git init).
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		return fmt.Errorf("create workspace directory %q: %w", workspaceDir, err)
	}

	// Step 3: Ensure a project_permission row exists for the project.
	// Using Upsert is idempotent - creates if absent, no-op if exists.
	perm := domain.NewProjectPermission(project.ID)
	if err := permRepo.Upsert(ctx, perm); err != nil {
		return fmt.Errorf("upsert codemint project permission: %w", err)
	}

	// Step 4: Ensure at least one active session exists for the project.
	count, err := sessionRepo.CountActiveByProjectID(ctx, project.ID)
	if err != nil {
		return fmt.Errorf("count active sessions for codemint project: %w", err)
	}
	if count == 0 {
		session := domain.NewSession(project.ID)
		if err := sessionRepo.Create(ctx, session); err != nil {
			return fmt.Errorf("create codemint session: %w", err)
		}
	}

	return nil
}
