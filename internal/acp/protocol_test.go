package acp

import (
	"encoding/json"
	"testing"
)

func TestMessage_RoundTrip_Request(t *testing.T) {
	params := map[string]string{"key": "value"}
	msg, err := NewRequest(MethodInitialize, params)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.JSONRPC != JSONRPCVersion {
		t.Errorf("JSONRPC = %q; want %q", decoded.JSONRPC, JSONRPCVersion)
	}
	if decoded.Method != MethodInitialize {
		t.Errorf("Method = %q; want %q", decoded.Method, MethodInitialize)
	}
	if !decoded.IsRequest() {
		t.Error("IsRequest() = false; want true")
	}
	if decoded.IsResponse() {
		t.Error("IsResponse() = true; want false")
	}
	if decoded.IsNotification() {
		t.Error("IsNotification() = true; want false")
	}
}

func TestMessage_RoundTrip_Response(t *testing.T) {
	result := InitializeResult{
		ProtocolVersion: 1,
		AgentInfo:       &Implementation{Name: "opencode", Version: "1.0.0"},
		AgentCapabilities: AgentCapabilities{
			LoadSession: true,
		},
	}
	msg, err := NewResponseWithIntID(42, result)
	if err != nil {
		t.Fatalf("NewResponseWithIntID: %v", err)
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.JSONRPC != JSONRPCVersion {
		t.Errorf("JSONRPC = %q; want %q", decoded.JSONRPC, JSONRPCVersion)
	}
	if decoded.GetID() != 42 {
		t.Errorf("GetID() = %d; want 42", decoded.GetID())
	}
	if !decoded.IsResponse() {
		t.Error("IsResponse() = false; want true")
	}

	var parsedResult InitializeResult
	if err := decoded.ParseResult(&parsedResult); err != nil {
		t.Fatalf("ParseResult: %v", err)
	}
	if parsedResult.AgentInfo == nil || parsedResult.AgentInfo.Name != "opencode" {
		t.Errorf("AgentInfo.Name = %q; want %q", parsedResult.AgentInfo.Name, "opencode")
	}
	if !parsedResult.AgentCapabilities.LoadSession {
		t.Error("AgentCapabilities.LoadSession = false; want true")
	}
}

func TestMessage_RoundTrip_Notification(t *testing.T) {
	update := SessionUpdate{
		SessionID: "sess-123",
		Update: UpdateBody{
			Kind: UpdateKindAgentMessageChunk,
		},
	}
	msg, err := NewNotification(MethodSessionUpdate, update)
	if err != nil {
		t.Fatalf("NewNotification: %v", err)
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.JSONRPC != JSONRPCVersion {
		t.Errorf("JSONRPC = %q; want %q", decoded.JSONRPC, JSONRPCVersion)
	}
	if decoded.Method != MethodSessionUpdate {
		t.Errorf("Method = %q; want %q", decoded.Method, MethodSessionUpdate)
	}
	if !decoded.IsNotification() {
		t.Error("IsNotification() = false; want true")
	}

	parsedUpdate, err := decoded.ParseSessionUpdate()
	if err != nil {
		t.Fatalf("ParseSessionUpdate: %v", err)
	}
	if parsedUpdate.SessionID != "sess-123" {
		t.Errorf("SessionID = %q; want %q", parsedUpdate.SessionID, "sess-123")
	}
	if parsedUpdate.Update.Kind != UpdateKindAgentMessageChunk {
		t.Errorf("Update.Kind = %q; want %q", parsedUpdate.Update.Kind, UpdateKindAgentMessageChunk)
	}
}

func TestMessage_RoundTrip_Error(t *testing.T) {
	idBytes, _ := json.Marshal(99)
	msg := NewError(idBytes, ErrCodeInvalidParams, "invalid params")

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Error == nil {
		t.Fatal("Error = nil; want non-nil")
	}
	if decoded.Error.Code != ErrCodeInvalidParams {
		t.Errorf("Error.Code = %d; want %d", decoded.Error.Code, ErrCodeInvalidParams)
	}
	if decoded.Error.Message != "invalid params" {
		t.Errorf("Error.Message = %q; want %q", decoded.Error.Message, "invalid params")
	}
}

func TestUpdateBody_PreservesRaw(t *testing.T) {
	raw := `{"sessionUpdate":"agent_message_chunk","content":"hello","extra":true}`
	
	var body UpdateBody
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if body.Kind != UpdateKindAgentMessageChunk {
		t.Errorf("Kind = %q; want %q", body.Kind, UpdateKindAgentMessageChunk)
	}
	if string(body.Raw) != raw {
		t.Errorf("Raw = %q; want %q", string(body.Raw), raw)
	}
}

func TestUpdateBody_UnknownKind_ParsesWithoutError(t *testing.T) {
	raw := `{"sessionUpdate":"unknown_future_kind","data":"something"}`
	
	var body UpdateBody
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("Unmarshal unknown kind: %v", err)
	}

	if body.Kind != "unknown_future_kind" {
		t.Errorf("Kind = %q; want %q", body.Kind, "unknown_future_kind")
	}
	if string(body.Raw) != raw {
		t.Errorf("Raw not preserved for unknown kind")
	}
}

func TestNewRequest_UniqueIDs(t *testing.T) {
	msg1, _ := NewRequest("test", nil)
	msg2, _ := NewRequest("test", nil)

	id1 := msg1.GetID()
	id2 := msg2.GetID()

	if id1 == id2 {
		t.Errorf("requests have same ID: %d", id1)
	}
	if id2 <= id1 {
		t.Errorf("ID not incrementing: %d <= %d", id2, id1)
	}
}

func TestRPCError_Error(t *testing.T) {
	tests := []struct {
		name    string
		err     RPCError
		wantMsg string
	}{
		{
			name:    "without data",
			err:     RPCError{Code: -32600, Message: "invalid request"},
			wantMsg: "rpc error -32600: invalid request",
		},
		{
			name:    "with data",
			err:     RPCError{Code: -32600, Message: "invalid request", Data: json.RawMessage(`{"detail":"missing id"}`)},
			wantMsg: `rpc error -32600: invalid request (data: {"detail":"missing id"})`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.wantMsg {
				t.Errorf("Error() = %q; want %q", got, tt.wantMsg)
			}
		})
	}
}

