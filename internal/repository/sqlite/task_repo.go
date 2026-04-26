package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/repository"
)

// ErrInvalidTransition is returned when the requested status change violates
// the task state machine rules.
var ErrInvalidTransition = errors.New("sqlite: invalid task state transition")

// validFromStates maps each target TaskStatus to the set of source statuses
// that are permitted to transition into it. Claim owns Pending→Processing, so
// that edge is intentionally absent here. Terminal states (Completed,
// Reverted, Cancelled) have no outgoing edges and are therefore absent as keys.
//
// Transition table (derived from docs/plan/epic-01/1.5-task-state-machine-enforcement/tasks.md §2
// and extended by Story 1.9 to allow Failure→Awaiting for the crash-fallback flow):
//
//	Pending(0)    ← Processing(1)         [safe-recovery reset]
//	Processing(1) ← Awaiting(2), Success(3) [resume / revise]
//	Awaiting(2)   ← Processing(1), Failure(4) [crash fallback: Story 1.9]
//	Success(3)    ← Processing(1)
//	Failure(4)    ← Processing(1)
//	Completed(5)  ← Success(3)
//	Reverted(6)   ← Pending(0), Processing(1), Awaiting(2)
//	Cancelled(7)  ← Pending(0), Processing(1), Awaiting(2)
var validFromStates = map[domain.TaskStatus][]domain.TaskStatus{
	domain.TaskStatusPending:    {domain.TaskStatusProcessing},
	domain.TaskStatusProcessing: {domain.TaskStatusAwaiting, domain.TaskStatusSuccess},
	domain.TaskStatusAwaiting:   {domain.TaskStatusProcessing, domain.TaskStatusFailure},
	domain.TaskStatusSuccess:    {domain.TaskStatusProcessing},
	domain.TaskStatusFailure:    {domain.TaskStatusProcessing},
	domain.TaskStatusCompleted:  {domain.TaskStatusSuccess},
	domain.TaskStatusReverted:   {domain.TaskStatusPending, domain.TaskStatusProcessing, domain.TaskStatusAwaiting},
	domain.TaskStatusCancelled:  {domain.TaskStatusPending, domain.TaskStatusProcessing, domain.TaskStatusAwaiting},
}

// taskRepo is the SQLite implementation of repository.TaskRepository.
type taskRepo struct {
	db *sqlx.DB
}

// NewTaskRepo constructs a TaskRepository backed by the given SQLite connection.
func NewTaskRepo(db *sqlx.DB) repository.TaskRepository {
	return &taskRepo{db: db}
}

// Compile-time interface satisfaction check.
var _ repository.TaskRepository = (*taskRepo)(nil)

