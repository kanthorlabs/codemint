// Package agent defines agent interfaces and implementations for CodeMint.
// This file implements the SystemAssistant interface for handling non-project
// freeform conversational queries (Story 3.19).
package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"sync"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/domain"
)

// ErrProviderBinaryMissing is returned when the provider's binary cannot be found.
var ErrProviderBinaryMissing = errors.New("agent: provider binary not found")

// ErrAssistantNotConfigured is returned when the system assistant is not configured.
var ErrAssistantNotConfigured = errors.New("agent: system assistant not configured")

// ChatChunk represents a streaming chunk from the assistant's response.
type ChatChunk struct {
	// Text is the content of this chunk.
	Text string
	// Done indicates this is the final chunk (turn complete).
	Done bool
	// Err contains any error that occurred during streaming.
	Err error
}

// Note: Provider type is now defined in provider.go with full capabilities.

// AssistantSession provides the minimal session info needed by the assistant.
// This avoids an import cycle with orchestrator.ActiveSession.
type AssistantSession struct {
	// Session is the domain session (may be nil for global mode).
	Session *domain.Session
	// Project is the domain project (may be nil for non-project queries).
	Project *domain.Project
	// IsGlobal indicates whether this is a global (non-project) session.
	IsGlobal bool
}

// WorkerAttacher is an interface for attaching ACP workers to sessions.
// This is implemented by orchestrator.Runtime to avoid import cycles.
type WorkerAttacher interface {
	// AttachWorker spawns or retrieves an ACP worker for the given session and project.
	AttachWorker(ctx context.Context, sess *domain.Session, project *domain.Project) (*acp.Worker, error)
}

// SystemAssistant handles freeform conversational queries in non-project sessions.
// It provides a Q&A interface without tool calls or project context.
type SystemAssistant interface {
	// Ask sends a text query to the assistant and returns a channel for streaming responses.
	// The channel will receive ChatChunk values until Done=true or an error occurs.
	Ask(ctx context.Context, sess AssistantSession, text string) (<-chan ChatChunk, error)
	// AgentID returns the unique identifier for this assistant.
	AgentID() string
	// Provider returns the resolved provider for auditing and status reporting.
	Provider() *Provider
}

// acpAssistant implements SystemAssistant using the ACP worker plumbing.
// It delegates actual LLM calls to the per-session ACP worker.
type acpAssistant struct {
	attacher WorkerAttacher
	provider *Provider
	agentID  string
	logger   *slog.Logger

	// mu protects session-to-worker mapping.
	mu sync.RWMutex
	// workers tracks active workers per session for cleanup.
	workers map[string]*acp.Worker
}

// ACPAssistantConfig holds configuration for creating an ACP-backed assistant.
type ACPAssistantConfig struct {
	// Attacher is the worker attacher for spawning workers (required).
	Attacher WorkerAttacher
	// Provider is the resolved AI provider (required).
	Provider *Provider
	// Logger is the logger instance (optional, defaults to slog.Default()).
	Logger *slog.Logger
}

// NewACPAssistant creates a new SystemAssistant backed by the ACP worker.
// It validates that the provider's binary exists before returning.
func NewACPAssistant(cfg ACPAssistantConfig) (SystemAssistant, error) {
	if cfg.Attacher == nil {
		return nil, errors.New("agent: attacher is required")
	}
	if cfg.Provider == nil {
		return nil, errors.New("agent: provider is required")
	}

	// Check if the provider binary is available in PATH.
	if _, err := exec.LookPath(cfg.Provider.Command); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrProviderBinaryMissing, cfg.Provider.Command)
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &acpAssistant{
		attacher: cfg.Attacher,
		provider: cfg.Provider,
		agentID:  "system-assistant",
		logger:   logger,
		workers:  make(map[string]*acp.Worker),
	}, nil
}

// systemPrompt is the preamble sent to the assistant on first interaction.
const systemPrompt = `You are CodeMint's System Assistant. Answer general questions about CodeMint, this conversation, or programming. You have no project context. Be concise.`