func TestMessage_ParseResult_WithError(t *testing.T) {
	idBytes, _ := json.Marshal(1)
	msg := NewError(idBytes, ErrCodeInternalError, "internal error")

	var result any
	err := msg.ParseResult(&result)
	if err == nil {
		t.Error("ParseResult should return error when message has Error")
	}
	rpcErr, ok := err.(*RPCError)
	if !ok {
		t.Errorf("error should be *RPCError, got %T", err)
	}
	if rpcErr.Code != ErrCodeInternalError {
		t.Errorf("error code = %d; want %d", rpcErr.Code, ErrCodeInternalError)
	}
}

func TestSessionPromptParams_RoundTrip(t *testing.T) {
	params := SessionPromptParams{
		SessionID: "sess-abc",
		Prompt:    []ContentBlock{TextContent("explain this code")},
	}
	msg, err := NewRequest(MethodSessionPrompt, params)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	var parsedParams SessionPromptParams
	if err := decoded.ParseParams(&parsedParams); err != nil {
		t.Fatalf("ParseParams: %v", err)
	}

	if parsedParams.SessionID != "sess-abc" {
		t.Errorf("SessionID = %q; want %q", parsedParams.SessionID, "sess-abc")
	}
	if len(parsedParams.Prompt) != 1 || parsedParams.Prompt[0].Type != "text" || parsedParams.Prompt[0].Text != "explain this code" {
		t.Errorf("Prompt = %v; want [{Type: text, Text: explain this code}]", parsedParams.Prompt)
	}
}

// ============================================================================
// Round-trip tests for every typed payload per ACP spec
// ============================================================================

