package sqlite

import (
	"context"
	"testing"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/util/idgen"
)

func TestProjectRepo_Create(t *testing.T) {
	conn := openTestDB(t)
	repo := NewProjectRepo(conn)
	ctx := context.Background()

	project := domain.NewProject("test-project", "/tmp/workspace")

	err := repo.Create(ctx, project)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	// Verify the project was inserted.
	found, err := repo.FindByID(ctx, project.ID)
	if err != nil {
		t.Fatalf("FindByID returned error: %v", err)
	}
	if found == nil {
		t.Fatal("FindByID returned nil, expected project")
	}
	if found.ID != project.ID {
		t.Errorf("ID mismatch: got %q, want %q", found.ID, project.ID)
	}
	if found.Name != project.Name {
		t.Errorf("Name mismatch: got %q, want %q", found.Name, project.Name)
	}
	if found.WorkingDir != project.WorkingDir {
		t.Errorf("WorkingDir mismatch: got %q, want %q", found.WorkingDir, project.WorkingDir)
	}
	if found.YoloMode != project.YoloMode {
		t.Errorf("YoloMode mismatch: got %d, want %d", found.YoloMode, project.YoloMode)
	}
}

func TestProjectRepo_CreateDuplicateName(t *testing.T) {
	conn := openTestDB(t)
	repo := NewProjectRepo(conn)
	ctx := context.Background()

	project1 := domain.NewProject("duplicate-name", "/tmp/workspace1")
	project2 := domain.NewProject("duplicate-name", "/tmp/workspace2")
	project2.ID = idgen.MustNew() // Ensure different ID

	if err := repo.Create(ctx, project1); err != nil {
		t.Fatalf("Create first project returned error: %v", err)
	}

	err := repo.Create(ctx, project2)
	if err == nil {
		t.Fatal("Create duplicate name should have returned an error, got nil")
	}
	// Verify the error message contains context about the duplicate.
	if !contains(err.Error(), "duplicate-name") {
		t.Errorf("error should mention project name, got: %v", err)
	}
}

func TestProjectRepo_FindByID(t *testing.T) {
	conn := openTestDB(t)
	repo := NewProjectRepo(conn)
	ctx := context.Background()

	project := domain.NewProject("find-by-id-test", "/tmp/workspace")
	if err := repo.Create(ctx, project); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	found, err := repo.FindByID(ctx, project.ID)
	if err != nil {
		t.Fatalf("FindByID returned error: %v", err)
	}
	if found == nil {
		t.Fatal("FindByID returned nil, expected project")
	}
	if found.ID != project.ID {
		t.Errorf("ID mismatch: got %q, want %q", found.ID, project.ID)
	}
}

func TestProjectRepo_FindByID_NotFound(t *testing.T) {
	conn := openTestDB(t)
	repo := NewProjectRepo(conn)
	ctx := context.Background()

	found, err := repo.FindByID(ctx, idgen.MustNew())
	if err != nil {
		t.Fatalf("FindByID returned unexpected error: %v", err)
	}
	if found != nil {
		t.Errorf("expected nil for non-existent project, got %+v", found)
	}
}

func TestProjectRepo_FindByName(t *testing.T) {
	conn := openTestDB(t)
	repo := NewProjectRepo(conn)
	ctx := context.Background()

	project := domain.NewProject("find-by-name-test", "/tmp/workspace")
	if err := repo.Create(ctx, project); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	found, err := repo.FindByName(ctx, project.Name)
	if err != nil {
		t.Fatalf("FindByName returned error: %v", err)
	}
	if found == nil {
		t.Fatal("FindByName returned nil, expected project")
	}
	if found.Name != project.Name {
		t.Errorf("Name mismatch: got %q, want %q", found.Name, project.Name)
	}
}

func TestProjectRepo_FindByName_NotFound(t *testing.T) {
	conn := openTestDB(t)
	repo := NewProjectRepo(conn)
	ctx := context.Background()

	found, err := repo.FindByName(ctx, "nonexistent-project")
	if err != nil {
		t.Fatalf("FindByName returned unexpected error: %v", err)
	}
	if found != nil {
		t.Errorf("expected nil for non-existent project, got %+v", found)
	}
}

func TestProjectRepo_FindByName_CaseSensitive(t *testing.T) {
	conn := openTestDB(t)
	repo := NewProjectRepo(conn)
	ctx := context.Background()

	project := domain.NewProject("CaseSensitive", "/tmp/workspace")
	if err := repo.Create(ctx, project); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	// Search with different case should not find the project.
	found, err := repo.FindByName(ctx, "casesensitive")
	if err != nil {
		t.Fatalf("FindByName returned error: %v", err)
	}
	if found != nil {
		t.Errorf("FindByName should be case-sensitive; expected nil, got %+v", found)
	}

	// Search with exact case should find the project.
	found, err = repo.FindByName(ctx, "CaseSensitive")
	if err != nil {
		t.Fatalf("FindByName returned error: %v", err)
	}
	if found == nil {
		t.Fatal("FindByName with exact case should find project")
	}
}

