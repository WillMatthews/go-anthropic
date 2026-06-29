package anthropic

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync"
)

// DefaultMaxIterations is the iteration cap applied to a ToolRunner when none is
// configured. It bounds the number of model round-trips to prevent a runaway
// tool-use loop.
const DefaultMaxIterations = 10

// ErrMaxIterationsExceeded is returned by the runner when it reaches its
// MaxIterations cap before the model produces a final (non-tool-use) message.
var ErrMaxIterationsExceeded = errors.New("anthropic: tool runner exceeded max iterations")

// ToolCall describes a single tool_use block the model emitted.
type ToolCall struct {
	ID    string
	Name  string
	Input []byte
}

// ToolResult is the outcome of executing a ToolCall.
type ToolResult struct {
	Call ToolCall
	// Content is the textual tool_result content sent back to the model.
	Content string
	// IsError reports whether the result was returned with is_error=true.
	IsError bool
	// Err is the Go error that produced an error result, if any (handler
	// failure, unknown tool, input unmarshal error, or a veto). It is nil for
	// successful calls.
	Err error
}

// ToolRunner drives the tool-use loop to completion: it calls the Messages API,
// executes any tool_use blocks the model requests (in parallel), feeds the
// results back, and repeats until the model stops requesting tools or the
// iteration cap is hit.
//
// Construct one with Client.NewToolRunner. The exported fields may be set
// before the first Run/Next call to configure behavior.
type ToolRunner struct {
	client  *Client
	request MessagesRequest
	tools   map[string]Tool

	// MaxIterations caps the number of model round-trips. Defaults to
	// DefaultMaxIterations. A value <= 0 disables the cap (use with care).
	MaxIterations int

	// BeforeToolCall, if set, is invoked before each tool executes. Returning a
	// non-nil error vetoes that single call: the tool is not executed and an
	// is_error tool_result carrying the error message is fed back to the model
	// (the loop continues so the model can react). It may be called
	// concurrently for tool calls in the same turn.
	BeforeToolCall func(ctx context.Context, call ToolCall) error

	// AfterToolCall, if set, is invoked after each tool result is produced
	// (including error and vetoed results). It may be called concurrently for
	// tool calls in the same turn.
	AfterToolCall func(ctx context.Context, result ToolResult)

	iterations int
	done       bool
	lastResp   MessagesResponse
}

// NewToolRunner creates a ToolRunner for the given request and tools. Each
// tool's ToolDefinition is appended to the request's Tools, and the tools are
// registered for dispatch by name. The request is copied, so the caller's
// MessagesRequest is not mutated.
func (c *Client) NewToolRunner(request MessagesRequest, tools ...Tool) *ToolRunner {
	// Defensive copies so the caller's slices aren't mutated as the loop grows
	// the conversation.
	req := request
	req.Messages = append([]Message(nil), request.Messages...)
	req.Tools = append([]ToolDefinition(nil), request.Tools...)

	registry := make(map[string]Tool, len(tools))
	for _, t := range tools {
		if t == nil {
			continue
		}
		registry[t.Name()] = t
		req.Tools = append(req.Tools, t.Definition())
	}

	return &ToolRunner{
		client:        c,
		request:       req,
		tools:         registry,
		MaxIterations: DefaultMaxIterations,
	}
}

// RunToCompletion drives the loop until the model produces a final message
// (StopReason other than tool_use / pause_turn) and returns it. It returns
// ErrMaxIterationsExceeded if the iteration cap is reached first, along with
// the most recent response.
func (r *ToolRunner) RunToCompletion(ctx context.Context) (MessagesResponse, error) {
	for {
		resp, err := r.NextMessage(ctx)
		if err != nil {
			return resp, err
		}
		if r.done {
			return resp, nil
		}
	}
}

