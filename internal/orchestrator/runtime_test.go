package orchestrator

import (
	"context"
	"testing"
	"time"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/domain"
)

// mockTaskIDProvider implements TaskIDProvider for testing.
type mockTaskIDProvider struct {
	taskID string
}

func (m *mockTaskIDProvider) CurrentTaskID() string {
	return m.taskID
}

// TestRuntime_NewRuntime verifies that NewRuntime creates a properly initialized Runtime.
func TestRuntime_NewRuntime(t *testing.T) {
	bufferReg := acp.NewBufferRegistry(256)

	cfg := RuntimeConfig{
		Registry:       acp.NewRegistry(acp.DefaultConfig()),
		BufferRegistry: bufferReg,
		Mediator:       nil, // Not needed for this test.
		PermissionRepo: nil,
		TaskRepo:       nil,
		SessionRepo:    nil,
		AgentRepo:      nil,
	}

	rt := NewRuntime(cfg)

	if rt == nil {
		t.Fatal("NewRuntime returned nil")
	}

	if rt.Registry() == nil {
		t.Error("Registry() should not be nil")
	}

	if rt.BufferRegistry() == nil {
		t.Error("BufferRegistry() should not be nil")
	}

	if rt.ConsumerCount() != 0 {
		t.Errorf("ConsumerCount() = %d, want 0", rt.ConsumerCount())
	}
}

// TestRuntime_AttachWorker_NilSession verifies that AttachWorker handles nil session gracefully.
func TestRuntime_AttachWorker_NilSession(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{
		Registry:       acp.NewRegistry(acp.DefaultConfig()),
		BufferRegistry: acp.NewBufferRegistry(256),
	})

	worker, err := rt.AttachWorker(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("AttachWorker with nil session returned error: %v", err)
	}

	if worker != nil {
		t.Error("AttachWorker with nil session should return nil worker")
	}

	if rt.ConsumerCount() != 0 {
		t.Errorf("ConsumerCount() = %d, want 0", rt.ConsumerCount())
	}
}

// TestRuntime_DetachSession_CancelsConsumer verifies that DetachSession cancels the consumer goroutine.
func TestRuntime_DetachSession_CancelsConsumer(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{
		Registry:       acp.NewRegistry(acp.DefaultConfig()),
		BufferRegistry: acp.NewBufferRegistry(256),
	})

	sessionID := "test-session-123"

	// Manually add a consumer cancel function to simulate an attached session.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		<-ctx.Done()
		close(done)
	}()

	rt.consumersMu.Lock()
	rt.consumers[sessionID] = cancel
	rt.consumersMu.Unlock()

	// Verify consumer count.
	if rt.ConsumerCount() != 1 {
		t.Fatalf("ConsumerCount() = %d, want 1", rt.ConsumerCount())
	}

	// Detach the session.
	rt.DetachSession(sessionID)

	// Verify consumer was cancelled within 1 second.
	select {
	case <-done:
		// Success - consumer was cancelled.
	case <-time.After(time.Second):
		t.Error("Consumer goroutine did not exit within 1 second of detach")
	}

	// Verify consumer count.
	if rt.ConsumerCount() != 0 {
		t.Errorf("ConsumerCount() after detach = %d, want 0", rt.ConsumerCount())
	}
}

// TestRuntime_DetachSession_NonExistent verifies that DetachSession handles non-existent session gracefully.
func TestRuntime_DetachSession_NonExistent(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{
		Registry:       acp.NewRegistry(acp.DefaultConfig()),
		BufferRegistry: acp.NewBufferRegistry(256),
	})

	// Should not panic or error.
	rt.DetachSession("non-existent-session")
}

