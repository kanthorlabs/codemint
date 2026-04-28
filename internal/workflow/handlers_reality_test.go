package workflow

import (
	"context"
	"testing"

	"codemint.kanthorlabs.com/internal/domain"
)

func TestAppendTargetedContextHandler_SkippedTrue_WithReason(t *testing.T) {
	t.Parallel()

	handler := AppendTargetedContextHandler()

	args := HandlerArgs{
		WorkflowID: "wf-123",
		Task:       &domain.Task{ID: "task-456"},
		Output:     `{"skipped": true, "reason": "No existing code matches goal keywords ['webhook', 'handler']. This appears to be a greenfield implementation."}`,
	}

	err := handler(context.Background(), args)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}
}

func TestAppendTargetedContextHandler_SkippedTrue_EmptyReason(t *testing.T) {
	t.Parallel()

	handler := AppendTargetedContextHandler()

	args := HandlerArgs{
		WorkflowID: "wf-123",
		Task:       &domain.Task{ID: "task-456"},
		Output:     `{"skipped": true, "reason": ""}`,
	}

	err := handler(context.Background(), args)
	if err == nil {
		t.Fatal("Expected error for skipped=true with empty reason")
	}
	if err.Error() != "append_targeted_context: skipped=true requires non-empty reason" {
		t.Errorf("Error = %q, want 'append_targeted_context: skipped=true requires non-empty reason'", err.Error())
	}
}

func TestAppendTargetedContextHandler_SkippedTrue_WhitespaceReason(t *testing.T) {
	t.Parallel()

	handler := AppendTargetedContextHandler()

	args := HandlerArgs{
		WorkflowID: "wf-123",
		Task:       &domain.Task{ID: "task-456"},
		Output:     `{"skipped": true, "reason": "   "}`,
	}

	err := handler(context.Background(), args)
	if err == nil {
		t.Fatal("Expected error for skipped=true with whitespace-only reason")
	}
	if err.Error() != "append_targeted_context: skipped=true requires non-empty reason" {
		t.Errorf("Error = %q, want 'append_targeted_context: skipped=true requires non-empty reason'", err.Error())
	}
}

func TestAppendTargetedContextHandler_SkippedTrue_MissingReason(t *testing.T) {
	t.Parallel()

	handler := AppendTargetedContextHandler()

	args := HandlerArgs{
		WorkflowID: "wf-123",
		Task:       &domain.Task{ID: "task-456"},
		Output:     `{"skipped": true}`,
	}

	err := handler(context.Background(), args)
	if err == nil {
		t.Fatal("Expected error for skipped=true with missing reason")
	}
	if err.Error() != "append_targeted_context: skipped=true requires non-empty reason" {
		t.Errorf("Error = %q, want 'append_targeted_context: skipped=true requires non-empty reason'", err.Error())
	}
}

func TestAppendTargetedContextHandler_SkippedFalse_HappyPath(t *testing.T) {
	t.Parallel()

	handler := AppendTargetedContextHandler()

	args := HandlerArgs{
		WorkflowID: "wf-123",
		Task:       &domain.Task{ID: "task-456"},
		Output: `{
			"skipped": false,
			"keyword_hits": [
				{
					"keyword": "email",
					"files": [
						{"path": "internal/user/validation.go", "lines": [42, 67, 89]}
					]
				}
			],
			"files_read": {
				"internal/user/validation.go": "package user\n\nfunc ValidateEmail(email string) bool { ... }"
			},
			"token_budget_used": 18500
		}`,
	}

	err := handler(context.Background(), args)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}
}

func TestAppendTargetedContextHandler_SkippedFalse_WithImportHops(t *testing.T) {
	t.Parallel()

	handler := AppendTargetedContextHandler()

	args := HandlerArgs{
		WorkflowID: "wf-123",
		Task:       &domain.Task{ID: "task-456"},
		Output: `{
			"skipped": false,
			"keyword_hits": [
				{
					"keyword": "validation",
					"files": [
						{"path": "internal/user/validation.go", "lines": [1, 42]}
					]
				}
			],
			"files_read": {
				"internal/user/validation.go": "package user",
				"internal/user/service.go": "package user"
			},
			"import_hops": [
				{
					"from": "internal/user/validation.go",
					"to": "internal/user/service.go",
					"reason": "UserService is used in validation flow"
				}
			],
			"token_budget_used": 25000
		}`,
	}

	err := handler(context.Background(), args)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}
}

func TestAppendTargetedContextHandler_SkippedFalse_MissingKeywordHits(t *testing.T) {
	t.Parallel()

	handler := AppendTargetedContextHandler()

	args := HandlerArgs{
		WorkflowID: "wf-123",
		Task:       &domain.Task{ID: "task-456"},
		Output: `{
			"skipped": false,
			"files_read": {
				"internal/user/validation.go": "package user"
			},
			"token_budget_used": 1000
		}`,
	}

	err := handler(context.Background(), args)
	if err == nil {
		t.Fatal("Expected error for missing keyword_hits")
	}
	if err.Error() != "append_targeted_context: keyword_hits is required when skipped=false" {
		t.Errorf("Error = %q, want 'append_targeted_context: keyword_hits is required when skipped=false'", err.Error())
	}
}