func TestInitializeParams_RoundTrip(t *testing.T) {
	params := InitializeParams{
		ProtocolVersion: 1,
		ClientInfo:      &Implementation{Name: "test-client", Title: "Test Client", Version: "1.0.0"},
		ClientCapabilities: ClientCapabilities{
			FS:       &FSCapabilities{ReadTextFile: true, WriteTextFile: true},
			Terminal: true,
		},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded InitializeParams
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.ProtocolVersion != params.ProtocolVersion {
		t.Errorf("ProtocolVersion = %d; want %d", decoded.ProtocolVersion, params.ProtocolVersion)
	}
	if decoded.ClientInfo == nil || decoded.ClientInfo.Name != "test-client" {
		t.Errorf("ClientInfo.Name = %v; want test-client", decoded.ClientInfo)
	}
	if decoded.ClientCapabilities.FS == nil || !decoded.ClientCapabilities.FS.ReadTextFile {
		t.Errorf("ClientCapabilities.FS.ReadTextFile = false; want true")
	}
	if !decoded.ClientCapabilities.Terminal {
		t.Errorf("ClientCapabilities.Terminal = false; want true")
	}
}

func TestInitializeResult_RoundTrip(t *testing.T) {
	result := InitializeResult{
		ProtocolVersion: 1,
		AgentInfo:       &Implementation{Name: "test-agent", Version: "2.0.0"},
		AgentCapabilities: AgentCapabilities{
			LoadSession:        true,
			McpCapabilities:    &McpCapabilities{HTTP: true, SSE: false},
			PromptCapabilities: &PromptCapabilities{Image: true, Audio: false, EmbeddedContext: true},
		},
		AuthMethods: []AuthMethod{{ID: "api-key", Name: "API Key", Description: "Use API key"}},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded InitializeResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.ProtocolVersion != result.ProtocolVersion {
		t.Errorf("ProtocolVersion = %d; want %d", decoded.ProtocolVersion, result.ProtocolVersion)
	}
	if !decoded.AgentCapabilities.LoadSession {
		t.Errorf("AgentCapabilities.LoadSession = false; want true")
	}
	if decoded.AgentCapabilities.McpCapabilities == nil || !decoded.AgentCapabilities.McpCapabilities.HTTP {
		t.Errorf("AgentCapabilities.McpCapabilities.HTTP = false; want true")
	}
	if len(decoded.AuthMethods) != 1 || decoded.AuthMethods[0].ID != "api-key" {
		t.Errorf("AuthMethods = %v; want [{ID: api-key, ...}]", decoded.AuthMethods)
	}
}

func TestSessionNewParams_RoundTrip(t *testing.T) {
	params := SessionNewParams{
		Cwd: "/home/user/project",
		McpServers: []McpServer{
			{
				Name:    "filesystem",
				Command: "/path/to/mcp-server",
				Args:    []string{"--stdio"},
				Env:     []McpEnvVariable{{Name: "API_KEY", Value: "secret"}},
			},
		},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded SessionNewParams
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Cwd != params.Cwd {
		t.Errorf("Cwd = %q; want %q", decoded.Cwd, params.Cwd)
	}
	if len(decoded.McpServers) != 1 || decoded.McpServers[0].Name != "filesystem" {
		t.Errorf("McpServers = %v; want [{Name: filesystem, ...}]", decoded.McpServers)
	}
}

func TestSessionNewResult_RoundTrip(t *testing.T) {
	result := SessionNewResult{
		SessionID: "sess-xyz789",
		ConfigOptions: []SessionConfigOption{
			{ID: "model", Label: "Model", Values: []SessionConfigOptionValue{{ID: "gpt-4", Label: "GPT-4"}}, CurrentID: "gpt-4"},
		},
		Modes: &SessionModeState{
			AvailableModes: []SessionMode{{ID: "code", Name: "Code Mode"}},
			CurrentID:      "code",
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded SessionNewResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.SessionID != result.SessionID {
		t.Errorf("SessionID = %q; want %q", decoded.SessionID, result.SessionID)
	}
	if len(decoded.ConfigOptions) != 1 {
		t.Errorf("ConfigOptions length = %d; want 1", len(decoded.ConfigOptions))
	}
	if decoded.Modes == nil || decoded.Modes.CurrentID != "code" {
		t.Errorf("Modes.CurrentID = %v; want code", decoded.Modes)
	}
}

func TestSessionPromptResult_RoundTrip(t *testing.T) {
	result := SessionPromptResult{
		StopReason: StopReasonEndTurn,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded SessionPromptResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.StopReason != StopReasonEndTurn {
		t.Errorf("StopReason = %q; want %q", decoded.StopReason, StopReasonEndTurn)
	}
}

func TestContentBlock_AllTypes_RoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		block ContentBlock
	}{
		{
			name:  "text",
			block: TextContent("Hello world"),
		},
		{
			name:  "resource_link",
			block: ResourceLinkContent("file:///home/user/doc.pdf", "doc.pdf", "application/pdf"),
		},
		{
			name: "embedded_resource",
			block: EmbeddedResourceContent("file:///script.py", "text/x-python", "print('hello')"),
		},
		{
			name: "image",
			block: ContentBlock{
				Type:     "image",
				MimeType: "image/png",
				Data:     "iVBORw0KGgoAAAANSUhEUgAAAAE=",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.block)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}

			var decoded ContentBlock
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			if decoded.Type != tt.block.Type {
				t.Errorf("Type = %q; want %q", decoded.Type, tt.block.Type)
			}
		})
	}
}

func TestRequestPermissionParams_RoundTrip(t *testing.T) {
	params := RequestPermissionParams{
		SessionID: "sess-perm-test",
		ToolCall: ToolCallUpdate{
			ToolCallID: "call-123",
			Title:      "Execute bash command",
			Kind:       "execute",
			Status:     ToolCallStatusPending,
			RawInput:   json.RawMessage(`{"command":"ls -la"}`),
		},
		Options: []PermissionOption{
			{OptionID: "allow_once", Name: "Allow once", Kind: PermissionKindAllowOnce},
			{OptionID: "reject_once", Name: "Reject", Kind: PermissionKindRejectOnce},
		},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded RequestPermissionParams
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.SessionID != params.SessionID {
		t.Errorf("SessionID = %q; want %q", decoded.SessionID, params.SessionID)
	}
	if decoded.ToolCall.ToolCallID != "call-123" {
		t.Errorf("ToolCall.ToolCallID = %q; want call-123", decoded.ToolCall.ToolCallID)
	}
	if len(decoded.Options) != 2 {
		t.Errorf("Options length = %d; want 2", len(decoded.Options))
	}
	if decoded.Options[0].Kind != PermissionKindAllowOnce {
		t.Errorf("Options[0].Kind = %q; want %q", decoded.Options[0].Kind, PermissionKindAllowOnce)
	}
}

func TestRequestPermissionResult_RoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		result  RequestPermissionResult
		wantOut string
	}{
		{
			name:    "cancelled",
			result:  RequestPermissionResult{Outcome: CancelledOutcome()},
			wantOut: "cancelled",
		},
		{
			name:    "selected",
			result:  RequestPermissionResult{Outcome: SelectedOutcome("allow_once")},
			wantOut: "selected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.result)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}

			var decoded RequestPermissionResult
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			if decoded.Outcome.Outcome != tt.wantOut {
				t.Errorf("Outcome.Outcome = %q; want %q", decoded.Outcome.Outcome, tt.wantOut)
			}
		})
	}
}