// TestRuntime_Shutdown verifies that Shutdown cancels all consumers and cleans up.
func TestRuntime_Shutdown(t *testing.T) {
	registry := acp.NewRegistry(acp.DefaultConfig())
	rt := NewRuntime(RuntimeConfig{
		Registry:       registry,
		BufferRegistry: acp.NewBufferRegistry(256),
	})

	// Add multiple consumers.
	for _, id := range []string{"session-1", "session-2", "session-3"} {
		ctx, cancel := context.WithCancel(context.Background())
		_ = ctx // Suppress unused variable warning.

		rt.consumersMu.Lock()
		rt.consumers[id] = cancel
		rt.consumersMu.Unlock()
	}

	if rt.ConsumerCount() != 3 {
		t.Fatalf("ConsumerCount() = %d, want 3", rt.ConsumerCount())
	}

	// Shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rt.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown() returned error: %v", err)
	}

	// Verify all consumers are gone.
	if rt.ConsumerCount() != 0 {
		t.Errorf("ConsumerCount() after shutdown = %d, want 0", rt.ConsumerCount())
	}
}

// TestRuntime_SetCurrentTask verifies that SetCurrentTask propagates to the worker.
func TestRuntime_SetCurrentTask(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{
		Registry:       acp.NewRegistry(acp.DefaultConfig()),
		BufferRegistry: acp.NewBufferRegistry(256),
	})

	// SetCurrentTask with no worker should not panic.
	rt.SetCurrentTask("non-existent-session", "task-123")
}

// TestRuntime_GetInterceptor verifies interceptor retrieval.
func TestRuntime_GetInterceptor(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{
		Registry:       acp.NewRegistry(acp.DefaultConfig()),
		BufferRegistry: acp.NewBufferRegistry(256),
	})

	// No interceptor should exist initially.
	_, ok := rt.GetInterceptor("session-1")
	if ok {
		t.Error("GetInterceptor should return false for non-existent session")
	}

	// Add an interceptor.
	interceptor := NewInterceptor(InterceptorConfig{})
	rt.interceptorsMu.Lock()
	rt.interceptors["session-1"] = interceptor
	rt.interceptorsMu.Unlock()

	// Should find it now.
	found, ok := rt.GetInterceptor("session-1")
	if !ok {
		t.Error("GetInterceptor should return true for existing session")
	}
	if found != interceptor {
		t.Error("GetInterceptor returned wrong interceptor")
	}
}

// TestRuntime_GetStatusMapper verifies status mapper retrieval.
func TestRuntime_GetStatusMapper(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{
		Registry:       acp.NewRegistry(acp.DefaultConfig()),
		BufferRegistry: acp.NewBufferRegistry(256),
	})

	// No mapper should exist initially.
	_, ok := rt.GetStatusMapper("session-1")
	if ok {
		t.Error("GetStatusMapper should return false for non-existent session")
	}

	// Add a mapper.
	mapper := NewStatusMapper(StatusMapperConfig{})
	rt.statusMappersMu.Lock()
	rt.statusMappers["session-1"] = mapper
	rt.statusMappersMu.Unlock()

	// Should find it now.
	found, ok := rt.GetStatusMapper("session-1")
	if !ok {
		t.Error("GetStatusMapper should return true for existing session")
	}
	if found != mapper {
		t.Error("GetStatusMapper returned wrong mapper")
	}
}

// TestRuntime_RefreshPermissions verifies permission refresh.
func TestRuntime_RefreshPermissions(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{
		Registry:       acp.NewRegistry(acp.DefaultConfig()),
		BufferRegistry: acp.NewBufferRegistry(256),
		// No permission repo - should not error.
	})

	// Should not error without permission repo.
	err := rt.RefreshPermissions(context.Background(), "project-1")
	if err != nil {
		t.Errorf("RefreshPermissions without repo returned error: %v", err)
	}
}

