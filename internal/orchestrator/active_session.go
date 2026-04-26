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
// The callback receives the new project (may be nil if switching to global mode).
type ProjectSwitchCallback func(*domain.Project)

// ActiveSession holds the runtime state for the current user session. It is
// consulted by the Dispatcher to determine how to route natural-language input
// and which commands are permitted.
//
// When IsGlobal is true the session operates outside a specific code project;
// Project and Session are nil. When IsGlobal is false both fields must be
// non-nil and the session is bound to a particular codebase.
type ActiveSession struct {
	// ClientMode describes the runtime environment (CLI terminal or daemon/CUI).
	ClientMode registry.ClientMode
	// ClientID is a unique identifier for this client instance (format: "{mode}:{uuid}").
	ClientID string
	// IsGlobal indicates that this session has no associated project.
	IsGlobal bool
	// IsSuspended indicates that another client has taken over this session.
	// The client can reclaim ownership by typing any input.
	IsSuspended bool
	// Project is the active code project. Nil when IsGlobal is true.
	Project *domain.Project
	// Session is the active execution session. Nil when IsGlobal is true.
	Session *domain.Session
	// YoloEnabled mirrors Project.YoloMode for quick access.
	YoloEnabled bool
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

// GetIsGlobal satisfies registry.ActiveSessionInfo.
func (a *ActiveSession) GetIsGlobal() bool { return a.IsGlobal }

// GetSessionID satisfies registry.MutableSessionInfo.
func (a *ActiveSession) GetSessionID() string {
	if a.Session == nil {
		return ""
	}
	return a.Session.ID
}

// GetClientID satisfies registry.MutableSessionInfo.
func (a *ActiveSession) GetClientID() string { return a.ClientID }

// SetSession satisfies registry.MutableSessionInfo.
// It also fires any registered project switch callbacks if the project changes.
func (a *ActiveSession) SetSession(session any, project any, yoloEnabled bool) {
	oldProjectID := ""
	if a.Project != nil {
		oldProjectID = a.Project.ID
	}

	if session == nil {
		a.Session = nil
		a.Project = nil
		a.IsGlobal = true
		a.YoloEnabled = false

		// Fire callbacks if project changed.
		if oldProjectID != "" {
			a.fireProjectSwitchCallbacks(nil)
		}
		return
	}

	newProject := project.(*domain.Project)
	a.Session = session.(*domain.Session)
	a.Project = newProject
	a.IsGlobal = false
	a.YoloEnabled = yoloEnabled

	// Fire callbacks if project changed.
	if oldProjectID != newProject.ID {
		a.fireProjectSwitchCallbacks(newProject)
	}
}

// SetSuspended satisfies registry.MutableSessionInfo.
func (a *ActiveSession) SetSuspended(suspended bool) {
	a.IsSuspended = suspended
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
	return a.Project
}

// GetSession returns the active session, or nil.
func (a *ActiveSession) GetSession() *domain.Session {
	return a.Session
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
// The callback receives the new project (may be nil if switching to global mode).
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

// Compile-time assertion: *ActiveSession must satisfy registry.ActiveSessionInfo.
var _ registry.ActiveSessionInfo = (*ActiveSession)(nil)

// Compile-time assertion: *ActiveSession must satisfy registry.MutableSessionInfo.
var _ registry.MutableSessionInfo = (*ActiveSession)(nil)

// Compile-time assertion: *ActiveSession must satisfy registry.VerbositySessionInfo.
var _ registry.VerbositySessionInfo = (*ActiveSession)(nil)
