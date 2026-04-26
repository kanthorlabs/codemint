package sqlite

import (
	"context"
	"errors"
	"testing"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/repository"
	"codemint.kanthorlabs.com/internal/util/idgen"
)

func setupSessionFixtures(t *testing.T) (sessionRepo repository.SessionRepository, projectRepo repository.ProjectRepository, projectID string) {
	t.Helper()
	conn := openTestDB(t)
	sessionRepo = NewSessionRepo(conn)
	projectRepo = NewProjectRepo(conn)
	ctx := context.Background()

	// Create a project for sessions to reference.
	project := domain.NewProject("session-test-project", "/tmp/workspace")
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Create project returned error: %v", err)
	}
	return sessionRepo, projectRepo, project.ID
}

func TestSessionRepo_Create(t *testing.T) {
	repo, _, projectID := setupSessionFixtures(t)
	ctx := context.Background()

	session := domain.NewSession(projectID)
	err := repo.Create(ctx, session)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	// Verify the session was inserted.
	found, err := repo.FindByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("FindByID returned error: %v", err)
	}
	if found == nil {
		t.Fatal("FindByID returned nil, expected session")
	}
	if found.ID != session.ID {
		t.Errorf("ID mismatch: got %q, want %q", found.ID, session.ID)
	}
	if found.ProjectID != projectID {
		t.Errorf("ProjectID mismatch: got %q, want %q", found.ProjectID, projectID)
	}
	if found.Status != domain.SessionStatusActive {
		t.Errorf("Status mismatch: got %d, want %d (Active)", found.Status, domain.SessionStatusActive)
	}
}

func TestSessionRepo_CreateWhileActiveExists(t *testing.T) {
	repo, _, projectID := setupSessionFixtures(t)
	ctx := context.Background()

	// Create first active session.
	session1 := domain.NewSession(projectID)
	if err := repo.Create(ctx, session1); err != nil {
		t.Fatalf("Create first session returned error: %v", err)
	}

	// Attempt to create second active session for the same project.
	session2 := domain.NewSession(projectID)
	err := repo.Create(ctx, session2)

	if err == nil {
		t.Fatal("Create second active session should have returned error")
	}
	if !errors.Is(err, repository.ErrActiveSessionExists) {
		t.Errorf("expected ErrActiveSessionExists, got: %v", err)
	}
}

func TestSessionRepo_FindByID(t *testing.T) {
	repo, _, projectID := setupSessionFixtures(t)
	ctx := context.Background()

	session := domain.NewSession(projectID)
	if err := repo.Create(ctx, session); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	found, err := repo.FindByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("FindByID returned error: %v", err)
	}
	if found == nil {
		t.Fatal("FindByID returned nil, expected session")
	}
	if found.ID != session.ID {
		t.Errorf("ID mismatch: got %q, want %q", found.ID, session.ID)
	}
}

func TestSessionRepo_FindByID_NotFound(t *testing.T) {
	repo, _, _ := setupSessionFixtures(t)
	ctx := context.Background()

	found, err := repo.FindByID(ctx, idgen.MustNew())
	if err != nil {
		t.Fatalf("FindByID returned unexpected error: %v", err)
	}
	if found != nil {
		t.Errorf("expected nil for non-existent session, got %+v", found)
	}
}

func TestSessionRepo_FindActiveByProjectID(t *testing.T) {
	repo, _, projectID := setupSessionFixtures(t)
	ctx := context.Background()

	session := domain.NewSession(projectID)
	if err := repo.Create(ctx, session); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	found, err := repo.FindActiveByProjectID(ctx, projectID)
	if err != nil {
		t.Fatalf("FindActiveByProjectID returned error: %v", err)
	}
	if found == nil {
		t.Fatal("FindActiveByProjectID returned nil, expected session")
	}
	if found.ID != session.ID {
		t.Errorf("ID mismatch: got %q, want %q", found.ID, session.ID)
	}
	if found.Status != domain.SessionStatusActive {
		t.Errorf("Status mismatch: got %d, want %d (Active)", found.Status, domain.SessionStatusActive)
	}
}

