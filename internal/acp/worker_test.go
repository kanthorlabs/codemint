package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mockACPScript provides a builder for creating mock ACP server bash scripts.
// It handles the common patterns needed for testing Worker functionality.
type mockACPScript struct {
	// capabilities configures the initialize response (spec-compliant)
	capabilities struct {
		loadSession bool
	}
	// sessionID is returned by session/new response
	sessionID string
	// afterHandshake specifies what to do after init + session/new
	afterHandshake afterHandshakeBehavior
	// signalHandler configures trap for SIGTERM
	signalHandler signalHandlerType
	// notifications to send after handshake
	notifications []string
	// notificationDelay before sending notifications
	notificationDelay time.Duration
	// echoRequests echoes back any request with an ID
	echoRequests bool
	// useMethodLoop indicates whether to use a method handling loop
	useMethodLoop bool
	// handleMethods specifies which methods to handle in a loop
	handleMethods []string
}

type afterHandshakeBehavior int

const (
	// afterHandshakeWait reads stdin forever (default)
	afterHandshakeWait afterHandshakeBehavior = iota
	// afterHandshakeExit exits immediately
	afterHandshakeExit
	// afterHandshakeSleep sleeps forever
	afterHandshakeSleep
	// afterHandshakeNone does nothing (for custom loop handling)
	afterHandshakeNone
)

type signalHandlerType int

const (
	// signalHandlerNone - no signal handling (default)
	signalHandlerNone signalHandlerType = iota
	// signalHandlerExitOnSIGTERM - exit 0 on SIGTERM
	signalHandlerExitOnSIGTERM
	// signalHandlerIgnoreSIGTERM - ignore SIGTERM
	signalHandlerIgnoreSIGTERM
)

// newMockACPScript creates a new mock script builder with default settings.
func newMockACPScript() *mockACPScript {
	return &mockACPScript{
		sessionID:      "test-session",
		afterHandshake: afterHandshakeWait,
	}
}

// withCapabilities sets the capabilities in the initialize response.
// Per ACP spec, capabilities use agentCapabilities with loadSession etc.
func (m *mockACPScript) withCapabilities(loadSession, _ bool) *mockACPScript {
	m.capabilities.loadSession = loadSession
	return m
}

// withSessionID sets the session ID returned by session/new.
func (m *mockACPScript) withSessionID(id string) *mockACPScript {
	m.sessionID = id
	return m
}

// withAfterHandshake sets what happens after the handshake completes.
func (m *mockACPScript) withAfterHandshake(behavior afterHandshakeBehavior) *mockACPScript {
	m.afterHandshake = behavior
	return m
}

// withSignalHandler sets the signal handling behavior.
func (m *mockACPScript) withSignalHandler(handler signalHandlerType) *mockACPScript {
	m.signalHandler = handler
	return m
}

// withNotifications adds notifications to send after handshake.
func (m *mockACPScript) withNotifications(delay time.Duration, notifications ...string) *mockACPScript {
	m.notificationDelay = delay
	m.notifications = notifications
	return m
}

// withEchoRequests enables echoing back any request with an ID.
func (m *mockACPScript) withEchoRequests() *mockACPScript {
	m.echoRequests = true
	return m
}

// withMethodHandlers sets up a loop that handles specific methods.
func (m *mockACPScript) withMethodHandlers(methods ...string) *mockACPScript {
	m.useMethodLoop = true
	m.handleMethods = methods
	m.afterHandshake = afterHandshakeNone
	return m
}

