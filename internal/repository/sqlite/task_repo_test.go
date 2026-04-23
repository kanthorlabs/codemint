package sqlite

import (
	"context"
	"errors"
	"testing"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/util/idgen"
)

func setupTaskFixtures(t *testing.T) (repo *taskRepo, projectID, sessionID, agentID string) {
	t.Helper()
	db := openTestDB(t)
	repo = &taskRepo{db: db}
	ctx := context.Background()

	projectID = idgen.MustNew()
	sessionID = idgen.MustNew()
	agentID = idgen.MustNew()

	if _, err := db.ExecContext(ctx,
		`INSERT INTO project (id, name, working_dir, yolo_mode) VALUES (?, ?, ?, ?)`,
		projectID, "test-project", "/tmp", 0,
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO agent (id, name, type, assistant) VALUES (?, ?, ?, ?)`,
		agentID, "human", 0, "",
	); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO session (id, project_id, status) VALUES (?, ?, ?)`,
		sessionID, projectID, 0,
	); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	return repo, projectID, sessionID, agentID
}

func insertRawTask(t *testing.T, repo *taskRepo, task *domain.Task) {
	t.Helper()
	ctx := context.Background()
	_, err := repo.db.ExecContext(ctx, `
		INSERT INTO task
		  (id, project_id, session_id, workflow_id, assignee_id,
		   seq_epic, seq_story, seq_task, type, status, input, output)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.ProjectID, task.SessionID, task.WorkflowID, task.AssigneeID,
		task.SeqEpic, task.SeqStory, task.SeqTask,
		int(task.Type), int(task.Status),
		string(task.Input), string(task.Output),
	)
	if err != nil {
		t.Fatalf("insertRawTask %q: %v", task.ID, err)
	}
}

// TestNext_HierarchicalOrdering asserts that Next returns the task with the
// lowest (seq_epic, seq_story, seq_task) position, not the first inserted.
func TestNext_HierarchicalOrdering(t *testing.T) {
	repo, projectID, sessionID, agentID := setupTaskFixtures(t)
	ctx := context.Background()

	// Insert out-of-order: seq_task 2 before seq_task 1.
	t2 := &domain.Task{
		ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
		AssigneeID: agentID, SeqEpic: 1, SeqStory: 1, SeqTask: 2,
		Type: domain.TaskTypeCoding, Status: domain.TaskStatusPending,
	}
	t1 := &domain.Task{
		ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
		AssigneeID: agentID, SeqEpic: 1, SeqStory: 1, SeqTask: 1,
		Type: domain.TaskTypeCoding, Status: domain.TaskStatusPending,
	}
	insertRawTask(t, repo, t2)
	insertRawTask(t, repo, t1)

	next, err := repo.Next(ctx, sessionID)
	if err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	if next == nil {
		t.Fatal("Next returned nil, expected a task")
	}
	if next.ID != t1.ID {
		t.Errorf("Next returned task %q (seq_task=%d); want task %q (seq_task=1)",
			next.ID, next.SeqTask, t1.ID)
	}
}

// TestNext_ReturnsNilWhenEmpty asserts that Next returns nil when no
// actionable tasks exist.
func TestNext_ReturnsNilWhenEmpty(t *testing.T) {
	repo, _, sessionID, _ := setupTaskFixtures(t)
	next, err := repo.Next(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	if next != nil {
		t.Errorf("expected nil, got task %q", next.ID)
	}
}

// TestClaim_Atomicity asserts that Claim transitions Pending→Processing and
// that a second Claim on the same task is rejected.
func TestClaim_Atomicity(t *testing.T) {
	repo, projectID, sessionID, agentID := setupTaskFixtures(t)
	ctx := context.Background()

	task := &domain.Task{
		ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
		AssigneeID: agentID, SeqEpic: 1, SeqStory: 1, SeqTask: 1,
		Type: domain.TaskTypeCoding, Status: domain.TaskStatusPending,
	}
	insertRawTask(t, repo, task)

	// First claim must succeed.
	if err := repo.Claim(ctx, task.ID); err != nil {
		t.Fatalf("first Claim failed: %v", err)
	}

	// Verify the status is now Processing.
	var status int
	if err := repo.db.QueryRowContext(ctx, `SELECT status FROM task WHERE id = ?`, task.ID).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != int(domain.TaskStatusProcessing) {
		t.Errorf("after Claim: got status %d, want %d (Processing)", status, domain.TaskStatusProcessing)
	}

	// Second claim on the same task must fail.
	if err := repo.Claim(ctx, task.ID); err == nil {
		t.Error("second Claim should have returned an error, got nil")
	}
}

// TestUpdateStatus_TerminalLock asserts that UpdateStatus refuses transitions
// into terminal states (Failure, Completed, Reverted, Cancelled) from states
// that are not permitted by the state machine.
func TestUpdateStatus_TerminalLock(t *testing.T) {
	repo, projectID, sessionID, agentID := setupTaskFixtures(t)
	ctx := context.Background()

	// Success → Failure is not a valid transition.
	task := &domain.Task{
		ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
		AssigneeID: agentID, SeqEpic: 1, SeqStory: 1, SeqTask: 1,
		Type: domain.TaskTypeCoding, Status: domain.TaskStatusSuccess,
	}
	insertRawTask(t, repo, task)

	err := repo.UpdateStatus(ctx, task.ID, domain.TaskStatusFailure, "")
	if err == nil {
		t.Fatal("UpdateStatus on a terminal task should have returned an error, got nil")
	}
}

// TestUpdateStatus_ValidTransitions asserts that every documented valid
// transition is accepted and the status is persisted correctly.
func TestUpdateStatus_ValidTransitions(t *testing.T) {
	cases := []struct {
		name   string
		from   domain.TaskStatus
		to     domain.TaskStatus
	}{
		{"Processing→Pending (recovery)", domain.TaskStatusProcessing, domain.TaskStatusPending},
		{"Processing→Awaiting", domain.TaskStatusProcessing, domain.TaskStatusAwaiting},
		{"Processing→Success", domain.TaskStatusProcessing, domain.TaskStatusSuccess},
		{"Processing→Failure", domain.TaskStatusProcessing, domain.TaskStatusFailure},
		{"Processing→Reverted", domain.TaskStatusProcessing, domain.TaskStatusReverted},
		{"Processing→Cancelled", domain.TaskStatusProcessing, domain.TaskStatusCancelled},
		{"Awaiting→Processing", domain.TaskStatusAwaiting, domain.TaskStatusProcessing},
		{"Awaiting→Reverted", domain.TaskStatusAwaiting, domain.TaskStatusReverted},
		{"Awaiting→Cancelled", domain.TaskStatusAwaiting, domain.TaskStatusCancelled},
		{"Success→Processing (revise)", domain.TaskStatusSuccess, domain.TaskStatusProcessing},
		{"Success→Completed", domain.TaskStatusSuccess, domain.TaskStatusCompleted},
		{"Pending→Reverted", domain.TaskStatusPending, domain.TaskStatusReverted},
		{"Pending→Cancelled", domain.TaskStatusPending, domain.TaskStatusCancelled},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, projectID, sessionID, agentID := setupTaskFixtures(t)
			ctx := context.Background()

			task := &domain.Task{
				ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
				AssigneeID: agentID, SeqEpic: 1, SeqStory: 1, SeqTask: 1,
				Type: domain.TaskTypeCoding, Status: tc.from,
			}
			insertRawTask(t, repo, task)

			if err := repo.UpdateStatus(ctx, task.ID, tc.to, "output"); err != nil {
				t.Fatalf("UpdateStatus(%v→%v) unexpected error: %v", tc.from, tc.to, err)
			}

			var got int
			if err := repo.db.QueryRowContext(ctx,
				`SELECT status FROM task WHERE id = ?`, task.ID,
			).Scan(&got); err != nil {
				t.Fatalf("read status: %v", err)
			}
			if got != int(tc.to) {
				t.Errorf("status after transition: got %d, want %d", got, tc.to)
			}
		})
	}
}

// TestUpdateStatus_InvalidTransitions asserts that invalid state transitions
// are rejected with ErrInvalidTransition.
func TestUpdateStatus_InvalidTransitions(t *testing.T) {
	cases := []struct {
		name string
		from domain.TaskStatus
		to   domain.TaskStatus
	}{
		{"Pending→Processing (must use Claim)", domain.TaskStatusPending, domain.TaskStatusProcessing},
		{"Pending→Success", domain.TaskStatusPending, domain.TaskStatusSuccess},
		{"Success→Failure", domain.TaskStatusSuccess, domain.TaskStatusFailure},
		{"Failure→Processing", domain.TaskStatusFailure, domain.TaskStatusProcessing},
		{"Completed→Processing", domain.TaskStatusCompleted, domain.TaskStatusProcessing},
		{"Reverted→Pending", domain.TaskStatusReverted, domain.TaskStatusPending},
		{"Cancelled→Pending", domain.TaskStatusCancelled, domain.TaskStatusPending},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, projectID, sessionID, agentID := setupTaskFixtures(t)
			ctx := context.Background()

			task := &domain.Task{
				ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
				AssigneeID: agentID, SeqEpic: 1, SeqStory: 1, SeqTask: 1,
				Type: domain.TaskTypeCoding, Status: tc.from,
			}
			insertRawTask(t, repo, task)

			err := repo.UpdateStatus(ctx, task.ID, tc.to, "")
			if err == nil {
				t.Fatalf("UpdateStatus(%v→%v) should have failed, got nil", tc.from, tc.to)
			}
			if !errors.Is(err, ErrInvalidTransition) {
				t.Errorf("expected ErrInvalidTransition, got: %v", err)
			}
		})
	}
}

// TestFindInterrupted identifies tasks stuck in Processing state.
func TestFindInterrupted(t *testing.T) {
	repo, projectID, sessionID, agentID := setupTaskFixtures(t)
	ctx := context.Background()

	processing := &domain.Task{
		ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
		AssigneeID: agentID, SeqEpic: 1, SeqStory: 1, SeqTask: 1,
		Type: domain.TaskTypeCoding, Status: domain.TaskStatusProcessing,
	}
	pending := &domain.Task{
		ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
		AssigneeID: agentID, SeqEpic: 1, SeqStory: 1, SeqTask: 2,
		Type: domain.TaskTypeCoding, Status: domain.TaskStatusPending,
	}
	insertRawTask(t, repo, processing)
	insertRawTask(t, repo, pending)

	interrupted, err := repo.FindInterrupted(ctx, sessionID)
	if err != nil {
		t.Fatalf("FindInterrupted returned error: %v", err)
	}
	if len(interrupted) != 1 {
		t.Fatalf("expected 1 interrupted task, got %d", len(interrupted))
	}
	if interrupted[0].ID != processing.ID {
		t.Errorf("wrong task returned: got %q, want %q", interrupted[0].ID, processing.ID)
	}
}
