package anthropic

import (
	"context"
	"encoding/json"

	"github.com/liushuangls/go-anthropic/v2/jsonschema"
)

// Tool is a named, callable tool that a ToolRunner can dispatch to by name.
//
// The typical way to build one is NewTool, which derives the input schema from
// a Go type and wraps a typed handler. A Tool exposes its ToolDefinition so it
// can also be dropped directly into MessagesRequest.Tools when driving the loop
// manually.
type Tool interface {
	// Definition returns the ToolDefinition advertised to the model.
	Definition() ToolDefinition
	// Name is the tool name the model uses to invoke it.
	Name() string
	// Call dispatches the tool with the raw JSON input from a tool_use block
	// and returns the textual tool_result content. A non-nil error is surfaced
	// to the model as a tool_result with is_error=true.
	Call(ctx context.Context, input json.RawMessage) (string, error)
}

// FuncTool is a Tool backed by a typed Go function. Construct it with NewTool.
type FuncTool struct {
	def    ToolDefinition
	handle func(ctx context.Context, input json.RawMessage) (string, error)
}

// NewTool builds a Tool from a typed handler function. The input schema is
// derived from T via the jsonschema package, so callers don't hand-write
// schemas:
//
//	type WeatherInput struct {
//	    City string `json:"city" jsonschema:"required,description=The city name"`
//	}
//
//	tool, err := anthropic.NewTool(
//	    "get_weather",
//	    "Get weather for a city",
//	    func(ctx context.Context, in WeatherInput) (string, error) {
//	        return fmt.Sprintf("It's sunny in %s", in.City), nil
//	    },
//	)
//
// The handler receives the tool_use input unmarshaled into T. The string it
// returns becomes the tool_result content; if it returns an error, the runner
// produces a tool_result with is_error=true carrying the error message.
func NewTool[T any](
	name, description string,
	fn func(ctx context.Context, in T) (string, error),
) (*FuncTool, error) {
	var zero T
	schema, err := jsonschema.GenerateSchemaForType(zero)
	if err != nil {
		return nil, err
	}

	def := ToolDefinition{
		Name:        name,
		Description: description,
		InputSchema: schema,
	}

	handle := func(ctx context.Context, raw json.RawMessage) (string, error) {
		var in T
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &in); err != nil {
				return "", err
			}
		}
		return fn(ctx, in)
	}

	return &FuncTool{def: def, handle: handle}, nil
}

// Definition returns the underlying ToolDefinition, suitable for inclusion in
// MessagesRequest.Tools.
func (t *FuncTool) Definition() ToolDefinition { return t.def }

// Name returns the tool name.
func (t *FuncTool) Name() string { return t.def.Name }

// Call unmarshals the raw input into the handler's parameter type and invokes
// it.
func (t *FuncTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	return t.handle(ctx, input)
}

var _ Tool = (*FuncTool)(nil)
