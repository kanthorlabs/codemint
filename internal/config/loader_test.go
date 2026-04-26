package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_ValidYAML(t *testing.T) {
	content := `
workflows:
  - type: 0
    name: "Project Coding"
    description: "Coding tasks"
    triggers: ["implement", "fix"]
  - type: 1
    name: "Communication"
    description: "Inquiries"
`
	tmpFile := writeTestFile(t, content)

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if len(cfg.Workflows) != 2 {
		t.Errorf("expected 2 workflows, got %d", len(cfg.Workflows))
	}

	if cfg.Workflows[0].Name != "Project Coding" {
		t.Errorf("workflow[0].Name = %q, want 'Project Coding'", cfg.Workflows[0].Name)
	}

	if len(cfg.Workflows[0].Triggers) != 2 {
		t.Errorf("workflow[0].Triggers = %v, want 2 items", cfg.Workflows[0].Triggers)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	content := `
workflows:
  - type: invalid_not_a_number
    name: "Bad"
`
	tmpFile := writeTestFile(t, content)

	_, err := Load(tmpFile)
	if err == nil {
		t.Fatal("expected parse error for invalid YAML, got nil")
	}

	if !strings.Contains(err.Error(), "parse yaml") {
		t.Errorf("error should mention parse yaml: %v", err)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}

	if cfg == nil {
		t.Fatal("expected empty config, got nil")
	}

	if len(cfg.Workflows) != 0 {
		t.Errorf("expected 0 workflows for missing file, got %d", len(cfg.Workflows))
	}
}

func TestLoad_EmptyFile(t *testing.T) {
	tmpFile := writeTestFile(t, "")

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if len(cfg.Workflows) != 0 {
		t.Errorf("expected 0 workflows for empty file, got %d", len(cfg.Workflows))
	}
}

// writeTestFile creates a temp file with the given content and returns its path.
func writeTestFile(t *testing.T, content string) string {
	t.Helper()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	return path
}