func TestSessionRepo_FindActiveByProjectID_NoActive(t *testing.T) {
	repo, _, projectID := setupSessionFixtures(t)
	ctx := context.Background()

	// No active session exists.
	found, err := repo.FindActiveByProjectID(ctx, projectID)
	if err != nil {
		t.Fatalf("FindActiveByProjectID returned unexpected error: %v", err)
	}
	if found != nil {
		t.Errorf("expected nil when no active session exists, got %+v", found)
	}
}

func TestSessionRepo_FindActiveByProjectID_AfterArchive(t *testing.T) {
	repo, _, projectID := setupSessionFixtures(t)
	ctx := context.Background()

	session := domain.NewSession(projectID)
	if err := repo.Create(ctx, session); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	// Archive the session.
	if err := repo.Archive(ctx, session.ID); err != nil {
		t.Fatalf("Archive returned error: %v", err)
	}

	// Should return nil now that session is archived.
	found, err := repo.FindActiveByProjectID(ctx, projectID)
	if err != nil {
		t.Fatalf("FindActiveByProjectID returned error: %v", err)
	}
	if found != nil {
		t.Errorf("expected nil after archive, got %+v", found)
	}
}

func TestSessionRepo_Archive(t *testing.T) {
	repo, _, projectID := setupSessionFixtures(t)
	ctx := context.Background()

	session := domain.NewSession(projectID)
	if err := repo.Create(ctx, session); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	// Archive the session.
	if err := repo.Archive(ctx, session.ID); err != nil {
		t.Fatalf("Archive returned error: %v", err)
	}

	// Verify the status changed.
	found, err := repo.FindByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("FindByID returned error: %v", err)
	}
	if found.Status != domain.SessionStatusArchived {
		t.Errorf("Status mismatch: got %d, want %d (Archived)", found.Status, domain.SessionStatusArchived)
	}
}

func TestSessionRepo_Archive_NotFound(t *testing.T) {
	repo, _, _ := setupSessionFixtures(t)
	ctx := context.Background()

	err := repo.Archive(ctx, idgen.MustNew())
	if err == nil {
		t.Fatal("Archive on non-existent session should have returned error")
	}
	if !contains(err.Error(), "not found") && !contains(err.Error(), "already archived") {
		t.Errorf("error should mention 'not found' or 'already archived', got: %v", err)
	}
}

func TestSessionRepo_Archive_AlreadyArchived(t *testing.T) {
	repo, _, projectID := setupSessionFixtures(t)
	ctx := context.Background()

	session := domain.NewSession(projectID)
	if err := repo.Create(ctx, session); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	// Archive once.
	if err := repo.Archive(ctx, session.ID); err != nil {
		t.Fatalf("First Archive returned error: %v", err)
	}

	// Archive again should fail.
	err := repo.Archive(ctx, session.ID)
	if err == nil {
		t.Fatal("Archive on already-archived session should have returned error")
	}
}

func TestSessionRepo_ArchiveThenCreateNew(t *testing.T) {
	repo, _, projectID := setupSessionFixtures(t)
	ctx := context.Background()

	// Create and archive first session.
	session1 := domain.NewSession(projectID)
	if err := repo.Create(ctx, session1); err != nil {
		t.Fatalf("Create session1 returned error: %v", err)
	}
	if err := repo.Archive(ctx, session1.ID); err != nil {
		t.Fatalf("Archive session1 returned error: %v", err)
	}

	// Should now be able to create a new active session.
	session2 := domain.NewSession(projectID)
	if err := repo.Create(ctx, session2); err != nil {
		t.Fatalf("Create session2 after archive returned error: %v", err)
	}

	// Verify the new session is active.
	found, err := repo.FindActiveByProjectID(ctx, projectID)
	if err != nil {
		t.Fatalf("FindActiveByProjectID returned error: %v", err)
	}
	if found == nil {
		t.Fatal("expected active session after archive+create")
	}
	if found.ID != session2.ID {
		t.Errorf("active session ID mismatch: got %q, want %q", found.ID, session2.ID)
	}
}

