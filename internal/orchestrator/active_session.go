// Package orchestrator coordinates task execution, session management, and
// command dispatching for CodeMint.
package orchestrator

import (
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
	// IsGlobal indicates that this session has no associated project.
	IsGlobal bool
	// Project is the active code project. Nil when IsGlobal is true.
	Project *domain.Project
	// Session is the active execution session. Nil when IsGlobal is true.
	Session *domain.Session
	// YoloEnabled mirrors Project.YoloMode for quick access.
	YoloEnabled bool
}

// GetClientMode satisfies registry.ActiveSessionInfo.
func (a *ActiveSession) GetClientMode() registry.ClientMode { return a.ClientMode }

// GetIsGlobal satisfies registry.ActiveSessionInfo.
func (a *ActiveSession) GetIsGlobal() bool { return a.IsGlobal }

// Compile-time assertion: *ActiveSession must satisfy registry.ActiveSessionInfo.
var _ registry.ActiveSessionInfo = (*ActiveSession)(nil)
