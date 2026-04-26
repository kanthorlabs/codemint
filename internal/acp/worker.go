package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// Default configuration values.
const (
	DefaultCommand          = "opencode"
	DefaultHandshakeTimeout = 10 * time.Second
	DefaultOutChannelSize   = 256
	DefaultGracePeriod      = 3 * time.Second
)

// ErrHandshakeTimeout is returned when the initialize handshake times out.
var ErrHandshakeTimeout = errors.New("acp: initialize handshake timeout")

// ErrWorkerExited is returned when attempting to send to a stopped worker.
var ErrWorkerExited = errors.New("acp: worker process exited")

// ErrWorkerNotStarted is returned when the worker failed to start.
var ErrWorkerNotStarted = errors.New("acp: worker not started")

// ErrWorkerClosed is returned when attempting to reset context on a closed worker.
var ErrWorkerClosed = errors.New("acp: worker is closed")

// WorkerConfig configures the ACP worker process.
type WorkerConfig struct {
	Command          string        // Executable name or path (default: "opencode")
	Args             []string      // Arguments (default: ["acp"])
	Cwd              string        // Working directory for the process
	Env              []string      // Additional environment variables (appended to os.Environ())
	HandshakeTimeout time.Duration // Timeout for initialize handshake (default: 10s)
	SystemPrompt     string        // System prompt with memory injection for the session
}

// DefaultConfig returns a WorkerConfig with default values.
func DefaultConfig() WorkerConfig {
	return WorkerConfig{
		Command:          DefaultCommand,
		Args:             []string{"acp"},
		HandshakeTimeout: DefaultHandshakeTimeout,
	}
}

// Worker manages a single ACP-compatible CLI process.
type Worker struct {
	cfg    WorkerConfig
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	// Communication channels
	out      chan Message
	done     chan struct{}
	exitErr  error
	exitOnce sync.Once

	// Write synchronization
	writeMu sync.Mutex

	// Pending requests waiting for responses
	pending   map[int64]chan Message
	pendingMu sync.Mutex

	// Capabilities from initialize handshake
	capabilities InitializeResult

	// acpSessionID is the current ACP session ID.
	// Updated after session/new calls.
	acpSessionID   string
	acpSessionIDMu sync.RWMutex

	// currentTaskID is the ID of the task currently being processed by this worker.
	// Empty string for ad-hoc /acp prompts. Used by StatusMapper (Story 3.7).
	currentTaskID   string
	currentTaskIDMu sync.RWMutex
}

// Spawn creates and starts a new ACP worker process.
// It performs the initialize handshake before returning.
func Spawn(ctx context.Context, cfg WorkerConfig) (*Worker, error) {
	// Apply defaults
	if cfg.Command == "" {
		cfg.Command = DefaultCommand
	}
	if len(cfg.Args) == 0 {
		cfg.Args = []string{"acp"}
	}
	if cfg.HandshakeTimeout == 0 {
		cfg.HandshakeTimeout = DefaultHandshakeTimeout
	}

	// Find the executable
	cmdPath, err := exec.LookPath(cfg.Command)
	if err != nil {
		return nil, fmt.Errorf("acp: command not found: %s: %w", cfg.Command, err)
	}

	// Create command
	cmd := exec.CommandContext(ctx, cmdPath, cfg.Args...)
	if cfg.Cwd != "" {
		cmd.Dir = cfg.Cwd
	}
	if len(cfg.Env) > 0 {
		cmd.Env = append(cmd.Environ(), cfg.Env...)
	}

	// Set up pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("acp: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("acp: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		stdout.Close()
		return nil, fmt.Errorf("acp: stderr pipe: %w", err)
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		stderr.Close()
		return nil, fmt.Errorf("acp: start process: %w", err)
	}

	w := &Worker{
		cfg:     cfg,
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		out:     make(chan Message, DefaultOutChannelSize),
		done:    make(chan struct{}),
		pending: make(map[int64]chan Message),
	}

	// Start reader goroutines
	go w.readStdout()
	go w.readStderr()
	go w.waitProcess()

	// Perform initialize handshake
	if err := w.initialize(ctx); err != nil {
		w.Stop()
		return nil, err
	}

	// Create initial session with system prompt (memory injection)
	sessionID, err := w.createSession(ctx, cfg.SystemPrompt)
	if err != nil {
		w.Stop()
		return nil, fmt.Errorf("acp: create initial session: %w", err)
	}
	w.setACPSessionID(sessionID)

	return w, nil
}

