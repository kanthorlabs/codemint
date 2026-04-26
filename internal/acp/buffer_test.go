package acp

import (
	"sync"
	"testing"
	"time"
)

func TestNewRingBuffer(t *testing.T) {
	tests := []struct {
		name    string
		cap     int
		wantCap int
	}{
		{"positive capacity", 100, 100},
		{"zero uses default", 0, DefaultBufferCapacity},
		{"negative uses default", -5, DefaultBufferCapacity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := NewRingBuffer(tt.cap)
			if buf.cap != tt.wantCap {
				t.Errorf("NewRingBuffer(%d).cap = %d, want %d", tt.cap, buf.cap, tt.wantCap)
			}
		})
	}
}

func TestRingBuffer_Push(t *testing.T) {
	buf := NewRingBuffer(3)

	// Initially empty
	if got := buf.Len(); got != 0 {
		t.Errorf("Len() = %d, want 0", got)
	}

	// Push one event
	buf.Push(Event{Kind: EventMessage})
	if got := buf.Len(); got != 1 {
		t.Errorf("Len() = %d, want 1", got)
	}

	// Push two more
	buf.Push(Event{Kind: EventThinking})
	buf.Push(Event{Kind: EventToolCall})
	if got := buf.Len(); got != 3 {
		t.Errorf("Len() = %d, want 3", got)
	}
}

func TestRingBuffer_WrapAround(t *testing.T) {
	buf := NewRingBuffer(3)

	// Push 5 events into a buffer of capacity 3
	buf.PushWithTimestamp(Event{Kind: EventMessage, ACPSessionID: "1"}, time.Unix(1, 0))
	buf.PushWithTimestamp(Event{Kind: EventMessage, ACPSessionID: "2"}, time.Unix(2, 0))
	buf.PushWithTimestamp(Event{Kind: EventMessage, ACPSessionID: "3"}, time.Unix(3, 0))
	buf.PushWithTimestamp(Event{Kind: EventMessage, ACPSessionID: "4"}, time.Unix(4, 0))
	buf.PushWithTimestamp(Event{Kind: EventMessage, ACPSessionID: "5"}, time.Unix(5, 0))

	// Should have 3 events (oldest 2 were overwritten)
	if got := buf.Len(); got != 3 {
		t.Errorf("Len() = %d, want 3", got)
	}

	// Snapshot should return events in chronological order: 3, 4, 5
	snapshot := buf.Snapshot()
	if len(snapshot) != 3 {
		t.Fatalf("Snapshot() len = %d, want 3", len(snapshot))
	}

	wantIDs := []string{"3", "4", "5"}
	for i, want := range wantIDs {
		if got := snapshot[i].Event.ACPSessionID; got != want {
			t.Errorf("Snapshot()[%d].Event.ACPSessionID = %s, want %s", i, got, want)
		}
	}

	// Verify timestamps are in order
	for i := 1; i < len(snapshot); i++ {
		if !snapshot[i].Timestamp.After(snapshot[i-1].Timestamp) {
			t.Errorf("Timestamps not in order: %v >= %v", snapshot[i-1].Timestamp, snapshot[i].Timestamp)
		}
	}
}

func TestRingBuffer_Snapshot_Empty(t *testing.T) {
	buf := NewRingBuffer(10)
	snapshot := buf.Snapshot()
	if snapshot != nil {
		t.Errorf("Snapshot() = %v, want nil for empty buffer", snapshot)
	}
}

func TestRingBuffer_Snapshot_PartiallyFilled(t *testing.T) {
	buf := NewRingBuffer(10)
	buf.PushWithTimestamp(Event{Kind: EventMessage, ACPSessionID: "a"}, time.Unix(1, 0))
	buf.PushWithTimestamp(Event{Kind: EventThinking, ACPSessionID: "b"}, time.Unix(2, 0))

	snapshot := buf.Snapshot()
	if len(snapshot) != 2 {
		t.Fatalf("Snapshot() len = %d, want 2", len(snapshot))
	}

	if snapshot[0].Event.ACPSessionID != "a" || snapshot[1].Event.ACPSessionID != "b" {
		t.Errorf("Snapshot order incorrect: got %v, %v", snapshot[0].Event.ACPSessionID, snapshot[1].Event.ACPSessionID)
	}
}

func TestRingBuffer_Clear(t *testing.T) {
	buf := NewRingBuffer(5)
	buf.Push(Event{Kind: EventMessage})
	buf.Push(Event{Kind: EventThinking})

	buf.Clear()

	if got := buf.Len(); got != 0 {
		t.Errorf("Len() after Clear() = %d, want 0", got)
	}

	snapshot := buf.Snapshot()
	if snapshot != nil {
		t.Errorf("Snapshot() after Clear() = %v, want nil", snapshot)
	}
}

func TestRingBuffer_Concurrent(t *testing.T) {
	buf := NewRingBuffer(100)
	var wg sync.WaitGroup

	// Concurrent pushes
	for i := range 10 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := range 50 {
				buf.Push(Event{Kind: EventKind(n), ACPSessionID: string(rune('A' + j))})
			}
		}(i)
	}

	// Concurrent snapshots
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				_ = buf.Snapshot()
			}
		}()
	}

	wg.Wait()

	// Should not panic and should have exactly 100 events (full buffer)
	if got := buf.Len(); got != 100 {
		t.Errorf("Len() = %d, want 100", got)
	}
}

func TestNewBufferRegistry(t *testing.T) {
	reg := NewBufferRegistry(128)
	if reg.cap != 128 {
		t.Errorf("cap = %d, want 128", reg.cap)
	}

	reg = NewBufferRegistry(0)
	if reg.cap != DefaultBufferCapacity {
		t.Errorf("cap = %d, want %d", reg.cap, DefaultBufferCapacity)
	}
}

