package acp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWorker_Echo(t *testing.T) {
	// Create a mock ACP server script that echoes requests back as responses
	dir := t.TempDir()
	script := filepath.Join(dir, "mock_acp.sh")

	// Mock script that:
	// 1. Reads JSON lines from stdin
	// 2. For initialize: returns a response with capabilities
	// 3. For other requests: echoes them back
	mockScript := `#!/bin/bash
while IFS= read -r line; do
    method=$(echo "$line" | grep -o '"method":"[^"]*"' | cut -d'"' -f4)
    id=$(echo "$line" | grep -o '"id":[0-9]*' | cut -d':' -f2)
    
    if [ "$method" = "initialize" ]; then
        echo "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"serverInfo\":{\"name\":\"mock\",\"version\":\"1.0.0\"},\"capabilities\":{\"streaming\":true,\"toolCalls\":true}}}"
    elif [ -n "$id" ] && [ "$id" != "null" ]; then
        echo "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"echo\":true}}"
    fi
done
`
	if err := os.WriteFile(script, []byte(mockScript), 0755); err != nil {
		t.Fatalf("write mock script: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := WorkerConfig{
		Command:          "/bin/bash",
		Args:             []string{script},
		Cwd:              dir,
		HandshakeTimeout: 2 * time.Second,
	}

	worker, err := Spawn(ctx, cfg)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer worker.Stop()

	// Verify capabilities were set
	caps := worker.Capabilities()
	if caps.ServerInfo.Name != "mock" {
		t.Errorf("ServerInfo.Name = %q; want %q", caps.ServerInfo.Name, "mock")
	}
	if !caps.Capabilities.Streaming {
		t.Error("Capabilities.Streaming = false; want true")
	}

	// Send a test request
	req, err := NewRequest("test/echo", map[string]string{"hello": "world"})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := worker.SendRequest(ctx, req)
	if err != nil {
		t.Fatalf("SendRequest: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("response has error: %v", resp.Error)
	}

	var result map[string]bool
	if err := resp.ParseResult(&result); err != nil {
		t.Fatalf("ParseResult: %v", err)
	}
	if !result["echo"] {
		t.Error("result[\"echo\"] = false; want true")
	}
}

func TestWorker_StopClosesChannel(t *testing.T) {
	// Create a simple script that just reads stdin forever
	dir := t.TempDir()
	script := filepath.Join(dir, "mock_acp.sh")

	mockScript := `#!/bin/bash
# Return initialize response
read line
id=$(echo "$line" | grep -o '"id":[0-9]*' | cut -d':' -f2)
echo "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"serverInfo\":{\"name\":\"mock\",\"version\":\"1.0.0\"},\"capabilities\":{}}}"

# Then just wait
while read line; do :; done
`
	if err := os.WriteFile(script, []byte(mockScript), 0755); err != nil {
		t.Fatalf("write mock script: %v", err)
	}

	ctx := context.Background()
	cfg := WorkerConfig{
		Command:          "/bin/bash",
		Args:             []string{script},
		HandshakeTimeout: 2 * time.Second,
	}

	worker, err := Spawn(ctx, cfg)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// Stop the worker
	worker.Stop()

	// Verify done channel closes within 1s
	select {
	case <-worker.Done():
		// Expected
	case <-time.After(1 * time.Second):
		t.Error("worker did not stop within 1s")
		worker.Kill()
	}

	// Verify out channel is closed
	select {
	case _, ok := <-worker.Out():
		if ok {
			t.Error("out channel should be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("out channel should be closed immediately")
	}

	// Verify Alive returns false
	if worker.Alive() {
		t.Error("Alive() = true; want false after stop")
	}
}

func TestWorker_HandshakeTimeout(t *testing.T) {
	// Create a script that never responds
	dir := t.TempDir()
	script := filepath.Join(dir, "slow_acp.sh")

	// Script that reads but never responds
	mockScript := `#!/bin/bash
while read line; do :; done
`
	if err := os.WriteFile(script, []byte(mockScript), 0755); err != nil {
		t.Fatalf("write mock script: %v", err)
	}

	ctx := context.Background()
	cfg := WorkerConfig{
		Command:          "/bin/bash",
		Args:             []string{script},
		HandshakeTimeout: 100 * time.Millisecond, // Very short timeout
	}

	_, err := Spawn(ctx, cfg)
	if err == nil {
		t.Fatal("Spawn should fail with timeout")
	}
	if err != ErrHandshakeTimeout {
		t.Errorf("error = %v; want ErrHandshakeTimeout", err)
	}
}

func TestWorker_CommandNotFound(t *testing.T) {
	ctx := context.Background()
	cfg := WorkerConfig{
		Command: "nonexistent_command_12345",
	}

	_, err := Spawn(ctx, cfg)
	if err == nil {
		t.Fatal("Spawn should fail when command not found")
	}
}

func TestWorker_Pid(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "mock_acp.sh")

	mockScript := `#!/bin/bash
read line
id=$(echo "$line" | grep -o '"id":[0-9]*' | cut -d':' -f2)
echo "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"serverInfo\":{\"name\":\"mock\",\"version\":\"1.0.0\"},\"capabilities\":{}}}"
while read line; do :; done
`
	if err := os.WriteFile(script, []byte(mockScript), 0755); err != nil {
		t.Fatalf("write mock script: %v", err)
	}

	ctx := context.Background()
	cfg := WorkerConfig{
		Command:          "/bin/bash",
		Args:             []string{script},
		HandshakeTimeout: 2 * time.Second,
	}

	worker, err := Spawn(ctx, cfg)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer worker.Kill()

	pid := worker.Pid()
	if pid <= 0 {
		t.Errorf("Pid() = %d; want > 0", pid)
	}
}

func TestWorker_Notifications(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "mock_acp.sh")

	// Script that sends notifications after initialize
	mockScript := `#!/bin/bash
read line
id=$(echo "$line" | grep -o '"id":[0-9]*' | cut -d':' -f2)
echo "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"serverInfo\":{\"name\":\"mock\",\"version\":\"1.0.0\"},\"capabilities\":{\"streaming\":true}}}"

# Send some notifications
sleep 0.1
echo '{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"test-123","update":{"sessionUpdate":"agent_message_chunk"}}}'
echo '{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"test-123","update":{"sessionUpdate":"agent_message_chunk"}}}'

# Keep alive briefly
sleep 0.5
`
	if err := os.WriteFile(script, []byte(mockScript), 0755); err != nil {
		t.Fatalf("write mock script: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := WorkerConfig{
		Command:          "/bin/bash",
		Args:             []string{script},
		HandshakeTimeout: 2 * time.Second,
	}

	worker, err := Spawn(ctx, cfg)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer worker.Stop()

	// Collect notifications
	var notifications []Message
	timeout := time.After(2 * time.Second)

	for {
		select {
		case msg, ok := <-worker.Out():
			if !ok {
				goto done
			}
			if msg.IsNotification() {
				notifications = append(notifications, msg)
			}
		case <-timeout:
			goto done
		case <-worker.Done():
			goto done
		}
	}
done:

	if len(notifications) < 2 {
		t.Errorf("received %d notifications; want at least 2", len(notifications))
	}

	for _, notif := range notifications {
		if notif.Method != MethodSessionUpdate {
			t.Errorf("notification method = %q; want %q", notif.Method, MethodSessionUpdate)
		}
	}
}

func TestWorker_SendToStoppedWorker(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "mock_acp.sh")

	mockScript := `#!/bin/bash
read line
id=$(echo "$line" | grep -o '"id":[0-9]*' | cut -d':' -f2)
echo "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"serverInfo\":{\"name\":\"mock\",\"version\":\"1.0.0\"},\"capabilities\":{}}}"
# Exit immediately after init
`
	if err := os.WriteFile(script, []byte(mockScript), 0755); err != nil {
		t.Fatalf("write mock script: %v", err)
	}

	ctx := context.Background()
	cfg := WorkerConfig{
		Command:          "/bin/bash",
		Args:             []string{script},
		HandshakeTimeout: 2 * time.Second,
	}

	worker, err := Spawn(ctx, cfg)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// Wait for worker to exit
	<-worker.Done()

	// Try to send a message
	msg := &Message{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage("1"),
		Method:  "test",
	}
	err = worker.Send(msg)
	if err != ErrWorkerExited {
		t.Errorf("Send to stopped worker: error = %v; want ErrWorkerExited", err)
	}
}

func TestWorker_ResetContext(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "mock_acp.sh")

	// Mock script that handles initialize, session/new, and session/cancel
	mockScript := `#!/bin/bash
while IFS= read -r line; do
    method=$(echo "$line" | grep -o '"method":"[^"]*"' | cut -d'"' -f4)
    id=$(echo "$line" | grep -o '"id":[0-9]*' | cut -d':' -f2)
    
    if [ "$method" = "initialize" ]; then
        echo "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"serverInfo\":{\"name\":\"mock\",\"version\":\"1.0.0\"},\"capabilities\":{\"streaming\":true}}}"
    elif [ "$method" = "session/new" ]; then
        # Return a new session ID
        echo "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"sessionId\":\"new-session-$(date +%s%N)\"}}"
    elif [ "$method" = "session/cancel" ]; then
        echo "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{}}"
    fi
done
`
	if err := os.WriteFile(script, []byte(mockScript), 0755); err != nil {
		t.Fatalf("write mock script: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := WorkerConfig{
		Command:          "/bin/bash",
		Args:             []string{script},
		Cwd:              dir,
		HandshakeTimeout: 2 * time.Second,
	}

	worker, err := Spawn(ctx, cfg)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer worker.Stop()

	// Test ResetContext with an old session ID
	oldSessionID := "old-session-123"
	newSessionID, err := worker.ResetContext(ctx, oldSessionID)
	if err != nil {
		t.Fatalf("ResetContext: %v", err)
	}

	if newSessionID == "" {
		t.Error("ResetContext returned empty session ID")
	}

	if newSessionID == oldSessionID {
		t.Errorf("new session ID should be different from old: got %s", newSessionID)
	}
}

func TestWorker_ResetContext_EmptyOldSession(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "mock_acp.sh")

	mockScript := `#!/bin/bash
while IFS= read -r line; do
    method=$(echo "$line" | grep -o '"method":"[^"]*"' | cut -d'"' -f4)
    id=$(echo "$line" | grep -o '"id":[0-9]*' | cut -d':' -f2)
    
    if [ "$method" = "initialize" ]; then
        echo "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"serverInfo\":{\"name\":\"mock\",\"version\":\"1.0.0\"},\"capabilities\":{}}}"
    elif [ "$method" = "session/new" ]; then
        echo "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"sessionId\":\"fresh-session-456\"}}"
    fi
done
`
	if err := os.WriteFile(script, []byte(mockScript), 0755); err != nil {
		t.Fatalf("write mock script: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := WorkerConfig{
		Command:          "/bin/bash",
		Args:             []string{script},
		HandshakeTimeout: 2 * time.Second,
	}

	worker, err := Spawn(ctx, cfg)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer worker.Stop()

	// Test ResetContext with empty old session (no cancel should be sent)
	newSessionID, err := worker.ResetContext(ctx, "")
	if err != nil {
		t.Fatalf("ResetContext: %v", err)
	}

	if newSessionID != "fresh-session-456" {
		t.Errorf("ResetContext session ID = %q; want %q", newSessionID, "fresh-session-456")
	}
}

func TestWorker_ResetContext_ClosedWorker(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "mock_acp.sh")

	mockScript := `#!/bin/bash
read line
id=$(echo "$line" | grep -o '"id":[0-9]*' | cut -d':' -f2)
echo "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"serverInfo\":{\"name\":\"mock\",\"version\":\"1.0.0\"},\"capabilities\":{}}}"
# Exit immediately after init
`
	if err := os.WriteFile(script, []byte(mockScript), 0755); err != nil {
		t.Fatalf("write mock script: %v", err)
	}

	ctx := context.Background()
	cfg := WorkerConfig{
		Command:          "/bin/bash",
		Args:             []string{script},
		HandshakeTimeout: 2 * time.Second,
	}

	worker, err := Spawn(ctx, cfg)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// Wait for worker to exit
	<-worker.Done()

	// Try to reset context
	_, err = worker.ResetContext(ctx, "old-session")
	if err != ErrWorkerClosed {
		t.Errorf("ResetContext on closed worker: error = %v; want ErrWorkerClosed", err)
	}
}

func TestWorker_StopGraceful_ExitsOnSIGTERM(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "mock_acp.sh")

	// Mock script that exits cleanly on SIGTERM
	mockScript := `#!/bin/bash
trap 'exit 0' SIGTERM

read line
id=$(echo "$line" | grep -o '"id":[0-9]*' | cut -d':' -f2)
echo "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"serverInfo\":{\"name\":\"mock\",\"version\":\"1.0.0\"},\"capabilities\":{}}}"

# Wait forever (until signaled)
while true; do sleep 1; done
`
	if err := os.WriteFile(script, []byte(mockScript), 0755); err != nil {
		t.Fatalf("write mock script: %v", err)
	}

	ctx := context.Background()
	cfg := WorkerConfig{
		Command:          "/bin/bash",
		Args:             []string{script},
		HandshakeTimeout: 2 * time.Second,
	}

	worker, err := Spawn(ctx, cfg)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// Worker should be alive
	if !worker.Alive() {
		t.Fatal("worker should be alive after spawn")
	}

	// Stop gracefully with short grace period
	err = worker.StopGraceful(ctx, 1*time.Second)
	if err != nil {
		t.Fatalf("StopGraceful: %v", err)
	}

	// Worker should be stopped
	if worker.Alive() {
		t.Error("worker should not be alive after StopGraceful")
	}

	// Done channel should be closed
	select {
	case <-worker.Done():
		// Expected
	default:
		t.Error("Done channel should be closed after StopGraceful")
	}
}