// build generates the bash script content.
func (m *mockACPScript) build() string {
	var sb strings.Builder

	sb.WriteString("#!/bin/bash\n")

	// Signal handler (must be at the top)
	switch m.signalHandler {
	case signalHandlerExitOnSIGTERM:
		sb.WriteString("trap 'exit 0' SIGTERM\n")
	case signalHandlerIgnoreSIGTERM:
		sb.WriteString("trap '' SIGTERM\n")
	}
	sb.WriteString("\n")

	// If using method handlers loop, use that pattern
	if m.useMethodLoop || m.echoRequests {
		m.buildMethodLoop(&sb)
		return sb.String()
	}

	// Standard sequential pattern: initialize + session/new
	m.buildHandshake(&sb)

	// Notifications
	if len(m.notifications) > 0 {
		if m.notificationDelay > 0 {
			sb.WriteString(fmt.Sprintf("sleep %.1f\n", m.notificationDelay.Seconds()))
		}
		for _, notif := range m.notifications {
			sb.WriteString(fmt.Sprintf("echo '%s'\n", notif))
		}
	}

	// After handshake behavior
	switch m.afterHandshake {
	case afterHandshakeWait:
		sb.WriteString("while read line; do :; done\n")
	case afterHandshakeExit:
		// Script ends, process exits
	case afterHandshakeSleep:
		sb.WriteString("while true; do sleep 1; done\n")
	}

	return sb.String()
}

// buildHandshake writes the standard initialize + session/new handshake.
// Per ACP spec: InitializeResult has protocolVersion, agentInfo, agentCapabilities, authMethods
func (m *mockACPScript) buildHandshake(sb *strings.Builder) {
	// Initialize response (spec-compliant format)
	sb.WriteString("read line\n")
	sb.WriteString("id=$(echo \"$line\" | grep -o '\"id\":[0-9]*' | cut -d':' -f2)\n")
	capsJSON := m.agentCapabilitiesJSON()
	sb.WriteString(fmt.Sprintf("echo \"{\\\"jsonrpc\\\":\\\"2.0\\\",\\\"id\\\":$id,\\\"result\\\":{\\\"protocolVersion\\\":1,\\\"agentInfo\\\":{\\\"name\\\":\\\"mock\\\",\\\"version\\\":\\\"1.0.0\\\"},\\\"agentCapabilities\\\":%s}}\"\n", capsJSON))
	sb.WriteString("\n")

	// Session/new response
	sb.WriteString("read line\n")
	sb.WriteString("id=$(echo \"$line\" | grep -o '\"id\":[0-9]*' | cut -d':' -f2)\n")
	sb.WriteString(fmt.Sprintf("echo \"{\\\"jsonrpc\\\":\\\"2.0\\\",\\\"id\\\":$id,\\\"result\\\":{\\\"sessionId\\\":\\\"%s\\\"}}\"\n", m.sessionID))
	sb.WriteString("\n")
}

// buildMethodLoop writes a loop that handles methods by name.
func (m *mockACPScript) buildMethodLoop(sb *strings.Builder) {
	sb.WriteString("while IFS= read -r line; do\n")
	sb.WriteString("    method=$(echo \"$line\" | grep -o '\"method\":\"[^\"]*\"' | cut -d'\"' -f4)\n")
	sb.WriteString("    id=$(echo \"$line\" | grep -o '\"id\":[0-9]*' | cut -d':' -f2)\n")
	sb.WriteString("    \n")

	// Always handle initialize (spec-compliant format)
	capsJSON := m.agentCapabilitiesJSON()
	sb.WriteString("    if [ \"$method\" = \"initialize\" ]; then\n")
	sb.WriteString(fmt.Sprintf("        echo \"{\\\"jsonrpc\\\":\\\"2.0\\\",\\\"id\\\":$id,\\\"result\\\":{\\\"protocolVersion\\\":1,\\\"agentInfo\\\":{\\\"name\\\":\\\"mock\\\",\\\"version\\\":\\\"1.0.0\\\"},\\\"agentCapabilities\\\":%s}}\"\n", capsJSON))

	// Always handle session/new
	sb.WriteString("    elif [ \"$method\" = \"session/new\" ]; then\n")
	if m.sessionID != "" && !strings.Contains(m.sessionID, "$") {
		sb.WriteString(fmt.Sprintf("        echo \"{\\\"jsonrpc\\\":\\\"2.0\\\",\\\"id\\\":$id,\\\"result\\\":{\\\"sessionId\\\":\\\"%s\\\"}}\"\n", m.sessionID))
	} else {
		// Dynamic session ID
		sb.WriteString("        echo \"{\\\"jsonrpc\\\":\\\"2.0\\\",\\\"id\\\":$id,\\\"result\\\":{\\\"sessionId\\\":\\\"new-session-$(date +%s%N)\\\"}}\"\n")
	}

	// Handle additional methods
	for _, method := range m.handleMethods {
		if method == "initialize" || method == "session/new" {
			continue // Already handled
		}
		sb.WriteString(fmt.Sprintf("    elif [ \"$method\" = \"%s\" ]; then\n", method))
		sb.WriteString("        echo \"{\\\"jsonrpc\\\":\\\"2.0\\\",\\\"id\\\":$id,\\\"result\\\":{}}\"\n")
	}

	// Echo any other request with an ID
	if m.echoRequests {
		sb.WriteString("    elif [ -n \"$id\" ] && [ \"$id\" != \"null\" ]; then\n")
		sb.WriteString("        echo \"{\\\"jsonrpc\\\":\\\"2.0\\\",\\\"id\\\":$id,\\\"result\\\":{\\\"echo\\\":true}}\"\n")
	}

	sb.WriteString("    fi\n")
	sb.WriteString("done\n")
}

