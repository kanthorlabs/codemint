// Package idgen provides UUID generation utilities for CodeMint entities.
// All primary and foreign keys use UUID v7, which is lexicographically
// sortable by timestamp, keeping SQLite B-Tree insertions sequential.
package idgen

import (
	"fmt"

	"github.com/google/uuid"
)

// New generates a new UUID v7 and returns its canonical string representation.
// Returns an error if the system entropy source fails.
func New() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("idgen: generate uuid v7: %w", err)
	}
	return id.String(), nil
}

// MustNew generates a new UUID v7 and panics if generation fails.
// Use this in contexts where ID generation failure is unrecoverable
// (e.g., entity constructors during normal operation).
func MustNew() string {
	id, err := New()
	if err != nil {
		panic(err)
	}
	return id
}
