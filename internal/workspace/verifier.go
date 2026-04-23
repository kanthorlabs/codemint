// Package workspace provides utilities for inspecting the state of the
// local file system workspace managed by CodeMint.
package workspace

import (
	"fmt"
	"os/exec"
	"strings"
)

// Verifier checks the cleanliness of a Git working directory.
type Verifier struct {
	dir string
}

// NewVerifier creates a Verifier rooted at the given directory.
func NewVerifier(dir string) *Verifier {
	return &Verifier{dir: dir}
}

// IsDirty reports whether the Git working tree has any uncommitted changes.
// It runs `git status --porcelain` and returns true if any output is produced.
// Returns an error if the command fails (e.g., the directory is not a Git repo).
func (v *Verifier) IsDirty() (bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = v.dir

	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("workspace: git status in %q: %w", v.dir, err)
	}

	return strings.TrimSpace(string(out)) != "", nil
}
