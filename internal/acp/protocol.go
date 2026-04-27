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
// Per ACP spec: https://agentclientprotocol.com/protocol/schema.md (SessionUpdate union)
const (
	// Core message updates
	UpdateKindUserMessageChunk  = "user_message_chunk"
	UpdateKindAgentMessageChunk = "agent_message_chunk"
	UpdateKindAgentThoughtChunk = "agent_thought_chunk"

	// Tool-related updates
	UpdateKindToolCall       = "tool_call"
	UpdateKindToolCallUpdate = "tool_call_update"

	// Planning update
	UpdateKindPlan = "plan"

	// Turn lifecycle updates
	UpdateKindTurnStart = "turn_start"
	UpdateKindTurnEnd   = "turn_end"

	// Session state updates
	UpdateKindSessionInfoUpdate       = "session_info_update"
	UpdateKindAvailableCommandsUpdate = "available_commands_update"
	UpdateKindCurrentModeUpdate       = "current_mode_update"
	UpdateKindConfigOptionUpdate      = "config_option_update"
)

// Standard JSON-RPC 2.0 error codes.
const (
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternalError  = -32603
)

// ACP-specific error codes.
const (
	// ErrCodeAuthRequired indicates that authentication is required before
	// the operation can proceed. The client should call authenticate.
	ErrCodeAuthRequired = -32000
	// ErrCodeSessionNotFound indicates the specified session does not exist.
	ErrCodeSessionNotFound = -32001
	// ErrCodeResourceNotFound indicates that the requested resource
	// (session, terminal, etc.) was not found.
	ErrCodeResourceNotFound = -32002
)

// StopReason indicates why the agent stopped processing a turn.
type StopReason string

const (
	// StopReasonEndTurn indicates the language model finished responding
	// without requesting more tools.
	StopReasonEndTurn StopReason = "end_turn"
	// StopReasonMaxTokens indicates the maximum token limit was reached.
	StopReasonMaxTokens StopReason = "max_tokens"
	// StopReasonMaxTurnRequests indicates the maximum number of model requests
	// in a single turn was exceeded.
	StopReasonMaxTurnRequests StopReason = "max_turn_requests"
	// StopReasonRefusal indicates the agent refuses to continue.
	StopReasonRefusal StopReason = "refusal"
	// StopReasonCancelled indicates the client cancelled the turn.
	StopReasonCancelled StopReason = "cancelled"
)

// PermissionOptionKind is a hint to help clients choose appropriate icons
// and UI treatment for permission options.
type PermissionOptionKind string

const (
	// PermissionKindAllowOnce allows this operation only this time.
	PermissionKindAllowOnce PermissionOptionKind = "allow_once"
	// PermissionKindAllowAlways allows this operation and remembers the choice.
	PermissionKindAllowAlways PermissionOptionKind = "allow_always"
	// PermissionKindRejectOnce rejects this operation only this time.
	PermissionKindRejectOnce PermissionOptionKind = "reject_once"
	// PermissionKindRejectAlways rejects this operation and remembers the choice.
	PermissionKindRejectAlways PermissionOptionKind = "reject_always"
)

// ToolCallStatus indicates the current status of a tool call.
type ToolCallStatus string

const (
	ToolCallStatusPending    ToolCallStatus = "pending"
	ToolCallStatusInProgress ToolCallStatus = "in_progress"
	ToolCallStatusCompleted  ToolCallStatus = "completed"
	ToolCallStatusFailed     ToolCallStatus = "failed"
)

// PlanEntryPriority indicates the priority of a plan entry.
type PlanEntryPriority string

const (
	PlanEntryPriorityHigh   PlanEntryPriority = "high"
	PlanEntryPriorityMedium PlanEntryPriority = "medium"
	PlanEntryPriorityLow    PlanEntryPriority = "low"
)

// PlanEntryStatus indicates the status of a plan entry.
type PlanEntryStatus string

const (
	PlanEntryStatusPending    PlanEntryStatus = "pending"
	PlanEntryStatusInProgress PlanEntryStatus = "in_progress"
	PlanEntryStatusCompleted  PlanEntryStatus = "completed"
)

// PlanEntry represents a single entry in an agent plan.
type PlanEntry struct {
	Content  string            `json:"content"`
	Priority PlanEntryPriority `json:"priority,omitempty"`
	Status   PlanEntryStatus   `json:"status,omitempty"`
}

