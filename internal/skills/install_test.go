package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallSystemSkills(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	installed, err := InstallSystemSkills()
	if err != nil {
		t.Fatalf("InstallSystemSkills() error = %v", err)
	}

	// We should have installed at least one skill (seniorgodev).
	if len(installed) == 0 {
		t.Error("expected at least one installed skill")
	}

	// Check that files were extracted.
	agentsDir := filepath.Join(tmpDir, ".agents", "skills")
	for _, name := range installed {
		prefixedName := name
		skillDir := filepath.Join(agentsDir, prefixedName)

		// Check skill directory exists.
		if _, err := os.Stat(skillDir); err != nil {
			t.Errorf("skill dir %q not found: %v", skillDir, err)
		}

		// Check SKILL.md exists.
		skillFile := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(skillFile); err != nil {
			t.Errorf("SKILL.md not found in %q: %v", skillDir, err)
		}

		// Check Claude symlink was created.
		claudeSymlink := filepath.Join(tmpDir, ".claude", "skills", prefixedName)
		info, err := os.Lstat(claudeSymlink)
		if err != nil {
			t.Errorf("Claude symlink not created for %q: %v", prefixedName, err)
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("expected symlink at %q, got regular file/dir", claudeSymlink)
		}
	}
}

func TestInstallSystemSkills_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// First install.
	installed1, err := InstallSystemSkills()
	if err != nil {
		t.Fatalf("first InstallSystemSkills() error = %v", err)
	}

	// Second install should succeed and return same skills.
	installed2, err := InstallSystemSkills()
	if err != nil {
		t.Fatalf("second InstallSystemSkills() error = %v", err)
	}

	if len(installed1) != len(installed2) {
		t.Errorf("installed count changed: %d -> %d", len(installed1), len(installed2))
	}
}

func TestCleanupOrphanedSystemSkills(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// First install system skills.
	if _, err := InstallSystemSkills(); err != nil {
		t.Fatalf("InstallSystemSkills() error = %v", err)
	}

	// CleanupOrphanedSystemSkills is currently a no-op (safe behavior).
	// Just verify it doesn't error.
	if err := CleanupOrphanedSystemSkills(); err != nil {
		t.Fatalf("CleanupOrphanedSystemSkills() error = %v", err)
	}
}

func TestEnsureSystemSkills(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	installed, err := EnsureSystemSkills()
	if err != nil {
		t.Fatalf("EnsureSystemSkills() error = %v", err)
	}

	if len(installed) == 0 {
		t.Error("expected at least one installed skill")
	}

	// Verify seniorgodev was installed (known embedded skill).
	hasGodev := false
	for _, name := range installed {
		if name == "seniorgodev" {
			hasGodev = true
			break
		}
	}
	if !hasGodev {
		t.Errorf("expected seniorgodev to be installed, got: %v", installed)
	}
}

func TestInstallSystemSkills_ReinstallUpdatesContent(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// First install.
	installed, err := InstallSystemSkills()
	if err != nil {
		t.Fatalf("first InstallSystemSkills() error = %v", err)
	}
	if len(installed) == 0 {
		t.Fatal("expected at least one skill installed")
	}

	// Get path to first skill's SKILL.md.
	agentsDir := filepath.Join(tmpDir, ".agents", "skills")
	skillDir := filepath.Join(agentsDir, installed[0])
	skillFile := filepath.Join(skillDir, "SKILL.md")

	// Read original content.
	origContent, err := os.ReadFile(skillFile)
	if err != nil {
		t.Fatalf("read original SKILL.md: %v", err)
	}

	// Modify the file to simulate local changes.
	modifiedContent := []byte("# MODIFIED BY TEST\nThis should be overwritten.")
	if err := os.WriteFile(skillFile, modifiedContent, 0o644); err != nil {
		t.Fatalf("write modified SKILL.md: %v", err)
	}

	// Re-install should overwrite with embedded version.
	if _, err := InstallSystemSkills(); err != nil {
		t.Fatalf("second InstallSystemSkills() error = %v", err)
	}

	// Verify content was restored to original.
	newContent, err := os.ReadFile(skillFile)
	if err != nil {
		t.Fatalf("read restored SKILL.md: %v", err)
	}

	if string(newContent) != string(origContent) {
		t.Errorf("re-install did not restore original content\ngot: %s\nwant: %s",
			string(newContent)[:min(100, len(newContent))],
			string(origContent)[:min(100, len(origContent))])
	}
}

