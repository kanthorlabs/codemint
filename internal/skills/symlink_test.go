package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureClaudeSymlink(t *testing.T) {
	// Create temp directories to simulate ~/.agents/skills and ~/.claude/skills.
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, ".agents", "skills")
	claudeDir := filepath.Join(tmpDir, ".claude", "skills")

	// Create source skill directory.
	skillName := "test-skill"
	srcSkill := filepath.Join(agentsDir, skillName)
	if err := os.MkdirAll(srcSkill, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create SKILL.md to make it a valid skill.
	if err := os.WriteFile(filepath.Join(srcSkill, "SKILL.md"), []byte("# Test"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Override home directory for the test.
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Test: create symlink.
	if err := EnsureClaudeSymlink(skillName); err != nil {
		t.Fatalf("EnsureClaudeSymlink() error = %v", err)
	}

	// Verify symlink was created.
	dst := filepath.Join(claudeDir, skillName)
	info, err := os.Lstat(dst)
	if err != nil {
		t.Fatalf("symlink not created: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("expected symlink, got regular file/dir")
	}

	target, err := os.Readlink(dst)
	if err != nil {
		t.Fatal(err)
	}
	if target != srcSkill {
		t.Errorf("symlink target = %q, want %q", target, srcSkill)
	}

	// Test: idempotent - calling again should succeed.
	if err := EnsureClaudeSymlink(skillName); err != nil {
		t.Errorf("EnsureClaudeSymlink() second call error = %v", err)
	}
}

func TestEnsureClaudeSymlink_MissingSource(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	err := EnsureClaudeSymlink("nonexistent-skill")
	if err == nil {
		t.Fatal("expected error for missing source, got nil")
	}
}

func TestEnsureClaudeSymlink_ExistingRegularDir(t *testing.T) {
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, ".agents", "skills")
	claudeDir := filepath.Join(tmpDir, ".claude", "skills")

	skillName := "test-skill"
	srcSkill := filepath.Join(agentsDir, skillName)
	dstSkill := filepath.Join(claudeDir, skillName)

	// Create source.
	if err := os.MkdirAll(srcSkill, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create destination as regular directory (not symlink).
	if err := os.MkdirAll(dstSkill, 0o755); err != nil {
		t.Fatal(err)
	}

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	err := EnsureClaudeSymlink(skillName)
	if err == nil {
		t.Fatal("expected error for existing regular dir, got nil")
	}
}

func TestVerifyClaudeSymlinks(t *testing.T) {
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, ".agents", "skills")
	claudeDir := filepath.Join(tmpDir, ".claude", "skills")

	// Create agents skills.
	skills := []string{"skill-a", "skill-b", "codemint-system"}
	for _, name := range skills {
		if err := os.MkdirAll(filepath.Join(agentsDir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Create correct symlink for skill-a.
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(agentsDir, "skill-a"), filepath.Join(claudeDir, "skill-a")); err != nil {
		t.Fatal(err)
	}

	// skill-b has no symlink (missing).
	// codemint-system is skipped (system skill prefix).

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	warnings := VerifyClaudeSymlinks()

	// Should have one warning for skill-b.
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}

	if len(warnings) > 0 && !contains(warnings[0], "skill-b") {
		t.Errorf("warning should mention skill-b: %q", warnings[0])
	}
}

func TestIsSymlinkIntoAgentsSkills(t *testing.T) {
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, ".agents", "skills")
	claudeDir := filepath.Join(tmpDir, ".claude", "skills")

	// Create skill in agents dir.
	skillSrc := filepath.Join(agentsDir, "my-skill")
	if err := os.MkdirAll(skillSrc, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create symlink in claude dir pointing to agents skill.
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(claudeDir, "my-skill")
	if err := os.Symlink(skillSrc, symlinkPath); err != nil {
		t.Fatal(err)
	}

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Test symlink pointing to agents skills.
	if !IsSymlinkIntoAgentsSkills(symlinkPath) {
		t.Error("expected true for symlink into agents skills")
	}

	// Test regular directory (not symlink).
	if IsSymlinkIntoAgentsSkills(skillSrc) {
		t.Error("expected false for regular directory")
	}

	// Test non-existent path.
	if IsSymlinkIntoAgentsSkills(filepath.Join(tmpDir, "nonexistent")) {
		t.Error("expected false for non-existent path")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