// PlanUpdate represents a plan session update.
type PlanUpdate struct {
	SessionUpdate string      `json:"sessionUpdate"` // always "plan"
	Entries       []PlanEntry `json:"entries"`
}

// SlashCommand represents a slash command advertised by the agent.
type SlashCommand struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// AvailableCommandsUpdate represents an available_commands_update notification.
type AvailableCommandsUpdate struct {
	SessionUpdate string         `json:"sessionUpdate"` // always "available_commands_update"
	Commands      []SlashCommand `json:"commands"`
}

// CurrentModeUpdate represents a current_mode_update notification.
type CurrentModeUpdate struct {
	SessionUpdate string `json:"sessionUpdate"` // always "current_mode_update"
	ModeID        string `json:"modeId"`
}

// ConfigOptionUpdate represents a config_option_update notification.
type ConfigOptionUpdate struct {
	SessionUpdate string `json:"sessionUpdate"` // always "config_option_update"
	OptionID      string `json:"optionId"`
	ValueID       string `json:"valueId"`
}

// SessionInfoUpdate represents a session_info_update notification.
type SessionInfoUpdate struct {
	SessionUpdate string                `json:"sessionUpdate"` // always "session_info_update"
	ConfigOptions []SessionConfigOption `json:"configOptions,omitempty"`
	Modes         *SessionModeState     `json:"modes,omitempty"`
}

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

// IsAuthRequired returns true if this is an authentication required error.
func (e *RPCError) IsAuthRequired() bool {
	return e.Code == ErrCodeAuthRequired
}

// IsSessionNotFound returns true if this is a session not found error.
func (e *RPCError) IsSessionNotFound() bool {
	return e.Code == ErrCodeSessionNotFound
}

// IsResourceNotFound returns true if this is a resource not found error.
func (e *RPCError) IsResourceNotFound() bool {
	return e.Code == ErrCodeResourceNotFound
}

// SessionUpdate represents a session/update notification payload.
type SessionUpdate struct {
	SessionID string          `json:"sessionId"`
	Update    UpdateBody      `json:"update"`
	Meta      json.RawMessage `json:"_meta,omitempty"` // Opaque passthrough for agent-specific data
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
// Per ACP spec: https://agentclientprotocol.com/protocol/initialization
type InitializeParams struct {
	ProtocolVersion    int                `json:"protocolVersion"`
	ClientInfo         *Implementation    `json:"clientInfo,omitempty"`
	ClientCapabilities ClientCapabilities `json:"clientCapabilities,omitempty"`
}

// Implementation identifies a client or agent implementation.
// Per ACP spec, this will be required in future protocol versions.
type Implementation struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version"`
}

// ClientCapabilities represents capabilities supported by the client.
// Per ACP spec: https://agentclientprotocol.com/protocol/initialization#client-capabilities
type ClientCapabilities struct {
	FS       *FSCapabilities `json:"fs,omitempty"`
	Terminal bool            `json:"terminal,omitempty"`
}

// FSCapabilities represents file system capabilities.
type FSCapabilities struct {
	ReadTextFile  bool `json:"readTextFile,omitempty"`
	WriteTextFile bool `json:"writeTextFile,omitempty"`
}

// InitializeResult represents the result of the initialize request.
// Per ACP spec: https://agentclientprotocol.com/protocol/initialization
type InitializeResult struct {
	ProtocolVersion   int               `json:"protocolVersion"`
	AgentInfo         *Implementation   `json:"agentInfo,omitempty"`
	AgentCapabilities AgentCapabilities `json:"agentCapabilities,omitempty"`
	AuthMethods       []AuthMethod      `json:"authMethods,omitempty"`
	Meta              json.RawMessage   `json:"_meta,omitempty"` // Opaque passthrough for agent-specific data
}

// AgentCapabilities represents capabilities supported by the agent.
// Per ACP spec: https://agentclientprotocol.com/protocol/initialization#agent-capabilities
type AgentCapabilities struct {
	LoadSession         bool                 `json:"loadSession,omitempty"`
	McpCapabilities     *McpCapabilities     `json:"mcpCapabilities,omitempty"`
	PromptCapabilities  *PromptCapabilities  `json:"promptCapabilities,omitempty"`
	SessionCapabilities *SessionCapabilities `json:"sessionCapabilities,omitempty"`
}