// NextMessage performs a single step of the loop: it sends the current
// conversation to the Messages API, appends the assistant response to the
// history, and—if the model requested tools—executes them and appends a single
// user message containing all tool_result blocks.
//
// It returns the assistant response for the step. When the returned response is
// a final message, IsDone reports true on subsequent calls. NextMessage returns
// ErrMaxIterationsExceeded once the cap is reached.
func (r *ToolRunner) NextMessage(ctx context.Context) (MessagesResponse, error) {
	if r.done {
		return r.lastResp, nil
	}
	if r.MaxIterations > 0 && r.iterations >= r.MaxIterations {
		return r.lastResp, ErrMaxIterationsExceeded
	}
	if err := ctx.Err(); err != nil {
		return r.lastResp, err
	}
	r.iterations++

	resp, err := r.client.CreateMessages(ctx, r.request)
	if err != nil {
		return resp, err
	}
	r.lastResp = resp

	// Append the assistant response to the history before processing tool
	// calls so ordering (assistant turn, then tool results) is correct.
	r.request.Messages = append(r.request.Messages, Message{
		Role:    RoleAssistant,
		Content: resp.Content,
	})

	switch resp.StopReason {
	case MessagesStopReasonToolUse:
		results := r.executeToolCalls(ctx, resp.Content)
		if len(results) > 0 {
			r.request.Messages = append(r.request.Messages, Message{
				Role:    RoleUser,
				Content: results,
			})
		}
		return resp, nil
	case MessagesStopReasonPauseTurn:
		// Server-side tool pause: re-send the conversation as-is (the assistant
		// turn carrying the server_tool_use block is already appended) and the
		// server resumes where it left off. No extra user message is added.
		return resp, nil
	default:
		r.done = true
		return resp, nil
	}
}

// All returns an iterator over the assistant responses produced by the loop,
// one per step, stopping when the loop completes or an error occurs. The final
// pair carries any error (e.g. ErrMaxIterationsExceeded).
func (r *ToolRunner) All(ctx context.Context) iter.Seq2[MessagesResponse, error] {
	return func(yield func(MessagesResponse, error) bool) {
		for {
			resp, err := r.NextMessage(ctx)
			if !yield(resp, err) {
				return
			}
			if err != nil || r.done {
				return
			}
		}
	}
}

// IsDone reports whether the loop has reached a final message.
func (r *ToolRunner) IsDone() bool { return r.done }

// Iterations reports how many model round-trips have been made so far.
func (r *ToolRunner) Iterations() int { return r.iterations }

// Messages returns the current conversation history, including assistant turns
// and tool_result turns appended by the runner.
func (r *ToolRunner) Messages() []Message { return r.request.Messages }

func (r *ToolRunner) executeToolCalls(
	ctx context.Context,
	content []MessageContent,
) []MessageContent {
	var calls []ToolCall
	for _, block := range content {
		if block.Type == MessagesContentTypeToolUse && block.MessageContentToolUse != nil {
			calls = append(calls, ToolCall{
				ID:    block.MessageContentToolUse.ID,
				Name:  block.MessageContentToolUse.Name,
				Input: block.MessageContentToolUse.Input,
			})
		}
	}
	if len(calls) == 0 {
		return nil
	}

	// Execute in parallel, preserving result order to pair each result with its
	// tool_use_id.
	results := make([]MessageContent, len(calls))
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func(i int, call ToolCall) {
			defer wg.Done()
			res := r.runOne(ctx, call)
			if r.AfterToolCall != nil {
				r.AfterToolCall(ctx, res)
			}
			results[i] = NewToolResultMessageContent(res.Call.ID, res.Content, res.IsError)
		}(i, call)
	}
	wg.Wait()
	return results
}

func (r *ToolRunner) runOne(ctx context.Context, call ToolCall) ToolResult {
	if r.BeforeToolCall != nil {
		if err := r.BeforeToolCall(ctx, call); err != nil {
			return ToolResult{Call: call, Content: err.Error(), IsError: true, Err: err}
		}
	}

	tool, ok := r.tools[call.Name]
	if !ok {
		err := fmt.Errorf("unknown tool: %q", call.Name)
		return ToolResult{Call: call, Content: err.Error(), IsError: true, Err: err}
	}

	out, err := tool.Call(ctx, call.Input)
	if err != nil {
		return ToolResult{Call: call, Content: err.Error(), IsError: true, Err: err}
	}
	return ToolResult{Call: call, Content: out}
}
