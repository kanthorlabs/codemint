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
		ServerInfo: ServerInfo{Name: "opencode", Version: "1.0.0"},
		Capabilities: ServerCaps{
			Streaming: true,
			ToolCalls: true,
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
	if parsedResult.ServerInfo.Name != "opencode" {
		t.Errorf("ServerInfo.Name = %q; want %q", parsedResult.ServerInfo.Name, "opencode")
	}
	if !parsedResult.Capabilities.Streaming {
		t.Error("Capabilities.Streaming = false; want true")
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
		Prompt:    "explain this code",
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
	if parsedParams.Prompt != "explain this code" {
		t.Errorf("Prompt = %q; want %q", parsedParams.Prompt, "explain this code")
	}
}
