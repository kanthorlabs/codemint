package sqlite

import (
	"context"
	"testing"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/util/idgen"
)

func setupPermissionFixtures(t *testing.T) (repo *projectPermissionRepo, projectID string) {
	t.Helper()
	db := openTestDB(t)
	repo = &projectPermissionRepo{db: db}
	ctx := context.Background()

	projectID = idgen.MustNew()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO project (id, name, working_dir, yolo_mode) VALUES (?, ?, ?, ?)`,
		projectID, "test-project-"+projectID[:8], "/tmp", 0,
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	return repo, projectID
}

// TestFindByProjectID_ReturnsNilForNonExistent asserts that FindByProjectID
// returns nil, nil when no permission record exists for the project.
func TestFindByProjectID_ReturnsNilForNonExistent(t *testing.T) {
	repo, _ := setupPermissionFixtures(t)
	ctx := context.Background()

	// Query for a non-existent project.
	perm, err := repo.FindByProjectID(ctx, idgen.MustNew())
	if err != nil {
		t.Fatalf("FindByProjectID returned error: %v", err)
	}
	if perm != nil {
		t.Errorf("expected nil, got %+v", perm)
	}
}

// TestFindByProjectID_ReturnsExistingRecord asserts that FindByProjectID
// returns the correct permission record when one exists.
func TestFindByProjectID_ReturnsExistingRecord(t *testing.T) {
	repo, projectID := setupPermissionFixtures(t)
	ctx := context.Background()

	// Insert a permission record directly.
	permID := idgen.MustNew()
	allowedCmds := domain.NullableJSON(`["go test", "go build"]`)
	allowedDirs := domain.NullableJSON(`["/src", "/pkg"]`)
	blockedCmds := domain.NullableJSON(`["rm -rf"]`)

	if _, err := repo.db.ExecContext(ctx,
		`INSERT INTO project_permission (id, project_id, allowed_commands, allowed_directories, blocked_commands)
		 VALUES (?, ?, ?, ?, ?)`,
		permID, projectID, allowedCmds, allowedDirs, blockedCmds,
	); err != nil {
		t.Fatalf("insert permission: %v", err)
	}

	perm, err := repo.FindByProjectID(ctx, projectID)
	if err != nil {
		t.Fatalf("FindByProjectID returned error: %v", err)
	}
	if perm == nil {
		t.Fatal("expected permission record, got nil")
	}
	if perm.ID != permID {
		t.Errorf("ID: got %q, want %q", perm.ID, permID)
	}
	if perm.ProjectID != projectID {
		t.Errorf("ProjectID: got %q, want %q", perm.ProjectID, projectID)
	}
	if string(perm.AllowedCommands) != string(allowedCmds) {
		t.Errorf("AllowedCommands: got %s, want %s", perm.AllowedCommands, allowedCmds)
	}
	if string(perm.AllowedDirectories) != string(allowedDirs) {
		t.Errorf("AllowedDirectories: got %s, want %s", perm.AllowedDirectories, allowedDirs)
	}
	if string(perm.BlockedCommands) != string(blockedCmds) {
		t.Errorf("BlockedCommands: got %s, want %s", perm.BlockedCommands, blockedCmds)
	}
}

// TestUpsert_CreatesNewRecord asserts that Upsert creates a new permission
// record when none exists for the project.
func TestUpsert_CreatesNewRecord(t *testing.T) {
	repo, projectID := setupPermissionFixtures(t)
	ctx := context.Background()

	perm := &domain.ProjectPermission{
		ID:                 idgen.MustNew(),
		ProjectID:          projectID,
		AllowedCommands:    domain.NullableJSON(`["go test"]`),
		AllowedDirectories: domain.NullableJSON(`["/safe"]`),
		BlockedCommands:    domain.NullableJSON(`["rm"]`),
	}

	if err := repo.Upsert(ctx, perm); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}

	// Verify the record was created.
	found, err := repo.FindByProjectID(ctx, projectID)
	if err != nil {
		t.Fatalf("FindByProjectID returned error: %v", err)
	}
	if found == nil {
		t.Fatal("expected permission record after Upsert, got nil")
	}
	if found.ID != perm.ID {
		t.Errorf("ID: got %q, want %q", found.ID, perm.ID)
	}
}

// TestUpsert_UpdatesExistingRecord asserts that Upsert updates the JSON fields
// when a record already exists for the project (idempotency).
func TestUpsert_UpdatesExistingRecord(t *testing.T) {
	repo, projectID := setupPermissionFixtures(t)
	ctx := context.Background()

	// First upsert.
	perm1 := &domain.ProjectPermission{
		ID:              idgen.MustNew(),
		ProjectID:       projectID,
		AllowedCommands: domain.NullableJSON(`["go test"]`),
	}
	if err := repo.Upsert(ctx, perm1); err != nil {
		t.Fatalf("first Upsert returned error: %v", err)
	}

	// Second upsert with different values (same project_id).
	perm2 := &domain.ProjectPermission{
		ID:                 idgen.MustNew(), // Different ID, but same project_id.
		ProjectID:          projectID,
		AllowedCommands:    domain.NullableJSON(`["go build", "go test"]`),
		AllowedDirectories: domain.NullableJSON(`["/new-dir"]`),
		BlockedCommands:    domain.NullableJSON(`["dangerous-cmd"]`),
	}
	if err := repo.Upsert(ctx, perm2); err != nil {
		t.Fatalf("second Upsert returned error: %v", err)
	}

	// Verify the record was updated (not duplicated).
	found, err := repo.FindByProjectID(ctx, projectID)
	if err != nil {
		t.Fatalf("FindByProjectID returned error: %v", err)
	}
	if found == nil {
		t.Fatal("expected permission record after Upsert, got nil")
	}

	// ID should remain the original (ON CONFLICT keeps existing row).
	if found.ID != perm1.ID {
		t.Errorf("ID should remain original %q, got %q", perm1.ID, found.ID)
	}

	// JSON fields should be updated to new values.
	if string(found.AllowedCommands) != string(perm2.AllowedCommands) {
		t.Errorf("AllowedCommands: got %s, want %s", found.AllowedCommands, perm2.AllowedCommands)
	}
	if string(found.AllowedDirectories) != string(perm2.AllowedDirectories) {
		t.Errorf("AllowedDirectories: got %s, want %s", found.AllowedDirectories, perm2.AllowedDirectories)
	}
	if string(found.BlockedCommands) != string(perm2.BlockedCommands) {
		t.Errorf("BlockedCommands: got %s, want %s", found.BlockedCommands, perm2.BlockedCommands)
	}

	// Verify only one record exists.
	var count int
	if err := repo.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM project_permission WHERE project_id = ?`, projectID,
	).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 permission record, got %d", count)
	}
}

