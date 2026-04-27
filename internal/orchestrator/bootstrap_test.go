package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"

	_ "modernc.org/sqlite"

	"codemint.kanthorlabs.com/internal/db"
	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/repository/sqlite"
)

// openTestDB creates an in-memory SQLite DB and runs migrations.
func openTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	conn, err := sqlx.Connect("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if _, err := conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	goose.SetBaseFS(db.Migrations)
	goose.SetDialect("sqlite3") //nolint:errcheck
	if err := goose.Up(conn.DB, "migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestEnsureCodeMintProject_FreshDB(t *testing.T) {
	conn := openTestDB(t)
	projectRepo := sqlite.NewProjectRepo(conn)
	sessionRepo := sqlite.NewSessionRepo(conn)
	permRepo := sqlite.NewProjectPermissionRepo(conn)
	ctx := context.Background()

	// Create a temp directory for workspace.
	tmpDir := t.TempDir()
	workspaceDir := filepath.Join(tmpDir, "workspace")

	// Run bootstrap on fresh DB.
	err := EnsureCodeMintProject(ctx, workspaceDir, projectRepo, sessionRepo, permRepo)
	if err != nil {
		t.Fatalf("EnsureCodeMintProject returned error: %v", err)
	}

	// Assert project exists with kind=codemint.
	project, err := projectRepo.FindByName(ctx, CodeMintProjectName)
	if err != nil {
		t.Fatalf("FindByName returned error: %v", err)
	}
	if project == nil {
		t.Fatal("expected codemint project to exist")
	}
	if project.Kind != domain.ProjectKindCodeMint {
		t.Errorf("kind = %q, want %q", project.Kind, domain.ProjectKindCodeMint)
	}
	if project.WorkingDir != workspaceDir {
		t.Errorf("working_dir = %q, want %q", project.WorkingDir, workspaceDir)
	}

	// Assert permission row exists (all NULLs).
	perm, err := permRepo.FindByProjectID(ctx, project.ID)
	if err != nil {
		t.Fatalf("FindByProjectID returned error: %v", err)
	}
	if perm == nil {
		t.Fatal("expected permission row to exist")
	}
	if perm.AllowedCommands != nil {
		t.Errorf("allowed_commands should be NULL, got %v", perm.AllowedCommands)
	}

	// Assert active session exists.
	count, err := sessionRepo.CountActiveByProjectID(ctx, project.ID)
	if err != nil {
		t.Fatalf("CountActiveByProjectID returned error: %v", err)
	}
	if count != 1 {
		t.Errorf("active session count = %d, want 1", count)
	}

	// Assert workspace directory exists on disk.
	if _, err := os.Stat(workspaceDir); os.IsNotExist(err) {
		t.Errorf("workspace directory should exist on disk")
	}
}

func TestEnsureCodeMintProject_Idempotent(t *testing.T) {
	conn := openTestDB(t)
	projectRepo := sqlite.NewProjectRepo(conn)
	sessionRepo := sqlite.NewSessionRepo(conn)
	permRepo := sqlite.NewProjectPermissionRepo(conn)
	ctx := context.Background()

	tmpDir := t.TempDir()
	workspaceDir := filepath.Join(tmpDir, "workspace")

	// Run bootstrap twice.
	if err := EnsureCodeMintProject(ctx, workspaceDir, projectRepo, sessionRepo, permRepo); err != nil {
		t.Fatalf("first EnsureCodeMintProject returned error: %v", err)
	}
	if err := EnsureCodeMintProject(ctx, workspaceDir, projectRepo, sessionRepo, permRepo); err != nil {
		t.Fatalf("second EnsureCodeMintProject returned error: %v", err)
	}

	// Assert only one project exists.
	projects, err := projectRepo.List(ctx)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(projects) != 1 {
		t.Errorf("project count = %d, want 1", len(projects))
	}

	// Assert only one active session exists.
	project, _ := projectRepo.FindByName(ctx, CodeMintProjectName)
	count, err := sessionRepo.CountActiveByProjectID(ctx, project.ID)
	if err != nil {
		t.Fatalf("CountActiveByProjectID returned error: %v", err)
	}
	if count != 1 {
		t.Errorf("active session count = %d, want 1", count)
	}
}

func TestEnsureCodeMintProject_CreatesNewSessionWhenNoneActive(t *testing.T) {
	conn := openTestDB(t)
	projectRepo := sqlite.NewProjectRepo(conn)
	sessionRepo := sqlite.NewSessionRepo(conn)
	permRepo := sqlite.NewProjectPermissionRepo(conn)
	ctx := context.Background()

	tmpDir := t.TempDir()
	workspaceDir := filepath.Join(tmpDir, "workspace")

	// Run bootstrap to create project and session.
	if err := EnsureCodeMintProject(ctx, workspaceDir, projectRepo, sessionRepo, permRepo); err != nil {
		t.Fatalf("first EnsureCodeMintProject returned error: %v", err)
	}

	// Archive the session.
	project, _ := projectRepo.FindByName(ctx, CodeMintProjectName)
	session, _ := sessionRepo.FindActiveByProjectID(ctx, project.ID)
	if err := sessionRepo.Archive(ctx, session.ID); err != nil {
		t.Fatalf("Archive returned error: %v", err)
	}

	// Verify no active session.
	count, _ := sessionRepo.CountActiveByProjectID(ctx, project.ID)
	if count != 0 {
		t.Fatalf("expected 0 active sessions after archive, got %d", count)
	}

	// Re-run bootstrap - should create a new session.
	if err := EnsureCodeMintProject(ctx, workspaceDir, projectRepo, sessionRepo, permRepo); err != nil {
		t.Fatalf("second EnsureCodeMintProject returned error: %v", err)
	}

	// Assert exactly one new active session.
	count, err := sessionRepo.CountActiveByProjectID(ctx, project.ID)
	if err != nil {
		t.Fatalf("CountActiveByProjectID returned error: %v", err)
	}
	if count != 1 {
		t.Errorf("active session count = %d, want 1", count)
	}
}

func TestEnsureCodeMintProject_PreservesUserActiveSessions(t *testing.T) {
	conn := openTestDB(t)
	projectRepo := sqlite.NewProjectRepo(conn)
	sessionRepo := sqlite.NewSessionRepo(conn)
	permRepo := sqlite.NewProjectPermissionRepo(conn)
	ctx := context.Background()

	tmpDir := t.TempDir()
	workspaceDir := filepath.Join(tmpDir, "workspace")

	// Create a user project with an active session.
	userProject := domain.NewProject("user-project", "/tmp/user", domain.ProjectKindCoding)
	if err := projectRepo.Create(ctx, userProject); err != nil {
		t.Fatalf("Create user project returned error: %v", err)
	}
	userSession := domain.NewSession(userProject.ID)
	if err := sessionRepo.Create(ctx, userSession); err != nil {
		t.Fatalf("Create user session returned error: %v", err)
	}

	// Run bootstrap.
	if err := EnsureCodeMintProject(ctx, workspaceDir, projectRepo, sessionRepo, permRepo); err != nil {
		t.Fatalf("EnsureCodeMintProject returned error: %v", err)
	}

	// Assert user session is untouched.
	session, err := sessionRepo.FindByID(ctx, userSession.ID)
	if err != nil {
		t.Fatalf("FindByID returned error: %v", err)
	}
	if session == nil {
		t.Fatal("user session should still exist")
	}
	if session.Status != domain.SessionStatusActive {
		t.Errorf("user session status = %d, want Active (0)", session.Status)
	}
}

func TestEnsureCodeMintProject_WorkspaceDirAlreadyExists(t *testing.T) {
	conn := openTestDB(t)
	projectRepo := sqlite.NewProjectRepo(conn)
	sessionRepo := sqlite.NewSessionRepo(conn)
	permRepo := sqlite.NewProjectPermissionRepo(conn)
	ctx := context.Background()

	tmpDir := t.TempDir()
	workspaceDir := filepath.Join(tmpDir, "workspace")

	// Pre-create the workspace directory.
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("pre-mkdir returned error: %v", err)
	}

	// Bootstrap should succeed (no error on EEXIST).
	if err := EnsureCodeMintProject(ctx, workspaceDir, projectRepo, sessionRepo, permRepo); err != nil {
		t.Fatalf("EnsureCodeMintProject returned error: %v", err)
	}
}
