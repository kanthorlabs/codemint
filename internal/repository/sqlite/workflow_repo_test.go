package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/repository"
	"codemint.kanthorlabs.com/internal/util/idgen"
)

func setupWorkflowFixtures(t *testing.T) (workflowRepo repository.WorkflowRepository, sessionRepo repository.SessionRepository, projectRepo repository.ProjectRepository, sessionID string, projectID string) {
	t.Helper()
	conn := openTestDB(t)
	workflowRepo = NewWorkflowRepo(conn)
	sessionRepo = NewSessionRepo(conn)
	projectRepo = NewProjectRepo(conn)
	ctx := context.Background()

	// Create a project for sessions to reference.
	project := domain.NewProject("workflow-test-project", "/tmp/workspace", domain.ProjectKindCoding)
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Create project returned error: %v", err)
	}
	projectID = project.ID

	// Create a session for workflows to reference.
	session := domain.NewSession(projectID)
	if err := sessionRepo.Create(ctx, session); err != nil {
		t.Fatalf("Create session returned error: %v", err)
	}
	sessionID = session.ID

	return workflowRepo, sessionRepo, projectRepo, sessionID, projectID
}

func TestWorkflowRepo_Create(t *testing.T) {
	repo, _, _, sessionID, _ := setupWorkflowFixtures(t)
	ctx := context.Background()

	workflow := domain.NewWorkflow(sessionID, 0)
	workflow.FilePath = sql.NullString{String: "/path/to/WORKFLOW.yaml", Valid: true}
	workflow.StartedAt = sql.NullInt64{Int64: time.Now().Unix(), Valid: true}

	err := repo.Create(ctx, workflow)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	// Verify the workflow was inserted.
	found, err := repo.FindByID(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("FindByID returned error: %v", err)
	}
	if found == nil {
		t.Fatal("FindByID returned nil, expected workflow")
	}
	if found.ID != workflow.ID {
		t.Errorf("ID mismatch: got %q, want %q", found.ID, workflow.ID)
	}
	if found.SessionID != sessionID {
		t.Errorf("SessionID mismatch: got %q, want %q", found.SessionID, sessionID)
	}
	if found.Status != domain.WorkflowStatusActive {
		t.Errorf("Status mismatch: got %d, want %d (Active)", found.Status, domain.WorkflowStatusActive)
	}
	if found.FilePath.String != "/path/to/WORKFLOW.yaml" {
		t.Errorf("FilePath mismatch: got %q, want %q", found.FilePath.String, "/path/to/WORKFLOW.yaml")
	}
}

func TestWorkflowRepo_FindByID(t *testing.T) {
	repo, _, _, sessionID, _ := setupWorkflowFixtures(t)
	ctx := context.Background()

	workflow := domain.NewWorkflow(sessionID, 0)
	if err := repo.Create(ctx, workflow); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	found, err := repo.FindByID(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("FindByID returned error: %v", err)
	}
	if found == nil {
		t.Fatal("FindByID returned nil, expected workflow")
	}
	if found.ID != workflow.ID {
		t.Errorf("ID mismatch: got %q, want %q", found.ID, workflow.ID)
	}
}

func TestWorkflowRepo_FindByID_NotFound(t *testing.T) {
	repo, _, _, _, _ := setupWorkflowFixtures(t)
	ctx := context.Background()

	found, err := repo.FindByID(ctx, idgen.MustNew())
	if err != nil {
		t.Fatalf("FindByID returned unexpected error: %v", err)
	}
	if found != nil {
		t.Errorf("expected nil for non-existent workflow, got %+v", found)
	}
}

func TestWorkflowRepo_GetActiveForSession(t *testing.T) {
	repo, _, _, sessionID, _ := setupWorkflowFixtures(t)
	ctx := context.Background()

	workflow := domain.NewWorkflow(sessionID, 0)
	workflow.StartedAt = sql.NullInt64{Int64: time.Now().Unix(), Valid: true}
	if err := repo.Create(ctx, workflow); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	found, err := repo.GetActiveForSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetActiveForSession returned error: %v", err)
	}
	if found == nil {
		t.Fatal("GetActiveForSession returned nil, expected workflow")
	}
	if found.ID != workflow.ID {
		t.Errorf("ID mismatch: got %q, want %q", found.ID, workflow.ID)
	}
	if found.Status != domain.WorkflowStatusActive {
		t.Errorf("Status mismatch: got %d, want %d (Active)", found.Status, domain.WorkflowStatusActive)
	}
}

func TestWorkflowRepo_GetActiveForSession_NoActive(t *testing.T) {
	repo, _, _, sessionID, _ := setupWorkflowFixtures(t)
	ctx := context.Background()

	// No workflow exists.
	found, err := repo.GetActiveForSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetActiveForSession returned unexpected error: %v", err)
	}
	if found != nil {
		t.Errorf("expected nil when no active workflow exists, got %+v", found)
	}
}