// Create inserts a new task into the database.
func (r *taskRepo) Create(ctx context.Context, t *domain.Task) error {
	const query = `
		INSERT INTO task (
			id, project_id, session_id, workflow_id, assignee_id,
			seq_epic, seq_story, seq_task, type, status, timeout, input, output, client_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		t.ID, t.ProjectID, t.SessionID, t.WorkflowID, t.AssigneeID,
		t.SeqEpic, t.SeqStory, t.SeqTask, int(t.Type), int(t.Status),
		t.Timeout, t.Input, t.Output, t.ClientID,
	)
	if err != nil {
		return fmt.Errorf("sqlite: create task %q: %w", t.ID, err)
	}
	return nil
}

// Next returns the first actionable task (Pending=0 or Awaiting=2) in the
// given session, ordered hierarchically by (seq_epic, seq_story, seq_task).
func (r *taskRepo) Next(ctx context.Context, sessionID string) (*domain.Task, error) {
	const query = `
		SELECT id, project_id, session_id, workflow_id, assignee_id,
		       seq_epic, seq_story, seq_task, type, status, timeout, input, output, client_id
		FROM task
		WHERE session_id = ?
		  AND status IN (0, 2)
		ORDER BY seq_epic ASC, seq_story ASC, seq_task ASC
		LIMIT 1`

	var t domain.Task
	err := r.db.GetContext(ctx, &t, query, sessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("sqlite: next task for session %q: %w", sessionID, err)
	}
	return &t, nil
}

// Claim atomically transitions a task from Pending (0) to Processing (1).
// The UPDATE ... WHERE status = 0 ... RETURNING id is a single atomic
// statement: if the task is not Pending the WHERE clause matches nothing,
// RETURNING yields no row, and we surface ErrInvalidTransition without
// needing a separate SELECT or an explicit transaction.
func (r *taskRepo) Claim(ctx context.Context, taskID string) error {
	var returned string
	err := r.db.QueryRowContext(ctx,
		`UPDATE task SET status = ? WHERE id = ? AND status = ? RETURNING id`,
		int(domain.TaskStatusProcessing), taskID, int(domain.TaskStatusPending),
	).Scan(&returned)

	if errors.Is(err, sql.ErrNoRows) {
		// Either the task does not exist or it was not in Pending state.
		return fmt.Errorf("%w: task %q is not in Pending(0) state", ErrInvalidTransition, taskID)
	}
	if err != nil {
		return fmt.Errorf("sqlite: claim task %q: %w", taskID, err)
	}
	return nil
}

// UpdateStatus transitions a task to the given status and records the output,
// enforcing the state machine defined in validFromStates. The UPDATE …
// WHERE id = ? AND status IN (…) RETURNING id is a single atomic statement:
// if the current status is not among the allowed source states, the WHERE
// clause matches nothing and RETURNING yields no row, which we surface as
// ErrInvalidTransition without needing a separate SELECT or transaction.
func (r *taskRepo) UpdateStatus(ctx context.Context, taskID string, status domain.TaskStatus, output string) error {
	allowed, ok := validFromStates[status]
	if !ok || len(allowed) == 0 {
		return fmt.Errorf("%w: no valid source states for target %d", ErrInvalidTransition, status)
	}

	// Build "status IN (?, ?, …)".
	placeholders := strings.Repeat("?,", len(allowed))
	placeholders = placeholders[:len(placeholders)-1] // trim trailing comma
	query := fmt.Sprintf(
		`UPDATE task SET status = ?, output = ? WHERE id = ? AND status IN (%s) RETURNING id`,
		placeholders,
	)

	// Convert output to NullString: empty string → NULL in database.
	outputValue := domain.NewNullString(output)

	args := make([]any, 0, 3+len(allowed))
	args = append(args, int(status), outputValue, taskID)
	for _, s := range allowed {
		args = append(args, int(s))
	}

	var returned string
	err := r.db.QueryRowContext(ctx, query, args...).Scan(&returned)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: task %q cannot move to status %d", ErrInvalidTransition, taskID, status)
	}
	if err != nil {
		return fmt.Errorf("sqlite: update status task %q: %w", taskID, err)
	}
	return nil
}

// FindByID returns the Task with the given primary key. It returns an error
// wrapping sql.ErrNoRows (as a descriptive message) when no matching row exists.
func (r *taskRepo) FindByID(ctx context.Context, taskID string) (*domain.Task, error) {
	const query = `
		SELECT id, project_id, session_id, workflow_id, assignee_id,
		       seq_epic, seq_story, seq_task, type, status, timeout, input, output, client_id
		FROM task
		WHERE id = ?`

	var t domain.Task
	err := r.db.GetContext(ctx, &t, query, taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("sqlite: task %q not found", taskID)
		}
		return nil, fmt.Errorf("sqlite: find task by id %q: %w", taskID, err)
	}
	return &t, nil
}

// UpdateTaskStatus transitions a task to the given status without altering the
// output field. It reuses the same validFromStates machine as UpdateStatus so
// that all state-machine rules are enforced consistently.
func (r *taskRepo) UpdateTaskStatus(ctx context.Context, taskID string, status domain.TaskStatus) error {
	allowed, ok := validFromStates[status]
	if !ok || len(allowed) == 0 {
		return fmt.Errorf("%w: no valid source states for target %d", ErrInvalidTransition, status)
	}

	placeholders := strings.Repeat("?,", len(allowed))
	placeholders = placeholders[:len(placeholders)-1]
	query := fmt.Sprintf(
		`UPDATE task SET status = ? WHERE id = ? AND status IN (%s) RETURNING id`,
		placeholders,
	)

	args := make([]any, 0, 2+len(allowed))
	args = append(args, int(status), taskID)
	for _, s := range allowed {
		args = append(args, int(s))
	}

	var returned string
	err := r.db.QueryRowContext(ctx, query, args...).Scan(&returned)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: task %q cannot move to status %d", ErrInvalidTransition, taskID, status)
	}
	if err != nil {
		return fmt.Errorf("sqlite: update task status %q: %w", taskID, err)
	}
	return nil
}

// FindInterrupted returns all tasks in the given session that are stuck in
// the Processing (1) state, indicating the process may have crashed mid-execution.
func (r *taskRepo) FindInterrupted(ctx context.Context, sessionID string) ([]*domain.Task, error) {
	const query = `
		SELECT id, project_id, session_id, workflow_id, assignee_id,
		       seq_epic, seq_story, seq_task, type, status, timeout, input, output, client_id
		FROM task
		WHERE session_id = ?
		  AND status = 1
		ORDER BY seq_epic ASC, seq_story ASC, seq_task ASC`

	var tasks []*domain.Task
	if err := r.db.SelectContext(ctx, &tasks, query, sessionID); err != nil {
		return nil, fmt.Errorf("sqlite: find interrupted tasks for session %q: %w", sessionID, err)
	}
	return tasks, nil
}

// UpdateAssignee reassigns a task to a different agent. Used by the crash
// fallback flow (Story 1.9) to hand a failed task back to the human agent.
func (r *taskRepo) UpdateAssignee(ctx context.Context, taskID string, assigneeID string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE task SET assignee_id = ? WHERE id = ?`,
		assigneeID, taskID,
	)
	if err != nil {
		return fmt.Errorf("sqlite: update assignee for task %q: %w", taskID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: update assignee rows affected for task %q: %w", taskID, err)
	}
	if rows == 0 {
		return fmt.Errorf("sqlite: task %q not found", taskID)
	}
	return nil
}

