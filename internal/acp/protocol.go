// Package acp provides the Agent Communication Protocol types and worker lifecycle management.
// It implements JSON-RPC 2.0 communication with ACP-compatible CLI tools like OpenCode.
package acp

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
)

// JSON-RPC 2.0 version constant.
const JSONRPCVersion = "2.0"

// Method constants for ACP communication.
const (
	MethodInitialize        = "initialize"
	MethodShutdown          = "shutdown"
	MethodSessionNew        = "session/new"
	MethodSessionPrompt     = "session/prompt"
	MethodSessionCancel     = "session/cancel"
	MethodSessionUpdate     = "session/update"
	MethodRequestPermission = "session/request_permission"
)

// UpdateKind constants for session update notifications.
const (
	UpdateKindUserMessageChunk  = "user_message_chunk"
	UpdateKindAgentMessageChunk = "agent_message_chunk"
	UpdateKindAgentThoughtChunk = "agent_thought_chunk"
	UpdateKindToolCall          = "tool_call"
	UpdateKindToolCallUpdate    = "tool_call_update"
	UpdateKindPlan              = "plan"
)

// Standard JSON-RPC 2.0 error codes.
const (
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternalError  = -32603
)

// idCounter generates unique request IDs.
var idCounter atomic.Int64

// Message represents a JSON-RPC 2.0 message envelope.
// It can be a request, response, or notification.
type Message struct {
	JSONRPC string          `json:"jsonrpc"`          // always "2.0"
	ID      json.RawMessage `json:"id,omitempty"`     // request/response ID; omitted for notifications
	Method  string          `json:"method,omitempty"` // method name for requests/notifications
	Params  json.RawMessage `json:"params,omitempty"` // parameters for requests/notifications
	Result  json.RawMessage `json:"result,omitempty"` // result for successful responses
	Error   *RPCError       `json:"error,omitempty"`  // error for failed responses
}

// IsRequest returns true if this message is a request (has ID and Method).
func (m *Message) IsRequest() bool {
	return len(m.ID) > 0 && m.Method != ""
}

// IsResponse returns true if this message is a response (has ID, no Method).
func (m *Message) IsResponse() bool {
	return len(m.ID) > 0 && m.Method == ""
}

// IsNotification returns true if this message is a notification (has Method, no ID).
func (m *Message) IsNotification() bool {
	return len(m.ID) == 0 && m.Method != ""
}

// GetID returns the message ID as an int64, or 0 if not parseable.
func (m *Message) GetID() int64 {
	if len(m.ID) == 0 {
		return 0
	}
	var id int64
	if err := json.Unmarshal(m.ID, &id); err != nil {
		return 0
	}
	return id
}

// RPCError represents a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error implements the error interface.
func (e *RPCError) Error() string {
	if len(e.Data) > 0 {
		return fmt.Sprintf("rpc error %d: %s (data: %s)", e.Code, e.Message, string(e.Data))
	}
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

// SessionUpdate represents a session/update notification payload.
type SessionUpdate struct {
	SessionID string     `json:"sessionId"`
	Update    UpdateBody `json:"update"`
}

// UpdateBody represents the update content within a session update.
type UpdateBody struct {
	Kind string          `json:"sessionUpdate"` // user_message_chunk | agent_message_chunk | agent_thought_chunk | tool_call | tool_call_update | plan
	Raw  json.RawMessage `json:"-"`             // preserve original for circular buffer
}

// UnmarshalJSON implements custom unmarshaling to preserve the raw data.
func (u *UpdateBody) UnmarshalJSON(data []byte) error {
	// Store the raw data for later use
	u.Raw = make(json.RawMessage, len(data))
	copy(u.Raw, data)

	// Parse just the kind field
	var temp struct {
		Kind string `json:"sessionUpdate"`
	}
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}
	u.Kind = temp.Kind
	return nil
}

// MarshalJSON implements custom marshaling.
func (u UpdateBody) MarshalJSON() ([]byte, error) {
	if len(u.Raw) > 0 {
		return u.Raw, nil
	}
	return json.Marshal(struct {
		Kind string `json:"sessionUpdate"`
	}{Kind: u.Kind})
}

// InitializeParams represents the parameters for the initialize request.
type InitializeParams struct {
	ClientInfo    ClientInfo `json:"clientInfo"`
	Capabilities  Caps       `json:"capabilities"`
	WorkingDir    string     `json:"workingDir,omitempty"`
}

// ClientInfo identifies the client to the ACP server.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Caps represents client capabilities.
type Caps struct {
	// Add capability flags as needed
}

// InitializeResult represents the result of the initialize request.
type InitializeResult struct {
	ServerInfo   ServerInfo `json:"serverInfo"`
	Capabilities ServerCaps `json:"capabilities"`
}

// ServerInfo identifies the ACP server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerCaps represents server capabilities.
type ServerCaps struct {
	Streaming bool `json:"streaming,omitempty"`
	ToolCalls bool `json:"toolCalls,omitempty"`
	Planning  bool `json:"planning,omitempty"`
}

