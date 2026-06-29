package anthropic_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/liushuangls/go-anthropic/v2"
	"github.com/liushuangls/go-anthropic/v2/internal/test"
	"github.com/liushuangls/go-anthropic/v2/jsonschema"
)

type weatherInput struct {
	City string `json:"city" jsonschema:"required,description=The city name"`
}

// scriptedMessages returns an httptest handler that replays a fixed sequence of
// response bodies, one per call to /v1/messages. It records each request body
// for later assertions.
func scriptedMessages(
	t *testing.T,
	responses []string,
	recordedBodies *[][]byte,
	mu *sync.Mutex,
) test.Handler {
	t.Helper()
	var calls int32
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		mu.Lock()
		*recordedBodies = append(*recordedBodies, body)
		mu.Unlock()

		idx := int(atomic.AddInt32(&calls, 1)) - 1
		if idx >= len(responses) {
			t.Errorf("unexpected request #%d, only %d scripted", idx, len(responses))
			http.Error(w, "no more scripted responses", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(responses[idx]))
	}
}

func newRunnerClient(t *testing.T, handler test.Handler) *anthropic.Client {
	t.Helper()
	server := test.NewTestServer()
	server.RegisterHandler("/v1/messages", handler)
	ts := server.AnthropicTestServer()
	ts.Start()
	t.Cleanup(ts.Close)

	return anthropic.NewClient(
		test.GetTestToken(),
		anthropic.WithBaseURL(ts.URL+"/v1"),
		anthropic.WithAPIVersion(anthropic.APIVersion20230601),
		anthropic.WithEmptyMessagesLimit(100),
		anthropic.WithHTTPClient(http.DefaultClient),
	)
}

func toolUseResponse(blocks ...string) string {
	return fmt.Sprintf(
		`{"id":"msg_1","type":"message","role":"assistant","model":"claude-3-haiku-20240307","stop_reason":"tool_use","content":[%s],"usage":{"input_tokens":10,"output_tokens":5}}`,
		joinComma(blocks),
	)
}

func toolUseBlock(id, name, inputJSON string) string {
	return fmt.Sprintf(
		`{"type":"tool_use","id":%q,"name":%q,"input":%s}`,
		id, name, inputJSON,
	)
}

func endTurnText(text string) string {
	return fmt.Sprintf(
		`{"id":"msg_final","type":"message","role":"assistant","model":"claude-3-haiku-20240307","stop_reason":"end_turn","content":[{"type":"text","text":%q}],"usage":{"input_tokens":10,"output_tokens":5}}`,
		text,
	)
}

func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ","
		}
		out += p
	}
	return out
}