func TestWorkflowRepo_GetActiveForSession_AfterCompleted(t *testing.T) {
	repo, _, _, sessionID, _ := setupWorkflowFixtures(t)
	ctx := context.Background()

	workflow := domain.NewWorkflow(sessionID, 0)
	workflow.StartedAt = sql.NullInt64{Int64: time.Now().Unix(), Valid: true}
	if err := repo.Create(ctx, workflow); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	// Mark as completed.
	if err := repo.MarkCompleted(ctx, workflow.ID); err != nil {
		t.Fatalf("MarkCompleted returned error: %v", err)
	}

	// Should return nil now that workflow is completed.
	found, err := repo.GetActiveForSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetActiveForSession returned error: %v", err)
	}
	if found != nil {
		t.Errorf("expected nil after completion, got %+v", found)
	}
}

func TestWorkflowRepo_UpdateProgress(t *testing.T) {
	repo, _, _, sessionID, _ := setupWorkflowFixtures(t)
	ctx := context.Background()

	workflow := domain.NewWorkflow(sessionID, 0)
	if err := repo.Create(ctx, workflow); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	// Update progress.
	if err := repo.UpdateProgress(ctx, workflow.ID, "epic-1", "story-2"); err != nil {
		t.Fatalf("UpdateProgress returned error: %v", err)
	}

	// Verify the progress was updated.
	found, err := repo.FindByID(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("FindByID returned error: %v", err)
	}
	if !found.CurrentEpicID.Valid || found.CurrentEpicID.String != "epic-1" {
		t.Errorf("CurrentEpicID mismatch: got %q, want %q", found.CurrentEpicID.String, "epic-1")
	}
	if !found.CurrentStoryID.Valid || found.CurrentStoryID.String != "story-2" {
		t.Errorf("CurrentStoryID mismatch: got %q, want %q", found.CurrentStoryID.String, "story-2")
	}
}

func TestWorkflowRepo_UpdateProgress_NotFound(t *testing.T) {
	repo, _, _, _, _ := setupWorkflowFixtures(t)
	ctx := context.Background()

	err := repo.UpdateProgress(ctx, idgen.MustNew(), "epic-1", "story-1")
	if err == nil {
		t.Fatal("UpdateProgress on non-existent workflow should have returned error")
	}
	if !contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
}

func TestWorkflowRepo_MarkCompleted(t *testing.T) {
	repo, _, _, sessionID, _ := setupWorkflowFixtures(t)
	ctx := context.Background()

	workflow := domain.NewWorkflow(sessionID, 0)
	if err := repo.Create(ctx, workflow); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	// Mark as completed.
	if err := repo.MarkCompleted(ctx, workflow.ID); err != nil {
		t.Fatalf("MarkCompleted returned error: %v", err)
	}

	// Verify the status changed.
	found, err := repo.FindByID(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("FindByID returned error: %v", err)
	}
	if found.Status != domain.WorkflowStatusCompleted {
		t.Errorf("Status mismatch: got %d, want %d (Completed)", found.Status, domain.WorkflowStatusCompleted)
	}
	if !found.CompletedAt.Valid {
		t.Error("CompletedAt should be set after MarkCompleted")
	}
}

func TestWorkflowRepo_MarkCompleted_NotFound(t *testing.T) {
	repo, _, _, _, _ := setupWorkflowFixtures(t)
	ctx := context.Background()

	err := repo.MarkCompleted(ctx, idgen.MustNew())
	if err == nil {
		t.Fatal("MarkCompleted on non-existent workflow should have returned error")
	}
}

func TestWorkflowRepo_MarkCompleted_AlreadyCompleted(t *testing.T) {
	repo, _, _, sessionID, _ := setupWorkflowFixtures(t)
	ctx := context.Background()

	workflow := domain.NewWorkflow(sessionID, 0)
	if err := repo.Create(ctx, workflow); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	// Mark as completed once.
	if err := repo.MarkCompleted(ctx, workflow.ID); err != nil {
		t.Fatalf("First MarkCompleted returned error: %v", err)
	}

	// Mark as completed again should fail.
	err := repo.MarkCompleted(ctx, workflow.ID)
	if err == nil {
		t.Fatal("MarkCompleted on already-completed workflow should have returned error")
	}
}

func TestWorkflowRepo_MarkCancelled(t *testing.T) {
	repo, _, _, sessionID, _ := setupWorkflowFixtures(t)
	ctx := context.Background()

	workflow := domain.NewWorkflow(sessionID, 0)
	if err := repo.Create(ctx, workflow); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	// Mark as cancelled.
	if err := repo.MarkCancelled(ctx, workflow.ID); err != nil {
		t.Fatalf("MarkCancelled returned error: %v", err)
	}

	// Verify the status changed.
	found, err := repo.FindByID(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("FindByID returned error: %v", err)
	}
	if found.Status != domain.WorkflowStatusCancelled {
		t.Errorf("Status mismatch: got %d, want %d (Cancelled)", found.Status, domain.WorkflowStatusCancelled)
	}
}

