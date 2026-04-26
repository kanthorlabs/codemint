package acp

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"codemint.kanthorlabs.com/internal/domain"
)

func createMockACPScript(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "mock_acp.sh")

	mockScript := `#!/bin/bash
# Handle initialize request
read line
id=$(echo "$line" | grep -o '"id":[0-9]*' | cut -d':' -f2)
echo "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"serverInfo\":{\"name\":\"mock\",\"version\":\"1.0.0\"},\"capabilities\":{\"streaming\":true}}}"

# Keep running until stdin closes
while read line 2>/dev/null; do
    id=$(echo "$line" | grep -o '"id":[0-9]*' | cut -d':' -f2)
    if [ -n "$id" ] && [ "$id" != "null" ]; then
        echo "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"ok\":true}}"
    fi
done
`
	if err := os.WriteFile(script, []byte(mockScript), 0755); err != nil {
		t.Fatalf("write mock script: %v", err)
	}
	return script
}

func TestRegistry_GetOrSpawn_Idempotent(t *testing.T) {
	script := createMockACPScript(t)

	cfg := WorkerConfig{
		Command:          "/bin/bash",
		Args:             []string{script},
		HandshakeTimeout: 2 * time.Second,
	}

	registry := NewRegistry(cfg)
	defer registry.StopAll(context.Background())

	sess := domain.NewSession("project-123")
	project := &domain.Project{
		ID:         "project-123",
		WorkingDir: t.TempDir(),
	}

	ctx := context.Background()

	// Spawn first worker
	worker1, err := registry.GetOrSpawn(ctx, sess, project)
	if err != nil {
		t.Fatalf("GetOrSpawn 1: %v", err)
	}

	// Second call should return the same worker
	worker2, err := registry.GetOrSpawn(ctx, sess, project)
	if err != nil {
		t.Fatalf("GetOrSpawn 2: %v", err)
	}

	if worker1 != worker2 {
		t.Error("GetOrSpawn not idempotent: returned different workers")
	}

	// Verify only one worker
	if registry.Count() != 1 {
		t.Errorf("Count() = %d; want 1", registry.Count())
	}
}

func TestRegistry_GetOrSpawn_Concurrent(t *testing.T) {
	script := createMockACPScript(t)

	cfg := WorkerConfig{
		Command:          "/bin/bash",
		Args:             []string{script},
		HandshakeTimeout: 2 * time.Second,
	}

	registry := NewRegistry(cfg)
	defer registry.StopAll(context.Background())

	sess := domain.NewSession("project-123")
	project := &domain.Project{
		ID:         "project-123",
		WorkingDir: t.TempDir(),
	}

	ctx := context.Background()

	// Spawn multiple workers concurrently
	const numGoroutines = 10
	var wg sync.WaitGroup
	workers := make([]*Worker, numGoroutines)
	errors := make([]error, numGoroutines)

	for i := range numGoroutines {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			workers[idx], errors[idx] = registry.GetOrSpawn(ctx, sess, project)
		}(i)
	}
	wg.Wait()

	// Check for errors
	for i, err := range errors {
		if err != nil {
			t.Errorf("goroutine %d: GetOrSpawn: %v", i, err)
		}
	}

	// All should return the same worker
	firstWorker := workers[0]
	for i, w := range workers[1:] {
		if w != firstWorker {
			t.Errorf("goroutine %d returned different worker", i+1)
		}
	}

	// Only one worker should exist
	if registry.Count() != 1 {
		t.Errorf("Count() = %d; want 1", registry.Count())
	}
}