func TestClaudeSymlink_ResolvesToCorrectContent(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Install system skills.
	installed, err := InstallSystemSkills()
	if err != nil {
		t.Fatalf("InstallSystemSkills() error = %v", err)
	}
	if len(installed) == 0 {
		t.Fatal("expected at least one skill installed")
	}

	agentsDir := filepath.Join(tmpDir, ".agents", "skills")
	claudeDir := filepath.Join(tmpDir, ".claude", "skills")

	for _, name := range installed {
		prefixedName := name

		// Read SKILL.md via agents path (canonical).
		agentsSkillFile := filepath.Join(agentsDir, prefixedName, "SKILL.md")
		agentsContent, err := os.ReadFile(agentsSkillFile)
		if err != nil {
			t.Errorf("read agents SKILL.md for %q: %v", name, err)
			continue
		}

		// Read SKILL.md via Claude symlink path.
		claudeSkillFile := filepath.Join(claudeDir, prefixedName, "SKILL.md")
		claudeContent, err := os.ReadFile(claudeSkillFile)
		if err != nil {
			t.Errorf("read claude SKILL.md for %q: %v", name, err)
			continue
		}

		// Content should be identical (symlink resolves correctly).
		if string(agentsContent) != string(claudeContent) {
			t.Errorf("Claude symlink content mismatch for %q:\nagents: %s\nclaude: %s",
				name,
				string(agentsContent)[:min(50, len(agentsContent))],
				string(claudeContent)[:min(50, len(claudeContent))])
		}
	}
}

func TestRegistryGet_AfterSystemSkillsInstall(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Install system skills first.
	installed, err := InstallSystemSkills()
	if err != nil {
		t.Fatalf("InstallSystemSkills() error = %v", err)
	}
	if len(installed) == 0 {
		t.Fatal("expected at least one skill installed")
	}

	// Create registry and load all skills.
	reg := NewRegistry()
	if err := reg.LoadAll(); err != nil {
		t.Fatalf("Registry.LoadAll() error = %v", err)
	}

	// Verify we can get each installed system skill by name.
	for _, name := range installed {
		skill, ok := reg.GetByName(name)
		if !ok {
			t.Errorf("Registry.GetByName(%q) returned false, want true", name)
			continue
		}

		// Verify skill has content.
		if skill.Instruction == "" {
			t.Errorf("skill %q has empty Instruction", name)
		}

		// Verify skill Name matches.
		if skill.Name != name {
			t.Errorf("skill.Name = %q, want %q", skill.Name, name)
		}
	}

	// Verify registry.All() includes all installed skills by checking Names.
	all := reg.All()
	for _, name := range installed {
		found := false
		for _, skill := range all {
			if skill.Name == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Registry.All() missing skill with Name %q", name)
		}
	}
}

