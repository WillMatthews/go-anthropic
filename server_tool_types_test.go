package anthropic_test

import (
	"encoding/json"
	"testing"

	"github.com/liushuangls/go-anthropic/v2"
)

func TestUnmarshalWebFetchToolResult(t *testing.T) {
	data := `{"type":"web_fetch_tool_result","tool_use_id":"srvtoolu_1","content":{"type":"web_fetch_result","url":"https://example.com"}}`
	var mc anthropic.MessageContent
	if err := json.Unmarshal([]byte(data), &mc); err != nil {
		t.Fatalf("unmarshal error: %s", err)
	}
	if mc.Type != anthropic.MessagesContentTypeWebFetchToolResult {
		t.Fatalf("expected web_fetch_tool_result type, got %q", mc.Type)
	}
	if mc.WebFetchToolResult == nil {
		t.Fatalf("expected WebFetchToolResult populated, got nil")
	}
	if mc.WebFetchToolResult.ToolUseID == nil || *mc.WebFetchToolResult.ToolUseID != "srvtoolu_1" {
		t.Fatalf("unexpected tool_use_id: %+v", mc.WebFetchToolResult.ToolUseID)
	}
	if len(mc.WebFetchToolResult.Content) == 0 {
		t.Fatalf("expected raw content to be captured")
	}
}

func TestUnmarshalBashCodeExecutionToolResult(t *testing.T) {
	data := `{"type":"bash_code_execution_tool_result","tool_use_id":"srvtoolu_2","content":{"stdout":"hi","stderr":"","return_code":0}}`
	var mc anthropic.MessageContent
	if err := json.Unmarshal([]byte(data), &mc); err != nil {
		t.Fatalf("unmarshal error: %s", err)
	}
	if mc.CodeExecutionToolResult == nil {
		t.Fatalf("expected CodeExecutionToolResult populated, got nil")
	}
	if mc.CodeExecutionToolResult.ToolUseID == nil ||
		*mc.CodeExecutionToolResult.ToolUseID != "srvtoolu_2" {
		t.Fatalf("unexpected tool_use_id: %+v", mc.CodeExecutionToolResult.ToolUseID)
	}
}

func TestServerToolVersionConstructors(t *testing.T) {
	if d := anthropic.NewWebSearchToolDefinition(); d.Type != "web_search_20260209" {
		t.Fatalf("expected web_search_20260209, got %q", d.Type)
	}
	if d := anthropic.NewWebFetchToolDefinition(); d.Type != "web_fetch_20260209" {
		t.Fatalf("expected web_fetch_20260209, got %q", d.Type)
	}
	if d := anthropic.NewCodeExecutionToolDefinition(); d.Type != "code_execution_20260521" {
		t.Fatalf("expected code_execution_20260521, got %q", d.Type)
	}
}
