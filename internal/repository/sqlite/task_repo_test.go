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
	timeout := task.Timeout
	if timeout == 0 {
		timeout = domain.DefaultTaskTimeout
	}
	// Use NULL for empty workflow_id to avoid foreign key constraint failures.
	// Convert NullString fields to appropriate SQL values.
	var workflowID any
	if task.WorkflowID.Valid {
		workflowID = task.WorkflowID.String
	}
	var input any
	if task.Input.Valid {
		input = task.Input.String
	}
	var output any
	if task.Output.Valid {
		output = task.Output.String
	}
	_, err := repo.db.ExecContext(ctx, `
		INSERT INTO task
		  (id, project_id, session_id, workflow_id, assignee_id,
		   seq_epic, seq_story, seq_task, type, status, timeout, input, output)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.ProjectID, task.SessionID, workflowID, task.AssigneeID,
		task.SeqEpic, task.SeqStory, task.SeqTask,
		int(task.Type), int(task.Status),
		timeout,
		input, output,
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

// TestNext_HierarchicalOrdering_AcrossEpics asserts that tasks are ordered
// by seq_epic first, then seq_story, then seq_task. A task with lower seq_epic
// should be returned before one with higher seq_epic, regardless of other values.
func TestNext_HierarchicalOrdering_AcrossEpics(t *testing.T) {
	repo, projectID, sessionID, agentID := setupTaskFixtures(t)
	ctx := context.Background()

	// Insert tasks spanning multiple epics in reverse order.
	tasks := []*domain.Task{
		{
			ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
			AssigneeID: agentID, SeqEpic: 3, SeqStory: 1, SeqTask: 1,
			Type: domain.TaskTypeCoding, Status: domain.TaskStatusPending,
		},
		{
			ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
			AssigneeID: agentID, SeqEpic: 2, SeqStory: 5, SeqTask: 10,
			Type: domain.TaskTypeCoding, Status: domain.TaskStatusPending,
		},
		{
			ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
			AssigneeID: agentID, SeqEpic: 1, SeqStory: 99, SeqTask: 99,
			Type: domain.TaskTypeCoding, Status: domain.TaskStatusPending,
		},
	}

	// Insert in reverse order (epic 3, 2, 1).
	for _, task := range tasks {
		insertRawTask(t, repo, task)
	}

	// Should return epic 1 first despite having highest story/task numbers.
	next, err := repo.Next(ctx, sessionID)
	if err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	if next == nil {
		t.Fatal("Next returned nil, expected a task")
	}
	// tasks[2] is the one with SeqEpic=1.
	if next.ID != tasks[2].ID {
		t.Errorf("Next returned epic=%d; want epic=1 (task %q)",
			next.SeqEpic, tasks[2].ID)
	}
}

// TestNext_HierarchicalOrdering_AcrossStories asserts that within the same
// epic, tasks are ordered by seq_story, then seq_task.
func TestNext_HierarchicalOrdering_AcrossStories(t *testing.T) {
	repo, projectID, sessionID, agentID := setupTaskFixtures(t)
	ctx := context.Background()

	tasks := []*domain.Task{
		{
			ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
			AssigneeID: agentID, SeqEpic: 1, SeqStory: 3, SeqTask: 1,
			Type: domain.TaskTypeCoding, Status: domain.TaskStatusPending,
		},
		{
			ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
			AssigneeID: agentID, SeqEpic: 1, SeqStory: 2, SeqTask: 5,
			Type: domain.TaskTypeCoding, Status: domain.TaskStatusPending,
		},
		{
			ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
			AssigneeID: agentID, SeqEpic: 1, SeqStory: 1, SeqTask: 10,
			Type: domain.TaskTypeCoding, Status: domain.TaskStatusPending,
		},
	}

	for _, task := range tasks {
		insertRawTask(t, repo, task)
	}

	next, err := repo.Next(ctx, sessionID)
	if err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	if next == nil {
		t.Fatal("Next returned nil, expected a task")
	}
	// tasks[2] has SeqEpic=1, SeqStory=1 (lowest story in epic 1).
	if next.ID != tasks[2].ID {
		t.Errorf("Next returned story=%d; want story=1 (task %q)",
			next.SeqStory, tasks[2].ID)
	}
}

// TestNext_HierarchicalOrdering_FullTraversal asserts that repeated calls to
// Next+Claim process tasks in strict (seq_epic, seq_story, seq_task) order.
func TestNext_HierarchicalOrdering_FullTraversal(t *testing.T) {
	repo, projectID, sessionID, agentID := setupTaskFixtures(t)
	ctx := context.Background()

	// Insert tasks in random order; expected traversal order is by sequence tuple.
	tasks := []*domain.Task{
		{ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID, AssigneeID: agentID, SeqEpic: 2, SeqStory: 1, SeqTask: 1, Type: domain.TaskTypeCoding, Status: domain.TaskStatusPending},
		{ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID, AssigneeID: agentID, SeqEpic: 1, SeqStory: 2, SeqTask: 1, Type: domain.TaskTypeCoding, Status: domain.TaskStatusPending},
		{ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID, AssigneeID: agentID, SeqEpic: 1, SeqStory: 1, SeqTask: 2, Type: domain.TaskTypeCoding, Status: domain.TaskStatusPending},
		{ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID, AssigneeID: agentID, SeqEpic: 1, SeqStory: 1, SeqTask: 1, Type: domain.TaskTypeCoding, Status: domain.TaskStatusPending},
		{ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID, AssigneeID: agentID, SeqEpic: 1, SeqStory: 2, SeqTask: 2, Type: domain.TaskTypeCoding, Status: domain.TaskStatusPending},
	}

	for _, task := range tasks {
		insertRawTask(t, repo, task)
	}

	// Expected order: (1,1,1), (1,1,2), (1,2,1), (1,2,2), (2,1,1)
	expectedOrder := []struct{ epic, story, task int }{
		{1, 1, 1},
		{1, 1, 2},
		{1, 2, 1},
		{1, 2, 2},
		{2, 1, 1},
	}

	for i, exp := range expectedOrder {
		next, err := repo.Next(ctx, sessionID)
		if err != nil {
			t.Fatalf("iteration %d: Next returned error: %v", i, err)
		}
		if next == nil {
			t.Fatalf("iteration %d: Next returned nil, expected task", i)
		}
		if next.SeqEpic != exp.epic || next.SeqStory != exp.story || next.SeqTask != exp.task {
			t.Errorf("iteration %d: got (%d,%d,%d); want (%d,%d,%d)",
				i, next.SeqEpic, next.SeqStory, next.SeqTask,
				exp.epic, exp.story, exp.task)
		}
		// Claim the task to remove it from the actionable set.
		if err := repo.Claim(ctx, next.ID); err != nil {
			t.Fatalf("iteration %d: Claim failed: %v", i, err)
		}
	}

	// After processing all tasks, Next should return nil.
	final, err := repo.Next(ctx, sessionID)
	if err != nil {
		t.Fatalf("final Next returned error: %v", err)
	}
	if final != nil {
		t.Errorf("expected nil after all tasks claimed, got task %q", final.ID)
	}
}

// TestNext_HierarchicalOrdering_IncludesAwaitingTasks asserts that Awaiting
// tasks are included in the actionable set and ordered correctly.
func TestNext_HierarchicalOrdering_IncludesAwaitingTasks(t *testing.T) {
	repo, projectID, sessionID, agentID := setupTaskFixtures(t)
	ctx := context.Background()

	tasks := []*domain.Task{
		{
			ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
			AssigneeID: agentID, SeqEpic: 1, SeqStory: 1, SeqTask: 2,
			Type: domain.TaskTypeCoding, Status: domain.TaskStatusPending,
		},
		{
			ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
			AssigneeID: agentID, SeqEpic: 1, SeqStory: 1, SeqTask: 1,
			Type: domain.TaskTypeCoding, Status: domain.TaskStatusAwaiting,
		},
	}

	for _, task := range tasks {
		insertRawTask(t, repo, task)
	}

	next, err := repo.Next(ctx, sessionID)
	if err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	if next == nil {
		t.Fatal("Next returned nil, expected a task")
	}
	// tasks[1] has lower seq_task and is Awaiting (still actionable).
	if next.ID != tasks[1].ID {
		t.Errorf("Next returned task %q (seq_task=%d, status=%d); want Awaiting task %q",
			next.ID, next.SeqTask, next.Status, tasks[1].ID)
	}
	if next.Status != domain.TaskStatusAwaiting {
		t.Errorf("expected Awaiting status, got %d", next.Status)
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
		{"Failure→Awaiting (crash fallback: Story 1.9)", domain.TaskStatusFailure, domain.TaskStatusAwaiting},
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

// TestUpdateAssignee_Success asserts that UpdateAssignee reassigns the task
// to a new agent and the change is persisted.
func TestUpdateAssignee_Success(t *testing.T) {
	repo, projectID, sessionID, agentID := setupTaskFixtures(t)
	ctx := context.Background()

	// Insert a second agent to reassign to.
	newAgentID := idgen.MustNew()
	if _, err := repo.db.ExecContext(ctx,
		`INSERT INTO agent (id, name, type, assistant) VALUES (?, ?, ?, ?)`,
		newAgentID, "human2", 0, "",
	); err != nil {
		t.Fatalf("seed second agent: %v", err)
	}

	task := &domain.Task{
		ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
		AssigneeID: agentID, SeqEpic: 1, SeqStory: 1, SeqTask: 1,
		Type: domain.TaskTypeCoding, Status: domain.TaskStatusPending,
	}
	insertRawTask(t, repo, task)

	if err := repo.UpdateAssignee(ctx, task.ID, newAgentID); err != nil {
		t.Fatalf("UpdateAssignee returned unexpected error: %v", err)
	}

	var got string
	if err := repo.db.QueryRowContext(ctx,
		`SELECT assignee_id FROM task WHERE id = ?`, task.ID,
	).Scan(&got); err != nil {
		t.Fatalf("read assignee_id: %v", err)
	}
	if got != newAgentID {
		t.Errorf("assignee_id: got %q, want %q", got, newAgentID)
	}
}

// TestUpdateAssignee_NotFound asserts that UpdateAssignee returns an error
// when the task does not exist.
func TestUpdateAssignee_NotFound(t *testing.T) {
	repo, _, _, _ := setupTaskFixtures(t)
	err := repo.UpdateAssignee(context.Background(), idgen.MustNew(), idgen.MustNew())
	if err == nil {
		t.Fatal("expected error for non-existent task, got nil")
	}
}

// TestNullableFields_WorkflowID asserts that sql.NullString properly handles
// NULL workflow_id values scanned from the database.
func TestNullableFields_WorkflowID(t *testing.T) {
	repo, projectID, sessionID, agentID := setupTaskFixtures(t)
	ctx := context.Background()

	// Insert task with NULL workflow_id directly via SQL.
	taskID := idgen.MustNew()
	_, err := repo.db.ExecContext(ctx, `
		INSERT INTO task
		  (id, project_id, session_id, workflow_id, assignee_id,
		   seq_epic, seq_story, seq_task, type, status, timeout, input, output)
		VALUES (?, ?, ?, NULL, ?, 1, 1, 1, 0, 0, ?, NULL, NULL)`,
		taskID, projectID, sessionID, agentID, domain.DefaultTaskTimeout,
	)
	if err != nil {
		t.Fatalf("insert task with NULL workflow_id: %v", err)
	}

	// Query via TaskRepository.
	task, err := repo.FindByID(ctx, taskID)
	if err != nil {
		t.Fatalf("FindByID returned error: %v", err)
	}

	// Assert WorkflowID.Valid == false for NULL.
	if task.WorkflowID.Valid {
		t.Errorf("expected WorkflowID.Valid == false for NULL, got true with value %q",
			task.WorkflowID.String)
	}
}

// TestNullableFields_WorkflowID_NonNull asserts that sql.NullString properly
// handles non-NULL workflow_id values.
func TestNullableFields_WorkflowID_NonNull(t *testing.T) {
	repo, projectID, sessionID, agentID := setupTaskFixtures(t)
	ctx := context.Background()

	// Create a workflow to reference.
	workflowID := idgen.MustNew()
	_, err := repo.db.ExecContext(ctx,
		`INSERT INTO workflow (id, session_id, type) VALUES (?, ?, ?)`,
		workflowID, sessionID, 0,
	)
	if err != nil {
		t.Fatalf("insert workflow: %v", err)
	}

	// Insert task with valid workflow_id.
	taskID := idgen.MustNew()
	_, err = repo.db.ExecContext(ctx, `
		INSERT INTO task
		  (id, project_id, session_id, workflow_id, assignee_id,
		   seq_epic, seq_story, seq_task, type, status, timeout, input, output)
		VALUES (?, ?, ?, ?, ?, 1, 1, 1, 0, 0, ?, NULL, NULL)`,
		taskID, projectID, sessionID, workflowID, agentID, domain.DefaultTaskTimeout,
	)
	if err != nil {
		t.Fatalf("insert task with workflow_id: %v", err)
	}

	// Query via TaskRepository.
	task, err := repo.FindByID(ctx, taskID)
	if err != nil {
		t.Fatalf("FindByID returned error: %v", err)
	}

	// Assert WorkflowID.Valid == true and value matches.
	if !task.WorkflowID.Valid {
		t.Error("expected WorkflowID.Valid == true for non-NULL, got false")
	}
	if task.WorkflowID.String != workflowID {
		t.Errorf("WorkflowID.String: got %q, want %q", task.WorkflowID.String, workflowID)
	}
}

// TestNullableFields_InputOutput asserts that sql.NullString handles NULL and
// non-NULL input/output values correctly.
func TestNullableFields_InputOutput(t *testing.T) {
	repo, projectID, sessionID, agentID := setupTaskFixtures(t)
	ctx := context.Background()

	// Insert task with NULL input and output.
	taskID := idgen.MustNew()
	_, err := repo.db.ExecContext(ctx, `
		INSERT INTO task
		  (id, project_id, session_id, workflow_id, assignee_id,
		   seq_epic, seq_story, seq_task, type, status, timeout, input, output)
		VALUES (?, ?, ?, NULL, ?, 1, 1, 1, 0, 0, ?, NULL, NULL)`,
		taskID, projectID, sessionID, agentID, domain.DefaultTaskTimeout,
	)
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}

	task, err := repo.FindByID(ctx, taskID)
	if err != nil {
		t.Fatalf("FindByID returned error: %v", err)
	}

	// Both Input and Output should be NULL (Valid == false).
	if task.Input.Valid {
		t.Errorf("expected Input.Valid == false for NULL, got true with value %q", task.Input.String)
	}
	if task.Output.Valid {
		t.Errorf("expected Output.Valid == false for NULL, got true with value %q", task.Output.String)
	}
}

// TestUpdateStatus_StoresNullForEmptyOutput asserts that UpdateStatus stores
// NULL for empty output strings.
func TestUpdateStatus_StoresNullForEmptyOutput(t *testing.T) {
	repo, projectID, sessionID, agentID := setupTaskFixtures(t)
	ctx := context.Background()

	task := &domain.Task{
		ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
		AssigneeID: agentID, SeqEpic: 1, SeqStory: 1, SeqTask: 1,
		Type: domain.TaskTypeCoding, Status: domain.TaskStatusProcessing,
	}
	insertRawTask(t, repo, task)

	// Update with empty output → should store NULL.
	if err := repo.UpdateStatus(ctx, task.ID, domain.TaskStatusSuccess, ""); err != nil {
		t.Fatalf("UpdateStatus returned error: %v", err)
	}

	// Verify output is NULL in database.
	var output any
	if err := repo.db.QueryRowContext(ctx,
		`SELECT output FROM task WHERE id = ?`, task.ID,
	).Scan(&output); err != nil {
		t.Fatalf("read output: %v", err)
	}
	if output != nil {
		t.Errorf("expected NULL output, got %v", output)
	}
}

// TestUpdateStatus_StoresValueForNonEmptyOutput asserts that UpdateStatus
// stores the actual value for non-empty output strings.
func TestUpdateStatus_StoresValueForNonEmptyOutput(t *testing.T) {
	repo, projectID, sessionID, agentID := setupTaskFixtures(t)
	ctx := context.Background()

	task := &domain.Task{
		ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
		AssigneeID: agentID, SeqEpic: 1, SeqStory: 1, SeqTask: 1,
		Type: domain.TaskTypeCoding, Status: domain.TaskStatusProcessing,
	}
	insertRawTask(t, repo, task)

	// Update with non-empty output.
	expectedOutput := `{"result": "success"}`
	if err := repo.UpdateStatus(ctx, task.ID, domain.TaskStatusSuccess, expectedOutput); err != nil {
		t.Fatalf("UpdateStatus returned error: %v", err)
	}

	// Verify output is stored correctly.
	var output string
	if err := repo.db.QueryRowContext(ctx,
		`SELECT output FROM task WHERE id = ?`, task.ID,
	).Scan(&output); err != nil {
		t.Fatalf("read output: %v", err)
	}
	if output != expectedOutput {
		t.Errorf("output: got %q, want %q", output, expectedOutput)
	}
}

// TestMostRecentActive_ReturnsProcessing asserts that MostRecentActive returns
// the most recently created task in Processing status.
func TestMostRecentActive_ReturnsProcessing(t *testing.T) {
	repo, projectID, sessionID, agentID := setupTaskFixtures(t)
	ctx := context.Background()

	tasks := []*domain.Task{
		{
			ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
			AssigneeID: agentID, SeqEpic: 1, SeqStory: 1, SeqTask: 1,
			Type: domain.TaskTypeCoding, Status: domain.TaskStatusPending,
		},
		{
			ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
			AssigneeID: agentID, SeqEpic: 1, SeqStory: 1, SeqTask: 2,
			Type: domain.TaskTypeCoding, Status: domain.TaskStatusProcessing,
		},
		{
			ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
			AssigneeID: agentID, SeqEpic: 1, SeqStory: 1, SeqTask: 3,
			Type: domain.TaskTypeCoding, Status: domain.TaskStatusSuccess,
		},
	}

	for _, task := range tasks {
		insertRawTask(t, repo, task)
	}

	result, err := repo.MostRecentActive(ctx, sessionID)
	if err != nil {
		t.Fatalf("MostRecentActive returned error: %v", err)
	}
	if result == nil {
		t.Fatal("MostRecentActive returned nil, expected Processing task")
	}
	if result.ID != tasks[1].ID {
		t.Errorf("MostRecentActive returned task %q, want %q", result.ID, tasks[1].ID)
	}
	if result.Status != domain.TaskStatusProcessing {
		t.Errorf("MostRecentActive status = %d, want Processing", result.Status)
	}
}

// TestMostRecentActive_ReturnsAwaiting asserts that MostRecentActive returns
// the most recently created task in Awaiting status.
func TestMostRecentActive_ReturnsAwaiting(t *testing.T) {
	repo, projectID, sessionID, agentID := setupTaskFixtures(t)
	ctx := context.Background()

	tasks := []*domain.Task{
		{
			ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
			AssigneeID: agentID, SeqEpic: 1, SeqStory: 1, SeqTask: 1,
			Type: domain.TaskTypeCoding, Status: domain.TaskStatusPending,
		},
		{
			ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
			AssigneeID: agentID, SeqEpic: 1, SeqStory: 1, SeqTask: 2,
			Type: domain.TaskTypeCoding, Status: domain.TaskStatusAwaiting,
		},
	}

	for _, task := range tasks {
		insertRawTask(t, repo, task)
	}

	result, err := repo.MostRecentActive(ctx, sessionID)
	if err != nil {
		t.Fatalf("MostRecentActive returned error: %v", err)
	}
	if result == nil {
		t.Fatal("MostRecentActive returned nil, expected Awaiting task")
	}
	if result.ID != tasks[1].ID {
		t.Errorf("MostRecentActive returned task %q, want %q", result.ID, tasks[1].ID)
	}
}

// TestMostRecentActive_ReturnsLatestByID asserts that when multiple active
// tasks exist, the one with the highest (most recent) ID is returned.
func TestMostRecentActive_ReturnsLatestByID(t *testing.T) {
	repo, projectID, sessionID, agentID := setupTaskFixtures(t)
	ctx := context.Background()

	// Insert multiple Processing tasks; last inserted has highest ID (ULID).
	tasks := []*domain.Task{
		{
			ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
			AssigneeID: agentID, SeqEpic: 1, SeqStory: 1, SeqTask: 1,
			Type: domain.TaskTypeCoding, Status: domain.TaskStatusProcessing,
		},
		{
			ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
			AssigneeID: agentID, SeqEpic: 1, SeqStory: 1, SeqTask: 2,
			Type: domain.TaskTypeCoding, Status: domain.TaskStatusProcessing,
		},
	}

	for _, task := range tasks {
		insertRawTask(t, repo, task)
	}

	result, err := repo.MostRecentActive(ctx, sessionID)
	if err != nil {
		t.Fatalf("MostRecentActive returned error: %v", err)
	}
	if result == nil {
		t.Fatal("MostRecentActive returned nil, expected a task")
	}
	// Last inserted task has highest ID.
	if result.ID != tasks[1].ID {
		t.Errorf("MostRecentActive returned task %q, want %q (most recent)", result.ID, tasks[1].ID)
	}
}

// TestMostRecentActive_ReturnsNilWhenNoActive asserts that MostRecentActive
// returns nil when no task is in Processing or Awaiting status.
func TestMostRecentActive_ReturnsNilWhenNoActive(t *testing.T) {
	repo, projectID, sessionID, agentID := setupTaskFixtures(t)
	ctx := context.Background()

	tasks := []*domain.Task{
		{
			ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
			AssigneeID: agentID, SeqEpic: 1, SeqStory: 1, SeqTask: 1,
			Type: domain.TaskTypeCoding, Status: domain.TaskStatusPending,
		},
		{
			ID: idgen.MustNew(), ProjectID: projectID, SessionID: sessionID,
			AssigneeID: agentID, SeqEpic: 1, SeqStory: 1, SeqTask: 2,
			Type: domain.TaskTypeCoding, Status: domain.TaskStatusSuccess,
		},
	}

	for _, task := range tasks {
		insertRawTask(t, repo, task)
	}

	result, err := repo.MostRecentActive(ctx, sessionID)
	if err != nil {
		t.Fatalf("MostRecentActive returned error: %v", err)
	}
	if result != nil {
		t.Errorf("MostRecentActive returned task %q, expected nil", result.ID)
	}
}

// TestMostRecentActive_ReturnsNilWhenEmpty asserts that MostRecentActive
// returns nil when no tasks exist for the session.
func TestMostRecentActive_ReturnsNilWhenEmpty(t *testing.T) {
	repo, _, sessionID, _ := setupTaskFixtures(t)

	result, err := repo.MostRecentActive(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("MostRecentActive returned error: %v", err)
	}
	if result != nil {
		t.Errorf("MostRecentActive returned task %q, expected nil for empty session", result.ID)
	}
}
