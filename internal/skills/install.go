package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// InstallSystemSkills extracts embedded skills to ~/.agents/skills/<name>
// and creates Claude symlinks at ~/.claude/skills/<name>. Returns the
// list of installed skill names.
//
// This function is idempotent: re-running it will overwrite existing files
// with the latest embedded versions, ensuring the user always has up-to-date
// system skills.
func InstallSystemSkills() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("skills: resolve home dir: %w", err)
	}

	agentsDir := filepath.Join(home, ".agents", "skills")

	// Ensure target directory exists.
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		return nil, fmt.Errorf("skills: create agents dir: %w", err)
	}

	// Walk embedded FS and extract each skill.
	var installed []string
	seenDirs := make(map[string]bool)

	err = fs.WalkDir(embeddedFS, "embedded", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip root "embedded" directory.
		if path == "embedded" {
			return nil
		}

		// path format: embedded/<skill-name>/... or embedded/<skill-name>
		relPath := strings.TrimPrefix(path, "embedded/")
		parts := strings.SplitN(relPath, "/", 2)
		skillName := parts[0]

		// Track which skills we're installing.
		if !seenDirs[skillName] {
			seenDirs[skillName] = true
			installed = append(installed, skillName)
		}

		// Target path: ~/.agents/skills/<skill-name>/...
		var targetPath string
		if len(parts) == 1 {
			// Skill directory itself.
			targetPath = filepath.Join(agentsDir, skillName)
		} else {
			// File or subdirectory within the skill.
			targetPath = filepath.Join(agentsDir, skillName, parts[1])
		}

		if d.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}

		// Read file from embedded FS and write to disk.
		data, readErr := embeddedFS.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read embedded %q: %w", path, readErr)
		}

		// Ensure parent directory exists.
		if mkErr := os.MkdirAll(filepath.Dir(targetPath), 0o755); mkErr != nil {
			return fmt.Errorf("create parent dir for %q: %w", targetPath, mkErr)
		}

		return os.WriteFile(targetPath, data, 0o644)
	})

	if err != nil {
		return nil, fmt.Errorf("skills: extract embedded: %w", err)
	}

	// Create Claude symlinks for each installed system skill.
	for _, name := range installed {
		if symlinkErr := EnsureClaudeSymlink(name); symlinkErr != nil {
			return installed, fmt.Errorf("skills: symlink %q: %w", name, symlinkErr)
		}
	}

	return installed, nil
}

// EmbeddedSkillNames returns the list of skill names from the embedded FS.
// Used to identify which skills are "system" skills vs user-installed.
func EmbeddedSkillNames() []string {
	entries, err := fs.ReadDir(embeddedFS, "embedded")
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
}

// IsEmbeddedSkill returns true if the given skill name is a system skill
// (i.e., exists in the embedded FS).
func IsEmbeddedSkill(name string) bool {
	for _, n := range EmbeddedSkillNames() {
		if n == name {
			return true
		}
	}
	return false
}

// CleanupOrphanedSystemSkills removes system skills from ~/.agents/skills
// that are no longer present in the embedded FS. This ensures users don't
// have stale system skills after upgrades.
//
// Only removes skills that were previously installed as system skills
// (identified by checking if they exist in embedded FS history).
func CleanupOrphanedSystemSkills() error {
	// For now, we don't track historical system skills, so we can't safely
	// identify orphans without risking deletion of user-installed skills
	// that happen to have the same name as a removed system skill.
	//
	// TODO: Track installed system skills in a manifest file to enable
	// safe orphan cleanup.
	return nil
}

// EnsureSystemSkills is the main entry point for system skill management at
// boot time. It:
//  1. Installs/updates embedded skills to ~/.agents/skills/<name>
//  2. Creates Claude symlinks
//  3. Logs warnings about third-party skill symlinks
//
// Returns the list of installed system skill names.
func EnsureSystemSkills() ([]string, error) {
	installed, err := InstallSystemSkills()
	if err != nil {
		return nil, err
	}

	if err := CleanupOrphanedSystemSkills(); err != nil {
		return installed, err
	}

	// Log warnings about third-party skills (non-blocking).
	LogClaudeSymlinkWarnings()

	return installed, nil
}