func TestSessionRepo_ListByProjectID(t *testing.T) {
	repo, _, projectID := setupSessionFixtures(t)
	ctx := context.Background()

	// Initially empty.
	sessions, err := repo.ListByProjectID(ctx, projectID)
	if err != nil {
		t.Fatalf("ListByProjectID returned error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected empty list, got %d sessions", len(sessions))
	}

	// Create session, archive, create another.
	session1 := domain.NewSession(projectID)
	if err := repo.Create(ctx, session1); err != nil {
		t.Fatalf("Create session1 returned error: %v", err)
	}
	if err := repo.Archive(ctx, session1.ID); err != nil {
		t.Fatalf("Archive session1 returned error: %v", err)
	}

	session2 := domain.NewSession(projectID)
	if err := repo.Create(ctx, session2); err != nil {
		t.Fatalf("Create session2 returned error: %v", err)
	}

	// List should return both sessions.
	sessions, err = repo.ListByProjectID(ctx, projectID)
	if err != nil {
		t.Fatalf("ListByProjectID returned error: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestSessionRepo_ListByProjectID_FiltersByProject(t *testing.T) {
	conn := openTestDB(t)
	sessionRepo := NewSessionRepo(conn)
	projectRepo := NewProjectRepo(conn)
	ctx := context.Background()

	// Create two projects.
	project1 := domain.NewProject("project1", "/tmp/p1")
	project2 := domain.NewProject("project2", "/tmp/p2")
	if err := projectRepo.Create(ctx, project1); err != nil {
		t.Fatalf("Create project1 returned error: %v", err)
	}
	if err := projectRepo.Create(ctx, project2); err != nil {
		t.Fatalf("Create project2 returned error: %v", err)
	}

	// Create session for each project.
	session1 := domain.NewSession(project1.ID)
	session2 := domain.NewSession(project2.ID)
	if err := sessionRepo.Create(ctx, session1); err != nil {
		t.Fatalf("Create session1 returned error: %v", err)
	}
	if err := sessionRepo.Create(ctx, session2); err != nil {
		t.Fatalf("Create session2 returned error: %v", err)
	}

	// List for project1 should only return its session.
	sessions, err := sessionRepo.ListByProjectID(ctx, project1.ID)
	if err != nil {
		t.Fatalf("ListByProjectID returned error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session for project1, got %d", len(sessions))
	}
	if sessions[0].ID != session1.ID {
		t.Errorf("session ID mismatch: got %q, want %q", sessions[0].ID, session1.ID)
	}
}

func TestSessionRepo_ActiveSessionAcrossProjects(t *testing.T) {
	conn := openTestDB(t)
	sessionRepo := NewSessionRepo(conn)
	projectRepo := NewProjectRepo(conn)
	ctx := context.Background()

	// Create two projects.
	project1 := domain.NewProject("project-a", "/tmp/a")
	project2 := domain.NewProject("project-b", "/tmp/b")
	if err := projectRepo.Create(ctx, project1); err != nil {
		t.Fatalf("Create project1 returned error: %v", err)
	}
	if err := projectRepo.Create(ctx, project2); err != nil {
		t.Fatalf("Create project2 returned error: %v", err)
	}

	// Each project can have its own active session.
	session1 := domain.NewSession(project1.ID)
	session2 := domain.NewSession(project2.ID)

	if err := sessionRepo.Create(ctx, session1); err != nil {
		t.Fatalf("Create session for project1 returned error: %v", err)
	}
	if err := sessionRepo.Create(ctx, session2); err != nil {
		t.Fatalf("Create session for project2 returned error: %v", err)
	}

	// Both should be findable as active.
	found1, err := sessionRepo.FindActiveByProjectID(ctx, project1.ID)
	if err != nil {
		t.Fatalf("FindActiveByProjectID project1 returned error: %v", err)
	}
	if found1 == nil {
		t.Error("expected active session for project1")
	}

	found2, err := sessionRepo.FindActiveByProjectID(ctx, project2.ID)
	if err != nil {
		t.Fatalf("FindActiveByProjectID project2 returned error: %v", err)
	}
	if found2 == nil {
		t.Error("expected active session for project2")
	}
}
