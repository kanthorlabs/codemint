// Package xdg provides XDG Base Directory Specification path resolution
// for CodeMint's data and configuration storage.
//
// On Linux and macOS, paths follow the XDG spec:
//   - Data: $XDG_DATA_HOME/codemint (default: ~/.local/share/codemint)
//   - Config: $XDG_CONFIG_HOME/codemint (default: ~/.config/codemint)
//
// On Windows, paths use standard Windows locations:
//   - Data: %LOCALAPPDATA%\codemint
//   - Config: %APPDATA%\codemint
package xdg

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// AppName is the application identifier used in directory paths.
const AppName = "codemint"

// DataDir returns the base directory for application data storage.
// This is where the SQLite database and other persistent data reside.
//
// Precedence (Unix): $XDG_DATA_HOME/codemint > ~/.local/share/codemint
// Windows: %LOCALAPPDATA%\codemint
func DataDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), AppName)
	}

	if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
		return filepath.Join(xdgData, AppName)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback to current directory if home is unavailable.
		return filepath.Join(".", ".local", "share", AppName)
	}
	return filepath.Join(home, ".local", "share", AppName)
}

// ConfigDir returns the base directory for user configuration files.
//
// Precedence (Unix): $XDG_CONFIG_HOME/codemint > ~/.config/codemint
// Windows: %APPDATA%\codemint
func ConfigDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), AppName)
	}

	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		return filepath.Join(xdgConfig, AppName)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback to current directory if home is unavailable.
		return filepath.Join(".", ".config", AppName)
	}
	return filepath.Join(home, ".config", AppName)
}

// MemoryDir returns the directory for Adaptive Learning System storage.
// This is where the LLM Wiki / memory files reside.
func MemoryDir() string {
	return filepath.Join(DataDir(), "memory")
}

// DatabasePath returns the default path for the SQLite database file.
func DatabasePath() string {
	return filepath.Join(DataDir(), "codemint.db")
}

// StateDir returns the base directory for runtime state files (logs, sockets).
// This follows the XDG Base Directory spec for state data.
//
// Precedence (Unix): $XDG_STATE_HOME/codemint > ~/.local/state/codemint
// Windows: %LOCALAPPDATA%\codemint\state
func StateDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), AppName, "state")
	}

	if xdgState := os.Getenv("XDG_STATE_HOME"); xdgState != "" {
		return filepath.Join(xdgState, AppName)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback to current directory if home is unavailable.
		return filepath.Join(".", ".local", "state", AppName)
	}
	return filepath.Join(home, ".local", "state", AppName)
}

// EnsureDirs creates all required application directories if they do not exist.
// This function is idempotent and safe to call multiple times.
//
// Created directories:
//   - DataDir() - For database and persistent data
//   - MemoryDir() - For Adaptive Learning System / LLM Wiki
//   - ConfigDir() - For user configuration files
func EnsureDirs() error {
	dirs := []string{
		DataDir(),
		MemoryDir(),
		ConfigDir(),
		StateDir(),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("xdg: create directory %q: %w", dir, err)
		}
	}

	return nil
}