func TestToolRunnerRunToCompletion(t *testing.T) {
	var bodies [][]byte
	var mu sync.Mutex

	responses := []string{
		toolUseResponse(toolUseBlock("toolu_1", "get_weather", `{"city":"Paris"}`)),
		endTurnText("It is sunny in Paris."),
	}
	client := newRunnerClient(t, scriptedMessages(t, responses, &bodies, &mu))

	var gotCity string
	var called int32
	tool, err := anthropic.NewTool(
		"get_weather",
		"Get weather for a city",
		func(ctx context.Context, in weatherInput) (string, error) {
			atomic.AddInt32(&called, 1)
			gotCity = in.City
			return "sunny, 72F", nil
		},
	)
	if err != nil {
		t.Fatalf("NewTool: %v", err)
	}

	runner := client.NewToolRunner(anthropic.MessagesRequest{
		Model:     anthropic.ModelClaude3Haiku20240307,
		MaxTokens: 100,
		Messages: []anthropic.Message{
			anthropic.NewUserTextMessage("What is the weather in Paris?"),
		},
	}, tool)

	final, err := runner.RunToCompletion(context.Background())
	if err != nil {
		t.Fatalf("RunToCompletion: %v", err)
	}

	if final.StopReason != anthropic.MessagesStopReasonEndTurn {
		t.Errorf("stop reason = %q, want end_turn", final.StopReason)
	}
	if got := final.GetFirstContentText(); got != "It is sunny in Paris." {
		t.Errorf("final text = %q", got)
	}
	if atomic.LoadInt32(&called) != 1 {
		t.Errorf("tool called %d times, want 1", called)
	}
	if gotCity != "Paris" {
		t.Errorf("typed input city = %q, want Paris", gotCity)
	}

	// Second request must include the tool_result fed back to the model.
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(bodies))
	}
	var second anthropic.MessagesRequest
	if err := json.Unmarshal(bodies[1], &second); err != nil {
		t.Fatalf("unmarshal second request: %v", err)
	}
	// user, assistant(tool_use), user(tool_result)
	if len(second.Messages) != 3 {
		t.Fatalf("second request messages = %d, want 3", len(second.Messages))
	}
	last := second.Messages[2]
	if last.Role != anthropic.RoleUser {
		t.Errorf("last message role = %q, want user", last.Role)
	}
	tr := last.Content[0].MessageContentToolResult
	if tr == nil || tr.ToolUseID == nil || *tr.ToolUseID != "toolu_1" {
		t.Errorf("tool_result not paired to toolu_1: %+v", last.Content[0])
	}
	if got := tr.Content[0].GetText(); got != "sunny, 72F" {
		t.Errorf("tool_result content = %q", got)
	}
}

func TestToolRunnerParallelTools(t *testing.T) {
	var bodies [][]byte
	var mu sync.Mutex

	responses := []string{
		toolUseResponse(
			toolUseBlock("toolu_a", "get_weather", `{"city":"Paris"}`),
			toolUseBlock("toolu_b", "get_weather", `{"city":"London"}`),
		),
		endTurnText("done"),
	}
	client := newRunnerClient(t, scriptedMessages(t, responses, &bodies, &mu))

	var count int32
	tool, err := anthropic.NewTool(
		"get_weather",
		"Get weather",
		func(ctx context.Context, in weatherInput) (string, error) {
			atomic.AddInt32(&count, 1)
			return "weather in " + in.City, nil
		},
	)
	if err != nil {
		t.Fatalf("NewTool: %v", err)
	}

	runner := client.NewToolRunner(anthropic.MessagesRequest{
		Model:     anthropic.ModelClaude3Haiku20240307,
		MaxTokens: 100,
		Messages:  []anthropic.Message{anthropic.NewUserTextMessage("weather?")},
	}, tool)

	if _, err := runner.RunToCompletion(context.Background()); err != nil {
		t.Fatalf("RunToCompletion: %v", err)
	}
	if atomic.LoadInt32(&count) != 2 {
		t.Errorf("tool called %d times, want 2", count)
	}

	mu.Lock()
	defer mu.Unlock()
	var second anthropic.MessagesRequest
	if err := json.Unmarshal(bodies[1], &second); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	results := second.Messages[len(second.Messages)-1]
	if len(results.Content) != 2 {
		t.Fatalf("expected 2 tool_result blocks in one user message, got %d", len(results.Content))
	}
	// Order preserved & paired to the correct tool_use_id.
	if id := results.Content[0].MessageContentToolResult.ToolUseID; id == nil || *id != "toolu_a" {
		t.Errorf("first result not paired to toolu_a")
	}
	if id := results.Content[1].MessageContentToolResult.ToolUseID; id == nil || *id != "toolu_b" {
		t.Errorf("second result not paired to toolu_b")
	}
	if got := results.Content[0].MessageContentToolResult.Content[0].GetText(); got != "weather in Paris" {
		t.Errorf("first result content = %q", got)
	}
	if got := results.Content[1].MessageContentToolResult.Content[0].GetText(); got != "weather in London" {
		t.Errorf("second result content = %q", got)
	}
}

