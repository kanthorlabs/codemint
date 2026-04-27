package repl

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"codemint.kanthorlabs.com/internal/db"
	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/registry"
	"codemint.kanthorlabs.com/internal/repository/sqlite"
)

func TestProjectOpen_RequiresPath(t *testing.T) {
	deps := &ProjectCommandDeps{}
	handler := projectOpenHandler(deps)

	result, err := handler(context.Background(), nil, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Message == "" || result.Action != registry.ActionNone {
		t.Errorf("expected usage message, got %q", result.Message)
	}
}

func TestProjectOpen_DirectoryNotFound(t *testing.T) {
	deps := &ProjectCommandDeps{}
	handler := projectOpenHandler(deps)

	result, err := handler(context.Background(), nil, []string{"/nonexistent/path/xyz"}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Message != "Directory not found: /nonexistent/path/xyz" {
		t.Errorf("expected 'Directory not found' message, got %q", result.Message)
	}
}

func TestProjectOpen_NotADirectory(t *testing.T) {
	// Create a temporary file (not a directory).
	tmpfile, err := os.CreateTemp("", "testfile")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	deps := &ProjectCommandDeps{}
	handler := projectOpenHandler(deps)

	result, err := handler(context.Background(), nil, []string{tmpfile.Name()}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Message == "" {
		t.Errorf("expected 'not a directory' message, got empty")
	}
}

func TestProjectOpen_NotGitRepository(t *testing.T) {
	// Create a temporary directory without git.
	tmpdir, err := os.MkdirTemp("", "testdir")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	deps := &ProjectCommandDeps{}
	handler := projectOpenHandler(deps)

	result, err := handler(context.Background(), nil, []string{tmpdir}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Message == "" {
		t.Errorf("expected 'not a git repository' message, got empty")
	}
}

func TestProjectOpen_CreatesProject(t *testing.T) {
	// Create a temporary git repository.
	tmpdir, err := os.MkdirTemp("", "testgit")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	// Initialize git.
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpdir
	if err := cmd.Run(); err != nil {
		t.Skipf("git not available: %v", err)
	}

	// Set up in-memory database.
	database, err := db.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if err := db.RunMigrations(database); err != nil {
		t.Fatal(err)
	}

	projectRepo := sqlite.NewProjectRepo(database)
	sessionRepo := sqlite.NewSessionRepo(database)
	permissionRepo := sqlite.NewProjectPermissionRepo(database)
	activeSession := &mockMutableSession{}

	deps := &ProjectCommandDeps{
		ProjectRepo:    projectRepo,
		SessionRepo:    sessionRepo,
		PermissionRepo: permissionRepo,
		ActiveSession:  activeSession,
	}
	handler := projectOpenHandler(deps)

	ctx := context.Background()
	result, err := handler(ctx, nil, []string{tmpdir}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have created the project.
	if result.Message == "" {
		t.Errorf("expected success message, got empty")
	}

	// Verify project was created in database.
	projectName := filepath.Base(tmpdir)
	project, err := projectRepo.FindByName(ctx, projectName)
	if err != nil {
		t.Fatalf("find project: %v", err)
	}
	if project == nil {
		t.Fatal("project should exist in database")
	}
	if project.Kind != domain.ProjectKindCoding {
		t.Errorf("project kind = %s, want %s", project.Kind, domain.ProjectKindCoding)
	}
	if project.WorkingDir != tmpdir {
		t.Errorf("project working dir = %s, want %s", project.WorkingDir, tmpdir)
	}

	// Verify session was created.
	session, err := sessionRepo.FindActiveByProjectID(ctx, project.ID)
	if err != nil {
		t.Fatalf("find session: %v", err)
	}
	if session == nil {
		t.Fatal("session should exist")
	}

	// Verify active session was updated.
	if !activeSession.sessionSet {
		t.Error("SetSession should have been called")
	}
}

func TestProjectOpen_ReusesExistingProject(t *testing.T) {
	// Create a temporary git repository.
	tmpdir, err := os.MkdirTemp("", "testgit")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	// Initialize git.
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpdir
	if err := cmd.Run(); err != nil {
		t.Skipf("git not available: %v", err)
	}

	// Set up in-memory database.
	database, err := db.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if err := db.RunMigrations(database); err != nil {
		t.Fatal(err)
	}

	projectRepo := sqlite.NewProjectRepo(database)
	sessionRepo := sqlite.NewSessionRepo(database)
	permissionRepo := sqlite.NewProjectPermissionRepo(database)
	activeSession := &mockMutableSession{}

	// Pre-create the project.
	projectName := filepath.Base(tmpdir)
	project := domain.NewProject(projectName, tmpdir, domain.ProjectKindCoding)
	ctx := context.Background()
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatal(err)
	}

	deps := &ProjectCommandDeps{
		ProjectRepo:    projectRepo,
		SessionRepo:    sessionRepo,
		PermissionRepo: permissionRepo,
		ActiveSession:  activeSession,
	}
	handler := projectOpenHandler(deps)

	result, err := handler(ctx, nil, []string{tmpdir}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should indicate switched, not created.
	if result.Message == "" {
		t.Errorf("expected success message, got empty")
	}

	// Verify no duplicate project was created.
	projects, err := projectRepo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, p := range projects {
		if p.Name == projectName {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 project with name %q, got %d", projectName, count)
	}
}

func TestProjectList_Empty(t *testing.T) {
	// Set up in-memory database.
	database, err := db.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if err := db.RunMigrations(database); err != nil {
		t.Fatal(err)
	}

	projectRepo := sqlite.NewProjectRepo(database)

	deps := &ProjectCommandDeps{
		ProjectRepo: projectRepo,
	}
	handler := projectListHandler(deps)

	result, err := handler(context.Background(), nil, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Message != "No projects registered. Use /project-open <path> to create one." {
		t.Errorf("unexpected message: %q", result.Message)
	}
}

func TestProjectList_ShowsProjects(t *testing.T) {
	// Set up in-memory database.
	database, err := db.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if err := db.RunMigrations(database); err != nil {
		t.Fatal(err)
	}

	projectRepo := sqlite.NewProjectRepo(database)
	ctx := context.Background()

	// Create projects.
	p1 := domain.NewProject("project-a", "/path/a", domain.ProjectKindCoding)
	p2 := domain.NewProject("project-b", "/path/b", domain.ProjectKindCodeMint)
	projectRepo.Create(ctx, p1)
	projectRepo.Create(ctx, p2)

	deps := &ProjectCommandDeps{
		ProjectRepo: projectRepo,
	}
	handler := projectListHandler(deps)

	result, err := handler(ctx, nil, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
}

// mockMutableSession implements registry.MutableSessionInfo for testing.
type mockMutableSession struct {
	sessionSet bool
	clientMode registry.ClientMode
	projectID  string
}

func (m *mockMutableSession) GetClientMode() registry.ClientMode {
	return m.clientMode
}

func (m *mockMutableSession) GetIsCodeMint() bool {
	return false
}

func (m *mockMutableSession) GetSessionID() string {
	return ""
}

func (m *mockMutableSession) GetProjectID() string {
	return m.projectID
}

func (m *mockMutableSession) GetClientID() string {
	return "test-client"
}

func (m *mockMutableSession) SetSession(session any, project any, yoloEnabled bool) {
	m.sessionSet = true
}

func (m *mockMutableSession) SetSuspended(suspended bool) {}

func (m *mockMutableSession) SetClientMode(mode registry.ClientMode) {
	m.clientMode = mode
}

// mockProviderRegistry implements ProviderLister for testing.
type mockProviderRegistry struct {
	names []string
}

func (m *mockProviderRegistry) Names() []string {
	return m.names
}

func TestProjectAssistant_NoActiveProject(t *testing.T) {
	activeSession := &mockMutableSession{projectID: ""} // No project

	deps := &ProjectCommandDeps{
		ActiveSession: activeSession,
	}
	handler := projectAssistantHandler(deps)

	result, err := handler(context.Background(), nil, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Message != "No active project. Use /project-open to open a project first." {
		t.Errorf("unexpected message: %q", result.Message)
	}
}

func TestProjectAssistant_ShowsCurrentBinding(t *testing.T) {
	// Set up in-memory database.
	database, err := db.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if err := db.RunMigrations(database); err != nil {
		t.Fatal(err)
	}

	projectRepo := sqlite.NewProjectRepo(database)
	ctx := context.Background()

	// Create a project.
	project := domain.NewProject("test-project", "/path/test", domain.ProjectKindCoding)
	projectRepo.Create(ctx, project)

	activeSession := &mockMutableSession{projectID: project.ID}

	deps := &ProjectCommandDeps{
		ProjectRepo:   projectRepo,
		ActiveSession: activeSession,
	}
	handler := projectAssistantHandler(deps)

	result, err := handler(ctx, nil, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
	if !strings.Contains(result.Message, "test-project") {
		t.Errorf("expected project name in message, got: %q", result.Message)
	}
}

func TestProjectAssistant_SetProvider(t *testing.T) {
	// Set up in-memory database.
	database, err := db.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if err := db.RunMigrations(database); err != nil {
		t.Fatal(err)
	}

	projectRepo := sqlite.NewProjectRepo(database)
	ctx := context.Background()

	// Create a project.
	project := domain.NewProject("test-project", "/path/test", domain.ProjectKindCoding)
	projectRepo.Create(ctx, project)

	activeSession := &mockMutableSession{projectID: project.ID}
	providerRegistry := &mockProviderRegistry{names: []string{"opencode", "codex", "claude-code"}}

	deps := &ProjectCommandDeps{
		ProjectRepo:      projectRepo,
		ActiveSession:    activeSession,
		ProviderRegistry: providerRegistry,
	}
	handler := projectAssistantHandler(deps)

	result, err := handler(ctx, nil, []string{"codex"}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Message, "codex") {
		t.Errorf("expected provider name in message, got: %q", result.Message)
	}

	// Verify database was updated.
	updated, _ := projectRepo.FindByID(ctx, project.ID)
	if !updated.AssistantProvider.Valid || updated.AssistantProvider.String != "codex" {
		t.Errorf("expected assistant_provider to be 'codex', got: %v", updated.AssistantProvider)
	}
}

func TestProjectAssistant_ResetProvider(t *testing.T) {
	// Set up in-memory database.
	database, err := db.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if err := db.RunMigrations(database); err != nil {
		t.Fatal(err)
	}

	projectRepo := sqlite.NewProjectRepo(database)
	ctx := context.Background()

	// Create a project with provider set.
	project := domain.NewProject("test-project", "/path/test", domain.ProjectKindCoding)
	projectRepo.Create(ctx, project)
	projectRepo.UpdateAssistantBinding(ctx, project.ID, "codex", "")

	activeSession := &mockMutableSession{projectID: project.ID}

	deps := &ProjectCommandDeps{
		ProjectRepo:   projectRepo,
		ActiveSession: activeSession,
	}
	handler := projectAssistantHandler(deps)

	result, err := handler(ctx, nil, []string{"reset"}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Message, "Cleared") {
		t.Errorf("expected cleared message, got: %q", result.Message)
	}

	// Verify database was updated.
	updated, _ := projectRepo.FindByID(ctx, project.ID)
	if updated.AssistantProvider.Valid && updated.AssistantProvider.String != "" {
		t.Errorf("expected assistant_provider to be cleared, got: %v", updated.AssistantProvider)
	}
}

func TestProjectAssistant_CodeMintProjectNotSupported(t *testing.T) {
	// Set up in-memory database.
	database, err := db.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if err := db.RunMigrations(database); err != nil {
		t.Fatal(err)
	}

	projectRepo := sqlite.NewProjectRepo(database)
	ctx := context.Background()

	// Create a CodeMint project.
	project := domain.NewProject("codemint", "/path/codemint", domain.ProjectKindCodeMint)
	projectRepo.Create(ctx, project)

	activeSession := &mockMutableSession{projectID: project.ID}

	deps := &ProjectCommandDeps{
		ProjectRepo:   projectRepo,
		ActiveSession: activeSession,
	}
	handler := projectAssistantHandler(deps)

	result, err := handler(ctx, nil, []string{"codex"}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Message, "not supported") {
		t.Errorf("expected 'not supported' message, got: %q", result.Message)
	}
}

func TestProjectAssistant_UnknownProvider(t *testing.T) {
	// Set up in-memory database.
	database, err := db.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if err := db.RunMigrations(database); err != nil {
		t.Fatal(err)
	}

	projectRepo := sqlite.NewProjectRepo(database)
	ctx := context.Background()

	// Create a project.
	project := domain.NewProject("test-project", "/path/test", domain.ProjectKindCoding)
	projectRepo.Create(ctx, project)

	activeSession := &mockMutableSession{projectID: project.ID}
	providerRegistry := &mockProviderRegistry{names: []string{"opencode", "codex"}}

	deps := &ProjectCommandDeps{
		ProjectRepo:      projectRepo,
		ActiveSession:    activeSession,
		ProviderRegistry: providerRegistry,
	}
	handler := projectAssistantHandler(deps)

	result, err := handler(ctx, nil, []string{"unknown-provider"}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Message, "Unknown provider") {
		t.Errorf("expected 'Unknown provider' message, got: %q", result.Message)
	}
}