// Out returns a read-only channel for receiving messages from the worker.
// The channel is closed when the worker process exits.
func (w *Worker) Out() <-chan Message {
	return w.out
}

// Send sends a message to the worker's stdin.
// It is safe for concurrent use.
func (w *Worker) Send(msg *Message) error {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()

	select {
	case <-w.done:
		return ErrWorkerExited
	default:
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("acp: marshal message: %w", err)
	}

	// Write JSON + newline
	data = append(data, '\n')
	if _, err := w.stdin.Write(data); err != nil {
		return fmt.Errorf("acp: write stdin: %w", err)
	}

	return nil
}

// SendRequest sends a request and waits for the corresponding response.
// It blocks until a response with matching ID is received or context is cancelled.
func (w *Worker) SendRequest(ctx context.Context, msg *Message) (*Message, error) {
	if !msg.IsRequest() {
		return nil, fmt.Errorf("acp: message is not a request")
	}

	id := msg.GetID()
	if id == 0 {
		return nil, fmt.Errorf("acp: request has invalid ID")
	}

	// Register pending request
	respCh := make(chan Message, 1)
	w.pendingMu.Lock()
	w.pending[id] = respCh
	w.pendingMu.Unlock()

	defer func() {
		w.pendingMu.Lock()
		delete(w.pending, id)
		w.pendingMu.Unlock()
	}()

	// Send the request
	if err := w.Send(msg); err != nil {
		return nil, err
	}

	// Wait for response
	select {
	case resp := <-respCh:
		return &resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-w.done:
		return nil, ErrWorkerExited
	}
}

// Pid returns the process ID of the worker.
func (w *Worker) Pid() int {
	if w.cmd.Process != nil {
		return w.cmd.Process.Pid
	}
	return 0
}

// Cwd returns the working directory of the worker.
func (w *Worker) Cwd() string {
	return w.cfg.Cwd
}

// Capabilities returns the server capabilities from the initialize handshake.
func (w *Worker) Capabilities() InitializeResult {
	return w.capabilities
}

// SetCurrentTask sets the ID of the task currently being processed by this worker.
// Pass empty string when the task is completed or for ad-hoc /acp prompts.
// This is used by StatusMapper (Story 3.7) to associate ACP events with tasks.
func (w *Worker) SetCurrentTask(taskID string) {
	w.currentTaskIDMu.Lock()
	defer w.currentTaskIDMu.Unlock()
	w.currentTaskID = taskID
}

// CurrentTaskID returns the ID of the task currently being processed.
// Returns empty string if no task is being processed (ad-hoc /acp prompts).
func (w *Worker) CurrentTaskID() string {
	w.currentTaskIDMu.RLock()
	defer w.currentTaskIDMu.RUnlock()
	return w.currentTaskID
}

// Wait blocks until the worker process exits and returns the exit error.
func (w *Worker) Wait() error {
	<-w.done
	return w.exitErr
}

// Done returns a channel that is closed when the worker exits.
func (w *Worker) Done() <-chan struct{} {
	return w.done
}

// Stop gracefully stops the worker by closing stdin.
// The process should exit on its own when stdin is closed.
// Deprecated: Use StopGraceful for proper two-phase shutdown.
func (w *Worker) Stop() {
	w.stdin.Close()
}

