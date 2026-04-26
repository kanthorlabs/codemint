// Package acp implements the Agent Communication Protocol client for CodeMint.
package acp

import (
	"os"
	"path/filepath"

	"codemint.kanthorlabs.com/internal/xdg"
)

// maxHotMemorySize is the maximum size (in bytes) for each hot memory file.
// Files larger than this are truncated with a marker.
const maxHotMemorySize = 32 * 1024 // 32 KiB

// truncationMarker is appended when a file exceeds maxHotMemorySize.
const truncationMarker = "\n... [truncated]"

// HotMemory contains the "hot" wiki files from project memory.
// These files are injected into the agent's context to enforce
// coding standards and decisions established during brainstorming.
type HotMemory struct {
	Preferences string // patterns/preferences.md
	Decisions   string // architecture/decisions.md
	BugsIndex   string // patterns/bugs/index.md
}

// LoadHotMemory reads the "hot" wiki files from the per-project memory directory.
// Missing files are not errors — the corresponding fields are empty strings.
// Files exceeding 32 KiB are truncated with a marker.
//
// Memory files are located at: ~/.local/share/codemint/memory/<project_id>/...
func LoadHotMemory(projectID string) (HotMemory, error) {
	if projectID == "" {
		return HotMemory{}, nil
	}

	baseDir := filepath.Join(xdg.MemoryDir(), projectID)

	// Define the hot memory file paths relative to the project memory dir.
	files := map[string]*string{
		"patterns/preferences.md":     nil, // placeholder for Preferences
		"architecture/decisions.md":   nil, // placeholder for Decisions
		"patterns/bugs/index.md":      nil, // placeholder for BugsIndex
	}

	var mem HotMemory
	files["patterns/preferences.md"] = &mem.Preferences
	files["architecture/decisions.md"] = &mem.Decisions
	files["patterns/bugs/index.md"] = &mem.BugsIndex

	for relPath, dest := range files {
		content, err := readHotMemoryFile(filepath.Join(baseDir, relPath))
		if err != nil {
			return HotMemory{}, err
		}
		*dest = content
	}

	return mem, nil
}

// readHotMemoryFile reads a single hot memory file.
// Returns empty string if the file does not exist.
// Truncates content if it exceeds maxHotMemorySize.
func readHotMemoryFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	if len(data) > maxHotMemorySize {
		// Truncate and append marker.
		truncated := data[:maxHotMemorySize-len(truncationMarker)]
		return string(truncated) + truncationMarker, nil
	}

	return string(data), nil
}

// IsEmpty returns true if all memory fields are empty.
func (m HotMemory) IsEmpty() bool {
	return m.Preferences == "" && m.Decisions == "" && m.BugsIndex == ""
}
