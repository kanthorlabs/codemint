package domain

import "testing"

// TestTaskStatus_IsTerminal verifies that IsTerminal correctly identifies
// terminal vs non-terminal task statuses.
func TestTaskStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		status     TaskStatus
		isTerminal bool
	}{
		{TaskStatusPending, false},
		{TaskStatusProcessing, false},
		{TaskStatusAwaiting, false},
		{TaskStatusSuccess, true},
		{TaskStatusFailure, true},
		{TaskStatusCompleted, true},
		{TaskStatusReverted, true},
		{TaskStatusCancelled, true},
	}

	for _, tt := range tests {
		got := tt.status.IsTerminal()
		if got != tt.isTerminal {
			t.Errorf("TaskStatus(%d).IsTerminal() = %v, want %v",
				tt.status, got, tt.isTerminal)
		}
	}
}
