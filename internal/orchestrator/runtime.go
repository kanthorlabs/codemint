// Package orchestrator coordinates task execution, session management, and
// command dispatching for CodeMint.
package orchestrator

import (
	"context"
	"log/slog"
	"sync"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/registry"
	"codemint.kanthorlabs.com/internal/repository"
)

// Runtime wires together the ACP Pipeline, Interceptor, StatusMapper, Fanout,
// BufferRegistry, and PipelineConsumer into a cohesive event processing system.
// It is the central point for attaching/detaching ACP workers to sessions.
//
// This implements Task 3.12.1: Runtime Wiring Helper.
type Runtime struct {
	// Core components.
	registry       *acp.Registry
	bufferRegistry *acp.BufferRegistry

	// Repositories for data access.
	permissionRepo repository.ProjectPermissionRepository
	taskRepo       repository.TaskRepository
	sessionRepo    repository.SessionRepository
	agentRepo      repository.AgentRepository

	// UI integration.
	mediator registry.UIMediator

	// Logger for runtime operations.
	logger *slog.Logger

	// consumers tracks active pipeline consumers per session.
	// Key: sessionID, Value: consumer context cancel function.
	consumers   map[string]context.CancelFunc
	consumersMu sync.RWMutex

	// interceptors tracks active interceptors per session.
	// Key: sessionID, Value: *Interceptor.
	interceptors   map[string]*Interceptor
	interceptorsMu sync.RWMutex

	// pipelines tracks active pipelines per session.
	// Key: sessionID, Value: *acp.Pipeline.
	pipelines   map[string]*acp.Pipeline
	pipelinesMu sync.RWMutex

	// statusMappers tracks active status mappers per session.
	// Key: sessionID, Value: *StatusMapper.
	statusMappers   map[string]*StatusMapper
	statusMappersMu sync.RWMutex

	// advanceCh is used by StatusMapper to signal task advancement.
	advanceCh chan struct{}
}

// RuntimeConfig holds the configuration for creating a Runtime.
type RuntimeConfig struct {
	// Registry is the ACP worker registry (required).
	Registry *acp.Registry
	// BufferRegistry stores per-task event buffers for /summary (required).
	BufferRegistry *acp.BufferRegistry
	// Mediator is the UI mediator for event broadcasting (required).
	Mediator registry.UIMediator
	// PermissionRepo is the project permission repository (required).
	PermissionRepo repository.ProjectPermissionRepository
	// TaskRepo is the task repository for status updates (required).
	TaskRepo repository.TaskRepository
	// SessionRepo is the session repository (required).
	SessionRepo repository.SessionRepository
	// AgentRepo is the agent repository (required).
	AgentRepo repository.AgentRepository
	// Logger is the logger for runtime operations (optional, defaults to slog.Default()).
	Logger *slog.Logger
	// AdvanceCh is used by StatusMapper to signal task advancement (optional).
	AdvanceCh chan struct{}
}

// NewRuntime creates a new Runtime with the provided configuration.
// All required fields in RuntimeConfig must be non-nil.
func NewRuntime(cfg RuntimeConfig) *Runtime {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Runtime{
		registry:       cfg.Registry,
		bufferRegistry: cfg.BufferRegistry,
		permissionRepo: cfg.PermissionRepo,
		taskRepo:       cfg.TaskRepo,
		sessionRepo:    cfg.SessionRepo,
		agentRepo:      cfg.AgentRepo,
		mediator:       cfg.Mediator,
		logger:         logger,
		consumers:      make(map[string]context.CancelFunc),
		interceptors:   make(map[string]*Interceptor),
		pipelines:      make(map[string]*acp.Pipeline),
		statusMappers:  make(map[string]*StatusMapper),
		advanceCh:      cfg.AdvanceCh,
	}
}

// Registry returns the underlying ACP worker registry.
func (rt *Runtime) Registry() *acp.Registry {
	return rt.registry
}

// BufferRegistry returns the event buffer registry for /summary.
func (rt *Runtime) BufferRegistry() *acp.BufferRegistry {
	return rt.bufferRegistry
}