// ============================================================================
// Golden fixture tests: decode verbatim spec examples → re-encode → verify
// ============================================================================

func TestGoldenFixture_Initialize(t *testing.T) {
	// From ACP spec: https://agentclientprotocol.com/protocol/initialization
	requestJSON := `{
		"jsonrpc": "2.0",
		"id": 0,
		"method": "initialize",
		"params": {
			"protocolVersion": 1,
			"clientCapabilities": {
				"fs": {
					"readTextFile": true,
					"writeTextFile": true
				},
				"terminal": true
			},
			"clientInfo": {
				"name": "my-client",
				"version": "1.0.0"
			}
		}
	}`

	var msg Message
	if err := json.Unmarshal([]byte(requestJSON), &msg); err != nil {
		t.Fatalf("Unmarshal request: %v", err)
	}

	if msg.Method != MethodInitialize {
		t.Errorf("Method = %q; want initialize", msg.Method)
	}

	var params InitializeParams
	if err := msg.ParseParams(&params); err != nil {
		t.Fatalf("ParseParams: %v", err)
	}

	if params.ProtocolVersion != 1 {
		t.Errorf("ProtocolVersion = %d; want 1", params.ProtocolVersion)
	}
	if params.ClientInfo == nil || params.ClientInfo.Name != "my-client" {
		t.Errorf("ClientInfo.Name = %v; want my-client", params.ClientInfo)
	}
	if params.ClientCapabilities.FS == nil || !params.ClientCapabilities.FS.ReadTextFile {
		t.Error("ClientCapabilities.FS.ReadTextFile = false; want true")
	}
}

func TestGoldenFixture_InitializeResponse(t *testing.T) {
	// From ACP spec
	responseJSON := `{
		"jsonrpc": "2.0",
		"id": 0,
		"result": {
			"protocolVersion": 1,
			"agentCapabilities": {
				"loadSession": true,
				"promptCapabilities": {
					"image": true,
					"audio": true,
					"embeddedContext": true
				},
				"mcpCapabilities": {
					"http": true,
					"sse": true
				}
			},
			"agentInfo": {
				"name": "my-agent",
				"version": "1.0.0"
			},
			"authMethods": []
		}
	}`

	var msg Message
	if err := json.Unmarshal([]byte(responseJSON), &msg); err != nil {
		t.Fatalf("Unmarshal response: %v", err)
	}

	var result InitializeResult
	if err := msg.ParseResult(&result); err != nil {
		t.Fatalf("ParseResult: %v", err)
	}

	if result.ProtocolVersion != 1 {
		t.Errorf("ProtocolVersion = %d; want 1", result.ProtocolVersion)
	}
	if result.AgentInfo == nil || result.AgentInfo.Name != "my-agent" {
		t.Errorf("AgentInfo.Name = %v; want my-agent", result.AgentInfo)
	}
	if !result.AgentCapabilities.LoadSession {
		t.Error("AgentCapabilities.LoadSession = false; want true")
	}
	if result.AgentCapabilities.McpCapabilities == nil || !result.AgentCapabilities.McpCapabilities.HTTP {
		t.Error("AgentCapabilities.McpCapabilities.HTTP = false; want true")
	}
}

