package xdg

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDataDir_WithXDGEnvVar(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("XDG_DATA_HOME not used on Windows")
	}

	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	got := DataDir()
	want := filepath.Join(tmpDir, AppName)

	if got != want {
		t.Errorf("DataDir() = %q; want %q", got, want)
	}
}

func TestDataDir_DefaultFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Default fallback test not applicable on Windows")
	}

	// Clear XDG_DATA_HOME to test default behavior.
	t.Setenv("XDG_DATA_HOME", "")

	got := DataDir()

	// Should contain ~/.local/share/codemint
	if !strings.Contains(got, filepath.Join(".local", "share", AppName)) {
		t.Errorf("DataDir() = %q; want path containing '.local/share/%s'", got, AppName)
	}
}

func TestConfigDir_WithXDGEnvVar(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("XDG_CONFIG_HOME not used on Windows")
	}

	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	got := ConfigDir()
	want := filepath.Join(tmpDir, AppName)

	if got != want {
		t.Errorf("ConfigDir() = %q; want %q", got, want)
	}
}

func TestConfigDir_DefaultFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Default fallback test not applicable on Windows")
	}

	// Clear XDG_CONFIG_HOME to test default behavior.
	t.Setenv("XDG_CONFIG_HOME", "")

	got := ConfigDir()

	// Should contain ~/.config/codemint
	if !strings.Contains(got, filepath.Join(".config", AppName)) {
		t.Errorf("ConfigDir() = %q; want path containing '.config/%s'", got, AppName)
	}
}

func TestMemoryDir(t *testing.T) {
	got := MemoryDir()

	// MemoryDir should be DataDir + "memory"
	if !strings.HasSuffix(got, filepath.Join(AppName, "memory")) {
		t.Errorf("MemoryDir() = %q; want path ending in '%s/memory'", got, AppName)
	}
}

func TestDatabasePath(t *testing.T) {
	got := DatabasePath()

	// DatabasePath should end with codemint/codemint.db
	if !strings.HasSuffix(got, filepath.Join(AppName, "codemint.db")) {
		t.Errorf("DatabasePath() = %q; want path ending in '%s/codemint.db'", got, AppName)
	}
}

func TestEnsureDirs_CreatesDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// Override XDG paths to use temp directory.
	if runtime.GOOS == "windows" {
		t.Setenv("LOCALAPPDATA", tmpDir)
		t.Setenv("APPDATA", tmpDir)
	} else {
		t.Setenv("XDG_DATA_HOME", filepath.Join(tmpDir, "data"))
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, "config"))
	}

	if err := EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs() returned error: %v", err)
	}

	// Verify directories were created.
	dirs := []string{
		DataDir(),
		MemoryDir(),
		ConfigDir(),
	}

	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("directory %q was not created: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%q exists but is not a directory", dir)
		}
	}
}

func TestEnsureDirs_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()

	// Override XDG paths to use temp directory.
	if runtime.GOOS == "windows" {
		t.Setenv("LOCALAPPDATA", tmpDir)
		t.Setenv("APPDATA", tmpDir)
	} else {
		t.Setenv("XDG_DATA_HOME", filepath.Join(tmpDir, "data"))
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, "config"))
	}

	// Call EnsureDirs multiple times.
	for i := range 3 {
		if err := EnsureDirs(); err != nil {
			t.Fatalf("EnsureDirs() call %d returned error: %v", i+1, err)
		}
	}

	// Verify directories still exist after multiple calls.
	info, err := os.Stat(DataDir())
	if err != nil {
		t.Errorf("DataDir() does not exist after multiple EnsureDirs calls: %v", err)
	}
	if !info.IsDir() {
		t.Error("DataDir() is not a directory")
	}
}

func TestEnsureDirs_CreatesNestedDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// Use a deeply nested path that doesn't exist.
	nestedPath := filepath.Join(tmpDir, "a", "b", "c", "data")

	if runtime.GOOS == "windows" {
		t.Setenv("LOCALAPPDATA", nestedPath)
		t.Setenv("APPDATA", filepath.Join(tmpDir, "a", "b", "c", "config"))
	} else {
		t.Setenv("XDG_DATA_HOME", nestedPath)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, "a", "b", "c", "config"))
	}

	if err := EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs() failed to create nested directories: %v", err)
	}

	// Verify nested directories were created.
	info, err := os.Stat(MemoryDir())
	if err != nil {
		t.Errorf("MemoryDir() was not created: %v", err)
	}
	if info != nil && !info.IsDir() {
		t.Error("MemoryDir() is not a directory")
	}
}

func TestAppName(t *testing.T) {
	if AppName != "codemint" {
		t.Errorf("AppName = %q; want %q", AppName, "codemint")
	}
}