// agentCapabilitiesJSON returns the agentCapabilities as a JSON object string.
// Per ACP spec: AgentCapabilities has loadSession, mcpCapabilities, promptCapabilities, sessionCapabilities
func (m *mockACPScript) agentCapabilitiesJSON() string {
	if !m.capabilities.loadSession {
		return "{}"
	}
	return "{\\\"loadSession\\\":true}"
}

// writeTo writes the script to a file and returns the path.
func (m *mockACPScript) writeTo(t *testing.T, dir string) string {
	t.Helper()
	script := filepath.Join(dir, "mock_acp.sh")
	if err := os.WriteFile(script, []byte(m.build()), 0755); err != nil {
		t.Fatalf("write mock script: %v", err)
	}
	return script
}

// spawnWorker creates a worker using this mock script.
func (m *mockACPScript) spawnWorker(t *testing.T, ctx context.Context) (*Worker, string) {
	t.Helper()
	dir := t.TempDir()
	script := m.writeTo(t, dir)

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
	return worker, dir
}

// --- Tests ---

func TestWorker_Echo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	worker, _ := newMockACPScript().
		withCapabilities(true, true).
		withEchoRequests().
		spawnWorker(t, ctx)
	defer worker.Stop()

	// Verify capabilities were set (spec-compliant format)
	caps := worker.Capabilities()
	if caps.AgentInfo == nil || caps.AgentInfo.Name != "mock" {
		t.Errorf("AgentInfo.Name = %q; want %q", caps.AgentInfo.Name, "mock")
	}
	if !caps.AgentCapabilities.LoadSession {
		t.Error("AgentCapabilities.LoadSession = false; want true")
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
	ctx := context.Background()

	worker, _ := newMockACPScript().
		withAfterHandshake(afterHandshakeWait).
		spawnWorker(t, ctx)

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
	// Create a script that never responds (special case - no mock builder)
	dir := t.TempDir()
	script := filepath.Join(dir, "slow_acp.sh")
	mockScript := "#!/bin/bash\nwhile read line; do :; done\n"
	if err := os.WriteFile(script, []byte(mockScript), 0755); err != nil {
		t.Fatalf("write mock script: %v", err)
	}

	ctx := context.Background()
	cfg := WorkerConfig{
		Command:          "/bin/bash",
		Args:             []string{script},
		HandshakeTimeout: 100 * time.Millisecond,
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
	ctx := context.Background()

	worker, _ := newMockACPScript().
		withAfterHandshake(afterHandshakeWait).
		spawnWorker(t, ctx)
	defer worker.Kill()

	pid := worker.Pid()
	if pid <= 0 {
		t.Errorf("Pid() = %d; want > 0", pid)
	}
}

func TestWorker_Notifications(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	notifications := []string{
		`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"test-123","update":{"sessionUpdate":"agent_message_chunk"}}}`,
		`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"test-123","update":{"sessionUpdate":"agent_message_chunk"}}}`,
	}

	worker, _ := newMockACPScript().
		withCapabilities(true, false).
		withNotifications(100*time.Millisecond, notifications...).
		withAfterHandshake(afterHandshakeSleep).
		spawnWorker(t, ctx)
	defer worker.Stop()

	// Collect notifications
	var received []Message
	timeout := time.After(2 * time.Second)

	for {
		select {
		case msg, ok := <-worker.Out():
			if !ok {
				goto done
			}
			if msg.IsNotification() {
				received = append(received, msg)
			}
		case <-timeout:
			goto done
		case <-worker.Done():
			goto done
		}
	}
done:

	if len(received) < 2 {
		t.Errorf("received %d notifications; want at least 2", len(received))
	}

	for _, notif := range received {
		if notif.Method != MethodSessionUpdate {
			t.Errorf("notification method = %q; want %q", notif.Method, MethodSessionUpdate)
		}
	}
}

func TestWorker_SendToStoppedWorker(t *testing.T) {
	ctx := context.Background()

	worker, _ := newMockACPScript().
		withAfterHandshake(afterHandshakeExit).
		spawnWorker(t, ctx)

	// Wait for worker to exit
	<-worker.Done()

	// Try to send a message
	msg := &Message{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage("1"),
		Method:  "test",
	}
	err := worker.Send(msg)
	if err != ErrWorkerExited {
		t.Errorf("Send to stopped worker: error = %v; want ErrWorkerExited", err)
	}
}

func TestWorker_ResetContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	worker, _ := newMockACPScript().
		withSessionID("dynamic").
		withMethodHandlers("session/cancel").
		spawnWorker(t, ctx)
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	worker, _ := newMockACPScript().
		withSessionID("fresh-session-456").
		withMethodHandlers().
		spawnWorker(t, ctx)
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
	ctx := context.Background()

	worker, _ := newMockACPScript().
		withAfterHandshake(afterHandshakeExit).
		spawnWorker(t, ctx)

	// Wait for worker to exit
	<-worker.Done()

	// Try to reset context
	_, err := worker.ResetContext(ctx, "old-session")
	if err != ErrWorkerClosed {
		t.Errorf("ResetContext on closed worker: error = %v; want ErrWorkerClosed", err)
	}
}

