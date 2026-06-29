package anthropic_test

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/liushuangls/go-anthropic/v2"
	"github.com/liushuangls/go-anthropic/v2/internal/test"
)

const retrySuccessBody = `{"id":"1","type":"message","role":"assistant",` +
	`"content":[{"type":"text","text":"hi"}],"model":"claude-3-haiku-20240307",` +
	`"stop_reason":"end_turn","stop_sequence":"","usage":{"input_tokens":1,"output_tokens":1}}`

func TestClientRetriesOnRetryableStatus(t *testing.T) {
	var calls int32

	server := test.NewTestServer()
	server.RegisterHandler("/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			// First attempt returns a retryable 529 (overloaded).
			w.WriteHeader(529)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"overloaded"}}`))
			return
		}
		_, _ = w.Write([]byte(retrySuccessBody))
	})

	ts := server.AnthropicTestServer()
	ts.Start()
	defer ts.Close()

	client := anthropic.NewClient(
		test.GetTestToken(),
		anthropic.WithBaseURL(ts.URL+"/v1"),
	)

	resp, err := client.CreateMessages(context.Background(), anthropic.MessagesRequest{
		Model:     anthropic.ModelClaude3Haiku20240307,
		MaxTokens: 10,
		Messages: []anthropic.Message{
			anthropic.NewUserTextMessage("hi"),
		},
	})
	if err != nil {
		t.Fatalf("CreateMessages error: %s", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected 2 calls (1 retry), got %d", got)
	}
	if resp.GetFirstContentText() != "hi" {
		t.Fatalf("unexpected response content: %q", resp.GetFirstContentText())
	}
}

func TestClientDoesNotRetryNonRetryableStatus(t *testing.T) {
	var calls int32

	server := test.NewTestServer()
	server.RegisterHandler("/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"bad"}}`))
	})

	ts := server.AnthropicTestServer()
	ts.Start()
	defer ts.Close()

	client := anthropic.NewClient(
		test.GetTestToken(),
		anthropic.WithBaseURL(ts.URL+"/v1"),
	)

	_, err := client.CreateMessages(context.Background(), anthropic.MessagesRequest{
		Model:     anthropic.ModelClaude3Haiku20240307,
		MaxTokens: 10,
		Messages: []anthropic.Message{
			anthropic.NewUserTextMessage("hi"),
		},
	})
	if err == nil {
		t.Fatalf("expected error for 400 response, got nil")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected exactly 1 call (no retry) for 400, got %d", got)
	}
}

func TestClientRetriesDisabled(t *testing.T) {
	var calls int32

	server := test.NewTestServer()
	server.RegisterHandler("/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(529)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"overloaded"}}`))
	})

	ts := server.AnthropicTestServer()
	ts.Start()
	defer ts.Close()

	client := anthropic.NewClient(
		test.GetTestToken(),
		anthropic.WithBaseURL(ts.URL+"/v1"),
		anthropic.WithMaxRetries(0),
	)

	_, err := client.CreateMessages(context.Background(), anthropic.MessagesRequest{
		Model:     anthropic.ModelClaude3Haiku20240307,
		MaxTokens: 10,
		Messages: []anthropic.Message{
			anthropic.NewUserTextMessage("hi"),
		},
	})
	if err == nil {
		t.Fatalf("expected error for 529 with retries disabled, got nil")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected exactly 1 call with retries disabled, got %d", got)
	}
}