// SessionNewParams represents the parameters for session/new.
type SessionNewParams struct {
	SessionID    string `json:"sessionId,omitempty"`    // optional; server generates if empty
	SystemPrompt string `json:"systemPrompt,omitempty"` // optional; injected memory/context for the session
}

// SessionNewResult represents the result of session/new.
type SessionNewResult struct {
	SessionID string `json:"sessionId"`
}

// SessionPromptParams represents the parameters for session/prompt.
type SessionPromptParams struct {
	SessionID string `json:"sessionId"`
	Prompt    string `json:"prompt"`
}

// SessionPromptResult represents the result of session/prompt.
type SessionPromptResult struct {
	Success bool `json:"success"`
}

// SessionCancelParams represents the parameters for session/cancel.
type SessionCancelParams struct {
	SessionID string `json:"sessionId"`
}

// PermissionRequest represents a tool-call permission request.
type PermissionRequest struct {
	SessionID  string          `json:"sessionId"`
	RequestID  string          `json:"requestId"`
	Tool       string          `json:"tool"`
	Parameters json.RawMessage `json:"parameters"`
}

// PermissionResponse represents the response to a permission request.
type PermissionResponse struct {
	RequestID string `json:"requestId"`
	Granted   bool   `json:"granted"`
	Reason    string `json:"reason,omitempty"`
}

// NewRequest creates a new JSON-RPC 2.0 request message.
func NewRequest(method string, params any) (*Message, error) {
	id := idCounter.Add(1)
	idBytes, err := json.Marshal(id)
	if err != nil {
		return nil, fmt.Errorf("marshal id: %w", err)
	}

	var paramsBytes json.RawMessage
	if params != nil {
		paramsBytes, err = json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("marshal params: %w", err)
		}
	}

	return &Message{
		JSONRPC: JSONRPCVersion,
		ID:      idBytes,
		Method:  method,
		Params:  paramsBytes,
	}, nil
}

// NewRequestWithID creates a new JSON-RPC 2.0 request message with a specific ID.
func NewRequestWithID(id int64, method string, params any) (*Message, error) {
	idBytes, err := json.Marshal(id)
	if err != nil {
		return nil, fmt.Errorf("marshal id: %w", err)
	}

	var paramsBytes json.RawMessage
	if params != nil {
		paramsBytes, err = json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("marshal params: %w", err)
		}
	}

	return &Message{
		JSONRPC: JSONRPCVersion,
		ID:      idBytes,
		Method:  method,
		Params:  paramsBytes,
	}, nil
}

// NewNotification creates a new JSON-RPC 2.0 notification (no ID).
func NewNotification(method string, params any) (*Message, error) {
	var paramsBytes json.RawMessage
	if params != nil {
		var err error
		paramsBytes, err = json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("marshal params: %w", err)
		}
	}

	return &Message{
		JSONRPC: JSONRPCVersion,
		Method:  method,
		Params:  paramsBytes,
	}, nil
}

// NewResponse creates a new JSON-RPC 2.0 success response message.
func NewResponse(id json.RawMessage, result any) (*Message, error) {
	var resultBytes json.RawMessage
	if result != nil {
		var err error
		resultBytes, err = json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("marshal result: %w", err)
		}
	}

	return &Message{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Result:  resultBytes,
	}, nil
}

// NewResponseWithIntID creates a new JSON-RPC 2.0 success response with an int64 ID.
func NewResponseWithIntID(id int64, result any) (*Message, error) {
	idBytes, err := json.Marshal(id)
	if err != nil {
		return nil, fmt.Errorf("marshal id: %w", err)
	}
	return NewResponse(idBytes, result)
}

// NewError creates a new JSON-RPC 2.0 error response message.
func NewError(id json.RawMessage, code int, message string) *Message {
	return &Message{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
		},
	}
}

// NewErrorWithData creates a new JSON-RPC 2.0 error response with additional data.
func NewErrorWithData(id json.RawMessage, code int, message string, data any) (*Message, error) {
	msg := NewError(id, code, message)
	if data != nil {
		dataBytes, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("marshal error data: %w", err)
		}
		msg.Error.Data = dataBytes
	}
	return msg, nil
}

// ParseResult unmarshals the result field into the provided target.
func (m *Message) ParseResult(target any) error {
	if m.Error != nil {
		return m.Error
	}
	if len(m.Result) == 0 {
		return nil
	}
	return json.Unmarshal(m.Result, target)
}

// ParseParams unmarshals the params field into the provided target.
func (m *Message) ParseParams(target any) error {
	if len(m.Params) == 0 {
		return nil
	}
	return json.Unmarshal(m.Params, target)
}

// ParseSessionUpdate parses a session/update notification params into SessionUpdate.
func (m *Message) ParseSessionUpdate() (*SessionUpdate, error) {
	if m.Method != MethodSessionUpdate {
		return nil, fmt.Errorf("not a session/update message: method=%s", m.Method)
	}
	var update SessionUpdate
	if err := m.ParseParams(&update); err != nil {
		return nil, fmt.Errorf("parse session update: %w", err)
	}
	return &update, nil
}