// StopGraceful implements a two-phase shutdown sequence:
// 1. If the agent advertises a graceful exit method (shutdown), send it and wait.
// 2. Otherwise (or after grace expires), send SIGTERM.
// 3. Wait up to grace again. If still running, send SIGKILL.
// 4. Close stdin and drain Wait().
//
// If grace is 0, DefaultGracePeriod (3s) is used.
// StopGraceful is idempotent: calling it multiple times is safe.
func (w *Worker) StopGraceful(ctx context.Context, grace time.Duration) error {
	// Check if already stopped
	select {
	case <-w.done:
		return nil
	default:
	}

	if grace == 0 {
		grace = DefaultGracePeriod
	}

	pid := w.Pid()
	slog.Debug("acp: stopping worker", "pid", pid, "grace", grace)

	// Phase 1: Try graceful shutdown via protocol (if agent supports it)
	// Use a short timeout for the shutdown request
	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, grace)
	defer shutdownCancel()

	if w.tryGracefulShutdown(shutdownCtx) {
		// Wait for process to exit
		select {
		case <-w.done:
			slog.Info("acp: worker stopped gracefully via shutdown", "pid", pid)
			return nil
		case <-shutdownCtx.Done():
			// Timeout, proceed to SIGTERM
		}
	}

	// Phase 2: Send SIGTERM
	if err := w.sendSignal(syscall.SIGTERM); err != nil {
		slog.Debug("acp: failed to send SIGTERM", "pid", pid, "error", err)
	} else {
		slog.Debug("acp: sent SIGTERM", "pid", pid)
	}

	// Wait for process to exit after SIGTERM
	select {
	case <-w.done:
		slog.Info("acp: worker stopped after SIGTERM", "pid", pid)
		return nil
	case <-time.After(grace):
		// Timeout, proceed to SIGKILL
	case <-ctx.Done():
		// Context cancelled, proceed to SIGKILL
	}

	// Phase 3: Send SIGKILL
	slog.Warn("acp: sending SIGKILL after grace period", "pid", pid)
	if err := w.Kill(); err != nil {
		return fmt.Errorf("acp: kill worker: %w", err)
	}

	// Wait for process to fully exit
	<-w.done

	slog.Info("acp: worker killed", "pid", pid)
	return nil
}

// tryGracefulShutdown attempts to send a shutdown request to the agent.
// Returns true if the request was sent successfully.
func (w *Worker) tryGracefulShutdown(ctx context.Context) bool {
	// Check if agent supports shutdown capability
	// Currently we attempt shutdown regardless, as some agents may support it
	// without explicitly advertising it
	req, err := NewRequest(MethodShutdown, nil)
	if err != nil {
		return false
	}

	_, err = w.SendRequest(ctx, req)
	return err == nil
}

// sendSignal sends a signal to the worker process.
func (w *Worker) sendSignal(sig syscall.Signal) error {
	if w.cmd.Process == nil {
		return ErrWorkerNotStarted
	}
	return w.cmd.Process.Signal(sig)
}

// Kill forcefully terminates the worker process.
func (w *Worker) Kill() error {
	if w.cmd.Process != nil {
		return w.cmd.Process.Kill()
	}
	return nil
}

// Alive returns true if the worker process is still running.
func (w *Worker) Alive() bool {
	select {
	case <-w.done:
		return false
	default:
		return true
	}
}

// ResetContext flushes the agent's working memory without killing the process.
// It sends a session/cancel for the current session (if supported) and creates
// a fresh ACP session via session/new. The same Cwd and system prompt are reused.
// Returns the new ACP session ID on success.
func (w *Worker) ResetContext(ctx context.Context, currentSessionID string) (string, error) {
	// Check if worker is still alive
	select {
	case <-w.done:
		return "", ErrWorkerClosed
	default:
	}

	oldSessionID := currentSessionID

	// Cancel the previous session if the agent advertises session/cancel capability
	// Note: Currently ServerCaps doesn't have a SessionCancel field, so we attempt
	// cancel and ignore errors (best effort)
	if oldSessionID != "" {
		cancelParams := SessionCancelParams{
			SessionID: oldSessionID,
		}
		cancelReq, err := NewRequest(MethodSessionCancel, cancelParams)
		if err == nil {
			// Send cancel request but don't block on response
			// Some agents may not support cancel, so we proceed regardless
			_, _ = w.SendRequest(ctx, cancelReq)
		}
	}

	// Create a new session with the same system prompt (memory re-injection)
	newSessionID, err := w.createSession(ctx, w.cfg.SystemPrompt)
	if err != nil {
		return "", err
	}

	w.setACPSessionID(newSessionID)

	slog.Debug("acp: context reset",
		"session_id", newSessionID,
		"old_acp", oldSessionID,
		"new_acp", newSessionID,
		"has_system_prompt", w.cfg.SystemPrompt != "",
	)

	return newSessionID, nil
}

