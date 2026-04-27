package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileRegistry_LoadAll(t *testing.T) {
	t.Run("loads embedded workflows", func(t *testing.T) {
		registry := NewFileRegistry()
		if err := registry.LoadAll(); err != nil {
			t.Fatalf("LoadAll() error: %v", err)
		}

		// The embedded brainstorming workflow should be loaded.
		wf, ok := registry.Get("brainstorming")
		if !ok {
			t.Fatal("Get(brainstorming) returned false, want true")
		}
		if wf.Name != "brainstorming" {
			t.Errorf("wf.Name = %q, want %q", wf.Name, "brainstorming")
		}
		if wf.Version != "1.0" {
			t.Errorf("wf.Version = %q, want %q", wf.Version, "1.0")
		}
		if len(wf.Epics) != 1 {
			t.Errorf("len(wf.Epics) = %d, want %d", len(wf.Epics), 1)
		}
	})

	t.Run("loads external workflows", func(t *testing.T) {
		// Create temp home directory with workflows.
		tmpHome := t.TempDir()
		origHome := os.Getenv("HOME")
		os.Setenv("HOME", tmpHome)
		defer os.Setenv("HOME", origHome)

		// Create external workflow directory.
		workflowDir := filepath.Join(tmpHome, ".local", "share", "codemint", "workflows", "custom")
		if err := os.MkdirAll(workflowDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		content := `name: custom
version: "2.0"
description: A custom workflow for testing.
epics:
  - id: main
    name: "Main Epic"
    stories:
      - id: step1
        name: "Step One"
`
		if err := os.WriteFile(filepath.Join(workflowDir, "WORKFLOW.yaml"), []byte(content), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}

		registry := NewFileRegistry()
		if err := registry.LoadAll(); err != nil {
			t.Fatalf("LoadAll() error: %v", err)
		}

		// Custom workflow should be loaded.
		wf, ok := registry.Get("custom")
		if !ok {
			t.Fatal("Get(custom) returned false, want true")
		}
		if wf.Version != "2.0" {
			t.Errorf("wf.Version = %q, want %q", wf.Version, "2.0")
		}

		// Embedded workflow should still be available.
		_, ok = registry.Get("brainstorming")
		if !ok {
			t.Error("Get(brainstorming) returned false, want true (embedded should still be loaded)")
		}
	})

	t.Run("embedded overrides external with same name", func(t *testing.T) {
		// Create temp home directory.
		tmpHome := t.TempDir()
		origHome := os.Getenv("HOME")
		os.Setenv("HOME", tmpHome)
		defer os.Setenv("HOME", origHome)

		// Create external workflow with same name as embedded (brainstorming).
		workflowDir := filepath.Join(tmpHome, ".local", "share", "codemint", "workflows", "brainstorming")
		if err := os.MkdirAll(workflowDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		content := `name: brainstorming
version: "external"
description: External override (should be replaced by embedded).
epics:
  - id: external
    name: "External Epic"
    stories:
      - id: step1
        name: "Step One"
`
		if err := os.WriteFile(filepath.Join(workflowDir, "WORKFLOW.yaml"), []byte(content), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}

		registry := NewFileRegistry()
		if err := registry.LoadAll(); err != nil {
			t.Fatalf("LoadAll() error: %v", err)
		}

		// Embedded version should win.
		wf, ok := registry.Get("brainstorming")
		if !ok {
			t.Fatal("Get(brainstorming) returned false, want true")
		}
		if wf.Version != "1.0" {
			t.Errorf("wf.Version = %q, want %q (embedded should override external)", wf.Version, "1.0")
		}
	})

	t.Run("handles missing external directory", func(t *testing.T) {
		// Create temp home directory without workflows.
		tmpHome := t.TempDir()
		origHome := os.Getenv("HOME")
		os.Setenv("HOME", tmpHome)
		defer os.Setenv("HOME", origHome)

		registry := NewFileRegistry()
		if err := registry.LoadAll(); err != nil {
			t.Fatalf("LoadAll() error: %v", err)
		}

		// Embedded workflows should still load.
		if registry.Len() == 0 {
			t.Error("Len() = 0, want > 0 (embedded workflows should load)")
		}
	})
}

func TestFileRegistry_Get(t *testing.T) {
	registry := NewFileRegistry()
	if err := registry.LoadAll(); err != nil {
		t.Fatalf("LoadAll() error: %v", err)
	}

	t.Run("returns existing workflow", func(t *testing.T) {
		wf, ok := registry.Get("brainstorming")
		if !ok {
			t.Fatal("Get(brainstorming) returned false, want true")
		}
		if wf == nil {
			t.Error("Get(brainstorming) returned nil workflow")
		}
	})

	t.Run("returns false for missing workflow", func(t *testing.T) {
		_, ok := registry.Get("nonexistent")
		if ok {
			t.Error("Get(nonexistent) returned true, want false")
		}
	})
}

func TestFileRegistry_All(t *testing.T) {
	registry := NewFileRegistry()
	if err := registry.LoadAll(); err != nil {
		t.Fatalf("LoadAll() error: %v", err)
	}

	workflows := registry.All()
	if len(workflows) == 0 {
		t.Fatal("All() returned empty slice, want at least one embedded workflow")
	}

	// Verify sorted by name.
	for i := 1; i < len(workflows); i++ {
		if workflows[i-1].Name > workflows[i].Name {
			t.Errorf("All() not sorted: %q > %q", workflows[i-1].Name, workflows[i].Name)
		}
	}
}

func TestFileRegistry_Names(t *testing.T) {
	registry := NewFileRegistry()
	if err := registry.LoadAll(); err != nil {
		t.Fatalf("LoadAll() error: %v", err)
	}

	names := registry.Names()
	if len(names) == 0 {
		t.Fatal("Names() returned empty slice, want at least one name")
	}

	// Verify sorted.
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Errorf("Names() not sorted: %q > %q", names[i-1], names[i])
		}
	}

	// Verify brainstorming is present.
	found := false
	for _, name := range names {
		if name == "brainstorming" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Names() does not contain 'brainstorming', want it present")
	}
}

func TestFileRegistry_Len(t *testing.T) {
	registry := NewFileRegistry()
	if registry.Len() != 0 {
		t.Errorf("Len() before LoadAll = %d, want 0", registry.Len())
	}

	if err := registry.LoadAll(); err != nil {
		t.Fatalf("LoadAll() error: %v", err)
	}

	if registry.Len() == 0 {
		t.Error("Len() after LoadAll = 0, want > 0")
	}
}

func TestFileRegistry_loadDir_invalidWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	workflowDir := filepath.Join(tmpDir, "invalid")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Invalid YAML content.
	if err := os.WriteFile(filepath.Join(workflowDir, "WORKFLOW.yaml"), []byte("invalid: yaml: content:"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	registry := NewFileRegistry()
	err := registry.loadDir(tmpDir)
	if err == nil {
		t.Fatal("loadDir() expected error for invalid YAML, got nil")
	}
}