// AttachWorker spawns (or retrieves) an ACP worker for the given session and project,
// builds the per-session Pipeline, loads project permissions into the Interceptor,
// and starts the PipelineConsumer goroutine.
//
// This is the main entry point for connecting a session to the ACP event flow.
// Returns the worker and any error that occurred during setup.
func (rt *Runtime) AttachWorker(ctx context.Context, sess *domain.Session, project *domain.Project) (*acp.Worker, error) {
	if sess == nil {
		return nil, nil
	}

	sessionID := sess.ID

	// Get or spawn the worker.
	worker, err := rt.registry.GetOrSpawn(ctx, sess, project)
	if err != nil {
		return nil, err
	}

	// Check if we already have a consumer running for this session.
	rt.consumersMu.RLock()
	_, exists := rt.consumers[sessionID]
	rt.consumersMu.RUnlock()
	if exists {
		// Already attached, return the worker.
		return worker, nil
	}

	// Load project permissions into the interceptor.
	var permissions *domain.ProjectPermission
	if project != nil && rt.permissionRepo != nil {
		permissions, err = rt.permissionRepo.FindByProjectID(ctx, project.ID)
		if err != nil {
			rt.logger.Warn("runtime: failed to load project permissions",
				"project_id", project.ID,
				"error", err,
			)
			// Continue without permissions (permissive by default).
		}
	}

	// Determine working directory.
	workingDir := ""
	projectID := ""
	if project != nil {
		workingDir = project.WorkingDir
		projectID = project.ID
	}

	// Create the interceptor for this session.
	interceptor := NewInterceptor(InterceptorConfig{
		PermRepo:   rt.permissionRepo,
		TaskRepo:   rt.taskRepo,
		AgentRepo:  rt.agentRepo,
		UI:         rt.mediator,
		Worker:     worker,
		Logger:     rt.logger,
		ProjectID:  projectID,
		WorkingDir: workingDir,
	})

	// Store the interceptor.
	rt.interceptorsMu.Lock()
	rt.interceptors[sessionID] = interceptor
	rt.interceptorsMu.Unlock()

	// Create the status mapper for this session.
	statusMapper := NewStatusMapper(StatusMapperConfig{
		TaskRepo:  rt.taskRepo,
		UI:        rt.mediator,
		Logger:    rt.logger,
		AdvanceCh: rt.advanceCh,
	})

	// Store the status mapper.
	rt.statusMappersMu.Lock()
	rt.statusMappers[sessionID] = statusMapper
	rt.statusMappersMu.Unlock()

	// Create the fanout for this session.
	fanout := NewFanout(FanoutConfig{
		UI:     rt.mediator,
		Logger: rt.logger,
	})

	// Create the pipeline from the worker's output channel.
	pipeline := acp.NewPipeline(worker.Out(), acp.PipelineConfig{
		BufferSize: acp.DefaultPipelineBufferSize,
		Logger:     rt.logger,
	})

	// Store the pipeline.
	rt.pipelinesMu.Lock()
	rt.pipelines[sessionID] = pipeline
	rt.pipelinesMu.Unlock()

	// Create the pipeline consumer.
	consumer := NewPipelineConsumer(PipelineConsumerConfig{
		Mapper:         statusMapper,
		Interceptor:    interceptor,
		Fanout:         fanout,
		Worker:         worker,
		BufferRegistry: rt.bufferRegistry,
		Logger:         rt.logger,
	})

	// Create a cancellable context for the consumer.
	consumerCtx, cancel := context.WithCancel(ctx)

	// Store the cancel function.
	rt.consumersMu.Lock()
	rt.consumers[sessionID] = cancel
	rt.consumersMu.Unlock()

	// Start the pipeline and consumer goroutines.
	go func() {
		defer func() {
			// Cleanup when the goroutine exits.
			rt.cleanupSession(sessionID)
		}()

		// Run the pipeline (routes events to Events and Halted channels).
		go pipeline.Run(consumerCtx)

		// Run the consumer (consumes from both channels).
		consumer.Run(consumerCtx, pipeline, sessionID)
	}()

	rt.logger.Info("runtime: attached worker and started consumer",
		"session_id", sessionID,
		"project_id", projectID,
		"has_permissions", permissions != nil,
	)

	return worker, nil
}

// DetachSession stops the pipeline consumer for the given session and cleans up resources.
// This should be called when archiving or shutting down a session.
func (rt *Runtime) DetachSession(sessionID string) {
	rt.consumersMu.Lock()
	cancel, exists := rt.consumers[sessionID]
	if exists {
		delete(rt.consumers, sessionID)
	}
	rt.consumersMu.Unlock()

	if exists && cancel != nil {
		cancel()
	}

	rt.cleanupSession(sessionID)

	rt.logger.Info("runtime: detached session", "session_id", sessionID)
}