// createSession creates a new ACP session with optional system prompt.
func (w *Worker) createSession(ctx context.Context, systemPrompt string) (string, error) {
	newParams := SessionNewParams{
		SystemPrompt: systemPrompt,
	}
	newReq, err := NewRequest(MethodSessionNew, newParams)
	if err != nil {
		return "", fmt.Errorf("acp: create session/new request: %w", err)
	}

	resp, err := w.SendRequest(ctx, newReq)
	if err != nil {
		return "", fmt.Errorf("acp: session/new request: %w", err)
	}

	if resp.Error != nil {
		return "", fmt.Errorf("acp: session/new failed: %w", resp.Error)
	}

	var result SessionNewResult
	if err := resp.ParseResult(&result); err != nil {
		return "", fmt.Errorf("acp: parse session/new result: %w", err)
	}

	return result.SessionID, nil
}

// ACPSessionID returns the current ACP session ID.
func (w *Worker) ACPSessionID() string {
	w.acpSessionIDMu.RLock()
	defer w.acpSessionIDMu.RUnlock()
	return w.acpSessionID
}

// setACPSessionID sets the current ACP session ID.
func (w *Worker) setACPSessionID(sessionID string) {
	w.acpSessionIDMu.Lock()
	defer w.acpSessionIDMu.Unlock()
	w.acpSessionID = sessionID
}

// SystemPrompt returns the system prompt configured for this worker.
func (w *Worker) SystemPrompt() string {
	return w.cfg.SystemPrompt
}

// initialize performs the JSON-RPC initialize handshake.
func (w *Worker) initialize(ctx context.Context) error {
	params := InitializeParams{
		ClientInfo: ClientInfo{
			Name:    "codemint",
			Version: "0.1.0",
		},
		WorkingDir: w.cfg.Cwd,
	}

	req, err := NewRequest(MethodInitialize, params)
	if err != nil {
		return fmt.Errorf("acp: create initialize request: %w", err)
	}

	// Apply handshake timeout
	ctx, cancel := context.WithTimeout(ctx, w.cfg.HandshakeTimeout)
	defer cancel()

	resp, err := w.SendRequest(ctx, req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return ErrHandshakeTimeout
		}
		return fmt.Errorf("acp: initialize handshake: %w", err)
	}

	if resp.Error != nil {
		return fmt.Errorf("acp: initialize failed: %w", resp.Error)
	}

	if err := resp.ParseResult(&w.capabilities); err != nil {
		return fmt.Errorf("acp: parse initialize result: %w", err)
	}

	slog.Debug("acp: initialized",
		"server", w.capabilities.ServerInfo.Name,
		"version", w.capabilities.ServerInfo.Version,
		"streaming", w.capabilities.Capabilities.Streaming,
		"toolCalls", w.capabilities.Capabilities.ToolCalls,
	)

	return nil
}

// readStdout reads JSON messages from the worker's stdout.
func (w *Worker) readStdout() {
	defer close(w.out)

	scanner := bufio.NewScanner(w.stdout)
	// Increase buffer size for large messages
	const maxScanTokenSize = 10 * 1024 * 1024 // 10 MB
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, maxScanTokenSize)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var msg Message
		if err := json.Unmarshal(line, &msg); err != nil {
			slog.Debug("acp: parse stdout message failed",
				"error", err,
				"line", string(line),
			)
			continue
		}

		// Check if this is a response to a pending request
		if msg.IsResponse() {
			id := msg.GetID()
			w.pendingMu.Lock()
			if ch, ok := w.pending[id]; ok {
				select {
				case ch <- msg:
				default:
				}
				w.pendingMu.Unlock()
				continue
			}
			w.pendingMu.Unlock()
		}

		// Forward to output channel
		select {
		case w.out <- msg:
		case <-w.done:
			return
		default:
			// Channel full, drop message and log
			slog.Warn("acp: output channel full, dropping message",
				"method", msg.Method,
			)
		}
	}

	if err := scanner.Err(); err != nil {
		slog.Debug("acp: stdout scanner error", "error", err)
	}
}

// readStderr reads and logs the worker's stderr.
func (w *Worker) readStderr() {
	scanner := bufio.NewScanner(w.stderr)
	for scanner.Scan() {
		slog.Debug("acp.stderr", "line", scanner.Text())
	}
}

// waitProcess waits for the worker process to exit.
func (w *Worker) waitProcess() {
	err := w.cmd.Wait()

	w.exitOnce.Do(func() {
		w.exitErr = err
		close(w.done)
	})
}
