// Package atomicio provides utilities for atomic file write operations.
// Writes are safe against partial writes caused by crashes, power failures,
// or disk-full conditions.
package atomicio

import (
	"fmt"
	"os"
)

// WriteAtomic writes data to path atomically using the write-to-temp-then-rename
// pattern:
//  1. Write data to a sibling temporary file (path + ".tmp").
//  2. Call f.Sync() to flush OS buffers to physical storage.
//  3. Call os.Rename(tmp, path) — an atomic OS-level operation on POSIX systems.
//
// This guarantees that path is either in its previous state or fully updated;
// it is never left in a truncated or partial state.
func WriteAtomic(path string, data []byte) error {
	tmp := path + ".tmp"

	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("atomicio: create temp file %q: %w", tmp, err)
	}

	if _, err := f.Write(data); err != nil {
		f.Close()         //nolint:errcheck
		os.Remove(tmp)    //nolint:errcheck
		return fmt.Errorf("atomicio: write temp file %q: %w", tmp, err)
	}

	if err := f.Sync(); err != nil {
		f.Close()         //nolint:errcheck
		os.Remove(tmp)    //nolint:errcheck
		return fmt.Errorf("atomicio: sync temp file %q: %w", tmp, err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmp) //nolint:errcheck
		return fmt.Errorf("atomicio: close temp file %q: %w", tmp, err)
	}

	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp) //nolint:errcheck
		return fmt.Errorf("atomicio: rename %q -> %q: %w", tmp, path, err)
	}

	return nil
}
