package orchestrator

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"codemint.kanthorlabs.com/internal/domain"
)

func TestResolveContextFiles_RejectsEscape(t *testing.T) {
	// Create a temporary directory to act as the project root.
	tmpDir, err := os.MkdirTemp("", "resolve_context_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	project := &domain.Project{
		ID:         "test-project",
		WorkingDir: tmpDir,
	}

	tests := []struct {
		name  string
		files []string
	}{
		{
			name:  "simple parent escape",
			files: []string{"../etc/passwd"},
		},
		{
			name:  "deep parent escape",
			files: []string{"../../../etc/passwd"},
		},
		{
			name:  "disguised escape",
			files: []string{"foo/../../bar/../../../etc/passwd"},
		},
		{
			name:  "escape with valid prefix",
			files: []string{"src/../../../etc/passwd"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveContextFiles(project, tt.files)
			if !errors.Is(err, ErrPathEscape) {
				t.Errorf("expected ErrPathEscape, got %v", err)
			}
		})
	}
}

func TestResolveContextFiles_AbsolutePathRejected(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "resolve_context_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	project := &domain.Project{
		ID:         "test-project",
		WorkingDir: tmpDir,
	}

	tests := []struct {
		name  string
		files []string
	}{
		{
			name:  "etc hosts",
			files: []string{"/etc/hosts"},
		},
		{
			name:  "absolute project path",
			files: []string{"/var/project/src/main.go"},
		},
		{
			name:  "windows absolute (unix test)",
			files: []string{"/C:/Users/test/file.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveContextFiles(project, tt.files)
			if !errors.Is(err, ErrPathEscape) {
				t.Errorf("expected ErrPathEscape, got %v", err)
			}
		})
	}
}

func TestResolveContextFiles_ValidPaths(t *testing.T) {
	// Create a temporary directory with some files.
	tmpDir, err := os.MkdirTemp("", "resolve_context_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test files.
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}

	mainFile := filepath.Join(srcDir, "main.go")
	if err := os.WriteFile(mainFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to create main.go: %v", err)
	}

	helperFile := filepath.Join(srcDir, "helper.go")
	if err := os.WriteFile(helperFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to create helper.go: %v", err)
	}

	project := &domain.Project{
		ID:         "test-project",
		WorkingDir: tmpDir,
	}

	files := []string{"src/main.go", "src/helper.go"}
	resolved, err := resolveContextFiles(project, files)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resolved) != 2 {
		t.Errorf("expected 2 resolved paths, got %d", len(resolved))
	}

	expectedMain := filepath.Join(tmpDir, "src", "main.go")
	expectedHelper := filepath.Join(tmpDir, "src", "helper.go")

	if resolved[0] != expectedMain {
		t.Errorf("resolved[0] mismatch: got %q, want %q", resolved[0], expectedMain)
	}
	if resolved[1] != expectedHelper {
		t.Errorf("resolved[1] mismatch: got %q, want %q", resolved[1], expectedHelper)
	}
}

func TestResolveContextFiles_MissingFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "resolve_context_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	project := &domain.Project{
		ID:         "test-project",
		WorkingDir: tmpDir,
	}

	// Reference a file that doesn't exist.
	files := []string{"nonexistent.go"}
	_, err = resolveContextFiles(project, files)
	if !errors.Is(err, ErrContextFileMissing) {
		t.Errorf("expected ErrContextFileMissing, got %v", err)
	}
}

func TestResolveContextFiles_EmptySlice(t *testing.T) {
	project := &domain.Project{
		ID:         "test-project",
		WorkingDir: "/tmp/test",
	}

	resolved, err := resolveContextFiles(project, nil)
	if err != nil {
		t.Errorf("unexpected error for nil slice: %v", err)
	}
	if resolved != nil {
		t.Errorf("expected nil for nil input, got %v", resolved)
	}

	resolved, err = resolveContextFiles(project, []string{})
	if err != nil {
		t.Errorf("unexpected error for empty slice: %v", err)
	}
	if resolved != nil {
		t.Errorf("expected nil for empty slice, got %v", resolved)
	}
}

func TestResolveContextFiles_NilProject(t *testing.T) {
	_, err := resolveContextFiles(nil, []string{"file.go"})
	if !errors.Is(err, ErrInvalidTaskInput) {
		t.Errorf("expected ErrInvalidTaskInput for nil project, got %v", err)
	}
}

func TestResolveContextFiles_EmptyWorkingDir(t *testing.T) {
	project := &domain.Project{
		ID:         "test-project",
		WorkingDir: "",
	}

	_, err := resolveContextFiles(project, []string{"file.go"})
	if !errors.Is(err, ErrInvalidTaskInput) {
		t.Errorf("expected ErrInvalidTaskInput for empty WorkingDir, got %v", err)
	}
}

func TestResolveContextFiles_PathWithDots(t *testing.T) {
	// Create a temporary directory with nested structure.
	tmpDir, err := os.MkdirTemp("", "resolve_context_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create nested directories.
	srcDir := filepath.Join(tmpDir, "src", "pkg")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src/pkg dir: %v", err)
	}

	// Create a file in nested dir.
	testFile := filepath.Join(srcDir, "test.go")
	if err := os.WriteFile(testFile, []byte("package pkg"), 0644); err != nil {
		t.Fatalf("failed to create test.go: %v", err)
	}

	project := &domain.Project{
		ID:         "test-project",
		WorkingDir: tmpDir,
	}

	// Valid path with single dot (current dir).
	files := []string{"./src/pkg/test.go"}
	resolved, err := resolveContextFiles(project, files)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved path, got %d", len(resolved))
	}

	expected := filepath.Join(tmpDir, "src", "pkg", "test.go")
	if resolved[0] != expected {
		t.Errorf("resolved[0] mismatch: got %q, want %q", resolved[0], expected)
	}
}