// Ask implements SystemAssistant.Ask.
// It spawns or retrieves an ACP worker for the session and sends the prompt.
func (a *acpAssistant) Ask(ctx context.Context, sess AssistantSession, text string) (<-chan ChatChunk, error) {
	// Create a result channel for streaming chunks.
	ch := make(chan ChatChunk, 16)

	// For global sessions without a project, we work from user's home directory.
	var session *domain.Session
	if sess.Session != nil {
		session = sess.Session
	} else {
		// Create a temporary session for global mode.
		session = &domain.Session{
			ID:     "global-assistant",
			Status: domain.SessionStatusActive,
		}
	}

	// Attach worker (project can be nil for global mode).
	worker, err := a.attacher.AttachWorker(ctx, session, sess.Project)
	if err != nil {
		close(ch)
		return ch, fmt.Errorf("agent: attach worker: %w", err)
	}

	// Track the worker for cleanup.
	a.mu.Lock()
	a.workers[session.ID] = worker
	a.mu.Unlock()

	// Build the prompt request.
	// Note: The system prompt is injected during worker creation via Registry.GetOrSpawn.
	// Here we only send the user's prompt.
	prompt := acp.SessionPromptParams{
		SessionID: worker.ACPSessionID(),
		Prompt:    text,
	}

	// Create the JSON-RPC request message.
	msg, err := acp.NewRequest(acp.MethodSessionPrompt, prompt)
	if err != nil {
		close(ch)
		return ch, fmt.Errorf("agent: create prompt request: %w", err)
	}

	// Send the prompt to the worker.
	if err := worker.Send(msg); err != nil {
		close(ch)
		return ch, fmt.Errorf("agent: send prompt: %w", err)
	}

	// Start consuming events from the worker's output channel.
	go a.consumeEvents(ctx, worker, ch)

	return ch, nil
}

// consumeEvents reads events from the worker and sends ChatChunks to the result channel.
func (a *acpAssistant) consumeEvents(ctx context.Context, worker *acp.Worker, ch chan<- ChatChunk) {
	defer close(ch)

	out := worker.Out()
	for {
		select {
		case <-ctx.Done():
			ch <- ChatChunk{Err: ctx.Err()}
			return
		case msg, ok := <-out:
			if !ok {
				// Worker closed.
				ch <- ChatChunk{Done: true}
				return
			}

			// Classify the event.
			classified := acp.Classify(msg)

			switch classified.Kind {
			case acp.EventMessage:
				// Extract text content from the chunk.
				content := acp.ExtractMessageContent(classified)
				if content != "" {
					ch <- ChatChunk{Text: content}
				}

			case acp.EventTurnEnd:
				// Turn complete - signal done.
				ch <- ChatChunk{Done: true}
				return

			case acp.EventTurnStart, acp.EventThinking, acp.EventPlan:
				// Ignore these events for chat responses.
				continue

			case acp.EventToolCall, acp.EventToolUpdate, acp.EventPermissionRequest:
				// System assistant should not make tool calls.
				// Log and ignore.
				a.logger.Warn("system assistant: unexpected tool event",
					"kind", classified.Kind,
					"tool", classified.ToolName,
				)
				continue

			default:
				// Log unknown event types for debugging.
				a.logger.Debug("system assistant: ignoring event",
					"kind", classified.Kind,
					"raw", string(classified.Raw),
				)
			}
		}
	}
}

// AgentID implements SystemAssistant.AgentID.
func (a *acpAssistant) AgentID() string {
	return a.agentID
}

// Provider implements SystemAssistant.Provider.
func (a *acpAssistant) Provider() *Provider {
	return a.provider
}

// nullAssistant is a test double that returns canned responses without spawning workers.
type nullAssistant struct {
	provider *Provider
	response string
}

// NewNullAssistant creates a test assistant that returns a canned response.
func NewNullAssistant(response string) SystemAssistant {
	return &nullAssistant{
		provider: &Provider{Name: "null", Command: "/dev/null"},
		response: response,
	}
}

// Ask implements SystemAssistant.Ask for testing.
func (n *nullAssistant) Ask(ctx context.Context, sess AssistantSession, text string) (<-chan ChatChunk, error) {
	ch := make(chan ChatChunk, 2)

	go func() {
		defer close(ch)

		// Simulate streaming response.
		ch <- ChatChunk{Text: n.response}
		ch <- ChatChunk{Done: true}
	}()

	return ch, nil
}

// AgentID implements SystemAssistant.AgentID.
func (n *nullAssistant) AgentID() string {
	return "null-assistant"
}

// Provider implements SystemAssistant.Provider.
func (n *nullAssistant) Provider() *Provider {
	return n.provider
}