func TestProjectRepo_Update(t *testing.T) {
	conn := openTestDB(t)
	repo := NewProjectRepo(conn)
	ctx := context.Background()

	project := domain.NewProject("update-test", "/tmp/original")
	if err := repo.Create(ctx, project); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	// Update the project.
	project.Name = "updated-name"
	project.WorkingDir = "/tmp/updated"
	project.YoloMode = int(domain.YoloModeOn)

	if err := repo.Update(ctx, project); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	// Verify the update was persisted.
	found, err := repo.FindByID(ctx, project.ID)
	if err != nil {
		t.Fatalf("FindByID returned error: %v", err)
	}
	if found.Name != "updated-name" {
		t.Errorf("Name not updated: got %q, want %q", found.Name, "updated-name")
	}
	if found.WorkingDir != "/tmp/updated" {
		t.Errorf("WorkingDir not updated: got %q, want %q", found.WorkingDir, "/tmp/updated")
	}
	if found.YoloMode != int(domain.YoloModeOn) {
		t.Errorf("YoloMode not updated: got %d, want %d", found.YoloMode, domain.YoloModeOn)
	}
}

func TestProjectRepo_Update_NotFound(t *testing.T) {
	conn := openTestDB(t)
	repo := NewProjectRepo(conn)
	ctx := context.Background()

	project := &domain.Project{
		ID:         idgen.MustNew(),
		Name:       "nonexistent",
		WorkingDir: "/tmp/workspace",
		YoloMode:   0,
	}

	err := repo.Update(ctx, project)
	if err == nil {
		t.Fatal("Update on non-existent project should have returned error")
	}
	if !contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
}

func TestProjectRepo_Update_DuplicateName(t *testing.T) {
	conn := openTestDB(t)
	repo := NewProjectRepo(conn)
	ctx := context.Background()

	project1 := domain.NewProject("project-one", "/tmp/workspace1")
	project2 := domain.NewProject("project-two", "/tmp/workspace2")

	if err := repo.Create(ctx, project1); err != nil {
		t.Fatalf("Create project1 returned error: %v", err)
	}
	if err := repo.Create(ctx, project2); err != nil {
		t.Fatalf("Create project2 returned error: %v", err)
	}

	// Try to rename project2 to project1's name.
	project2.Name = "project-one"
	err := repo.Update(ctx, project2)
	if err == nil {
		t.Fatal("Update with duplicate name should have returned error")
	}
}

func TestProjectRepo_Delete(t *testing.T) {
	conn := openTestDB(t)
	repo := NewProjectRepo(conn)
	ctx := context.Background()

	project := domain.NewProject("delete-test", "/tmp/workspace")
	if err := repo.Create(ctx, project); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	// Delete the project.
	if err := repo.Delete(ctx, project.ID); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	// Verify the project no longer exists.
	found, err := repo.FindByID(ctx, project.ID)
	if err != nil {
		t.Fatalf("FindByID returned error: %v", err)
	}
	if found != nil {
		t.Errorf("expected nil after delete, got %+v", found)
	}
}

func TestProjectRepo_Delete_NotFound(t *testing.T) {
	conn := openTestDB(t)
	repo := NewProjectRepo(conn)
	ctx := context.Background()

	err := repo.Delete(ctx, idgen.MustNew())
	if err == nil {
		t.Fatal("Delete on non-existent project should have returned error")
	}
	if !contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
}

func TestProjectRepo_List(t *testing.T) {
	conn := openTestDB(t)
	repo := NewProjectRepo(conn)
	ctx := context.Background()

	// Initially empty.
	projects, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("expected empty list, got %d projects", len(projects))
	}

	// Create multiple projects.
	project1 := domain.NewProject("alpha-project", "/tmp/alpha")
	project2 := domain.NewProject("beta-project", "/tmp/beta")
	project3 := domain.NewProject("gamma-project", "/tmp/gamma")

	for _, p := range []*domain.Project{project3, project1, project2} { // Insert out of order
		if err := repo.Create(ctx, p); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
	}

	// List should return all projects ordered by name.
	projects, err = repo.List(ctx)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(projects) != 3 {
		t.Fatalf("expected 3 projects, got %d", len(projects))
	}

	// Verify alphabetical order by name.
	expectedOrder := []string{"alpha-project", "beta-project", "gamma-project"}
	for i, expected := range expectedOrder {
		if projects[i].Name != expected {
			t.Errorf("project[%d]: got %q, want %q", i, projects[i].Name, expected)
		}
	}
}

func TestProjectRepo_Delete_CascadesToSessions(t *testing.T) {
	conn := openTestDB(t)
	projectRepo := NewProjectRepo(conn)
	sessionRepo := NewSessionRepo(conn)
	ctx := context.Background()

	// Create a project with a session.
	project := domain.NewProject("cascade-test", "/tmp/workspace")
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Create project returned error: %v", err)
	}

	session := domain.NewSession(project.ID)
	if err := sessionRepo.Create(ctx, session); err != nil {
		t.Fatalf("Create session returned error: %v", err)
	}

	// Delete the project - should cascade to session.
	if err := projectRepo.Delete(ctx, project.ID); err != nil {
		t.Fatalf("Delete project returned error: %v", err)
	}

	// Verify session was also deleted.
	found, err := sessionRepo.FindByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("FindByID session returned error: %v", err)
	}
	if found != nil {
		t.Errorf("session should have been deleted by cascade, got %+v", found)
	}
}