// TestCascadeDelete_RemovesPermissionWhenProjectDeleted asserts that deleting
// a project also deletes its associated permission record (CASCADE).
func TestCascadeDelete_RemovesPermissionWhenProjectDeleted(t *testing.T) {
	repo, projectID := setupPermissionFixtures(t)
	ctx := context.Background()

	// Create a permission record.
	perm := &domain.ProjectPermission{
		ID:              idgen.MustNew(),
		ProjectID:       projectID,
		AllowedCommands: domain.NullableJSON(`["go test"]`),
	}
	if err := repo.Upsert(ctx, perm); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}

	// Verify the record exists.
	found, err := repo.FindByProjectID(ctx, projectID)
	if err != nil {
		t.Fatalf("FindByProjectID before delete returned error: %v", err)
	}
	if found == nil {
		t.Fatal("expected permission record before delete, got nil")
	}

	// Delete the project.
	if _, err := repo.db.ExecContext(ctx, `DELETE FROM project WHERE id = ?`, projectID); err != nil {
		t.Fatalf("delete project: %v", err)
	}

	// Verify the permission record was cascaded.
	found, err = repo.FindByProjectID(ctx, projectID)
	if err != nil {
		t.Fatalf("FindByProjectID after delete returned error: %v", err)
	}
	if found != nil {
		t.Errorf("expected nil after CASCADE delete, got %+v", found)
	}
}

// TestUpsert_WithNilJSONFields asserts that Upsert handles nil JSON fields.
func TestUpsert_WithNilJSONFields(t *testing.T) {
	repo, projectID := setupPermissionFixtures(t)
	ctx := context.Background()

	perm := &domain.ProjectPermission{
		ID:                 idgen.MustNew(),
		ProjectID:          projectID,
		AllowedCommands:    nil, // nil = no restriction
		AllowedDirectories: nil,
		BlockedCommands:    nil,
	}

	if err := repo.Upsert(ctx, perm); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}

	found, err := repo.FindByProjectID(ctx, projectID)
	if err != nil {
		t.Fatalf("FindByProjectID returned error: %v", err)
	}
	if found == nil {
		t.Fatal("expected permission record, got nil")
	}
	// nil JSON fields should be stored as NULL and returned as nil.
	if found.AllowedCommands != nil {
		t.Errorf("AllowedCommands: expected nil, got %s", found.AllowedCommands)
	}
	if found.AllowedDirectories != nil {
		t.Errorf("AllowedDirectories: expected nil, got %s", found.AllowedDirectories)
	}
	if found.BlockedCommands != nil {
		t.Errorf("BlockedCommands: expected nil, got %s", found.BlockedCommands)
	}
}
