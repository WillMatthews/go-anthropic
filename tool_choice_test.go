package anthropic_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/liushuangls/go-anthropic/v2"
)

func TestToolChoiceMarshal(t *testing.T) {
	t.Run("disable_parallel_tool_use omitted when nil", func(t *testing.T) {
		tc := anthropic.ToolChoice{Type: anthropic.ToolChoiceTypeAuto}
		b, err := json.Marshal(tc)
		if err != nil {
			t.Fatalf("marshal error: %s", err)
		}
		if strings.Contains(string(b), "disable_parallel_tool_use") {
			t.Fatalf("expected field omitted when nil, got %s", string(b))
		}
	})

	t.Run("disable_parallel_tool_use present when set", func(t *testing.T) {
		v := true
		tc := anthropic.ToolChoice{
			Type:                   anthropic.ToolChoiceTypeTool,
			Name:                   "get_weather",
			DisableParallelToolUse: &v,
		}
		b, err := json.Marshal(tc)
		if err != nil {
			t.Fatalf("marshal error: %s", err)
		}
		got := string(b)
		if !strings.Contains(got, `"disable_parallel_tool_use":true`) {
			t.Fatalf("expected disable_parallel_tool_use:true, got %s", got)
		}
		if !strings.Contains(got, `"type":"tool"`) {
			t.Fatalf("expected type tool, got %s", got)
		}
	})
}
