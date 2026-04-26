package acp

import (
	"encoding/json"
	"strings"
)

// MemoryOverrideTag is the tag emitted by agents when they override project memory.
const MemoryOverrideTag = "[memory-override]"

// EventKind categorizes the type of event from the ACP stream.
type EventKind int

const (
	// EventUnknown represents an unrecognized event type.
	EventUnknown EventKind = iota
	// EventThinking represents agent_thought_chunk events.
	EventThinking
	// EventMessage represents agent_message_chunk / user_message_chunk events.
	EventMessage
	// EventPlan represents plan events.
	EventPlan
	// EventToolCall represents tool_call events (pre-execution announcement).
	EventToolCall
	// EventToolUpdate represents tool_call_update events.
	EventToolUpdate
	// EventPermissionRequest represents session/request_permission events.
	EventPermissionRequest
	// EventTurnStart represents turn start events.
	EventTurnStart
	// EventTurnEnd represents turn end events.
	EventTurnEnd
)

// String returns a human-readable name for the event kind.
func (k EventKind) String() string {
	switch k {
	case EventThinking:
		return "thinking"
	case EventMessage:
		return "message"
	case EventPlan:
		return "plan"
	case EventToolCall:
		return "tool_call"
	case EventToolUpdate:
		return "tool_update"
	case EventPermissionRequest:
		return "permission_request"
	case EventTurnStart:
		return "turn_start"
	case EventTurnEnd:
		return "turn_end"
	default:
		return "unknown"
	}
}

// Event represents a classified event from the ACP stream.
type Event struct {
	// Kind categorizes the event type.
	Kind EventKind
	// ACPSessionID is the session ID from the ACP agent.
	ACPSessionID string
	// Raw preserves the original message for downstream processing.
	Raw json.RawMessage
	// ToolName is the tool name for tool_call / permission_request events.
	ToolName string
	// ToolArgs contains the tool parameters for tool_call / permission_request events.
	ToolArgs json.RawMessage
	// Command is the shell command extracted from args (for bash/shell tools).
	Command string
	// Cwd is the working directory for shell commands.
	Cwd string
	// RequestID is the permission request ID for permission_request events.
	RequestID string
}

// toolCallParams represents the expected structure for tool_call update parameters.
type toolCallParams struct {
	Tool       string          `json:"tool"`
	Parameters json.RawMessage `json:"parameters"`
}

// shellToolArgs represents parameters for bash/shell tool calls.
type shellToolArgs struct {
	Command string `json:"command"`
	Cwd     string `json:"cwd"`
	Workdir string `json:"workdir"` // Alternative field name
}

// Classify converts a raw Message into a classified Event.
// Unknown payloads do not panic; Raw is preserved for all events.
func Classify(msg Message) Event {
	ev := Event{
		Kind: EventUnknown,
	}

	// Marshal the entire message as raw for downstream use
	rawBytes, err := json.Marshal(msg)
	if err == nil {
		ev.Raw = rawBytes
	}

	// Check for permission request first (it's a request, not a notification)
	if msg.Method == MethodRequestPermission {
		return classifyPermissionRequest(msg, ev)
	}

	// Only process session/update notifications
	if msg.Method != MethodSessionUpdate {
		return ev
	}

	update, err := msg.ParseSessionUpdate()
	if err != nil {
		return ev
	}

	ev.ACPSessionID = update.SessionID

	switch update.Update.Kind {
	case UpdateKindAgentThoughtChunk:
		ev.Kind = EventThinking
	case UpdateKindAgentMessageChunk, UpdateKindUserMessageChunk:
		ev.Kind = EventMessage
	case UpdateKindPlan:
		ev.Kind = EventPlan
	case UpdateKindToolCall:
		ev.Kind = EventToolCall
		extractToolCallInfo(&ev, update.Update.Raw)
	case UpdateKindToolCallUpdate:
		ev.Kind = EventToolUpdate
		extractToolCallInfo(&ev, update.Update.Raw)
	default:
		// Check for turn events or other known patterns
		ev.Kind = classifyByPattern(update.Update.Kind)
	}

	return ev
}

// classifyPermissionRequest handles session/request_permission messages.
func classifyPermissionRequest(msg Message, ev Event) Event {
	ev.Kind = EventPermissionRequest

	var req PermissionRequest
	if err := msg.ParseParams(&req); err != nil {
		return ev
	}

	ev.ACPSessionID = req.SessionID
	ev.RequestID = req.RequestID
	ev.ToolName = req.Tool
	ev.ToolArgs = req.Parameters

	// Extract shell command info if applicable
	extractShellCommand(&ev, req.Parameters)

	return ev
}

// extractToolCallInfo extracts tool name and arguments from tool_call updates.
func extractToolCallInfo(ev *Event, raw json.RawMessage) {
	var params toolCallParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return
	}

	ev.ToolName = params.Tool
	ev.ToolArgs = params.Parameters

	// Extract shell command info if applicable
	extractShellCommand(ev, params.Parameters)
}

// extractShellCommand extracts command and working directory from shell tool parameters.
func extractShellCommand(ev *Event, params json.RawMessage) {
	if len(params) == 0 {
		return
	}

	// Only extract for known shell tools
	if !isShellTool(ev.ToolName) {
		return
	}

	var args shellToolArgs
	if err := json.Unmarshal(params, &args); err != nil {
		return
	}

	ev.Command = args.Command
	// Use workdir if cwd is empty
	if args.Cwd != "" {
		ev.Cwd = args.Cwd
	} else {
		ev.Cwd = args.Workdir
	}
}

// isShellTool returns true if the tool name indicates a shell/bash tool.
func isShellTool(name string) bool {
	switch name {
	case "bash", "shell", "Bash", "Shell", "execute", "Execute":
		return true
	default:
		return false
	}
}

// classifyByPattern attempts to classify events by common pattern matching.
func classifyByPattern(kind string) EventKind {
	switch kind {
	case "turn_start":
		return EventTurnStart
	case "turn_end":
		return EventTurnEnd
	default:
		return EventUnknown
	}
}

// agentMessageChunk represents the structure of an agent_message_chunk update.
type agentMessageChunk struct {
	SessionUpdate string `json:"sessionUpdate"`
	Content       string `json:"content"`
	Text          string `json:"text"` // Alternative field name
}

// ExtractMessageContent extracts the text content from an agent message event.
// Returns empty string if the event is not a message event or content cannot be extracted.
func ExtractMessageContent(ev Event) string {
	if ev.Kind != EventMessage {
		return ""
	}

	// Try to parse the raw update body from the session update
	var update SessionUpdate
	if err := json.Unmarshal(ev.Raw, &update); err != nil {
		// Try parsing as just the update body
		var msgChunk agentMessageChunk
		if err := json.Unmarshal(ev.Raw, &msgChunk); err != nil {
			return ""
		}
		if msgChunk.Content != "" {
			return msgChunk.Content
		}
		return msgChunk.Text
	}

	// Parse the update body for content
	var msgChunk agentMessageChunk
	if err := json.Unmarshal(update.Update.Raw, &msgChunk); err != nil {
		return ""
	}
	if msgChunk.Content != "" {
		return msgChunk.Content
	}
	return msgChunk.Text
}

// ContainsMemoryOverrideTag checks if the event contains the memory override tag.
// This is used to detect when the agent intentionally overrides project preferences.
func ContainsMemoryOverrideTag(ev Event) bool {
	if ev.Kind != EventMessage {
		return false
	}

	content := ExtractMessageContent(ev)
	return strings.Contains(content, MemoryOverrideTag)
}