func TestToolRunnerMaxIterations(t *testing.T) {
	var bodies [][]byte
	var mu sync.Mutex

	// Always returns tool_use, so the runner can never finish normally.
	loop := toolUseResponse(toolUseBlock("toolu_x", "noop", `{}`))
	responses := []string{loop, loop, loop, loop, loop}
	client := newRunnerClient(t, scriptedMessages(t, responses, &bodies, &mu))

	tool, err := anthropic.NewTool(
		"noop",
		"does nothing",
		func(ctx context.Context, in struct{}) (string, error) { return "ok", nil },
	)
	if err != nil {
		t.Fatalf("NewTool: %v", err)
	}

	runner := client.NewToolRunner(anthropic.MessagesRequest{
		Model:     anthropic.ModelClaude3Haiku20240307,
		MaxTokens: 100,
		Messages:  []anthropic.Message{anthropic.NewUserTextMessage("go")},
	}, tool)
	runner.MaxIterations = 3

	_, err = runner.RunToCompletion(context.Background())
	if !errors.Is(err, anthropic.ErrMaxIterationsExceeded) {
		t.Fatalf("err = %v, want ErrMaxIterationsExceeded", err)
	}
	if runner.Iterations() != 3 {
		t.Errorf("iterations = %d, want 3", runner.Iterations())
	}
}

func TestToolRunnerUnknownTool(t *testing.T) {
	var bodies [][]byte
	var mu sync.Mutex

	responses := []string{
		toolUseResponse(toolUseBlock("toolu_1", "does_not_exist", `{}`)),
		endTurnText("recovered"),
	}
	client := newRunnerClient(t, scriptedMessages(t, responses, &bodies, &mu))

	tool, err := anthropic.NewTool(
		"get_weather", "Get weather",
		func(ctx context.Context, in weatherInput) (string, error) { return "x", nil },
	)
	if err != nil {
		t.Fatalf("NewTool: %v", err)
	}

	runner := client.NewToolRunner(anthropic.MessagesRequest{
		Model:     anthropic.ModelClaude3Haiku20240307,
		MaxTokens: 100,
		Messages:  []anthropic.Message{anthropic.NewUserTextMessage("hi")},
	}, tool)

	var afterResult anthropic.ToolResult
	runner.AfterToolCall = func(ctx context.Context, res anthropic.ToolResult) {
		afterResult = res
	}

	final, err := runner.RunToCompletion(context.Background())
	if err != nil {
		t.Fatalf("RunToCompletion should recover, got: %v", err)
	}
	if got := final.GetFirstContentText(); got != "recovered" {
		t.Errorf("final = %q", got)
	}
	if afterResult.Err == nil || !afterResult.IsError {
		t.Errorf("expected error result for unknown tool, got %+v", afterResult)
	}

	mu.Lock()
	defer mu.Unlock()
	var second anthropic.MessagesRequest
	if err := json.Unmarshal(bodies[1], &second); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	tr := second.Messages[len(second.Messages)-1].Content[0].MessageContentToolResult
	if tr == nil || tr.IsError == nil || !*tr.IsError {
		t.Errorf("expected is_error tool_result for unknown tool")
	}
}