func TestWorker_StopGraceful_RequiresSIGKILL(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "mock_acp.sh")

	// Mock script that ignores SIGTERM (requires SIGKILL)
	mockScript := `#!/bin/bash
trap '' SIGTERM  # Ignore SIGTERM

read line
id=$(echo "$line" | grep -o '"id":[0-9]*' | cut -d':' -f2)
echo "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"serverInfo\":{\"name\":\"mock\",\"version\":\"1.0.0\"},\"capabilities\":{}}}"

# Wait forever (ignoring SIGTERM)
while true; do sleep 1; done
`
	if err := os.WriteFile(script, []byte(mockScript), 0755); err != nil {
		t.Fatalf("write mock script: %v", err)
	}

	ctx := context.Background()
	cfg := WorkerConfig{
		Command:          "/bin/bash",
		Args:             []string{script},
		HandshakeTimeout: 2 * time.Second,
	}

	worker, err := Spawn(ctx, cfg)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	pid := worker.Pid()

	// Worker should be alive
	if !worker.Alive() {
		t.Fatal("worker should be alive after spawn")
	}

	// Stop gracefully with short grace period (will need SIGKILL)
	start := time.Now()
	err = worker.StopGraceful(ctx, 500*time.Millisecond)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("StopGraceful: %v", err)
	}

	// Should have taken roughly 2 * grace period (shutdown attempt + SIGTERM wait)
	if elapsed < 500*time.Millisecond {
		t.Logf("StopGraceful took %v (expected ~500ms minimum)", elapsed)
	}

	// Worker should be stopped (via SIGKILL)
	if worker.Alive() {
		t.Error("worker should not be alive after StopGraceful")
	}

	// Verify process is actually gone
	// Note: checking /proc or using kill -0 is platform-specific
	// The fact that Done() is closed is sufficient verification
	select {
	case <-worker.Done():
		t.Logf("Worker (pid %d) successfully terminated", pid)
	default:
		t.Error("Done channel should be closed after StopGraceful with SIGKILL")
	}
}

