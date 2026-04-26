// Package idgen provides UUID generation utilities for CodeMint entities.
// All primary and foreign keys use UUID v7, which is lexicographically
// sortable by timestamp, keeping SQLite B-Tree insertions sequential.
package idgen

import (
	"fmt"
	"time"

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

// ExtractTime extracts the timestamp from a UUID v7 string.
// UUIDv7 encodes a Unix millisecond timestamp in the first 48 bits.
// Returns time.Now() if the UUID is invalid.
func ExtractTime(uuidStr string) time.Time {
	id, err := uuid.Parse(uuidStr)
	if err != nil {
		return time.Now()
	}

	// UUIDv7 stores timestamp in the first 48 bits as milliseconds since Unix epoch.
	// The google/uuid library's Time() method returns time for version 1 UUIDs.
	// For UUIDv7, we need to extract manually from the first 6 bytes.
	bytes := id[:]
	msec := int64(bytes[0])<<40 |
		int64(bytes[1])<<32 |
		int64(bytes[2])<<24 |
		int64(bytes[3])<<16 |
		int64(bytes[4])<<8 |
		int64(bytes[5])

	return time.UnixMilli(msec)
}
