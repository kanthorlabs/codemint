// Package orchestrator coordinates task execution, session management, and
// command dispatching for CodeMint.
package orchestrator

import (
	"sync"

	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/registry"
)

// ProjectSwitchCallback is called when the active project changes.
// The callback receives the new project (may be nil if no project is active).
type ProjectSwitchCallback func(*domain.Project)

// ActiveSession holds the runtime state for the current user session. It is
// consulted by the Dispatcher to determine how to route natural-language input
// and which commands are permitted.
//
// Project and Session are always non-nil after bootstrap — the CodeMint
// sentinel project guarantees at least one session exists. Use
// IsCodeMintSession() to distinguish CodeMint sessions from Coding sessions.
type ActiveSession struct {
	// ClientMode describes the runtime environment (CLI terminal or daemon/CUI).
	ClientMode registry.ClientMode
	// ClientID is a unique identifier for this client instance (format: "{mode}:{uuid}").
	ClientID string

	// mu protects Session, Project, YoloEnabled, and IsSuspended from concurrent access.
	// The Heartbeat goroutine reads these while SetSession may write from the main goroutine.
	mu sync.RWMutex
	// IsSuspended indicates that another client has taken over this session.
	// The client can reclaim ownership by typing any input.
	isSuspended bool
	// project is the active code project. Always non-nil after bootstrap.
	project *domain.Project
	// session is the active execution session. Always non-nil after bootstrap.
	session *domain.Session
	// yoloEnabled mirrors Project.YoloMode for quick access.
	yoloEnabled bool
	// LastSeenTaskID tracks the last coordination task seen by this client.
	// Used to show "missed activity" when reclaiming a suspended session.
	LastSeenTaskID string
	// acpRegistry is the ACP worker registry for managing agent processes.
	acpRegistry *acp.Registry
	// acpRuntime is the ACP Runtime for pipeline management (Story 3.12).
	acpRuntime *Runtime
	// acpSessionID is the ACP session ID for the current session's worker.
	// This is the session ID returned by session/new from the ACP agent.
	ACPSessionID string
	// Verbosity controls how much output is shown in the TUI.
	// 0 = Task (everything), 1 = Story (no thinking), 2 = Epic (minimal).
	Verbosity int

	// inputSource tracks the source of the current inbound message for audit.
	// Set by DispatchInbound, cleared after dispatch completes.
	inputSource   string
	inputSourceMu sync.RWMutex
	// inputUserID tracks the user ID from the inbound message source.
	inputUserID string

	// projectSwitchCallbacks are invoked when the project changes.
	projectSwitchCallbacks []ProjectSwitchCallback
	projectSwitchMu        sync.RWMutex

	// wakeupCh signals the scheduler loop to check for new tasks.
	// Capacity of 1 allows coalescing multiple Wakeup() calls into one signal.
	wakeupCh   chan struct{}
	wakeupOnce sync.Once
}

// GetClientMode satisfies registry.ActiveSessionInfo.
func (a *ActiveSession) GetClientMode() registry.ClientMode { return a.ClientMode }

// GetIsCodeMint satisfies registry.ActiveSessionInfo.
// Returns true if this is a CodeMint sentinel session (non-project work).
func (a *ActiveSession) GetIsCodeMint() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.project != nil && a.project.Kind == domain.ProjectKindCodeMint
}

// IsCodeMintSession returns true if this is a CodeMint sentinel session.
// This is a convenience method that wraps GetIsCodeMint().
func (a *ActiveSession) IsCodeMintSession() bool {
	return a.GetIsCodeMint()
}

// GetSessionID satisfies registry.MutableSessionInfo.
func (a *ActiveSession) GetSessionID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.session == nil {
		return ""
	}
	return a.session.ID
}

// GetProjectID satisfies registry.MutableSessionInfo.
func (a *ActiveSession) GetProjectID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.project == nil {
		return ""
	}
	return a.project.ID
}

// GetClientID satisfies registry.MutableSessionInfo.
func (a *ActiveSession) GetClientID() string { return a.ClientID }

// SetSession satisfies registry.MutableSessionInfo.
// It also fires any registered project switch callbacks if the project changes.
func (a *ActiveSession) SetSession(session any, project any, yoloEnabled bool) {
	a.mu.Lock()
	oldProjectID := ""
	if a.project != nil {
		oldProjectID = a.project.ID
	}

	if session == nil {
		a.session = nil
		a.project = nil
		a.yoloEnabled = false
		a.mu.Unlock()

		// Fire callbacks if project changed.
		if oldProjectID != "" {
			a.fireProjectSwitchCallbacks(nil)
		}
		return
	}

	newProject := project.(*domain.Project)
	a.session = session.(*domain.Session)
	a.project = newProject
	a.yoloEnabled = yoloEnabled
	a.mu.Unlock()

	// Fire callbacks if project changed.
	if oldProjectID != newProject.ID {
		a.fireProjectSwitchCallbacks(newProject)
	}
}

// SetSuspended satisfies registry.MutableSessionInfo.
func (a *ActiveSession) SetSuspended(suspended bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.isSuspended = suspended
}

// GetSuspended returns whether this session is suspended.
func (a *ActiveSession) GetSuspended() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.isSuspended
}

// GetYoloEnabled returns whether YOLO mode is enabled.
func (a *ActiveSession) GetYoloEnabled() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.yoloEnabled
}

// SetClientMode satisfies registry.MutableSessionInfo.
func (a *ActiveSession) SetClientMode(mode registry.ClientMode) {
	a.ClientMode = mode
}

// SetACPRegistry sets the ACP worker registry for this session.
func (a *ActiveSession) SetACPRegistry(reg *acp.Registry) {
	a.acpRegistry = reg
}

