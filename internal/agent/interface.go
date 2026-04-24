// Package agent defines the CodingAgent interface that wraps the underlying
// ACP/Coding Agent CLI, enforcing the Accept/Revert contract for atomic undo.
package agent

import (
	"context"

	"codemint.kanthorlabs.com/internal/domain"
)

// CodingAgent is the abstraction over the underlying ACP/Coding Agent CLI.
// Implementations delegate actual file-system operations to the agent process;
// CodeMint only drives the state machine and records outcomes.
type CodingAgent interface {
	// ExecuteTask dispatches the task to the coding agent for processing.
	// Implementations must derive a context.WithTimeout from the provided ctx
	// using task.Timeout (milliseconds) so that the underlying os/exec process
	// is killed if it exceeds the deadline. On success the task status
	// transitions to Awaiting, awaiting human review.
	ExecuteTask(ctx context.Context, task *domain.Task) error

	// Accept finalises the agent's changes. The task name and metadata may be
	// used for commit messages or audit logs by the underlying implementation.
	Accept(ctx context.Context, task *domain.Task) error

	// Revert triggers the agent's native undo mechanism to roll back any
	// file-system changes made during ExecuteTask. If the agent returns an
	// error, the caller must treat this as an agent crash and transition the
	// task to TaskStatusFailure to trigger the fallback flow (Story 1.9).
	Revert(ctx context.Context, task *domain.Task) error
}