func TestSkillInvocation_EndToEnd(t *testing.T) {
	// This test verifies the complete flow:
	// 1. Install system skills
	// 2. Load registry
	// 3. Get skill by name
	// 4. Verify skill body can be used (non-empty, valid content)

	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Install and load.
	if _, err := InstallSystemSkills(); err != nil {
		t.Fatalf("InstallSystemSkills() error = %v", err)
	}

	reg := NewRegistry()
	if err := reg.LoadAll(); err != nil {
		t.Fatalf("Registry.LoadAll() error = %v", err)
	}

	// Get seniorgodev skill by name (known to exist in embedded).
	skillName := "seniorgodev"
	skill, ok := reg.GetByName(skillName)
	if !ok {
		t.Fatalf("expected skill %q to be registered", skillName)
	}

	// Verify skill properties.
	if skill.Name != skillName {
		t.Errorf("skill.Name = %q, want %q", skill.Name, skillName)
	}
	if skill.Instruction == "" {
		t.Error("skill.Instruction should not be empty")
	}
	if skill.ID == "" {
		t.Error("skill.ID should not be empty")
	}

	// Verify body contains expected content (from SKILL.md).
	if !strings.Contains(skill.Instruction, "Go") && !strings.Contains(skill.Instruction, "go") {
		t.Errorf("seniorgodev skill body should mention Go, got: %s",
			skill.Instruction[:min(200, len(skill.Instruction))])
	}
}

func TestGathererSkillLoads(t *testing.T) {
	// This test verifies the gatherer skill is properly embedded and loads.
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Install and load.
	installed, err := InstallSystemSkills()
	if err != nil {
		t.Fatalf("InstallSystemSkills() error = %v", err)
	}

	// Verify gatherer is in installed list.
	hasGatherer := false
	for _, name := range installed {
		if name == "gatherer" {
			hasGatherer = true
			break
		}
	}
	if !hasGatherer {
		t.Fatalf("expected gatherer to be in installed skills, got: %v", installed)
	}

	reg := NewRegistry()
	if err := reg.LoadAll(); err != nil {
		t.Fatalf("Registry.LoadAll() error = %v", err)
	}

	// Get gatherer skill by name.
	skill, ok := reg.GetByName("gatherer")
	if !ok {
		t.Fatal("expected gatherer skill to be registered")
	}

	// Verify skill properties.
	if skill.Name != "gatherer" {
		t.Errorf("skill.Name = %q, want %q", skill.Name, "gatherer")
	}
	if skill.Description == "" {
		t.Error("skill.Description should not be empty")
	}
	if skill.Instruction == "" {
		t.Error("skill.Instruction should not be empty")
	}

	// Verify body contains expected content (JSON output format).
	expectedPhrases := []string{
		"project_summary",
		"key_files",
		"tech_stack",
		"directory_tree",
	}
	for _, phrase := range expectedPhrases {
		if !strings.Contains(skill.Instruction, phrase) {
			t.Errorf("gatherer skill body should contain %q", phrase)
		}
	}
}

func TestGoalCaptureSkillLoads(t *testing.T) {
	// This test verifies the goal-capture skill is properly embedded and loads.
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Install and load.
	installed, err := InstallSystemSkills()
	if err != nil {
		t.Fatalf("InstallSystemSkills() error = %v", err)
	}

	// Verify goal-capture is in installed list.
	hasGoalCapture := false
	for _, name := range installed {
		if name == "goal-capture" {
			hasGoalCapture = true
			break
		}
	}
	if !hasGoalCapture {
		t.Fatalf("expected goal-capture to be in installed skills, got: %v", installed)
	}

	reg := NewRegistry()
	if err := reg.LoadAll(); err != nil {
		t.Fatalf("Registry.LoadAll() error = %v", err)
	}

	// Get goal-capture skill by name.
	skill, ok := reg.GetByName("goal-capture")
	if !ok {
		t.Fatal("expected goal-capture skill to be registered")
	}

	// Verify skill properties.
	if skill.Name != "goal-capture" {
		t.Errorf("skill.Name = %q, want %q", skill.Name, "goal-capture")
	}
	if skill.Description == "" {
		t.Error("skill.Description should not be empty")
	}
	if skill.Instruction == "" {
		t.Error("skill.Instruction should not be empty")
	}

	// Verify body contains expected content (goal and success criteria).
	expectedPhrases := []string{
		"goal_text",
		"success_criteria",
		"Pass 1",
		"Pass 2",
		"/lock-goal",
	}
	for _, phrase := range expectedPhrases {
		if !strings.Contains(skill.Instruction, phrase) {
			t.Errorf("goal-capture skill body should contain %q", phrase)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
