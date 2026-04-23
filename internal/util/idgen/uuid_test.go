package idgen

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestUUIDv7_Version asserts that New() produces a valid UUID version 7.
func TestUUIDv7_Version(t *testing.T) {
	raw, err := New()
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	id, err := uuid.Parse(raw)
	if err != nil {
		t.Fatalf("uuid.Parse(%q) failed: %v", raw, err)
	}

	if got := id.Version(); got != 7 {
		t.Errorf("expected UUID version 7, got %d", got)
	}
}

// TestUUIDv7_Sortable asserts that two UUIDs generated with a small sleep
// between them are lexicographically ordered (id1 < id2), proving that
// SQLite B-Tree insertions will remain sequential.
func TestUUIDv7_Sortable(t *testing.T) {
	id1 := MustNew()
	time.Sleep(2 * time.Millisecond)
	id2 := MustNew()

	if id1 >= id2 {
		t.Errorf("expected id1 < id2 for temporal ordering, got:\n  id1=%s\n  id2=%s", id1, id2)
	}
}