// ACPRegistry returns the ACP worker registry, or nil if not set.
func (a *ActiveSession) ACPRegistry() *acp.Registry {
	return a.acpRegistry
}

// GetProject returns the active project, or nil.
func (a *ActiveSession) GetProject() *domain.Project {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.project
}

// GetSession returns the active session, or nil.
func (a *ActiveSession) GetSession() *domain.Session {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.session
}

// GetACPSessionID returns the ACP session ID.
func (a *ActiveSession) GetACPSessionID() string {
	return a.ACPSessionID
}

// SetACPSessionID sets the ACP session ID.
func (a *ActiveSession) SetACPSessionID(id string) {
	a.ACPSessionID = id
}

// GetVerbosity returns the current verbosity level.
func (a *ActiveSession) GetVerbosity() int {
	return a.Verbosity
}

// SetVerbosity sets the verbosity level.
func (a *ActiveSession) SetVerbosity(level int) {
	a.Verbosity = level
}

// SetACPRuntime sets the ACP Runtime for this session (Story 3.12).
func (a *ActiveSession) SetACPRuntime(rt *Runtime) {
	a.acpRuntime = rt
}

// ACPRuntime returns the ACP Runtime, or nil if not set.
func (a *ActiveSession) ACPRuntime() *Runtime {
	return a.acpRuntime
}

// OnProjectSwitch registers a callback that is invoked when the active project changes.
// The callback receives the new project (may be nil if no project is active).
// This is used by the Runtime to reload permissions when the project changes (Task 3.12.3).
func (a *ActiveSession) OnProjectSwitch(fn ProjectSwitchCallback) {
	a.projectSwitchMu.Lock()
	defer a.projectSwitchMu.Unlock()
	a.projectSwitchCallbacks = append(a.projectSwitchCallbacks, fn)
}

// fireProjectSwitchCallbacks invokes all registered project switch callbacks.
func (a *ActiveSession) fireProjectSwitchCallbacks(project *domain.Project) {
	a.projectSwitchMu.RLock()
	callbacks := a.projectSwitchCallbacks
	a.projectSwitchMu.RUnlock()

	for _, cb := range callbacks {
		cb(project)
	}
}

// initWakeupCh initializes the wakeup channel if it hasn't been created yet.
// Uses sync.Once to ensure thread-safe single initialization.
func (a *ActiveSession) initWakeupCh() {
	a.wakeupOnce.Do(func() {
		a.wakeupCh = make(chan struct{}, 1)
	})
}

// WakeupCh returns a channel that signals when new tasks may be available.
// The scheduler should select on this channel alongside a fallback ticker.
// The channel has capacity 1, so multiple Wakeup() calls coalesce into one signal.
func (a *ActiveSession) WakeupCh() <-chan struct{} {
	a.initWakeupCh()
	return a.wakeupCh
}

// Wakeup signals the scheduler to check for new tasks.
// Multiple calls before the scheduler reads the channel coalesce into a single signal.
// This should be called from:
//   - Phase 5 Activation (Story 2.7) when newly generated tasks are committed
//   - /approve, /deny, /yolo commands when they resolve an awaiting task
//   - Mid-flight pivots (Story 2.8) that update pending tasks
func (a *ActiveSession) Wakeup() {
	a.initWakeupCh()
	// Non-blocking send: if the channel already has a signal, skip.
	// This coalesces multiple wakeups into one.
	select {
	case a.wakeupCh <- struct{}{}:
	default:
	}
}

// SetInputSource records the source of the current inbound message.
// Called by DispatchInbound before dispatch, cleared by ClearInputSource after.
func (a *ActiveSession) SetInputSource(source, userID string) {
	a.inputSourceMu.Lock()
	defer a.inputSourceMu.Unlock()
	a.inputSource = source
	a.inputUserID = userID
}

// ClearInputSource clears the input source metadata after dispatch.
func (a *ActiveSession) ClearInputSource() {
	a.inputSourceMu.Lock()
	defer a.inputSourceMu.Unlock()
	a.inputSource = ""
	a.inputUserID = ""
}

// GetInputSource returns the current input source and user ID.
// Returns ("", "") if not set (e.g., when using legacy Loop without multiplexer).
func (a *ActiveSession) GetInputSource() (source, userID string) {
	a.inputSourceMu.RLock()
	defer a.inputSourceMu.RUnlock()
	return a.inputSource, a.inputUserID
}

// NewActiveSession creates a new ActiveSession with the given session and project.
// This is primarily used for testing; production code typically uses the zero value
// and calls SetSession to initialize.
func NewActiveSession(session *domain.Session, project *domain.Project) *ActiveSession {
	return &ActiveSession{
		session: session,
		project: project,
	}
}

// NewActiveSessionWithYolo creates a new ActiveSession with YOLO mode enabled.
// This is primarily used for testing.
func NewActiveSessionWithYolo(session *domain.Session, project *domain.Project, yoloEnabled bool) *ActiveSession {
	return &ActiveSession{
		session:     session,
		project:     project,
		yoloEnabled: yoloEnabled,
	}
}

// Compile-time assertion: *ActiveSession must satisfy registry.ActiveSessionInfo.
var _ registry.ActiveSessionInfo = (*ActiveSession)(nil)

// Compile-time assertion: *ActiveSession must satisfy registry.MutableSessionInfo.
var _ registry.MutableSessionInfo = (*ActiveSession)(nil)

// Compile-time assertion: *ActiveSession must satisfy registry.VerbositySessionInfo.
var _ registry.VerbositySessionInfo = (*ActiveSession)(nil)
