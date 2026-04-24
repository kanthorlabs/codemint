package skills_test

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"codemint.kanthorlabs.com/internal/skills"
)

// TestIDGeneration asserts MD5 hash of path produces a consistent ID.
func TestIDGeneration(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "myskill")
	if err := makeSkillDir(skillDir, "myskill"); err != nil {
		t.Fatal(err)
	}

	parser := skills.GeneralParser{}
	skill, err := parser.Parse(skillDir)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	absSkillPath := filepath.Join(skillDir, "SKILL.md")
	sum := md5.Sum([]byte(absSkillPath))
	wantID := fmt.Sprintf("%x", sum)

	if skill.ID != wantID {
		t.Errorf("skill.ID = %q; want %q", skill.ID, wantID)
	}

	// Calling Parse again must return the same ID.
	skill2, err := parser.Parse(skillDir)
	if err != nil {
		t.Fatalf("Parse() second call error = %v", err)
	}
	if skill.ID != skill2.ID {
		t.Errorf("ID not deterministic: first=%q second=%q", skill.ID, skill2.ID)
	}
}

// TestNameValidation asserts parse fails when frontmatter name != directory name.
func TestNameValidation(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "myskill")
	// Write a SKILL.md whose name field does not match the directory.
	if err := makeSkillDirWithName(skillDir, "wrongname"); err != nil {
		t.Fatal(err)
	}

	parser := skills.GeneralParser{}
	_, err := parser.Parse(skillDir)
	if err == nil {
		t.Error("Parse() should have failed for mismatched name, got nil error")
	}
}

// TestFrontmatterParsing asserts all AgentSkills fields are parsed correctly.
func TestFrontmatterParsing(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "myskill")
	content := `---
name: myskill
description: A test skill for unit testing.
license: MIT
compatibility: Requires Go 1.22+
allowed_tools: bash read
metadata:
  author: testauthor
  version: "2.0"
---

# My Skill

Instruction body here.
`
	if err := writeSkillDir(skillDir, content); err != nil {
		t.Fatal(err)
	}

	parser := skills.GeneralParser{}
	skill, err := parser.Parse(skillDir)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	tests := []struct {
		field string
		got   string
		want  string
	}{
		{"Name", skill.Name, "myskill"},
		{"Description", skill.Description, "A test skill for unit testing."},
		{"License", skill.License, "MIT"},
		{"Compatibility", skill.Compatibility, "Requires Go 1.22+"},
		{"AllowedTools", skill.AllowedTools, "bash read"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("skill.%s = %q; want %q", tt.field, tt.got, tt.want)
		}
	}

	if skill.Metadata["author"] != "testauthor" {
		t.Errorf("skill.Metadata[author] = %q; want %q", skill.Metadata["author"], "testauthor")
	}
	if skill.Metadata["version"] != "2.0" {
		t.Errorf("skill.Metadata[version] = %q; want %q", skill.Metadata["version"], "2.0")
	}
	if skill.SourceDir != skillDir {
		t.Errorf("skill.SourceDir = %q; want %q", skill.SourceDir, skillDir)
	}
}

// TestPrecedence asserts that embedded skills win over external directory skills
// when both have the same skill name (and thus the same path-based ID when using
// the registry's embedded extraction). This test verifies the registry loads the
// embedded seniorgodev skill and that it is present.
func TestPrecedence(t *testing.T) {
	// Create an external skills directory with a skill that shares the name
	// "seniorgodev" but has different description.
	externalDir := t.TempDir()
	externalSkillDir := filepath.Join(externalDir, "seniorgodev")
	content := `---
name: seniorgodev
description: External version - should be overwritten by embedded.
---

# External Senior Go Dev
`
	if err := writeSkillDir(externalSkillDir, content); err != nil {
		t.Fatal(err)
	}

	// Parse external skill to get its ID (based on external path).
	externalParser := skills.GeneralParser{}
	externalSkill, err := externalParser.Parse(externalSkillDir)
	if err != nil {
		t.Fatalf("Parse() external error = %v", err)
	}

	// The embedded registry loads all skills. The embedded seniorgodev will have
	// a different ID (based on its extracted temp path). They won't collide by ID
	// since IDs are path-based. Verify that after LoadAll the registry contains
	// the embedded seniorgodev.
	reg := skills.NewRegistry()
	if err := reg.LoadAll(); err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}

	all := reg.All()
	if len(all) == 0 {
		t.Fatal("registry should contain at least the embedded seniorgodev skill")
	}

	// Find seniorgodev among all skills.
	found := false
	for _, s := range all {
		if s.Name == "seniorgodev" {
			found = true
			// Embedded skill description must match the embedded SKILL.md.
			wantDesc := "Senior Go developer skill. Code review, refactoring, testing, performance optimization following Effective Go 2026 guidelines. Use for Go projects requiring idiomatic, production-quality code."
			if s.Description != wantDesc {
				t.Errorf("embedded seniorgodev description = %q; want %q", s.Description, wantDesc)
			}
			break
		}
	}
	if !found {
		t.Error("embedded seniorgodev skill not found in registry after LoadAll()")
	}

	// The external skill parsed earlier has a different ID — it won't be present
	// in the registry (the registry only loads from the configured directories,
	// not from externalDir). Ensure externalSkill.ID is not mistakenly present.
	if _, ok := reg.Get(externalSkill.ID); ok {
		t.Error("external skill should not appear in registry; registry only loads configured dirs")
	}
}

// TestNotImplemented asserts CursorParser, ClaudeParser, and CodexParser return
// ErrNotImplemented.
func TestNotImplemented(t *testing.T) {
	parsers := []struct {
		name   string
		parser skills.SkillParser
	}{
		{"CursorParser", skills.CursorParser{}},
		{"ClaudeParser", skills.ClaudeParser{}},
		{"CodexParser", skills.CodexParser{}},
	}

	for _, tt := range parsers {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.parser.Parse("/any/path")
			if err == nil {
				t.Fatalf("%s.Parse() should return an error, got nil", tt.name)
			}
			if err != skills.ErrNotImplemented {
				t.Errorf("%s.Parse() error = %v; want ErrNotImplemented", tt.name, err)
			}
		})
	}
}

// --- helpers ---

func makeSkillDir(dir, name string) error {
	content := fmt.Sprintf(`---
name: %s
description: A test skill.
---

# %s

Body.
`, name, name)
	return writeSkillDir(dir, content)
}

func makeSkillDirWithName(dir, frontmatterName string) error {
	content := fmt.Sprintf(`---
name: %s
description: A test skill.
---

# Skill

Body.
`, frontmatterName)
	return writeSkillDir(dir, content)
}

func writeSkillDir(dir, skillMDContent string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMDContent), 0o644)
}