func TestBufferRegistry_Push(t *testing.T) {
	reg := NewBufferRegistry(10)

	// Push to session-default buffer (empty taskID)
	reg.Push("sess-1", "", Event{Kind: EventMessage, ACPSessionID: "msg-1"})
	reg.Push("sess-1", "", Event{Kind: EventThinking, ACPSessionID: "think-1"})

	// Push to task-specific buffer
	reg.Push("sess-1", "task-a", Event{Kind: EventToolCall, ACPSessionID: "tool-1"})

	// Verify session-default buffer
	snapshot := reg.Snapshot("sess-1", "")
	if len(snapshot) != 2 {
		t.Fatalf("session-default Snapshot len = %d, want 2", len(snapshot))
	}

	// Verify task-specific buffer
	snapshot = reg.Snapshot("sess-1", "task-a")
	if len(snapshot) != 1 {
		t.Fatalf("task-specific Snapshot len = %d, want 1", len(snapshot))
	}

	// Verify non-existent buffer
	snapshot = reg.Snapshot("sess-1", "task-b")
	if snapshot != nil {
		t.Errorf("non-existent buffer Snapshot = %v, want nil", snapshot)
	}
}

func TestBufferRegistry_Drop(t *testing.T) {
	reg := NewBufferRegistry(10)

	reg.Push("sess-1", "task-a", Event{Kind: EventMessage})
	reg.Push("sess-1", "task-b", Event{Kind: EventThinking})

	if !reg.Has("sess-1", "task-a") {
		t.Error("Has(sess-1, task-a) = false, want true")
	}

	// Drop task-a
	reg.Drop("sess-1", "task-a")

	if reg.Has("sess-1", "task-a") {
		t.Error("Has(sess-1, task-a) = true after Drop, want false")
	}

	// task-b should still exist
	if !reg.Has("sess-1", "task-b") {
		t.Error("Has(sess-1, task-b) = false after Drop(task-a), want true")
	}
}

func TestBufferRegistry_DropsOnTerminal(t *testing.T) {
	// This test verifies the pattern described in Task 3.10.1:
	// "drop when the task hits a terminal status (Story 3.7)"
	reg := NewBufferRegistry(10)

	// Simulate task lifecycle
	reg.Push("sess-1", "task-x", Event{Kind: EventTurnStart})
	reg.Push("sess-1", "task-x", Event{Kind: EventMessage})
	reg.Push("sess-1", "task-x", Event{Kind: EventTurnEnd})

	// Verify buffer exists
	if !reg.Has("sess-1", "task-x") {
		t.Error("buffer should exist before terminal status")
	}

	// Simulate terminal status (Success) - caller should call Drop
	reg.Drop("sess-1", "task-x")

	// Verify buffer is removed
	if reg.Has("sess-1", "task-x") {
		t.Error("buffer should be removed after Drop")
	}

	snapshot := reg.Snapshot("sess-1", "task-x")
	if snapshot != nil {
		t.Error("Snapshot should return nil after Drop")
	}
}

func TestBufferRegistry_DropSession(t *testing.T) {
	reg := NewBufferRegistry(10)

	reg.Push("sess-1", "", Event{Kind: EventMessage})
	reg.Push("sess-1", "task-a", Event{Kind: EventMessage})
	reg.Push("sess-1", "task-b", Event{Kind: EventMessage})
	reg.Push("sess-2", "task-c", Event{Kind: EventMessage})

	if reg.Count() != 4 {
		t.Errorf("Count() = %d, want 4", reg.Count())
	}

	// Drop all buffers for sess-1
	reg.DropSession("sess-1")

	if reg.Count() != 1 {
		t.Errorf("Count() after DropSession = %d, want 1", reg.Count())
	}

	// sess-2 should still exist
	if !reg.Has("sess-2", "task-c") {
		t.Error("sess-2 buffer should still exist")
	}
}

func TestBufferRegistry_Concurrent(t *testing.T) {
	reg := NewBufferRegistry(50)
	var wg sync.WaitGroup

	// Concurrent pushes to different buffers
	for i := range 10 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := range 20 {
				reg.Push("sess", string(rune('A'+n)), Event{Kind: EventKind(j % 5)})
			}
		}(i)
	}

	// Concurrent reads
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 10 {
				_ = reg.Snapshot("sess", string(rune('A'+i)))
				_ = reg.Has("sess", string(rune('A'+i)))
			}
		}()
	}

	wg.Wait()

	// Should have 10 buffers (A through J)
	if got := reg.Count(); got != 10 {
		t.Errorf("Count() = %d, want 10", got)
	}
}

func TestBufferRegistry_SessionDefault(t *testing.T) {
	// Test the session-default buffer pattern from Task 3.10.1:
	// "Also retain a 'session default' buffer keyed by (sessionID, '')"
	reg := NewBufferRegistry(10)

	// Push to session-default (empty taskID)
	reg.Push("sess-adhoc", "", Event{Kind: EventMessage, ACPSessionID: "1"})
	reg.Push("sess-adhoc", "", Event{Kind: EventThinking, ACPSessionID: "2"})

	// Verify it works
	snapshot := reg.Snapshot("sess-adhoc", "")
	if len(snapshot) != 2 {
		t.Errorf("session-default Snapshot len = %d, want 2", len(snapshot))
	}

	// Verify Has works with empty taskID
	if !reg.Has("sess-adhoc", "") {
		t.Error("Has with empty taskID should return true")
	}
}
