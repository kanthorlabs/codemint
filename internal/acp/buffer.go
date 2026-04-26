package acp

import (
	"sync"
	"time"
)

// DefaultBufferCapacity is the default number of events per buffer.
const DefaultBufferCapacity = 256

// TimestampedEvent wraps an Event with a timestamp for summary rendering.
type TimestampedEvent struct {
	Event     Event
	Timestamp time.Time
}

// RingBuffer is a fixed-capacity circular buffer for events.
// It stores the last N events, overwriting oldest entries when full.
type RingBuffer struct {
	mu   sync.Mutex
	cap  int
	data []TimestampedEvent
	head int
	full bool
}

// NewRingBuffer creates a new RingBuffer with the given capacity.
// If cap <= 0, DefaultBufferCapacity is used.
func NewRingBuffer(cap int) *RingBuffer {
	if cap <= 0 {
		cap = DefaultBufferCapacity
	}
	return &RingBuffer{
		cap:  cap,
		data: make([]TimestampedEvent, cap),
		head: 0,
		full: false,
	}
}

// Push adds an event to the buffer. If the buffer is full,
// the oldest event is overwritten.
func (r *RingBuffer) Push(ev Event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.data[r.head] = TimestampedEvent{
		Event:     ev,
		Timestamp: time.Now(),
	}
	r.head = (r.head + 1) % r.cap
	if r.head == 0 && !r.full {
		r.full = true
	}
}

// PushWithTimestamp adds an event with a specific timestamp (useful for testing).
func (r *RingBuffer) PushWithTimestamp(ev Event, ts time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.data[r.head] = TimestampedEvent{
		Event:     ev,
		Timestamp: ts,
	}
	r.head = (r.head + 1) % r.cap
	if r.head == 0 && !r.full {
		r.full = true
	}
}

// Snapshot returns a copy of all events in chronological order (oldest first).
func (r *RingBuffer) Snapshot() []TimestampedEvent {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.full && r.head == 0 {
		// Buffer is empty
		return nil
	}

	var result []TimestampedEvent
	if r.full {
		// Buffer has wrapped - read from head to end, then start to head
		result = make([]TimestampedEvent, r.cap)
		copy(result, r.data[r.head:])
		copy(result[r.cap-r.head:], r.data[:r.head])
	} else {
		// Buffer hasn't wrapped yet - read from start to head
		result = make([]TimestampedEvent, r.head)
		copy(result, r.data[:r.head])
	}
	return result
}

// Len returns the current number of events in the buffer.
func (r *RingBuffer) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.full {
		return r.cap
	}
	return r.head
}

// Clear removes all events from the buffer.
func (r *RingBuffer) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.head = 0
	r.full = false
}

// bufferKey uniquely identifies a buffer by session and task.
type bufferKey struct {
	SessionID string
	TaskID    string
}

// BufferRegistry manages per-task ring buffers.
// Each buffer is keyed by (sessionID, taskID).
// A session-default buffer uses an empty taskID.
type BufferRegistry struct {
	mu      sync.RWMutex
	buffers map[bufferKey]*RingBuffer
	cap     int
}

// NewBufferRegistry creates a new BufferRegistry with the given per-buffer capacity.
func NewBufferRegistry(cap int) *BufferRegistry {
	if cap <= 0 {
		cap = DefaultBufferCapacity
	}
	return &BufferRegistry{
		buffers: make(map[bufferKey]*RingBuffer),
		cap:     cap,
	}
}

// Push adds an event to the buffer for the given session and task.
// If the buffer doesn't exist, it is created.
func (br *BufferRegistry) Push(sessionID, taskID string, ev Event) {
	key := bufferKey{SessionID: sessionID, TaskID: taskID}

	br.mu.Lock()
	buf, ok := br.buffers[key]
	if !ok {
		buf = NewRingBuffer(br.cap)
		br.buffers[key] = buf
	}
	br.mu.Unlock()

	buf.Push(ev)
}

// Snapshot returns a copy of all events for the given session and task.
// Returns nil if no buffer exists for the key.
func (br *BufferRegistry) Snapshot(sessionID, taskID string) []TimestampedEvent {
	key := bufferKey{SessionID: sessionID, TaskID: taskID}

	br.mu.RLock()
	buf, ok := br.buffers[key]
	br.mu.RUnlock()

	if !ok {
		return nil
	}
	return buf.Snapshot()
}

// Drop removes the buffer for the given session and task.
// This should be called when a task reaches a terminal status.
func (br *BufferRegistry) Drop(sessionID, taskID string) {
	key := bufferKey{SessionID: sessionID, TaskID: taskID}

	br.mu.Lock()
	delete(br.buffers, key)
	br.mu.Unlock()
}

// DropSession removes all buffers for the given session.
func (br *BufferRegistry) DropSession(sessionID string) {
	br.mu.Lock()
	defer br.mu.Unlock()

	for key := range br.buffers {
		if key.SessionID == sessionID {
			delete(br.buffers, key)
		}
	}
}

// Count returns the number of active buffers.
func (br *BufferRegistry) Count() int {
	br.mu.RLock()
	defer br.mu.RUnlock()
	return len(br.buffers)
}

// Has checks if a buffer exists for the given session and task.
func (br *BufferRegistry) Has(sessionID, taskID string) bool {
	key := bufferKey{SessionID: sessionID, TaskID: taskID}

	br.mu.RLock()
	defer br.mu.RUnlock()
	_, ok := br.buffers[key]
	return ok
}