func TestAppendTargetedContextHandler_SkippedFalse_EmptyKeywordHits(t *testing.T) {
	t.Parallel()

	handler := AppendTargetedContextHandler()

	args := HandlerArgs{
		WorkflowID: "wf-123",
		Task:       &domain.Task{ID: "task-456"},
		Output: `{
			"skipped": false,
			"keyword_hits": [],
			"files_read": {
				"internal/user/validation.go": "package user"
			},
			"token_budget_used": 1000
		}`,
	}

	err := handler(context.Background(), args)
	if err == nil {
		t.Fatal("Expected error for empty keyword_hits")
	}
	if err.Error() != "append_targeted_context: keyword_hits is required when skipped=false" {
		t.Errorf("Error = %q, want 'append_targeted_context: keyword_hits is required when skipped=false'", err.Error())
	}
}

func TestAppendTargetedContextHandler_SkippedFalse_MissingFilesRead(t *testing.T) {
	t.Parallel()

	handler := AppendTargetedContextHandler()

	args := HandlerArgs{
		WorkflowID: "wf-123",
		Task:       &domain.Task{ID: "task-456"},
		Output: `{
			"skipped": false,
			"keyword_hits": [
				{"keyword": "email", "files": []}
			],
			"token_budget_used": 1000
		}`,
	}

	err := handler(context.Background(), args)
	if err == nil {
		t.Fatal("Expected error for missing files_read")
	}
	if err.Error() != "append_targeted_context: files_read is required when skipped=false" {
		t.Errorf("Error = %q, want 'append_targeted_context: files_read is required when skipped=false'", err.Error())
	}
}

func TestAppendTargetedContextHandler_SkippedFalse_EmptyFilesRead(t *testing.T) {
	t.Parallel()

	handler := AppendTargetedContextHandler()

	args := HandlerArgs{
		WorkflowID: "wf-123",
		Task:       &domain.Task{ID: "task-456"},
		Output: `{
			"skipped": false,
			"keyword_hits": [
				{"keyword": "email", "files": []}
			],
			"files_read": {},
			"token_budget_used": 1000
		}`,
	}

	err := handler(context.Background(), args)
	if err == nil {
		t.Fatal("Expected error for empty files_read")
	}
	if err.Error() != "append_targeted_context: files_read is required when skipped=false" {
		t.Errorf("Error = %q, want 'append_targeted_context: files_read is required when skipped=false'", err.Error())
	}
}

func TestAppendTargetedContextHandler_SkippedFalse_EmptyKeyword(t *testing.T) {
	t.Parallel()

	handler := AppendTargetedContextHandler()

	args := HandlerArgs{
		WorkflowID: "wf-123",
		Task:       &domain.Task{ID: "task-456"},
		Output: `{
			"skipped": false,
			"keyword_hits": [
				{"keyword": "", "files": []}
			],
			"files_read": {
				"internal/user/validation.go": "package user"
			},
			"token_budget_used": 1000
		}`,
	}

	err := handler(context.Background(), args)
	if err == nil {
		t.Fatal("Expected error for empty keyword")
	}
	if err.Error() != "append_targeted_context: keyword_hits[0].keyword is empty" {
		t.Errorf("Error = %q, want 'append_targeted_context: keyword_hits[0].keyword is empty'", err.Error())
	}
}

func TestAppendTargetedContextHandler_InvalidJSON(t *testing.T) {
	t.Parallel()

	handler := AppendTargetedContextHandler()

	args := HandlerArgs{
		WorkflowID: "wf-123",
		Output:     `{invalid json`,
	}

	err := handler(context.Background(), args)
	if err == nil {
		t.Fatal("Expected error for invalid JSON")
	}
	if !strContains(err.Error(), "invalid JSON") {
		t.Errorf("Error = %q, want to contain 'invalid JSON'", err.Error())
	}
}

func TestAppendTargetedContextHandler_EmptyOutput(t *testing.T) {
	t.Parallel()

	handler := AppendTargetedContextHandler()

	args := HandlerArgs{
		WorkflowID: "wf-123",
		Output:     "",
	}

	err := handler(context.Background(), args)
	if err == nil {
		t.Fatal("Expected error for empty output")
	}
	if err.Error() != "append_targeted_context: output is empty" {
		t.Errorf("Error = %q, want 'append_targeted_context: output is empty'", err.Error())
	}
}

func TestAppendTargetedContextHandler_DefaultSkippedIsFalse(t *testing.T) {
	t.Parallel()

	handler := AppendTargetedContextHandler()

	// When skipped is not specified, it defaults to false (Go zero value).
	// This should require keyword_hits and files_read.
	args := HandlerArgs{
		WorkflowID: "wf-123",
		Task:       &domain.Task{ID: "task-456"},
		Output: `{
			"keyword_hits": [
				{"keyword": "test", "files": []}
			],
			"files_read": {
				"test.go": "package test"
			}
		}`,
	}

	err := handler(context.Background(), args)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}
}
