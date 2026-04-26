package orchestrator

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"codemint.kanthorlabs.com/internal/domain"
	"github.com/google/shlex"
)

// Decision represents the outcome of evaluating a command against permissions.
type Decision int

const (
	// DecisionUnknown indicates the command is neither explicitly allowed nor blocked.
	DecisionUnknown Decision = iota
	// DecisionAllow indicates the command matches allowed rules and can be executed.
	DecisionAllow
	// DecisionBlock indicates the command matches blocked rules and must not execute.
	DecisionBlock
)

// String returns a human-readable name for the decision.
func (d Decision) String() string {
	switch d {
	case DecisionAllow:
		return "allow"
	case DecisionBlock:
		return "block"
	default:
		return "unknown"
	}
}

// Matcher evaluates commands against a project's permission configuration.
// It determines whether a command should be allowed, blocked, or requires
// human approval (unknown).
type Matcher struct {
	allowedCommands    []string
	allowedDirectories []string
	blockedCommands    []string
}

// NewMatcher creates a new Matcher from a ProjectPermission configuration.
// If perm is nil, the matcher treats all commands as unknown.
func NewMatcher(perm *domain.ProjectPermission) *Matcher {
	m := &Matcher{}
	if perm == nil {
		return m
	}

	// Parse allowed commands
	if perm.AllowedCommands != nil {
		var commands []string
		if err := json.Unmarshal(perm.AllowedCommands, &commands); err == nil {
			m.allowedCommands = commands
		}
	}

	// Parse allowed directories
	if perm.AllowedDirectories != nil {
		var dirs []string
		if err := json.Unmarshal(perm.AllowedDirectories, &dirs); err == nil {
			m.allowedDirectories = dirs
		}
	}

	// Parse blocked commands
	if perm.BlockedCommands != nil {
		var commands []string
		if err := json.Unmarshal(perm.BlockedCommands, &commands); err == nil {
			m.blockedCommands = commands
		}
	}

	return m
}

// Evaluate determines the decision for a command executed in the given working directory.
// The evaluation order is:
//  1. If command matches any blocked prefix → DecisionBlock
//  2. If command matches any allowed prefix AND cwd is inside allowed directories → DecisionAllow
//  3. Otherwise → DecisionUnknown
func (m *Matcher) Evaluate(command, cwd string) Decision {
	// Tokenize the command to get the command prefix for matching
	tokens, err := shlex.Split(command)
	if err != nil || len(tokens) == 0 {
		// Can't parse the command, treat as unknown
		return DecisionUnknown
	}

	// Check blocked commands first (highest priority)
	if m.matchesCommandPrefix(tokens, m.blockedCommands) {
		return DecisionBlock
	}

	// Check allowed commands and directories
	if m.matchesCommandPrefix(tokens, m.allowedCommands) {
		if m.isInAllowedDirectory(cwd) {
			return DecisionAllow
		}
	}

	return DecisionUnknown
}

// matchesCommandPrefix checks if the tokenized command matches any of the prefix patterns.
// For example, ["go", "test", "./..."] matches prefix "go test" or "go".
func (m *Matcher) matchesCommandPrefix(tokens []string, prefixes []string) bool {
	for _, prefix := range prefixes {
		prefixTokens, err := shlex.Split(prefix)
		if err != nil || len(prefixTokens) == 0 {
			continue
		}

		if matchesTokenPrefix(tokens, prefixTokens) {
			return true
		}
	}
	return false
}

// matchesTokenPrefix checks if tokens starts with prefixTokens.
func matchesTokenPrefix(tokens, prefixTokens []string) bool {
	if len(tokens) < len(prefixTokens) {
		return false
	}

	for i, pt := range prefixTokens {
		if tokens[i] != pt {
			return false
		}
	}
	return true
}

// isInAllowedDirectory checks if cwd is inside any of the allowed directories.
// An empty allowed directories list means all directories are allowed.
func (m *Matcher) isInAllowedDirectory(cwd string) bool {
	// If no directories are specified, allow all
	if len(m.allowedDirectories) == 0 {
		return true
	}

	// Clean and normalize the cwd
	cwd = filepath.Clean(cwd)

	for _, allowedDir := range m.allowedDirectories {
		allowedDir = filepath.Clean(allowedDir)

		// Check if cwd is the same as or a subdirectory of allowedDir
		if isSubdirectoryOf(cwd, allowedDir) {
			return true
		}
	}

	return false
}

// isSubdirectoryOf checks if child is the same as or a subdirectory of parent.
// This uses filepath.Rel to compute the relative path and checks for ".." traversal.
func isSubdirectoryOf(child, parent string) bool {
	// Get the relative path from parent to child
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}

	// If rel starts with "..", child is outside parent
	// If rel is ".", child equals parent
	// Otherwise, child is inside parent
	return !strings.HasPrefix(rel, "..") && rel != ".."
}