// ListCoordinationAfter returns all Coordination tasks (type=3) in the session
// with IDs greater than afterTaskID, ordered by ID (ascending).
func (r *taskRepo) ListCoordinationAfter(ctx context.Context, sessionID string, afterTaskID string) ([]*domain.Task, error) {
	var tasks []*domain.Task
	var err error

	if afterTaskID == "" {
		// Return all Coordination tasks.
		const query = `
			SELECT id, project_id, session_id, workflow_id, assignee_id,
			       seq_epic, seq_story, seq_task, type, status, timeout, input, output, client_id
			FROM task
			WHERE session_id = ?
			  AND type = ?
			ORDER BY id ASC`
		err = r.db.SelectContext(ctx, &tasks, query, sessionID, int(domain.TaskTypeCoordination))
	} else {
		// Return Coordination tasks after the given ID.
		const query = `
			SELECT id, project_id, session_id, workflow_id, assignee_id,
			       seq_epic, seq_story, seq_task, type, status, timeout, input, output, client_id
			FROM task
			WHERE session_id = ?
			  AND type = ?
			  AND id > ?
			ORDER BY id ASC`
		err = r.db.SelectContext(ctx, &tasks, query, sessionID, int(domain.TaskTypeCoordination), afterTaskID)
	}

	if err != nil {
		return nil, fmt.Errorf("sqlite: list coordination tasks after %q: %w", afterTaskID, err)
	}
	return tasks, nil
}

// ListBySession returns all tasks in the given session, ordered hierarchically
// by (seq_epic, seq_story, seq_task). Used by the /tasks command.
func (r *taskRepo) ListBySession(ctx context.Context, sessionID string) ([]*domain.Task, error) {
	const query = `
		SELECT id, project_id, session_id, workflow_id, assignee_id,
		       seq_epic, seq_story, seq_task, type, status, timeout, input, output, client_id
		FROM task
		WHERE session_id = ?
		ORDER BY seq_epic ASC, seq_story ASC, seq_task ASC`

	var tasks []*domain.Task
	if err := r.db.SelectContext(ctx, &tasks, query, sessionID); err != nil {
		return nil, fmt.Errorf("sqlite: list tasks for session %q: %w", sessionID, err)
	}
	return tasks, nil
}

// MostRecentActive returns the most recently active task in the session
// that is in Processing (1) or Awaiting (2) status. The "most recent" is
// determined by the highest ID (since IDs are ULIDs, lexicographically
// larger means more recently created).
func (r *taskRepo) MostRecentActive(ctx context.Context, sessionID string) (*domain.Task, error) {
	const query = `
		SELECT id, project_id, session_id, workflow_id, assignee_id,
		       seq_epic, seq_story, seq_task, type, status, timeout, input, output, client_id
		FROM task
		WHERE session_id = ?
		  AND status IN (?, ?)
		ORDER BY id DESC
		LIMIT 1`

	var t domain.Task
	err := r.db.GetContext(ctx, &t, query, sessionID,
		int(domain.TaskStatusProcessing), int(domain.TaskStatusAwaiting))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("sqlite: most recent active task for session %q: %w", sessionID, err)
	}
	return &t, nil
}