func TestWorker_StopGraceful_ExitsOnSIGTERM(t *testing.T) {
	ctx := context.Background()

	worker, _ := newMockACPScript().
		withSignalHandler(signalHandlerExitOnSIGTERM).
		withAfterHandshake(afterHandshakeSleep).
		spawnWorker(t, ctx)

	// Worker should be alive
	if !worker.Alive() {
		t.Fatal("worker should be alive after spawn")
	}

	// Stop gracefully with short grace period
	err := worker.StopGraceful(ctx, 1*time.Second)
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
	ctx := context.Background()

	worker, _ := newMockACPScript().
		withSignalHandler(signalHandlerIgnoreSIGTERM).
		withAfterHandshake(afterHandshakeSleep).
		spawnWorker(t, ctx)

	pid := worker.Pid()

	// Worker should be alive
	if !worker.Alive() {
		t.Fatal("worker should be alive after spawn")
	}

	// Stop gracefully with short grace period (will need SIGKILL)
	start := time.Now()
	err := worker.StopGraceful(ctx, 500*time.Millisecond)
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
	select {
	case <-worker.Done():
		t.Logf("Worker (pid %d) successfully terminated", pid)
	default:
		t.Error("Done channel should be closed after StopGraceful with SIGKILL")
	}
}

func TestWorker_StopGraceful_Idempotent(t *testing.T) {
	ctx := context.Background()

	worker, _ := newMockACPScript().
		withAfterHandshake(afterHandshakeWait).
		spawnWorker(t, ctx)

	// First stop
	err := worker.StopGraceful(ctx, 1*time.Second)
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
	ctx := context.Background()

	worker, _ := newMockACPScript().
		withSignalHandler(signalHandlerExitOnSIGTERM).
		withAfterHandshake(afterHandshakeSleep).
		spawnWorker(t, ctx)

	// Pass 0 grace period - should use DefaultGracePeriod (3s)
	err := worker.StopGraceful(ctx, 0)
	if err != nil {
		t.Fatalf("StopGraceful with 0 grace: %v", err)
	}

	if worker.Alive() {
		t.Error("worker should not be alive after StopGraceful")
	}
}