func TestGoldenFixture_SessionNew(t *testing.T) {
	// From ACP spec: https://agentclientprotocol.com/protocol/session-setup
	requestJSON := `{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "session/new",
		"params": {
			"cwd": "/home/user/project",
			"mcpServers": [
				{
					"name": "filesystem",
					"command": "/path/to/mcp-server",
					"args": ["--stdio"],
					"env": []
				}
			]
		}
	}`

	var msg Message
	if err := json.Unmarshal([]byte(requestJSON), &msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	var params SessionNewParams
	if err := msg.ParseParams(&params); err != nil {
		t.Fatalf("ParseParams: %v", err)
	}

	if params.Cwd != "/home/user/project" {
		t.Errorf("Cwd = %q; want /home/user/project", params.Cwd)
	}
	if len(params.McpServers) != 1 || params.McpServers[0].Name != "filesystem" {
		t.Errorf("McpServers = %v; want [{Name: filesystem, ...}]", params.McpServers)
	}
}

func TestGoldenFixture_SessionPrompt(t *testing.T) {
	// From ACP spec: https://agentclientprotocol.com/protocol/prompt-turn
	requestJSON := `{
		"jsonrpc": "2.0",
		"id": 2,
		"method": "session/prompt",
		"params": {
			"sessionId": "sess_abc123def456",
			"prompt": [
				{
					"type": "text",
					"text": "Can you analyze this code for potential issues?"
				},
				{
					"type": "resource",
					"resource": {
						"uri": "file:///home/user/project/main.py",
						"mimeType": "text/x-python",
						"text": "def process_data(items):\n    for item in items:\n        print(item)"
					}
				}
			]
		}
	}`

	var msg Message
	if err := json.Unmarshal([]byte(requestJSON), &msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	var params SessionPromptParams
	if err := msg.ParseParams(&params); err != nil {
		t.Fatalf("ParseParams: %v", err)
	}

	if params.SessionID != "sess_abc123def456" {
		t.Errorf("SessionID = %q; want sess_abc123def456", params.SessionID)
	}
	if len(params.Prompt) != 2 {
		t.Fatalf("Prompt length = %d; want 2", len(params.Prompt))
	}
	if params.Prompt[0].Type != "text" {
		t.Errorf("Prompt[0].Type = %q; want text", params.Prompt[0].Type)
	}
	if params.Prompt[1].Type != "resource" {
		t.Errorf("Prompt[1].Type = %q; want resource", params.Prompt[1].Type)
	}
	if params.Prompt[1].Resource == nil || params.Prompt[1].Resource.URI != "file:///home/user/project/main.py" {
		t.Errorf("Prompt[1].Resource.URI = %v; want file:///home/user/project/main.py", params.Prompt[1].Resource)
	}
}

func TestGoldenFixture_SessionCancel(t *testing.T) {
	// From ACP spec
	notificationJSON := `{
		"jsonrpc": "2.0",
		"method": "session/cancel",
		"params": {
			"sessionId": "sess_abc123def456"
		}
	}`

	var msg Message
	if err := json.Unmarshal([]byte(notificationJSON), &msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if msg.Method != MethodSessionCancel {
		t.Errorf("Method = %q; want session/cancel", msg.Method)
	}

	var params SessionCancelParams
	if err := msg.ParseParams(&params); err != nil {
		t.Fatalf("ParseParams: %v", err)
	}

	if params.SessionID != "sess_abc123def456" {
		t.Errorf("SessionID = %q; want sess_abc123def456", params.SessionID)
	}
}

func TestGoldenFixture_SessionUpdate(t *testing.T) {
	// From ACP spec: https://agentclientprotocol.com/protocol/prompt-turn
	notificationJSON := `{
		"jsonrpc": "2.0",
		"method": "session/update",
		"params": {
			"sessionId": "sess_abc123def456",
			"update": {
				"sessionUpdate": "agent_message_chunk",
				"content": {
					"type": "text",
					"text": "I'll analyze your code for potential issues. Let me examine it..."
				}
			}
		}
	}`

	var msg Message
	if err := json.Unmarshal([]byte(notificationJSON), &msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	update, err := msg.ParseSessionUpdate()
	if err != nil {
		t.Fatalf("ParseSessionUpdate: %v", err)
	}

	if update.SessionID != "sess_abc123def456" {
		t.Errorf("SessionID = %q; want sess_abc123def456", update.SessionID)
	}
	if update.Update.Kind != UpdateKindAgentMessageChunk {
		t.Errorf("Update.Kind = %q; want agent_message_chunk", update.Update.Kind)
	}
}

func TestGoldenFixture_RequestPermission(t *testing.T) {
	// From ACP spec: https://agentclientprotocol.com/protocol/tool-calls#requesting-permission
	requestJSON := `{
		"jsonrpc": "2.0",
		"id": 5,
		"method": "session/request_permission",
		"params": {
			"sessionId": "sess_abc123def456",
			"toolCall": {
				"toolCallId": "call_001"
			},
			"options": [
				{
					"optionId": "allow-once",
					"name": "Allow once",
					"kind": "allow_once"
				},
				{
					"optionId": "reject-once",
					"name": "Reject",
					"kind": "reject_once"
				}
			]
		}
	}`

	var msg Message
	if err := json.Unmarshal([]byte(requestJSON), &msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if msg.Method != MethodRequestPermission {
		t.Errorf("Method = %q; want session/request_permission", msg.Method)
	}

	var params RequestPermissionParams
	if err := msg.ParseParams(&params); err != nil {
		t.Fatalf("ParseParams: %v", err)
	}

	if params.SessionID != "sess_abc123def456" {
		t.Errorf("SessionID = %q; want sess_abc123def456", params.SessionID)
	}
	if params.ToolCall.ToolCallID != "call_001" {
		t.Errorf("ToolCall.ToolCallID = %q; want call_001", params.ToolCall.ToolCallID)
	}
	if len(params.Options) != 2 {
		t.Fatalf("Options length = %d; want 2", len(params.Options))
	}
	if params.Options[0].Kind != PermissionKindAllowOnce {
		t.Errorf("Options[0].Kind = %q; want allow_once", params.Options[0].Kind)
	}
}

func TestGoldenFixture_RequestPermissionResponse_Selected(t *testing.T) {
	// From ACP spec
	responseJSON := `{
		"jsonrpc": "2.0",
		"id": 5,
		"result": {
			"outcome": {
				"outcome": "selected",
				"optionId": "allow-once"
			}
		}
	}`

	var msg Message
	if err := json.Unmarshal([]byte(responseJSON), &msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	var result RequestPermissionResult
	if err := msg.ParseResult(&result); err != nil {
		t.Fatalf("ParseResult: %v", err)
	}

	if result.Outcome.Outcome != "selected" {
		t.Errorf("Outcome.Outcome = %q; want selected", result.Outcome.Outcome)
	}
	if result.Outcome.OptionID != "allow-once" {
		t.Errorf("Outcome.OptionID = %q; want allow-once", result.Outcome.OptionID)
	}
}

func TestGoldenFixture_RequestPermissionResponse_Cancelled(t *testing.T) {
	// From ACP spec
	responseJSON := `{
		"jsonrpc": "2.0",
		"id": 5,
		"result": {
			"outcome": {
				"outcome": "cancelled"
			}
		}
	}`

	var msg Message
	if err := json.Unmarshal([]byte(responseJSON), &msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	var result RequestPermissionResult
	if err := msg.ParseResult(&result); err != nil {
		t.Fatalf("ParseResult: %v", err)
	}

	if result.Outcome.Outcome != "cancelled" {
		t.Errorf("Outcome.Outcome = %q; want cancelled", result.Outcome.Outcome)
	}
}

// ============================================================================
// Tests for new ACP conformance types (T8-T13)
// ============================================================================

func TestRPCError_IsAuthRequired(t *testing.T) {
	tests := []struct {
		name string
		code int
		want bool
	}{
		{"auth required", ErrCodeAuthRequired, true},
		{"session not found", ErrCodeSessionNotFound, false},
		{"resource not found", ErrCodeResourceNotFound, false},
		{"invalid params", ErrCodeInvalidParams, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &RPCError{Code: tt.code, Message: "test"}
			if got := err.IsAuthRequired(); got != tt.want {
				t.Errorf("IsAuthRequired() = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestRPCError_IsSessionNotFound(t *testing.T) {
	err := &RPCError{Code: ErrCodeSessionNotFound, Message: "session not found"}
	if !err.IsSessionNotFound() {
		t.Error("IsSessionNotFound() = false; want true")
	}

	err2 := &RPCError{Code: ErrCodeAuthRequired, Message: "auth required"}
	if err2.IsSessionNotFound() {
		t.Error("IsSessionNotFound() = true; want false")
	}
}

func TestRPCError_IsResourceNotFound(t *testing.T) {
	err := &RPCError{Code: ErrCodeResourceNotFound, Message: "resource not found"}
	if !err.IsResourceNotFound() {
		t.Error("IsResourceNotFound() = false; want true")
	}
}

func TestPlanUpdate_RoundTrip(t *testing.T) {
	plan := PlanUpdate{
		SessionUpdate: UpdateKindPlan,
		Entries: []PlanEntry{
			{Content: "Implement feature", Priority: PlanEntryPriorityHigh, Status: PlanEntryStatusPending},
			{Content: "Write tests", Priority: PlanEntryPriorityMedium, Status: PlanEntryStatusInProgress},
			{Content: "Update docs", Priority: PlanEntryPriorityLow, Status: PlanEntryStatusCompleted},
		},
	}

	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded PlanUpdate
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.SessionUpdate != UpdateKindPlan {
		t.Errorf("SessionUpdate = %q; want %q", decoded.SessionUpdate, UpdateKindPlan)
	}
	if len(decoded.Entries) != 3 {
		t.Fatalf("Entries length = %d; want 3", len(decoded.Entries))
	}
	if decoded.Entries[0].Priority != PlanEntryPriorityHigh {
		t.Errorf("Entries[0].Priority = %q; want high", decoded.Entries[0].Priority)
	}
	if decoded.Entries[1].Status != PlanEntryStatusInProgress {
		t.Errorf("Entries[1].Status = %q; want in_progress", decoded.Entries[1].Status)
	}
}

func TestAvailableCommandsUpdate_RoundTrip(t *testing.T) {
	update := AvailableCommandsUpdate{
		SessionUpdate: UpdateKindAvailableCommandsUpdate,
		Commands: []SlashCommand{
			{Name: "/help", Description: "Show help information"},
			{Name: "/status", Description: "Show agent status"},
		},
	}

	data, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded AvailableCommandsUpdate
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.SessionUpdate != UpdateKindAvailableCommandsUpdate {
		t.Errorf("SessionUpdate = %q; want %q", decoded.SessionUpdate, UpdateKindAvailableCommandsUpdate)
	}
	if len(decoded.Commands) != 2 {
		t.Fatalf("Commands length = %d; want 2", len(decoded.Commands))
	}
	if decoded.Commands[0].Name != "/help" {
		t.Errorf("Commands[0].Name = %q; want /help", decoded.Commands[0].Name)
	}
}

func TestCurrentModeUpdate_RoundTrip(t *testing.T) {
	update := CurrentModeUpdate{
		SessionUpdate: UpdateKindCurrentModeUpdate,
		ModeID:        "code",
	}

	data, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded CurrentModeUpdate
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.ModeID != "code" {
		t.Errorf("ModeID = %q; want code", decoded.ModeID)
	}
}

func TestConfigOptionUpdate_RoundTrip(t *testing.T) {
	update := ConfigOptionUpdate{
		SessionUpdate: UpdateKindConfigOptionUpdate,
		OptionID:      "model",
		ValueID:       "gpt-4",
	}

	data, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded ConfigOptionUpdate
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.OptionID != "model" {
		t.Errorf("OptionID = %q; want model", decoded.OptionID)
	}
	if decoded.ValueID != "gpt-4" {
		t.Errorf("ValueID = %q; want gpt-4", decoded.ValueID)
	}
}

func TestSessionInfoUpdate_RoundTrip(t *testing.T) {
	update := SessionInfoUpdate{
		SessionUpdate: UpdateKindSessionInfoUpdate,
		ConfigOptions: []SessionConfigOption{
			{ID: "model", Label: "Model", Values: []SessionConfigOptionValue{{ID: "gpt-4", Label: "GPT-4"}}, CurrentID: "gpt-4"},
		},
		Modes: &SessionModeState{
			AvailableModes: []SessionMode{{ID: "code", Name: "Code Mode"}},
			CurrentID:      "code",
		},
	}

	data, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded SessionInfoUpdate
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(decoded.ConfigOptions) != 1 {
		t.Fatalf("ConfigOptions length = %d; want 1", len(decoded.ConfigOptions))
	}
	if decoded.Modes == nil || decoded.Modes.CurrentID != "code" {
		t.Errorf("Modes.CurrentID = %v; want code", decoded.Modes)
	}
}

// ============================================================================
// _meta passthrough tests (T13)
// ============================================================================

func TestMeta_Passthrough_InitializeResult(t *testing.T) {
	inputJSON := `{
		"protocolVersion": 1,
		"agentInfo": {"name": "test", "version": "1.0"},
		"agentCapabilities": {},
		"authMethods": [],
		"_meta": {"custom_field": "custom_value", "nested": {"key": 123}}
	}`

	var result InitializeResult
	if err := json.Unmarshal([]byte(inputJSON), &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if result.Meta == nil {
		t.Fatal("Meta is nil; want non-nil")
	}

	// Re-encode and verify _meta is preserved
	output, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Verify the _meta field is in the output
	if !json.Valid(output) {
		t.Fatal("Output is not valid JSON")
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatalf("Unmarshal decoded: %v", err)
	}

	meta, ok := decoded["_meta"].(map[string]interface{})
	if !ok {
		t.Fatal("_meta not found in output")
	}
	if meta["custom_field"] != "custom_value" {
		t.Errorf("_meta.custom_field = %v; want custom_value", meta["custom_field"])
	}
}

func TestMeta_Passthrough_SessionNewResult(t *testing.T) {
	inputJSON := `{
		"sessionId": "sess-123",
		"_meta": {"agent_version": "2.0.0"}
	}`

	var result SessionNewResult
	if err := json.Unmarshal([]byte(inputJSON), &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if result.Meta == nil {
		t.Fatal("Meta is nil; want non-nil")
	}

	output, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatalf("Unmarshal decoded: %v", err)
	}

	meta, ok := decoded["_meta"].(map[string]interface{})
	if !ok {
		t.Fatal("_meta not found in output")
	}
	if meta["agent_version"] != "2.0.0" {
		t.Errorf("_meta.agent_version = %v; want 2.0.0", meta["agent_version"])
	}
}

func TestMeta_Passthrough_SessionUpdate(t *testing.T) {
	inputJSON := `{
		"sessionId": "sess-456",
		"update": {"sessionUpdate": "agent_message_chunk"},
		"_meta": {"trace_id": "abc123"}
	}`

	var update SessionUpdate
	if err := json.Unmarshal([]byte(inputJSON), &update); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if update.Meta == nil {
		t.Fatal("Meta is nil; want non-nil")
	}

	output, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatalf("Unmarshal decoded: %v", err)
	}

	meta, ok := decoded["_meta"].(map[string]interface{})
	if !ok {
		t.Fatal("_meta not found in output")
	}
	if meta["trace_id"] != "abc123" {
		t.Errorf("_meta.trace_id = %v; want abc123", meta["trace_id"])
	}
}

func TestMeta_Passthrough_ToolCallUpdate(t *testing.T) {
	inputJSON := `{
		"toolCallId": "call-789",
		"title": "Execute command",
		"_meta": {"timing_ms": 150}
	}`

	var update ToolCallUpdate
	if err := json.Unmarshal([]byte(inputJSON), &update); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if update.Meta == nil {
		t.Fatal("Meta is nil; want non-nil")
	}

	output, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatalf("Unmarshal decoded: %v", err)
	}

	meta, ok := decoded["_meta"].(map[string]interface{})
	if !ok {
		t.Fatal("_meta not found in output")
	}
	if meta["timing_ms"] != float64(150) {
		t.Errorf("_meta.timing_ms = %v; want 150", meta["timing_ms"])
	}
}

func TestMeta_Passthrough_RequestPermissionParams(t *testing.T) {
	inputJSON := `{
		"sessionId": "sess-perm",
		"toolCall": {"toolCallId": "call-001"},
		"options": [],
		"_meta": {"priority": "high"}
	}`

	var params RequestPermissionParams
	if err := json.Unmarshal([]byte(inputJSON), &params); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if params.Meta == nil {
		t.Fatal("Meta is nil; want non-nil")
	}

	output, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatalf("Unmarshal decoded: %v", err)
	}

	meta, ok := decoded["_meta"].(map[string]interface{})
	if !ok {
		t.Fatal("_meta not found in output")
	}
	if meta["priority"] != "high" {
		t.Errorf("_meta.priority = %v; want high", meta["priority"])
	}
}