// McpCapabilities indicates which MCP transports the agent supports.
type McpCapabilities struct {
	HTTP bool `json:"http,omitempty"`
	SSE  bool `json:"sse,omitempty"`
}

// PromptCapabilities indicates which content types are supported in prompts.
type PromptCapabilities struct {
	Image           bool `json:"image,omitempty"`
	Audio           bool `json:"audio,omitempty"`
	EmbeddedContext bool `json:"embeddedContext,omitempty"`
}

// SessionCapabilities indicates which session methods are supported.
type SessionCapabilities struct {
	Resume *struct{} `json:"resume,omitempty"`
	Close  *struct{} `json:"close,omitempty"`
	List   *struct{} `json:"list,omitempty"`
}

// AuthMethod describes an available authentication method.
type AuthMethod struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Type can be "agent" (default) or other future types
	Type string `json:"type,omitempty"`
}

// McpEnvVariable represents an environment variable for MCP server configuration.
type McpEnvVariable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// McpServer represents an MCP server configuration for ACP session setup.
// Per ACP spec, all agents MUST support the stdio transport.
type McpServer struct {
	Name    string           `json:"name"`              // Human-readable identifier for the server
	Command string           `json:"command"`           // Absolute path to the MCP server executable
	Args    []string         `json:"args"`              // Command-line arguments to pass to the server
	Env     []McpEnvVariable `json:"env,omitempty"`     // Environment variables to set when launching
}

// SessionNewParams represents the parameters for session/new.
// Per ACP spec: https://agentclientprotocol.com/protocol/session-setup#creating-a-session
type SessionNewParams struct {
	Cwd        string      `json:"cwd"`        // required; working directory (absolute path)
	McpServers []McpServer `json:"mcpServers"` // required; MCP server configurations (can be empty array)
}

// SessionNewResult represents the result of session/new.
type SessionNewResult struct {
	SessionID     string                 `json:"sessionId"`
	ConfigOptions []SessionConfigOption  `json:"configOptions,omitempty"`
	Modes         *SessionModeState      `json:"modes,omitempty"`
	Meta          json.RawMessage        `json:"_meta,omitempty"` // Opaque passthrough for agent-specific data
}

// SessionConfigOption represents a session configuration option.
type SessionConfigOption struct {
	ID          string                     `json:"id"`
	Label       string                     `json:"label"`
	Description string                     `json:"description,omitempty"`
	Values      []SessionConfigOptionValue `json:"values"`
	CurrentID   string                     `json:"currentId"`
}

// SessionConfigOptionValue represents a value for a session config option.
type SessionConfigOptionValue struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// SessionModeState represents the current mode state for a session.
type SessionModeState struct {
	AvailableModes []SessionMode `json:"availableModes"`
	CurrentID      string        `json:"currentId"`
}

