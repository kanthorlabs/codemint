// Package orchestrator coordinates task execution, session management, and
// command dispatching for CodeMint.
package orchestrator

import (
	"codemint.kanthorlabs.com/internal/acp"
	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/registry"
)

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
	// acpSessionID is the ACP session ID for the current session's worker.
	// This is the session ID returned by session/new from the ACP agent.
	ACPSessionID string
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
func (a *ActiveSession) SetSession(session any, project any, yoloEnabled bool) {
	if session == nil {
		a.Session = nil
		a.Project = nil
		a.IsGlobal = true
		a.YoloEnabled = false
		return
	}
	a.Session = session.(*domain.Session)
	a.Project = project.(*domain.Project)
	a.IsGlobal = false
	a.YoloEnabled = yoloEnabled
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

// Compile-time assertion: *ActiveSession must satisfy registry.ActiveSessionInfo.
var _ registry.ActiveSessionInfo = (*ActiveSession)(nil)

// Compile-time assertion: *ActiveSession must satisfy registry.MutableSessionInfo.
var _ registry.MutableSessionInfo = (*ActiveSession)(nil)