func TestToolRunnerBeforeToolCallVeto(t *testing.T) {
	var bodies [][]byte
	var mu sync.Mutex

	responses := []string{
		toolUseResponse(toolUseBlock("toolu_1", "get_weather", `{"city":"Paris"}`)),
		endTurnText("ok"),
	}
	client := newRunnerClient(t, scriptedMessages(t, responses, &bodies, &mu))

	var executed int32
	tool, err := anthropic.NewTool(
		"get_weather", "Get weather",
		func(ctx context.Context, in weatherInput) (string, error) {
			atomic.AddInt32(&executed, 1)
			return "sunny", nil
		},
	)
	if err != nil {
		t.Fatalf("NewTool: %v", err)
	}

	runner := client.NewToolRunner(anthropic.MessagesRequest{
		Model:     anthropic.ModelClaude3Haiku20240307,
		MaxTokens: 100,
		Messages:  []anthropic.Message{anthropic.NewUserTextMessage("hi")},
	}, tool)

	var seen anthropic.ToolCall
	runner.BeforeToolCall = func(ctx context.Context, call anthropic.ToolCall) error {
		seen = call
		return errors.New("denied by policy")
	}

	if _, err := runner.RunToCompletion(context.Background()); err != nil {
		t.Fatalf("RunToCompletion: %v", err)
	}
	if atomic.LoadInt32(&executed) != 0 {
		t.Errorf("vetoed tool should not execute, ran %d times", executed)
	}
	if seen.Name != "get_weather" {
		t.Errorf("BeforeToolCall saw call %q", seen.Name)
	}

	mu.Lock()
	defer mu.Unlock()
	var second anthropic.MessagesRequest
	if err := json.Unmarshal(bodies[1], &second); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	tr := second.Messages[len(second.Messages)-1].Content[0].MessageContentToolResult
	if tr == nil || tr.IsError == nil || !*tr.IsError {
		t.Errorf("expected is_error tool_result for vetoed call")
	}
	if got := tr.Content[0].GetText(); got != "denied by policy" {
		t.Errorf("veto result content = %q", got)
	}
}

func TestGenerateSchemaForType(t *testing.T) {
	type nested struct {
		Score float64 `json:"score"`
	}
	type sample struct {
		City     string   `json:"city" jsonschema:"required,description=The city name"`
		Unit     string   `json:"unit,omitempty" jsonschema:"enum=celsius|fahrenheit"`
		Tags     []string `json:"tags"`
		Optional *int     `json:"optional"`
		Detail   nested   `json:"detail"`
		Ignored  string   `json:"-"`
		internal string
	}

	def, err := jsonschema.GenerateSchemaForType(sample{})
	if err != nil {
		t.Fatalf("GenerateSchemaForType: %v", err)
	}

	if def.Type != jsonschema.Object {
		t.Errorf("type = %q, want object", def.Type)
	}
	if _, ok := def.Properties["Ignored"]; ok {
		t.Error("json:\"-\" field should be skipped")
	}
	if _, ok := def.Properties["internal"]; ok {
		t.Error("unexported field should be skipped")
	}

	city := def.Properties["city"]
	if city.Type != jsonschema.String || city.Description != "The city name" {
		t.Errorf("city prop = %+v", city)
	}
	unit := def.Properties["unit"]
	if len(unit.Enum) != 2 || unit.Enum[0] != "celsius" || unit.Enum[1] != "fahrenheit" {
		t.Errorf("unit enum = %v", unit.Enum)
	}
	tags := def.Properties["tags"]
	if tags.Type != jsonschema.Array || tags.Items == nil || tags.Items.Type != jsonschema.String {
		t.Errorf("tags prop = %+v", tags)
	}
	detail := def.Properties["detail"]
	if detail.Type != jsonschema.Object {
		t.Errorf("detail prop type = %q", detail.Type)
	}
	if sc, ok := detail.Properties["score"]; !ok || sc.Type != jsonschema.Number {
		t.Errorf("nested score prop = %+v (ok=%v)", sc, ok)
	}

	// Required: city (explicit + no omitempty), tags (no omitempty), detail.
	// Not required: unit (omitempty), optional (pointer), and skipped fields.
	required := map[string]bool{}
	for _, r := range def.Required {
		required[r] = true
	}
	for _, want := range []string{"city", "tags", "detail"} {
		if !required[want] {
			t.Errorf("expected %q to be required; required=%v", want, def.Required)
		}
	}
	for _, notWant := range []string{"unit", "optional"} {
		if required[notWant] {
			t.Errorf("did not expect %q to be required; required=%v", notWant, def.Required)
		}
	}

	// Ensure it marshals to valid JSON usable as an input_schema.
	if _, err := json.Marshal(def); err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
}