func TestRegistry_Stop_RemovesEntry(t *testing.T) {
	script := createMockACPScript(t)

	cfg := WorkerConfig{
		Command:          "/bin/bash",
		Args:             []string{script},
		HandshakeTimeout: 2 * time.Second,
	}

	registry := NewRegistry(cfg)

	sess := domain.NewSession("project-123")
	project := &domain.Project{
		ID:         "project-123",
		WorkingDir: t.TempDir(),
	}

	ctx := context.Background()

	// Spawn worker
	_, err := registry.GetOrSpawn(ctx, sess, project)
	if err != nil {
		t.Fatalf("GetOrSpawn: %v", err)
	}

	// Verify it exists
	if _, ok := registry.Get(sess.ID); !ok {
		t.Error("Get returned false after spawn")
	}

	// Stop the worker
	if err := registry.Stop(ctx, sess.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Verify it's removed
	if _, ok := registry.Get(sess.ID); ok {
		t.Error("Get returned true after Stop")
	}

	if registry.Count() != 0 {
		t.Errorf("Count() = %d; want 0", registry.Count())
	}
}

func TestRegistry_MultipleSessions(t *testing.T) {
	script := createMockACPScript(t)

	cfg := WorkerConfig{
		Command:          "/bin/bash",
		Args:             []string{script},
		HandshakeTimeout: 2 * time.Second,
	}

	registry := NewRegistry(cfg)
	defer registry.StopAll(context.Background())

	ctx := context.Background()

	// Create multiple sessions
	sess1 := domain.NewSession("project-1")
	sess2 := domain.NewSession("project-2")
	sess3 := domain.NewSession("project-3")

	project := &domain.Project{
		ID:         "project-1",
		WorkingDir: t.TempDir(),
	}

	// Spawn workers for each session
	w1, err := registry.GetOrSpawn(ctx, sess1, project)
	if err != nil {
		t.Fatalf("GetOrSpawn sess1: %v", err)
	}

	w2, err := registry.GetOrSpawn(ctx, sess2, project)
	if err != nil {
		t.Fatalf("GetOrSpawn sess2: %v", err)
	}

	w3, err := registry.GetOrSpawn(ctx, sess3, project)
	if err != nil {
		t.Fatalf("GetOrSpawn sess3: %v", err)
	}

	// All should be different workers
	if w1 == w2 || w2 == w3 || w1 == w3 {
		t.Error("different sessions should have different workers")
	}

	// All should have different PIDs
	pids := map[int]bool{
		w1.Pid(): true,
		w2.Pid(): true,
		w3.Pid(): true,
	}
	if len(pids) != 3 {
		t.Error("workers should have different PIDs")
	}

	if registry.Count() != 3 {
		t.Errorf("Count() = %d; want 3", registry.Count())
	}
}

func TestRegistry_StopAll(t *testing.T) {
	script := createMockACPScript(t)

	cfg := WorkerConfig{
		Command:          "/bin/bash",
		Args:             []string{script},
		HandshakeTimeout: 2 * time.Second,
	}

	registry := NewRegistry(cfg)

	ctx := context.Background()

	// Create multiple sessions and workers
	for i := range 3 {
		sess := domain.NewSession("project")
		sess.ID = sess.ID + string(rune('a'+i)) // Make unique
		project := &domain.Project{
			ID:         "project",
			WorkingDir: t.TempDir(),
		}

		_, err := registry.GetOrSpawn(ctx, sess, project)
		if err != nil {
			t.Fatalf("GetOrSpawn %d: %v", i, err)
		}
	}

	if registry.Count() != 3 {
		t.Fatalf("Count() = %d; want 3 before StopAll", registry.Count())
	}

	// Stop all workers
	if err := registry.StopAll(ctx); err != nil {
		t.Fatalf("StopAll: %v", err)
	}

	// Wait a bit for cleanup goroutines
	time.Sleep(100 * time.Millisecond)

	if registry.Count() != 0 {
		t.Errorf("Count() = %d; want 0 after StopAll", registry.Count())
	}
}

func TestRegistry_NilSession(t *testing.T) {
	cfg := WorkerConfig{
		Command: "echo",
	}
	registry := NewRegistry(cfg)

	_, err := registry.GetOrSpawn(context.Background(), nil, nil)
	if err == nil {
		t.Error("GetOrSpawn with nil session should return error")
	}
}

func TestRegistry_DeadWorkerRespawn(t *testing.T) {
	script := createMockACPScript(t)

	cfg := WorkerConfig{
		Command:          "/bin/bash",
		Args:             []string{script},
		HandshakeTimeout: 2 * time.Second,
	}

	registry := NewRegistry(cfg)
	defer registry.StopAll(context.Background())

	sess := domain.NewSession("project-123")
	project := &domain.Project{
		ID:         "project-123",
		WorkingDir: t.TempDir(),
	}

	ctx := context.Background()

	// Spawn first worker
	worker1, err := registry.GetOrSpawn(ctx, sess, project)
	if err != nil {
		t.Fatalf("GetOrSpawn 1: %v", err)
	}
	pid1 := worker1.Pid()

	// Kill the worker
	worker1.Kill()
	<-worker1.Done()

	// Wait for cleanup goroutine
	time.Sleep(100 * time.Millisecond)

	// GetOrSpawn should create a new worker
	worker2, err := registry.GetOrSpawn(ctx, sess, project)
	if err != nil {
		t.Fatalf("GetOrSpawn 2: %v", err)
	}
	pid2 := worker2.Pid()

	if pid1 == pid2 {
		t.Error("respawned worker should have different PID")
	}

	if !worker2.Alive() {
		t.Error("new worker should be alive")
	}
}

func TestRegistry_EnvOverride(t *testing.T) {
	// Save and restore env
	oldVal := os.Getenv(EnvACPCommand)
	defer os.Setenv(EnvACPCommand, oldVal)

	os.Setenv(EnvACPCommand, "/custom/command")

	cfg := WorkerConfig{
		Command: "opencode",
		Args:    []string{"acp"},
	}

	registry := NewRegistry(cfg)

	// Check that the config was overridden
	if registry.cfg.Command != "/custom/command" {
		t.Errorf("Command = %q; want %q", registry.cfg.Command, "/custom/command")
	}
	if len(registry.cfg.Args) != 0 {
		t.Errorf("Args should be reset when using env override")
	}
}
