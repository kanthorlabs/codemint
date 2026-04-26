package acp

import (
	"encoding/json"
	"testing"
)

func TestEventKind_String(t *testing.T) {
	tests := []struct {
		kind EventKind
		want string
	}{
		{EventUnknown, "unknown"},
		{EventThinking, "thinking"},
		{EventMessage, "message"},
		{EventPlan, "plan"},
		{EventToolCall, "tool_call"},
		{EventToolUpdate, "tool_update"},
		{EventPermissionRequest, "permission_request"},
		{EventTurnStart, "turn_start"},
		{EventTurnEnd, "turn_end"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.kind.String(); got != tt.want {
				t.Errorf("EventKind.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name         string
		msg          Message
		wantKind     EventKind
		wantSession  string
		wantToolName string
		wantCommand  string
		wantCwd      string
		wantReqID    string
		skipRawCheck bool // Raw cannot be preserved for invalid JSON
	}{
		{
			name: "agent_thought_chunk",
			msg: Message{
				JSONRPC: JSONRPCVersion,
				Method:  MethodSessionUpdate,
				Params:  json.RawMessage(`{"sessionId":"sess-123","update":{"sessionUpdate":"agent_thought_chunk","content":"thinking..."}}`),
			},
			wantKind:    EventThinking,
			wantSession: "sess-123",
		},
		{
			name: "agent_message_chunk",
			msg: Message{
				JSONRPC: JSONRPCVersion,
				Method:  MethodSessionUpdate,
				Params:  json.RawMessage(`{"sessionId":"sess-456","update":{"sessionUpdate":"agent_message_chunk","content":"Hello"}}`),
			},
			wantKind:    EventMessage,
			wantSession: "sess-456",
		},
		{
			name: "user_message_chunk",
			msg: Message{
				JSONRPC: JSONRPCVersion,
				Method:  MethodSessionUpdate,
				Params:  json.RawMessage(`{"sessionId":"sess-789","update":{"sessionUpdate":"user_message_chunk","content":"Hi"}}`),
			},
			wantKind:    EventMessage,
			wantSession: "sess-789",
		},
		{
			name: "plan",
			msg: Message{
				JSONRPC: JSONRPCVersion,
				Method:  MethodSessionUpdate,
				Params:  json.RawMessage(`{"sessionId":"sess-plan","update":{"sessionUpdate":"plan","steps":[]}}`),
			},
			wantKind:    EventPlan,
			wantSession: "sess-plan",
		},
		{
			name: "tool_call_basic",
			msg: Message{
				JSONRPC: JSONRPCVersion,
				Method:  MethodSessionUpdate,
				Params:  json.RawMessage(`{"sessionId":"sess-tool","update":{"sessionUpdate":"tool_call","tool":"read","parameters":{"path":"/foo"}}}`),
			},
			wantKind:     EventToolCall,
			wantSession:  "sess-tool",
			wantToolName: "read",
		},
		{
			name: "tool_call_bash_with_command",
			msg: Message{
				JSONRPC: JSONRPCVersion,
				Method:  MethodSessionUpdate,
				Params:  json.RawMessage(`{"sessionId":"sess-bash","update":{"sessionUpdate":"tool_call","tool":"bash","parameters":{"command":"ls -la","cwd":"/home/user"}}}`),
			},
			wantKind:     EventToolCall,
			wantSession:  "sess-bash",
			wantToolName: "bash",
			wantCommand:  "ls -la",
			wantCwd:      "/home/user",
		},
		{
			name: "tool_call_shell_with_workdir",
			msg: Message{
				JSONRPC: JSONRPCVersion,
				Method:  MethodSessionUpdate,
				Params:  json.RawMessage(`{"sessionId":"sess-shell","update":{"sessionUpdate":"tool_call","tool":"shell","parameters":{"command":"npm install","workdir":"/app"}}}`),
			},
			wantKind:     EventToolCall,
			wantSession:  "sess-shell",
			wantToolName: "shell",
			wantCommand:  "npm install",
			wantCwd:      "/app",
		},
		{
			name: "tool_call_update",
			msg: Message{
				JSONRPC: JSONRPCVersion,
				Method:  MethodSessionUpdate,
				Params:  json.RawMessage(`{"sessionId":"sess-update","update":{"sessionUpdate":"tool_call_update","tool":"bash","parameters":{"command":"echo done"}}}`),
			},
			wantKind:     EventToolUpdate,
			wantSession:  "sess-update",
			wantToolName: "bash",
			wantCommand:  "echo done",
		},
		{
			name: "permission_request",
			msg: Message{
				JSONRPC: JSONRPCVersion,
				ID:      json.RawMessage(`1`),
				Method:  MethodRequestPermission,
				Params:  json.RawMessage(`{"sessionId":"sess-perm","requestId":"req-001","tool":"bash","parameters":{"command":"rm -rf /tmp","cwd":"/home"}}`),
			},
			wantKind:     EventPermissionRequest,
			wantSession:  "sess-perm",
			wantToolName: "bash",
			wantCommand:  "rm -rf /tmp",
			wantCwd:      "/home",
			wantReqID:    "req-001",
		},
		{
			name: "turn_start",
			msg: Message{
				JSONRPC: JSONRPCVersion,
				Method:  MethodSessionUpdate,
				Params:  json.RawMessage(`{"sessionId":"sess-turn","update":{"sessionUpdate":"turn_start"}}`),
			},
			wantKind:    EventTurnStart,
			wantSession: "sess-turn",
		},
		{
			name: "turn_end",
			msg: Message{
				JSONRPC: JSONRPCVersion,
				Method:  MethodSessionUpdate,
				Params:  json.RawMessage(`{"sessionId":"sess-turn","update":{"sessionUpdate":"turn_end"}}`),
			},
			wantKind:    EventTurnEnd,
			wantSession: "sess-turn",
		},
		{
			name: "unknown_method",
			msg: Message{
				JSONRPC: JSONRPCVersion,
				Method:  "some/other_method",
				Params:  json.RawMessage(`{}`),
			},
			wantKind: EventUnknown,
		},
		{
			name: "unknown_update_kind",
			msg: Message{
				JSONRPC: JSONRPCVersion,
				Method:  MethodSessionUpdate,
				Params:  json.RawMessage(`{"sessionId":"sess-unk","update":{"sessionUpdate":"custom_event","data":{}}}`),
			},
			wantKind:    EventUnknown,
			wantSession: "sess-unk",
		},
		{
			name: "invalid_params_json",
			msg: Message{
				JSONRPC: JSONRPCVersion,
				Method:  MethodSessionUpdate,
				Params:  json.RawMessage(`{invalid json`),
			},
			wantKind:     EventUnknown,
			skipRawCheck: true, // json.Marshal fails for invalid JSON in Params
		},
		{
			name:     "empty_message",
			msg:      Message{},
			wantKind: EventUnknown,
		},
		{
			name: "response_message_ignored",
			msg: Message{
				JSONRPC: JSONRPCVersion,
				ID:      json.RawMessage(`1`),
				Result:  json.RawMessage(`{"success":true}`),
			},
			wantKind: EventUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.msg)

			if got.Kind != tt.wantKind {
				t.Errorf("Kind = %v, want %v", got.Kind, tt.wantKind)
			}
			if got.ACPSessionID != tt.wantSession {
				t.Errorf("ACPSessionID = %v, want %v", got.ACPSessionID, tt.wantSession)
			}
			if got.ToolName != tt.wantToolName {
				t.Errorf("ToolName = %v, want %v", got.ToolName, tt.wantToolName)
			}
			if got.Command != tt.wantCommand {
				t.Errorf("Command = %v, want %v", got.Command, tt.wantCommand)
			}
			if got.Cwd != tt.wantCwd {
				t.Errorf("Cwd = %v, want %v", got.Cwd, tt.wantCwd)
			}
			if got.RequestID != tt.wantReqID {
				t.Errorf("RequestID = %v, want %v", got.RequestID, tt.wantReqID)
			}

			// Verify Raw is preserved for valid cases
			if !tt.skipRawCheck && got.Raw == nil {
				t.Error("Raw should be preserved for valid messages")
			}
		})
	}
}

func TestClassify_RawPreserved(t *testing.T) {
	msg := Message{
		JSONRPC: JSONRPCVersion,
		Method:  MethodSessionUpdate,
		Params:  json.RawMessage(`{"sessionId":"sess-raw","update":{"sessionUpdate":"agent_message_chunk","content":"test"}}`),
	}

	ev := Classify(msg)

	if ev.Raw == nil {
		t.Fatal("Raw should not be nil")
	}

	// Verify we can unmarshal the raw back to a message
	var restored Message
	if err := json.Unmarshal(ev.Raw, &restored); err != nil {
		t.Fatalf("failed to unmarshal Raw: %v", err)
	}

	if restored.Method != msg.Method {
		t.Errorf("restored Method = %v, want %v", restored.Method, msg.Method)
	}
}

func TestClassify_NoPanicOnMalformedInput(t *testing.T) {
	// This test ensures Classify does not panic on various malformed inputs
	testCases := []Message{
		{},
		{Method: MethodSessionUpdate},
		{Method: MethodSessionUpdate, Params: nil},
		{Method: MethodSessionUpdate, Params: json.RawMessage(`null`)},
		{Method: MethodSessionUpdate, Params: json.RawMessage(`[]`)},
		{Method: MethodSessionUpdate, Params: json.RawMessage(`"string"`)},
		{Method: MethodRequestPermission, Params: json.RawMessage(`{invalid}`)},
	}

	for i, msg := range testCases {
		// Should not panic
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("case %d: Classify panicked: %v", i, r)
				}
			}()
			_ = Classify(msg)
		}()
	}
}

func TestIsShellTool(t *testing.T) {
	shellTools := []string{"bash", "shell", "Bash", "Shell", "execute", "Execute"}
	nonShellTools := []string{"read", "write", "grep", "search", ""}

	for _, tool := range shellTools {
		if !isShellTool(tool) {
			t.Errorf("isShellTool(%q) = false, want true", tool)
		}
	}

	for _, tool := range nonShellTools {
		if isShellTool(tool) {
			t.Errorf("isShellTool(%q) = true, want false", tool)
		}
	}
}
