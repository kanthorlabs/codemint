package acp

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"codemint.kanthorlabs.com/internal/domain"
)

// Environment variable to override the ACP command.
const EnvACPCommand = "CODEMINT_ACP_CMD"

// Default timeout for stopping workers.
const DefaultStopTimeout = 5 * time.Second

// Registry maintains a 1:1 mapping between active sessions and ACP workers.
// It is safe for concurrent use.
type Registry struct {
	mu      sync.RWMutex
	workers map[string]*Worker // key = session.ID
	cfg     WorkerConfig
}

// NewRegistry creates a new worker registry with the given configuration.
func NewRegistry(cfg WorkerConfig) *Registry {
	// Check for environment override
	if cmd := os.Getenv(EnvACPCommand); cmd != "" {
		cfg.Command = cmd
		cfg.Args = nil // Reset args when using custom command
	}

	return &Registry{
		workers: make(map[string]*Worker),
		cfg:     cfg,
	}
}

// GetOrSpawn returns an existing worker for the session, or spawns a new one.
// It is idempotent: concurrent calls for the same session return the same worker.
func (r *Registry) GetOrSpawn(ctx context.Context, sess *domain.Session, project *domain.Project) (*Worker, error) {
	if sess == nil {
		return nil, fmt.Errorf("acp: session is nil")
	}

	// Fast path: check if worker exists
	r.mu.RLock()
	if worker, ok := r.workers[sess.ID]; ok {
		if worker.Alive() {
			r.mu.RUnlock()
			return worker, nil
		}
	}
	r.mu.RUnlock()

	// Slow path: need to spawn
	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock
	if worker, ok := r.workers[sess.ID]; ok {
		if worker.Alive() {
			return worker, nil
		}
		// Worker died, clean up
		delete(r.workers, sess.ID)
	}

	// Build config for this session
	cfg := r.cfg
	if project != nil && project.WorkingDir != "" {
		cfg.Cwd = project.WorkingDir
	}

	// Load and inject project memory (Story 3.11)
	if project != nil && project.ID != "" {
		systemPrompt, err := BuildSystemPromptFromProjectID(project.ID)
		if err != nil {
			slog.Warn("acp: failed to load project memory",
				"projectID", project.ID,
				"error", err,
			)
			// Continue with empty system prompt - memory is optional
		} else {
			cfg.SystemPrompt = systemPrompt
		}
	}

	// Spawn the worker
	worker, err := Spawn(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// Store in registry
	r.workers[sess.ID] = worker

	slog.Info("acp: worker spawned",
		"sessionID", sess.ID,
		"pid", worker.Pid(),
		"cwd", worker.Cwd(),
		"hasMemory", cfg.SystemPrompt != "",
	)

	// Start cleanup goroutine for when worker exits
	go r.watchWorker(sess.ID, worker)

	return worker, nil
}

// Get returns the worker for the given session ID, if it exists and is alive.
func (r *Registry) Get(sessionID string) (*Worker, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	worker, ok := r.workers[sessionID]
	if !ok {
		return nil, false
	}
	if !worker.Alive() {
		return nil, false
	}
	return worker, true
}

// Stop stops the worker for the given session ID and removes it from the registry.
func (r *Registry) Stop(ctx context.Context, sessionID string) error {
	r.mu.Lock()
	worker, ok := r.workers[sessionID]
	if !ok {
		r.mu.Unlock()
		return nil
	}
	delete(r.workers, sessionID)
	r.mu.Unlock()

	return r.stopWorker(ctx, worker)
}

// StopAll stops all workers in the registry.
func (r *Registry) StopAll(ctx context.Context) error {
	r.mu.Lock()
	workers := make([]*Worker, 0, len(r.workers))
	for _, w := range r.workers {
		workers = append(workers, w)
	}
	r.workers = make(map[string]*Worker)
	r.mu.Unlock()

	var firstErr error
	for _, worker := range workers {
		if err := r.stopWorker(ctx, worker); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

// Count returns the number of active workers.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, w := range r.workers {
		if w.Alive() {
			count++
		}
	}
	return count
}

// WorkerStatus contains information about a worker's state.
type WorkerStatus struct {
	PID       int
	State     string // "running", "stopped"
	Cwd       string
	StartedAt time.Time // Approximate start time (when first observed).
}

// Status returns status information about the worker for the given session ID.
// Returns nil if no worker exists for the session.
func (r *Registry) Status(sessionID string) *WorkerStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	worker, ok := r.workers[sessionID]
	if !ok {
		return nil
	}

	state := "stopped"
	if worker.Alive() {
		state = "running"
	}

	return &WorkerStatus{
		PID:   worker.Pid(),
		State: state,
		Cwd:   worker.Cwd(),
		// Note: Worker doesn't track start time, so we leave this zero.
		// A future enhancement could add start time tracking to Worker.
	}
}

// stopWorker gracefully stops a worker using two-phase shutdown.
func (r *Registry) stopWorker(ctx context.Context, worker *Worker) error {
	return worker.StopGraceful(ctx, DefaultStopTimeout)
}

// watchWorker monitors a worker and removes it from the registry when it exits.
func (r *Registry) watchWorker(sessionID string, worker *Worker) {
	<-worker.Done()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Only remove if this is still the worker for this session
	if existing, ok := r.workers[sessionID]; ok && existing == worker {
		delete(r.workers, sessionID)
		slog.Info("acp: worker exited", "sessionID", sessionID, "pid", worker.Pid())
	}
}
