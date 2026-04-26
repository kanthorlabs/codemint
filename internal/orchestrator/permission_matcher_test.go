package orchestrator

import (
	"encoding/json"
	"testing"

	"codemint.kanthorlabs.com/internal/domain"
)

func TestMatcher_Evaluate(t *testing.T) {
	tests := []struct {
		name     string
		perm     *domain.ProjectPermission
		command  string
		cwd      string
		expected Decision
	}{
		{
			name:     "nil permission returns unknown",
			perm:     nil,
			command:  "go test ./...",
			cwd:      "/project",
			expected: DecisionUnknown,
		},
		{
			name: "empty permission returns unknown",
			perm: &domain.ProjectPermission{
				AllowedCommands:    nil,
				AllowedDirectories: nil,
				BlockedCommands:    nil,
			},
			command:  "go test ./...",
			cwd:      "/project",
			expected: DecisionUnknown,
		},
		{
			name: "allowed command in allowed directory",
			perm: &domain.ProjectPermission{
				AllowedCommands:    mustJSON([]string{"go test", "go build"}),
				AllowedDirectories: mustJSON([]string{"/project"}),
			},
			command:  "go test ./...",
			cwd:      "/project",
			expected: DecisionAllow,
		},
		{
			name: "allowed command in subdirectory of allowed directory",
			perm: &domain.ProjectPermission{
				AllowedCommands:    mustJSON([]string{"go test"}),
				AllowedDirectories: mustJSON([]string{"/project"}),
			},
			command:  "go test ./...",
			cwd:      "/project/internal/orchestrator",
			expected: DecisionAllow,
		},
		{
			name: "allowed command but outside allowed directory",
			perm: &domain.ProjectPermission{
				AllowedCommands:    mustJSON([]string{"go test"}),
				AllowedDirectories: mustJSON([]string{"/project"}),
			},
			command:  "go test ./...",
			cwd:      "/other/directory",
			expected: DecisionUnknown,
		},
		{
			name: "blocked command takes precedence over allowed",
			perm: &domain.ProjectPermission{
				AllowedCommands:    mustJSON([]string{"rm"}),
				AllowedDirectories: mustJSON([]string{"/project"}),
				BlockedCommands:    mustJSON([]string{"rm -rf"}),
			},
			command:  "rm -rf /",
			cwd:      "/project",
			expected: DecisionBlock,
		},
		{
			name: "blocked command without allowed returns block",
			perm: &domain.ProjectPermission{
				BlockedCommands: mustJSON([]string{"sudo", "rm -rf"}),
			},
			command:  "sudo apt-get install",
			cwd:      "/project",
			expected: DecisionBlock,
		},
		{
			name: "partial prefix match",
			perm: &domain.ProjectPermission{
				AllowedCommands:    mustJSON([]string{"go"}),
				AllowedDirectories: mustJSON([]string{"/project"}),
			},
			command:  "go test -v -race ./...",
			cwd:      "/project",
			expected: DecisionAllow,
		},
		{
			name: "no prefix match returns unknown",
			perm: &domain.ProjectPermission{
				AllowedCommands:    mustJSON([]string{"go test"}),
				AllowedDirectories: mustJSON([]string{"/project"}),
			},
			command:  "go build ./...",
			cwd:      "/project",
			expected: DecisionUnknown,
		},
		{
			name: "empty directories list allows all directories",
			perm: &domain.ProjectPermission{
				AllowedCommands:    mustJSON([]string{"echo"}),
				AllowedDirectories: nil, // Empty means all directories allowed
			},
			command:  "echo hello",
			cwd:      "/any/directory",
			expected: DecisionAllow,
		},
		{
			name: "command with shell metacharacters",
			perm: &domain.ProjectPermission{
				AllowedCommands:    mustJSON([]string{"echo"}),
				AllowedDirectories: mustJSON([]string{"/project"}),
			},
			command:  `echo "hello world" | grep hello`,
			cwd:      "/project",
			expected: DecisionAllow, // shlex parses this correctly
		},
		{
			name: "relative path traversal outside allowed dir",
			perm: &domain.ProjectPermission{
				AllowedCommands:    mustJSON([]string{"cat"}),
				AllowedDirectories: mustJSON([]string{"/project/src"}),
			},
			command:  "cat file.txt",
			cwd:      "/project", // /project is NOT inside /project/src
			expected: DecisionUnknown,
		},
		{
			name: "multiple allowed directories",
			perm: &domain.ProjectPermission{
				AllowedCommands:    mustJSON([]string{"ls"}),
				AllowedDirectories: mustJSON([]string{"/project/src", "/project/tests"}),
			},
			command:  "ls -la",
			cwd:      "/project/tests/unit",
			expected: DecisionAllow,
		},
		{
			name: "exact directory match",
			perm: &domain.ProjectPermission{
				AllowedCommands:    mustJSON([]string{"pwd"}),
				AllowedDirectories: mustJSON([]string{"/project"}),
			},
			command:  "pwd",
			cwd:      "/project",
			expected: DecisionAllow,
		},
		{
			name: "empty command returns unknown",
			perm: &domain.ProjectPermission{
				AllowedCommands:    mustJSON([]string{"go"}),
				AllowedDirectories: mustJSON([]string{"/project"}),
			},
			command:  "",
			cwd:      "/project",
			expected: DecisionUnknown,
		},
		{
			name: "unparseable command returns unknown",
			perm: &domain.ProjectPermission{
				AllowedCommands:    mustJSON([]string{"go"}),
				AllowedDirectories: mustJSON([]string{"/project"}),
			},
			command:  `echo "unclosed quote`,
			cwd:      "/project",
			expected: DecisionUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewMatcher(tt.perm)
			result := matcher.Evaluate(tt.command, tt.cwd)
			if result != tt.expected {
				t.Errorf("Evaluate(%q, %q) = %s; want %s", tt.command, tt.cwd, result, tt.expected)
			}
		})
	}
}

func TestDecision_String(t *testing.T) {
	tests := []struct {
		decision Decision
		expected string
	}{
		{DecisionUnknown, "unknown"},
		{DecisionAllow, "allow"},
		{DecisionBlock, "block"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.decision.String(); got != tt.expected {
				t.Errorf("Decision.String() = %q; want %q", got, tt.expected)
			}
		})
	}
}

func TestIsSubdirectoryOf(t *testing.T) {
	tests := []struct {
		name     string
		child    string
		parent   string
		expected bool
	}{
		{"exact match", "/project", "/project", true},
		{"subdirectory", "/project/src", "/project", true},
		{"deep subdirectory", "/project/src/internal/foo", "/project", true},
		{"parent directory", "/project", "/project/src", false},
		{"sibling directory", "/other", "/project", false},
		{"with trailing slashes", "/project/src/", "/project/", true},
		{"relative components", "/project/../project/src", "/project", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSubdirectoryOf(tt.child, tt.parent)
			if result != tt.expected {
				t.Errorf("isSubdirectoryOf(%q, %q) = %v; want %v", tt.child, tt.parent, result, tt.expected)
			}
		})
	}
}

// mustJSON marshals v to JSON and returns it as NullableJSON.
// Panics on error (test helper only).
func mustJSON(v any) domain.NullableJSON {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return domain.NullableJSON(data)
}
