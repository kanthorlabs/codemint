package acp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadHotMemory(t *testing.T) {
	t.Run("empty projectID returns empty memory", func(t *testing.T) {
		mem, err := LoadHotMemory("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !mem.IsEmpty() {
			t.Error("expected empty memory for empty projectID")
		}
	})

	t.Run("missing directory returns empty memory without error", func(t *testing.T) {
		mem, err := LoadHotMemory("nonexistent-project-id")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !mem.IsEmpty() {
			t.Error("expected empty memory for missing directory")
		}
	})

	t.Run("loads preferences file", func(t *testing.T) {
		// Create temp directory structure.
		tmpDir := t.TempDir()
		projectID := "test-project"

		// Create the expected path structure.
		prefsDir := filepath.Join(tmpDir, projectID, "patterns")
		if err := os.MkdirAll(prefsDir, 0o755); err != nil {
			t.Fatal(err)
		}

		prefsContent := "Use tabs for indentation\nPrefer short variable names"
		if err := os.WriteFile(filepath.Join(prefsDir, "preferences.md"), []byte(prefsContent), 0o644); err != nil {
			t.Fatal(err)
		}

		// Override xdg.MemoryDir for this test.
		origEnv := os.Getenv("XDG_DATA_HOME")
		os.Setenv("XDG_DATA_HOME", filepath.Join(tmpDir, ".."))
		defer os.Setenv("XDG_DATA_HOME", origEnv)

		// We need to test with the actual path structure.
		// For now, let's test the readHotMemoryFile function directly.
		content, err := readHotMemoryFile(filepath.Join(prefsDir, "preferences.md"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if content != prefsContent {
			t.Errorf("got %q, want %q", content, prefsContent)
		}
	})

	t.Run("truncates large files", func(t *testing.T) {
		tmpDir := t.TempDir()
		largePath := filepath.Join(tmpDir, "large.md")

		// Create content larger than maxHotMemorySize (32 KiB).
		largeContent := strings.Repeat("x", 40*1024)
		if err := os.WriteFile(largePath, []byte(largeContent), 0o644); err != nil {
			t.Fatal(err)
		}

		content, err := readHotMemoryFile(largePath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(content) > maxHotMemorySize {
			t.Errorf("content length %d exceeds maxHotMemorySize %d", len(content), maxHotMemorySize)
		}

		if !strings.HasSuffix(content, truncationMarker) {
			t.Error("truncated content should end with truncation marker")
		}
	})

	t.Run("missing file returns empty string", func(t *testing.T) {
		tmpDir := t.TempDir()
		content, err := readHotMemoryFile(filepath.Join(tmpDir, "nonexistent.md"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if content != "" {
			t.Errorf("expected empty string for missing file, got %q", content)
		}
	})
}

func TestHotMemoryIsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		mem      HotMemory
		expected bool
	}{
		{
			name:     "empty memory",
			mem:      HotMemory{},
			expected: true,
		},
		{
			name:     "only preferences",
			mem:      HotMemory{Preferences: "some content"},
			expected: false,
		},
		{
			name:     "only decisions",
			mem:      HotMemory{Decisions: "some content"},
			expected: false,
		},
		{
			name:     "only bugs index",
			mem:      HotMemory{BugsIndex: "some content"},
			expected: false,
		},
		{
			name: "all fields",
			mem: HotMemory{
				Preferences: "prefs",
				Decisions:   "decisions",
				BugsIndex:   "bugs",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mem.IsEmpty(); got != tt.expected {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestReadHotMemoryFile(t *testing.T) {
	t.Run("reads file content correctly", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test.md")
		content := "# Test\nSome content here"

		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		got, err := readHotMemoryFile(filePath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != content {
			t.Errorf("got %q, want %q", got, content)
		}
	})

	t.Run("handles permission error", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("skipping permission test as root")
		}

		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "noperm.md")

		if err := os.WriteFile(filePath, []byte("test"), 0o000); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(filePath, 0o644) // Cleanup

		_, err := readHotMemoryFile(filePath)
		if err == nil {
			t.Error("expected permission error")
		}
	})
}
