package skills

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// EnsureClaudeSymlink creates ~/.claude/skills/<name> as a symlink to
// ~/.agents/skills/<name>. Returns an error if:
//   - the source ~/.agents/skills/<name> does not exist
//   - the destination exists and is not a symlink (would clobber real content)
//   - the destination exists as a symlink to a different target
//
// No-op if the correct symlink already exists.
func EnsureClaudeSymlink(skillName string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("skills: resolve home dir: %w", err)
	}
	src := filepath.Join(home, ".agents", "skills", skillName)
	dst := filepath.Join(home, ".claude", "skills", skillName)

	// Check source exists.
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("skills: source %q missing: %w", src, err)
	}

	// Check destination state.
	info, err := os.Lstat(dst)
	if errors.Is(err, os.ErrNotExist) {
		// Create parent directory if needed.
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("skills: create dir %q: %w", filepath.Dir(dst), err)
		}
		return os.Symlink(src, dst)
	}
	if err != nil {
		return fmt.Errorf("skills: stat %q: %w", dst, err)
	}

	// Destination exists - check if it's a symlink.
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("skills: %q exists and is not a symlink; refusing to overwrite", dst)
	}

	// Check symlink target.
	target, err := os.Readlink(dst)
	if err != nil {
		return fmt.Errorf("skills: readlink %q: %w", dst, err)
	}
	if target != src {
		return fmt.Errorf("skills: %q points to %q, expected %q", dst, target, src)
	}

	// Correct symlink already exists.
	return nil
}

// VerifyClaudeSymlinks walks ~/.agents/skills and returns one warning per
// skill whose Claude symlink is missing, wrong, or replaced by a real dir.
// Never fails — used to feed boot-time slog.Warn lines.
//
// Skills in ~/.agents/skills/codemint-* are assumed to be system skills and
// are skipped (they're auto-symlinked by InstallSystemSkills).
func VerifyClaudeSymlinks() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return []string{fmt.Sprintf("skills: resolve home dir: %v", err)}
	}

	agentsDir := filepath.Join(home, ".agents", "skills")
	claudeDir := filepath.Join(home, ".claude", "skills")

	entries, err := os.ReadDir(agentsDir)
	if os.IsNotExist(err) {
		return nil // No agents skills directory.
	}
	if err != nil {
		return []string{fmt.Sprintf("skills: read %q: %v", agentsDir, err)}
	}

	var warnings []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()

		// Skip system skills (codemint-*) - they're auto-managed.
		if strings.HasPrefix(name, "codemint-") {
			continue
		}

		src := filepath.Join(agentsDir, name)
		dst := filepath.Join(claudeDir, name)

		// Check Claude symlink.
		info, err := os.Lstat(dst)
		if errors.Is(err, os.ErrNotExist) {
			warnings = append(warnings, fmt.Sprintf("skill %q has no Claude symlink at %s; Claude Code agents will not see it", name, dst))
			continue
		}
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("skill %q: stat %q: %v", name, dst, err))
			continue
		}

		if info.Mode()&os.ModeSymlink == 0 {
			warnings = append(warnings, fmt.Sprintf("skill %q at %s is a regular dir, not a symlink; may diverge from canonical %s", name, dst, src))
			continue
		}

		target, err := os.Readlink(dst)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("skill %q: readlink %q: %v", name, dst, err))
			continue
		}
		if target != src {
			warnings = append(warnings, fmt.Sprintf("skill %q symlink points to %s, expected %s", name, target, src))
		}
		// Correct symlink — silent.
	}

	return warnings
}

// LogClaudeSymlinkWarnings calls VerifyClaudeSymlinks and logs each warning.
// Called at boot time to alert users about third-party skill symlink issues.
func LogClaudeSymlinkWarnings() {
	for _, warn := range VerifyClaudeSymlinks() {
		slog.Warn(warn)
	}
}

// IsSymlinkIntoAgentsSkills checks if the given path is a symlink pointing
// into ~/.agents/skills/. Used by the registry's loadDir to avoid
// double-registration of skills.
func IsSymlinkIntoAgentsSkills(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false
	}

	target, err := os.Readlink(path)
	if err != nil {
		return false
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	agentsSkillsDir := filepath.Join(home, ".agents", "skills")

	// Check if target starts with the agents skills directory.
	return strings.HasPrefix(target, agentsSkillsDir)
}