func TestWorkflowRepo_MarkCancelled_NotFound(t *testing.T) {
	repo, _, _, _, _ := setupWorkflowFixtures(t)
	ctx := context.Background()

	err := repo.MarkCancelled(ctx, idgen.MustNew())
	if err == nil {
		t.Fatal("MarkCancelled on non-existent workflow should have returned error")
	}
}

func TestWorkflowRepo_ListByFilePath(t *testing.T) {
	repo, _, _, sessionID, _ := setupWorkflowFixtures(t)
	ctx := context.Background()

	filePath := "/path/to/WORKFLOW.yaml"

	// Create two workflows with the same file path.
	workflow1 := domain.NewWorkflow(sessionID, 0)
	workflow1.FilePath = sql.NullString{String: filePath, Valid: true}
	workflow1.StartedAt = sql.NullInt64{Int64: time.Now().Add(-time.Hour).Unix(), Valid: true}
	if err := repo.Create(ctx, workflow1); err != nil {
		t.Fatalf("Create workflow1 returned error: %v", err)
	}

	// Mark first as completed to allow creating another.
	if err := repo.MarkCompleted(ctx, workflow1.ID); err != nil {
		t.Fatalf("MarkCompleted returned error: %v", err)
	}

	workflow2 := domain.NewWorkflow(sessionID, 0)
	workflow2.FilePath = sql.NullString{String: filePath, Valid: true}
	workflow2.StartedAt = sql.NullInt64{Int64: time.Now().Unix(), Valid: true}
	if err := repo.Create(ctx, workflow2); err != nil {
		t.Fatalf("Create workflow2 returned error: %v", err)
	}

	// Create a workflow with different file path.
	workflow3 := domain.NewWorkflow(sessionID, 0)
	workflow3.FilePath = sql.NullString{String: "/different/path.yaml", Valid: true}
	if err := repo.Create(ctx, workflow3); err != nil {
		t.Fatalf("Create workflow3 returned error: %v", err)
	}

	// List by file path should return only the two with matching path.
	workflows, err := repo.ListByFilePath(ctx, filePath)
	if err != nil {
		t.Fatalf("ListByFilePath returned error: %v", err)
	}
	if len(workflows) != 2 {
		t.Fatalf("expected 2 workflows, got %d", len(workflows))
	}

	// Should be ordered by started_at DESC (most recent first).
	if workflows[0].ID != workflow2.ID {
		t.Errorf("expected most recent workflow first, got %q, want %q", workflows[0].ID, workflow2.ID)
	}
}

func TestWorkflowRepo_ListBySession(t *testing.T) {
	repo, _, _, sessionID, _ := setupWorkflowFixtures(t)
	ctx := context.Background()

	// Initially empty.
	workflows, err := repo.ListBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("ListBySession returned error: %v", err)
	}
	if len(workflows) != 0 {
		t.Errorf("expected empty list, got %d workflows", len(workflows))
	}

	// Create two workflows.
	workflow1 := domain.NewWorkflow(sessionID, 0)
	workflow1.StartedAt = sql.NullInt64{Int64: time.Now().Add(-time.Hour).Unix(), Valid: true}
	if err := repo.Create(ctx, workflow1); err != nil {
		t.Fatalf("Create workflow1 returned error: %v", err)
	}
	if err := repo.MarkCompleted(ctx, workflow1.ID); err != nil {
		t.Fatalf("MarkCompleted returned error: %v", err)
	}

	workflow2 := domain.NewWorkflow(sessionID, 0)
	workflow2.StartedAt = sql.NullInt64{Int64: time.Now().Unix(), Valid: true}
	if err := repo.Create(ctx, workflow2); err != nil {
		t.Fatalf("Create workflow2 returned error: %v", err)
	}

	// List should return both workflows.
	workflows, err = repo.ListBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("ListBySession returned error: %v", err)
	}
	if len(workflows) != 2 {
		t.Fatalf("expected 2 workflows, got %d", len(workflows))
	}
}

func TestWorkflowStatusString(t *testing.T) {
	tests := []struct {
		status   domain.WorkflowStatus
		expected string
	}{
		{domain.WorkflowStatusActive, "Active"},
		{domain.WorkflowStatusCompleted, "Completed"},
		{domain.WorkflowStatusCancelled, "Cancelled"},
		{domain.WorkflowStatus(99), "Unknown"},
	}

	for _, tc := range tests {
		got := tc.status.String()
		if got != tc.expected {
			t.Errorf("WorkflowStatus(%d).String() = %q, want %q", tc.status, got, tc.expected)
		}
	}
}