// cleanupSession removes all resources associated with a session.
func (rt *Runtime) cleanupSession(sessionID string) {
	// Cancel pending approvals in the interceptor.
	rt.interceptorsMu.Lock()
	if interceptor, ok := rt.interceptors[sessionID]; ok {
		interceptor.CancelPendingApprovals()
		delete(rt.interceptors, sessionID)
	}
	rt.interceptorsMu.Unlock()

	// Remove the pipeline.
	rt.pipelinesMu.Lock()
	delete(rt.pipelines, sessionID)
	rt.pipelinesMu.Unlock()

	// Remove the status mapper.
	rt.statusMappersMu.Lock()
	delete(rt.statusMappers, sessionID)
	rt.statusMappersMu.Unlock()

	// Drop buffers for this session.
	if rt.bufferRegistry != nil {
		rt.bufferRegistry.DropSession(sessionID)
	}
}

// RefreshPermissions reloads project permissions for the given project and
// updates all sessions that are using this project.
// This is called when /permission-allow or /permission-block modifies permissions.
func (rt *Runtime) RefreshPermissions(ctx context.Context, projectID string) error {
	if rt.permissionRepo == nil {
		return nil
	}

	// Load the updated permissions.
	_, err := rt.permissionRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		rt.logger.Warn("runtime: failed to refresh permissions",
			"project_id", projectID,
			"error", err,
		)
		return err
	}

	// Note: The interceptor evaluates permissions on each command via evaluateCommand,
	// which calls permRepo.FindByProjectID(). So we don't need to push permissions
	// to interceptors - they'll pick up the changes on the next evaluation.

	rt.logger.Info("runtime: permissions refreshed", "project_id", projectID)
	return nil
}

// ConsumerCount returns the number of active pipeline consumers.
// Useful for testing and monitoring.
func (rt *Runtime) ConsumerCount() int {
	rt.consumersMu.RLock()
	defer rt.consumersMu.RUnlock()
	return len(rt.consumers)
}

// GetInterceptor returns the interceptor for a session, if any.
func (rt *Runtime) GetInterceptor(sessionID string) (*Interceptor, bool) {
	rt.interceptorsMu.RLock()
	defer rt.interceptorsMu.RUnlock()
	interceptor, ok := rt.interceptors[sessionID]
	return interceptor, ok
}

// GetStatusMapper returns the status mapper for a session, if any.
func (rt *Runtime) GetStatusMapper(sessionID string) (*StatusMapper, bool) {
	rt.statusMappersMu.RLock()
	defer rt.statusMappersMu.RUnlock()
	mapper, ok := rt.statusMappers[sessionID]
	return mapper, ok
}

// SetCurrentTask sets the current task ID for the worker associated with a session.
// This is called by the scheduler before sending a prompt to the ACP agent.
func (rt *Runtime) SetCurrentTask(sessionID, taskID string) {
	worker, ok := rt.registry.Get(sessionID)
	if ok && worker != nil {
		worker.SetCurrentTask(taskID)
	}

	// Clear the idempotency tracking in the status mapper for any previous task.
	rt.statusMappersMu.RLock()
	mapper, ok := rt.statusMappers[sessionID]
	rt.statusMappersMu.RUnlock()
	if ok && mapper != nil {
		mapper.ClearTask(taskID)
	}
}

// Shutdown stops all consumers and workers gracefully.
// This should be called when the application is shutting down.
func (rt *Runtime) Shutdown(ctx context.Context) error {
	// Cancel all consumers.
	rt.consumersMu.Lock()
	for sessionID, cancel := range rt.consumers {
		if cancel != nil {
			cancel()
		}
		delete(rt.consumers, sessionID)
	}
	rt.consumersMu.Unlock()

	// Cleanup all interceptors.
	rt.interceptorsMu.Lock()
	for sessionID, interceptor := range rt.interceptors {
		interceptor.CancelPendingApprovals()
		delete(rt.interceptors, sessionID)
	}
	rt.interceptorsMu.Unlock()

	// Clear all pipelines.
	rt.pipelinesMu.Lock()
	for sessionID := range rt.pipelines {
		delete(rt.pipelines, sessionID)
	}
	rt.pipelinesMu.Unlock()

	// Clear all status mappers.
	rt.statusMappersMu.Lock()
	for sessionID := range rt.statusMappers {
		delete(rt.statusMappers, sessionID)
	}
	rt.statusMappersMu.Unlock()

	// Stop all workers in the registry.
	if rt.registry != nil {
		return rt.registry.StopAll(ctx)
	}

	return nil
}
