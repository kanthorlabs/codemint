package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// targetedContextOutput is the expected JSON structure from the targeted-gatherer skill.
type targetedContextOutput struct {
	Skipped         bool                      `json:"skipped"`
	Reason          string                    `json:"reason,omitempty"`
	KeywordHits     []keywordHit              `json:"keyword_hits,omitempty"`
	FilesRead       map[string]string         `json:"files_read,omitempty"`
	ImportHops      []importHop               `json:"import_hops,omitempty"`
	TokenBudgetUsed int                       `json:"token_budget_used,omitempty"`
}

// keywordHit represents a keyword and the files where it was found.
type keywordHit struct {
	Keyword string     `json:"keyword"`
	Files   []fileHit  `json:"files"`
}

// fileHit represents a file path and the lines where a keyword was found.
type fileHit struct {
	Path  string `json:"path"`
	Lines []int  `json:"lines"`
}

// importHop represents a single import hop followed during targeted gathering.
type importHop struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason"`
}

// AppendTargetedContextHandler returns a HandlerFunc that validates the targeted-gatherer
// skill output. The handler validates the JSON structure but does NOT merge outputs —
// each task's output remains in its own row. Downstream stories read both task outputs
// from the workflow's task list.
//
// Expected output format (skipped=false):
//
//	{
//	  "skipped": false,
//	  "keyword_hits": [...],
//	  "files_read": {...},
//	  "token_budget_used": 18500
//	}
//
// Expected output format (skipped=true):
//
//	{
//	  "skipped": true,
//	  "reason": "No existing code matches goal keywords..."
//	}
//
// Validation:
//   - If skipped=true, reason must be non-empty
//   - If skipped=false, keyword_hits and files_read must be present
//
// Returns error on invalid JSON or validation failure — these become task Failure.
func AppendTargetedContextHandler() HandlerFunc {
	return func(ctx context.Context, args HandlerArgs) error {
		if args.Output == "" {
			return errors.New("append_targeted_context: output is empty")
		}

		// Parse the JSON output.
		var parsed targetedContextOutput
		if err := json.Unmarshal([]byte(args.Output), &parsed); err != nil {
			return fmt.Errorf("append_targeted_context: invalid JSON: %w", err)
		}

		// Validate based on skipped status.
		if parsed.Skipped {
			// Skipped case: reason is required.
			if strings.TrimSpace(parsed.Reason) == "" {
				return errors.New("append_targeted_context: skipped=true requires non-empty reason")
			}
			// Valid skip case — output stays on the task row as-is.
			return nil
		}

		// Non-skipped case: validate required fields.
		if len(parsed.KeywordHits) == 0 {
			return errors.New("append_targeted_context: keyword_hits is required when skipped=false")
		}

		if len(parsed.FilesRead) == 0 {
			return errors.New("append_targeted_context: files_read is required when skipped=false")
		}

		// Validate keyword_hits structure.
		for i, hit := range parsed.KeywordHits {
			if strings.TrimSpace(hit.Keyword) == "" {
				return fmt.Errorf("append_targeted_context: keyword_hits[%d].keyword is empty", i)
			}
		}

		// Validate files_read has non-empty paths.
		for path := range parsed.FilesRead {
			if strings.TrimSpace(path) == "" {
				return errors.New("append_targeted_context: files_read contains empty path key")
			}
		}

		// Output stays on the task row; no merge step needed in v1.
		// The handler exists so malformed JSON converts cleanly to a task Failure.
		return nil
	}
}
