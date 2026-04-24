package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"codemint.kanthorlabs.com/internal/domain"
)

// Registry aggregates skills from multiple source directories and the embedded
// core skills. Embedded skills always have the highest precedence: if the same
// Skill ID is found in an external directory and the embedded set, the embedded
// version wins.
type Registry struct {
	skills map[string]domain.Skill
}

// NewRegistry creates an empty Registry. Call LoadAll to populate it.
func NewRegistry() *Registry {
	return &Registry{skills: make(map[string]domain.Skill)}
}

// LoadAll scans external skill directories in ascending-precedence order, then
// loads the embedded core skills last so they always win on ID collisions.
//
// External directories are scanned with GeneralParser until provider-specific
// parsers are implemented. Directories that do not exist are silently skipped.
func (r *Registry) LoadAll() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("skills: resolve home dir: %w", err)
	}

	targets := []struct {
		path   string
		parser SkillParser
	}{
		{filepath.Join(home, ".agents", "skills"), GeneralParser{}},
		{filepath.Join(home, ".codex", "skills"), CodexParser{}},
		{filepath.Join(home, ".claude", "skills"), ClaudeParser{}},
		{filepath.Join(home, ".cursor", "skills"), CursorParser{}},
		{filepath.Join(home, ".local", "share", "codemint", "skills"), GeneralParser{}},
	}

	for _, t := range targets {
		if err := r.loadDir(t.path, t.parser); err != nil {
			return err
		}
	}

	// Load embedded skills last — highest precedence.
	if err := r.loadEmbedded(); err != nil {
		return fmt.Errorf("skills: load embedded: %w", err)
	}

	return nil
}

// All returns a copy of all registered skills keyed by ID.
func (r *Registry) All() map[string]domain.Skill {
	out := make(map[string]domain.Skill, len(r.skills))
	for k, v := range r.skills {
		out[k] = v
	}
	return out
}

// Get returns the skill with the given ID, if present.
func (r *Registry) Get(id string) (domain.Skill, bool) {
	s, ok := r.skills[id]
	return s, ok
}

// loadDir iterates over immediate subdirectories of dirPath and parses each
// as a skill. Non-existent directories are silently skipped. Parse failures
// for individual skills are returned as errors.
func (r *Registry) loadDir(dirPath string, parser SkillParser) error {
	entries, err := os.ReadDir(dirPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("skills: read dir %q: %w", dirPath, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillDir := filepath.Join(dirPath, entry.Name())
		// Skip directories that do not contain a SKILL.md file.
		if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); os.IsNotExist(err) {
			continue
		}
		skill, err := parser.Parse(skillDir)
		if err != nil {
			return fmt.Errorf("skills: parse %q: %w", skillDir, err)
		}
		r.skills[skill.ID] = *skill
	}
	return nil
}

// loadEmbedded extracts embedded skills to a temp directory, parses them, and
// registers them with the highest precedence.
func (r *Registry) loadEmbedded() error {
	tmpDir, err := os.MkdirTemp("", "codemint-skills-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := extractEmbedded(tmpDir); err != nil {
		return fmt.Errorf("extract embedded skills: %w", err)
	}

	return r.loadDir(filepath.Join(tmpDir, "embedded"), GeneralParser{})
}

// extractEmbedded writes the embedded FS contents rooted at "embedded" into dst.
func extractEmbedded(dst string) error {
	return fs.WalkDir(embeddedFS, "embedded", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		target := filepath.Join(dst, path)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		data, err := embeddedFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embedded %q: %w", path, err)
		}
		return os.WriteFile(target, data, 0o644)
	})
}