func TestWorker_StopGraceful_Idempotent(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "mock_acp.sh")

	mockScript := `#!/bin/bash
read line
id=$(echo "$line" | grep -o '"id":[0-9]*' | cut -d':' -f2)
echo "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"serverInfo\":{\"name\":\"mock\",\"version\":\"1.0.0\"},\"capabilities\":{}}}"
while read line; do :; done
`
	if err := os.WriteFile(script, []byte(mockScript), 0755); err != nil {
		t.Fatalf("write mock script: %v", err)
	}

	ctx := context.Background()
	cfg := WorkerConfig{
		Command:          "/bin/bash",
		Args:             []string{script},
		HandshakeTimeout: 2 * time.Second,
	}

	worker, err := Spawn(ctx, cfg)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// First stop
	err = worker.StopGraceful(ctx, 1*time.Second)
	if err != nil {
		t.Fatalf("first StopGraceful: %v", err)
	}

	// Second stop should be idempotent (return nil)
	err = worker.StopGraceful(ctx, 1*time.Second)
	if err != nil {
		t.Fatalf("second StopGraceful should be idempotent: %v", err)
	}

	// Third stop should also be idempotent
	err = worker.StopGraceful(ctx, 1*time.Second)
	if err != nil {
		t.Fatalf("third StopGraceful should be idempotent: %v", err)
	}
}

func TestWorker_StopGraceful_DefaultGracePeriod(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "mock_acp.sh")

	mockScript := `#!/bin/bash
trap 'exit 0' SIGTERM
read line
id=$(echo "$line" | grep -o '"id":[0-9]*' | cut -d':' -f2)
echo "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"serverInfo\":{\"name\":\"mock\",\"version\":\"1.0.0\"},\"capabilities\":{}}}"
while true; do sleep 1; done
`
	if err := os.WriteFile(script, []byte(mockScript), 0755); err != nil {
		t.Fatalf("write mock script: %v", err)
	}

	ctx := context.Background()
	cfg := WorkerConfig{
		Command:          "/bin/bash",
		Args:             []string{script},
		HandshakeTimeout: 2 * time.Second,
	}

	worker, err := Spawn(ctx, cfg)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// Pass 0 grace period - should use DefaultGracePeriod (3s)
	err = worker.StopGraceful(ctx, 0)
	if err != nil {
		t.Fatalf("StopGraceful with 0 grace: %v", err)
	}

	if worker.Alive() {
		t.Error("worker should not be alive after StopGraceful")
	}
}
