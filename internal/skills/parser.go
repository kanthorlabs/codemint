// Package skills provides skill parsing and registry functionality
// following the AgentSkills specification.
package skills

import (
	"crypto/md5"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codemint.kanthorlabs.com/internal/domain"
	"github.com/yuin/goldmark"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// ErrNotImplemented is returned by provider-specific parsers that have not yet
// been implemented. A concrete implementation will be added once provider-specific
// drift from the AgentSkills spec is identified.
var ErrNotImplemented = errors.New("parser not implemented")

// SkillParser parses a skill directory into a domain.Skill.
type SkillParser interface {
	Parse(dirPath string) (*domain.Skill, error)
}

// GeneralParser fully implements the AgentSkills specification.
// It is the default parser used for all skill directories.
type GeneralParser struct{}

// Parse reads the SKILL.md file in dirPath, parses its YAML frontmatter and
// Markdown body, validates the name field, generates the deterministic ID,
// and scans scripts/ and references/ subdirectories.
func (p GeneralParser) Parse(dirPath string) (*domain.Skill, error) {
	absDir, err := filepath.Abs(dirPath)
	if err != nil {
		return nil, fmt.Errorf("skills: resolve abs path %q: %w", dirPath, err)
	}

	skillPath := filepath.Join(absDir, "SKILL.md")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		return nil, fmt.Errorf("skills: read SKILL.md at %q: %w", skillPath, err)
	}

	md := goldmark.New(goldmark.WithExtensions(meta.Meta))
	ctx := parser.NewContext()
	reader := text.NewReader(data)
	doc := md.Parser().Parse(reader, parser.WithContext(ctx))
	_ = doc

	frontmatter := meta.Get(ctx)

	skill := &domain.Skill{
		ID:        skillID(skillPath),
		SourceDir: absDir,
	}

	if err := parseFrontmatter(frontmatter, skill); err != nil {
		return nil, fmt.Errorf("skills: parse frontmatter at %q: %w", skillPath, err)
	}

	// Validate name matches directory name.
	dirName := filepath.Base(absDir)
	if skill.Name != dirName {
		return nil, fmt.Errorf("skills: name %q does not match directory name %q", skill.Name, dirName)
	}

	// Extract instruction (body without frontmatter).
	skill.Instruction = extractBody(data)

	if err := scanScripts(absDir, skill); err != nil {
		return nil, fmt.Errorf("skills: scan scripts at %q: %w", absDir, err)
	}

	if err := scanReferences(absDir, skill); err != nil {
		return nil, fmt.Errorf("skills: scan references at %q: %w", absDir, err)
	}

	return skill, nil
}

// CursorParser will parse Cursor-specific skill directories.
// Returns ErrNotImplemented until Cursor-specific drift from the AgentSkills
// spec is identified and a concrete implementation is warranted.
type CursorParser struct{}

// Parse returns ErrNotImplemented.
func (p CursorParser) Parse(_ string) (*domain.Skill, error) {
	return nil, ErrNotImplemented
}

// ClaudeParser will parse Claude CLI-specific skill directories.
// Returns ErrNotImplemented until Claude-specific drift is identified.
type ClaudeParser struct{}

// Parse returns ErrNotImplemented.
func (p ClaudeParser) Parse(_ string) (*domain.Skill, error) {
	return nil, ErrNotImplemented
}

// CodexParser will parse Codex-specific skill directories.
// Returns ErrNotImplemented until Codex-specific drift is identified.
type CodexParser struct{}

// Parse returns ErrNotImplemented.
func (p CodexParser) Parse(_ string) (*domain.Skill, error) {
	return nil, ErrNotImplemented
}

// skillID returns the MD5 hash of the absolute SKILL.md path as a hex string.
func skillID(absSkillPath string) string {
	sum := md5.Sum([]byte(absSkillPath))
	return fmt.Sprintf("%x", sum)
}

// parseFrontmatter maps goldmark-meta output into the Skill struct fields.
func parseFrontmatter(fm map[string]any, skill *domain.Skill) error {
	name, _ := fm["name"].(string)
	if name == "" {
		return errors.New("missing required field: name")
	}
	skill.Name = name

	description, _ := fm["description"].(string)
	if description == "" {
		return errors.New("missing required field: description")
	}
	skill.Description = description

	skill.License, _ = fm["license"].(string)
	skill.Compatibility, _ = fm["compatibility"].(string)
	skill.AllowedTools, _ = fm["allowed_tools"].(string)

	if raw, ok := fm["metadata"]; ok {
		switch v := raw.(type) {
		case map[any]any:
			skill.Metadata = make(map[string]string, len(v))
			for mk, mv := range v {
				k, _ := mk.(string)
				val, _ := mv.(string)
				if k != "" {
					skill.Metadata[k] = val
				}
			}
		case map[string]any:
			skill.Metadata = make(map[string]string, len(v))
			for k, mv := range v {
				val, _ := mv.(string)
				skill.Metadata[k] = val
			}
		}
	}

	return nil
}

// extractBody returns the Markdown content after the YAML frontmatter block.
func extractBody(data []byte) string {
	content := string(data)
	if !strings.HasPrefix(content, "---") {
		return content
	}
	// Find closing --- delimiter.
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return content
	}
	// Skip past the closing delimiter and optional newline.
	body := rest[idx+4:]
	return strings.TrimPrefix(body, "\n")
}

// scanScripts populates skill.Scripts from the scripts/ subdirectory.
func scanScripts(absDir string, skill *domain.Skill) error {
	scriptsDir := filepath.Join(absDir, "scripts")
	entries, err := os.ReadDir(scriptsDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		skill.Scripts = append(skill.Scripts, domain.SkillScript{
			Name:       name,
			Executable: filepath.Join("scripts", entry.Name()),
		})
	}
	return nil
}

// scanReferences populates skill.References from the references/ subdirectory.
func scanReferences(absDir string, skill *domain.Skill) error {
	refsDir := filepath.Join(absDir, "references")
	entries, err := os.ReadDir(refsDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		skill.References = append(skill.References, filepath.Join("references", entry.Name()))
	}
	return nil
}