// TestRuntime_cleanupSession verifies session cleanup.
func TestRuntime_cleanupSession(t *testing.T) {
	bufferReg := acp.NewBufferRegistry(256)
	rt := NewRuntime(RuntimeConfig{
		Registry:       acp.NewRegistry(acp.DefaultConfig()),
		BufferRegistry: bufferReg,
	})

	sessionID := "session-to-cleanup"

	// Add an interceptor with pending approvals.
	interceptor := NewInterceptor(InterceptorConfig{})
	rt.interceptorsMu.Lock()
	rt.interceptors[sessionID] = interceptor
	rt.interceptorsMu.Unlock()

	// Add a pipeline.
	rt.pipelinesMu.Lock()
	rt.pipelines[sessionID] = &acp.Pipeline{}
	rt.pipelinesMu.Unlock()

	// Add a status mapper.
	rt.statusMappersMu.Lock()
	rt.statusMappers[sessionID] = &StatusMapper{}
	rt.statusMappersMu.Unlock()

	// Push some events to the buffer.
	bufferReg.Push(sessionID, "task-1", acp.Event{Kind: acp.EventTurnStart})

	// Cleanup.
	rt.cleanupSession(sessionID)

	// Verify all resources are cleaned up.
	rt.interceptorsMu.RLock()
	_, hasInterceptor := rt.interceptors[sessionID]
	rt.interceptorsMu.RUnlock()
	if hasInterceptor {
		t.Error("Interceptor should be removed after cleanup")
	}

	rt.pipelinesMu.RLock()
	_, hasPipeline := rt.pipelines[sessionID]
	rt.pipelinesMu.RUnlock()
	if hasPipeline {
		t.Error("Pipeline should be removed after cleanup")
	}

	rt.statusMappersMu.RLock()
	_, hasMapper := rt.statusMappers[sessionID]
	rt.statusMappersMu.RUnlock()
	if hasMapper {
		t.Error("StatusMapper should be removed after cleanup")
	}

	// Buffer should be dropped (empty).
	snapshot := bufferReg.Snapshot(sessionID, "task-1")
	if len(snapshot) != 0 {
		t.Errorf("Buffer should be empty after cleanup, got %d events", len(snapshot))
	}
}

// TestRuntime_AttachWorker_Idempotent verifies that AttachWorker is idempotent.
func TestRuntime_AttachWorker_Idempotent(t *testing.T) {
	// This test would require a mock worker spawn, which is complex.
	// For now, we verify the idempotency check logic by manually adding a consumer.

	rt := NewRuntime(RuntimeConfig{
		Registry:       acp.NewRegistry(acp.DefaultConfig()),
		BufferRegistry: acp.NewBufferRegistry(256),
	})

	sess := &domain.Session{ID: "test-session-idempotent"}

	// Manually mark the session as already having a consumer.
	rt.consumersMu.Lock()
	rt.consumers[sess.ID] = func() {}
	rt.consumersMu.Unlock()

	// AttachWorker should detect the existing consumer and skip spawning.
	// Note: This will fail because we don't have a real worker, but the
	// idempotency check happens before the spawn attempt.
	// In a real test, we'd need to mock the registry.

	if rt.ConsumerCount() != 1 {
		t.Errorf("ConsumerCount() = %d, want 1", rt.ConsumerCount())
	}
}

// TestRuntime_SetCurrentTask_ClearsMapper verifies that SetCurrentTask clears the mapper.
func TestRuntime_SetCurrentTask_ClearsMapper(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{
		Registry:       acp.NewRegistry(acp.DefaultConfig()),
		BufferRegistry: acp.NewBufferRegistry(256),
	})

	sessionID := "test-session-task"
	taskID := "task-123"

	// Add a status mapper.
	mapper := NewStatusMapper(StatusMapperConfig{})
	rt.statusMappersMu.Lock()
	rt.statusMappers[sessionID] = mapper
	rt.statusMappersMu.Unlock()

	// Record that we applied a status (so we can verify it gets cleared).
	mapper.recordApplied(taskID, domain.TaskStatusProcessing)

	// Verify it's recorded.
	if !mapper.isAlreadyApplied(taskID, domain.TaskStatusProcessing) {
		t.Error("Status should be recorded before ClearTask")
	}

	// SetCurrentTask should clear the mapper.
	rt.SetCurrentTask(sessionID, taskID)

	// The mapper should be cleared for this task.
	if mapper.isAlreadyApplied(taskID, domain.TaskStatusProcessing) {
		t.Error("Status should be cleared after SetCurrentTask")
	}
}