// SessionMode represents an available session mode.
type SessionMode struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// ContentBlock represents a content block in ACP protocol.
// Per ACP spec, this follows the same structure as MCP ContentBlock.
// https://agentclientprotocol.com/protocol/content
type ContentBlock struct {
	Type string `json:"type"` // "text", "image", "audio", "resource", "resource_link"

	// For type="text"
	Text string `json:"text,omitempty"`

	// For type="image"
	MimeType string `json:"mimeType,omitempty"`
	Data     string `json:"data,omitempty"` // base64 encoded

	// For type="resource"
	Resource *EmbeddedResource `json:"resource,omitempty"`

	// For type="resource_link"
	URI         string `json:"uri,omitempty"`
	Name        string `json:"name,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Size        *int64 `json:"size,omitempty"`

	// Common optional field
	Annotations *Annotations `json:"annotations,omitempty"`
}

// EmbeddedResource represents a resource embedded in a content block.
type EmbeddedResource struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	// Text resource
	Text string `json:"text,omitempty"`
	// Blob resource (base64 encoded)
	Blob string `json:"blob,omitempty"`
}

// Annotations provides optional metadata about how content should be used or displayed.
type Annotations struct {
	Audience     []string `json:"audience,omitempty"`
	Priority     *float64 `json:"priority,omitempty"`
	LastModified string   `json:"lastModified,omitempty"`
}

// TextContent creates a text content block from a string.
func TextContent(text string) ContentBlock {
	return ContentBlock{Type: "text", Text: text}
}

// ResourceLinkContent creates a resource_link content block.
func ResourceLinkContent(uri, name, mimeType string) ContentBlock {
	return ContentBlock{
		Type:     "resource_link",
		URI:      uri,
		Name:     name,
		MimeType: mimeType,
	}
}

// EmbeddedResourceContent creates a resource content block with embedded text.
func EmbeddedResourceContent(uri, mimeType, text string) ContentBlock {
	return ContentBlock{
		Type: "resource",
		Resource: &EmbeddedResource{
			URI:      uri,
			MimeType: mimeType,
			Text:     text,
		},
	}
}

// SessionPromptParams represents the parameters for session/prompt.
// Per ACP spec: https://agentclientprotocol.com/protocol/prompt-turn#1-user-message
type SessionPromptParams struct {
	SessionID string         `json:"sessionId"`
	Prompt    []ContentBlock `json:"prompt"` // Array of content blocks per ACP spec
}

// SessionPromptResult represents the result of session/prompt.
// Per ACP spec: https://agentclientprotocol.com/protocol/prompt-turn#4-check-for-completion
type SessionPromptResult struct {
	StopReason StopReason `json:"stopReason"`
}

// SessionCancelParams represents the parameters for session/cancel.
type SessionCancelParams struct {
	SessionID string `json:"sessionId"`
}

// RequestPermissionParams represents the parameters for session/request_permission.
// Per ACP spec: https://agentclientprotocol.com/protocol/tool-calls#requesting-permission
type RequestPermissionParams struct {
	SessionID string             `json:"sessionId"`
	ToolCall  ToolCallUpdate     `json:"toolCall"`
	Options   []PermissionOption `json:"options"`
	Meta      json.RawMessage    `json:"_meta,omitempty"` // Opaque passthrough for agent-specific data
}

// ToolCallUpdate represents a tool call update notification.
// Per ACP spec: https://agentclientprotocol.com/protocol/tool-calls
type ToolCallUpdate struct {
	ToolCallID string               `json:"toolCallId"`
	Title      string               `json:"title,omitempty"`
	Kind       string               `json:"kind,omitempty"` // read, edit, delete, move, search, execute, think, fetch, other
	Status     ToolCallStatus       `json:"status,omitempty"`
	Content    []ToolCallContent    `json:"content,omitempty"`
	Locations  []ToolCallLocation   `json:"locations,omitempty"`
	RawInput   json.RawMessage      `json:"rawInput,omitempty"`
	RawOutput  json.RawMessage      `json:"rawOutput,omitempty"`
	Meta       json.RawMessage      `json:"_meta,omitempty"` // Opaque passthrough for agent-specific data
}

// ToolCallContent represents content produced by a tool call.
type ToolCallContent struct {
	Type string `json:"type"` // "content", "diff", "terminal"

	// For type="content"
	Content *ContentBlock `json:"content,omitempty"`

	// For type="diff"
	Path    string `json:"path,omitempty"`
	OldText string `json:"oldText,omitempty"`
	NewText string `json:"newText,omitempty"`

	// For type="terminal"
	TerminalID string `json:"terminalId,omitempty"`
}

// ToolCallLocation represents a file location affected by a tool call.
type ToolCallLocation struct {
	Path string `json:"path"`
	Line *int   `json:"line,omitempty"`
}

// PermissionOption represents an available permission choice.
// Per ACP spec: https://agentclientprotocol.com/protocol/tool-calls#permission-options
type PermissionOption struct {
	OptionID string               `json:"optionId"`
	Name     string               `json:"name"`
	Kind     PermissionOptionKind `json:"kind"`
}

// RequestPermissionResult represents the response to a permission request.
// Per ACP spec: https://agentclientprotocol.com/protocol/tool-calls#requesting-permission
type RequestPermissionResult struct {
	Outcome RequestPermissionOutcome `json:"outcome"`
}

// RequestPermissionOutcome represents the user's decision on a permission request.
// This is a discriminated union: either "cancelled" or {outcome: "selected", optionId: string}
type RequestPermissionOutcome struct {
	Outcome  string `json:"outcome"` // "cancelled" or "selected"
	OptionID string `json:"optionId,omitempty"`
}

// CancelledOutcome returns a cancelled permission outcome.
func CancelledOutcome() RequestPermissionOutcome {
	return RequestPermissionOutcome{Outcome: "cancelled"}
}

// SelectedOutcome returns a selected permission outcome.
func SelectedOutcome(optionID string) RequestPermissionOutcome {
	return RequestPermissionOutcome{Outcome: "selected", OptionID: optionID}
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
