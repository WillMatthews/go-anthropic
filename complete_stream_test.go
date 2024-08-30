package anthropic_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/liushuangls/go-anthropic/v2"
	"github.com/liushuangls/go-anthropic/v2/internal/test"
	"github.com/liushuangls/go-anthropic/v2/internal/test/checks"
)

var (
	testCompletionStreamContent = []string{"My", " name", " is", " Claude", "."}
)

func TestCreateCompleteStream(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		server := test.NewTestServer()
		server.RegisterHandler("/v1/complete", handlerCompleteStream)

		ts := server.AnthropicTestServer()
		ts.Start()
		defer ts.Close()

		baseUrl := ts.URL + "/v1"
		client := anthropic.NewClient(
			test.GetTestToken(),
			anthropic.WithBaseURL(baseUrl),
		)
		var receivedContent string
		resp, err := client.CreateCompleteStream(context.Background(), anthropic.CompleteStreamRequest{
			CompleteRequest: anthropic.CompleteRequest{
				Model:             anthropic.ModelClaudeInstant1Dot2,
				Prompt:            "\n\nHuman: What is your name?\n\nAssistant:",
				MaxTokensToSample: 1000,
			},
			OnCompletion: func(data anthropic.CompleteResponse) {
				receivedContent += data.Completion
				//t.Logf("CreateCompleteStream OnCompletion data: %+v", data)
			},
			OnPing:  func(data anthropic.CompleteStreamPingData) {},
			OnError: func(response anthropic.ErrorResponse) {},
		})
		if err != nil {
			t.Fatalf("CreateCompleteStream error: %s", err)
		}

		expected := strings.Join(testCompletionStreamContent, "")
		if receivedContent != expected {
			t.Fatalf("CreateCompleteStream content not match expected: %s, got: %s", expected, receivedContent)
		}
		if resp.Completion != expected {
			t.Fatalf("CreateCompleteStream content not match expected: %s, got: %s", expected, resp.Completion)
		}
		t.Logf("CreateCompleteStream resp: %+v", resp)
	})

	t.Run("Does not error for empty unknown messages below limit", func(t *testing.T) {
		emptyMessagesLimit := 100
		server := test.NewTestServer()
		server.RegisterHandler("/v1/messages",
			handlerCompleteStreamEmptyMessages(emptyMessagesLimit-1, "fake: {}"),
		)

		ts := server.AnthropicTestServer()
		ts.Start()
		defer ts.Close()
		baseUrl := ts.URL + "/v1"

		client := anthropic.NewClient(
			test.GetTestToken(),
			anthropic.WithBaseURL(baseUrl),
			anthropic.WithEmptyMessagesLimit(uint(emptyMessagesLimit)),
		)
		_, err := client.CreateCompleteStream(context.Background(), anthropic.CompleteStreamRequest{
			CompleteRequest: anthropic.CompleteRequest{
				Model:             anthropic.ModelClaudeInstant1Dot2,
				Prompt:            "\n\nHuman: What is your name?\n\nAssistant:",
				MaxTokensToSample: 1000,
			},
		})
		if err != nil {
			t.Fatalf("CreateCompleteStream error: %s", err)
		}
	})

	t.Run("Error for empty unknown messages above limit", func(t *testing.T) {
		emptyMessagesLimit := 100
		server := test.NewTestServer()
		server.RegisterHandler("/v1/messages",
			handlerCompleteStreamEmptyMessages(emptyMessagesLimit+1, "fake: {}"),
		)

		ts := server.AnthropicTestServer()
		ts.Start()
		defer ts.Close()
		baseUrl := ts.URL + "/v1"

		client := anthropic.NewClient(
			test.GetTestToken(),
			anthropic.WithBaseURL(baseUrl),
			anthropic.WithEmptyMessagesLimit(uint(emptyMessagesLimit)),
		)
		_, err := client.CreateCompleteStream(context.Background(), anthropic.CompleteStreamRequest{
			CompleteRequest: anthropic.CompleteRequest{
				Model:             anthropic.ModelClaudeInstant1Dot2,
				Prompt:            "\n\nHuman: What is your name?\n\nAssistant:",
				MaxTokensToSample: 1000,
			},
		})
		if err != nil {
			t.Fatalf("CreateCompleteStream error: %s", err)
		}
	})

}

func TestCompleteStreamError(t *testing.T) {
	server := test.NewTestServer()
	server.RegisterHandler("/v1/complete", handlerCompleteStream)

	ts := server.AnthropicTestServer()
	ts.Start()
	defer ts.Close()

	baseUrl := ts.URL + "/v1"
	client := anthropic.NewClient(
		test.GetTestToken(),
		anthropic.WithBaseURL(baseUrl),
	)
	var receivedContent string
	param := anthropic.CompleteStreamRequest{
		CompleteRequest: anthropic.CompleteRequest{
			Model:             anthropic.ModelClaudeInstant1Dot2,
			Prompt:            "\n\nHuman: What is your name?\n\nAssistant:",
			MaxTokensToSample: 1000,
			//Temperature:       &temperature,
		},
		OnCompletion: func(data anthropic.CompleteResponse) {
			receivedContent += data.Completion
			//t.Logf("CreateCompleteStream OnCompletion data: %+v", data)
		},
		OnPing:  func(data anthropic.CompleteStreamPingData) {},
		OnError: func(response anthropic.ErrorResponse) {},
	}
	param.SetTemperature(2)
	_, err := client.CreateCompleteStream(context.Background(), param)
	checks.HasError(t, err, "should error")

	var e *anthropic.APIError
	if !errors.As(err, &e) {
		t.Fatal("should api error")
	}

	t.Logf("CreateCompleteStream error: %+v", err)
}

func handlerCompleteStream(w http.ResponseWriter, r *http.Request) {
	request, err := getCompleteRequest(r)
	if err != nil {
		http.Error(w, "request error", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")

	var dataBytes []byte

	dataBytes = append(dataBytes, []byte("event: ping\n")...)
	dataBytes = append(dataBytes, []byte(`data: {"type": "ping"}`+"\n\n")...)

	if request.Temperature != nil && *request.Temperature > 1 {
		dataBytes = append(dataBytes, []byte("event: error\n")...)
		dataBytes = append(dataBytes, []byte(`data: {"type": "error", "error": {"type": "overloaded_error", "message": "Overloaded"}}`+"\n\n")...)
	}

	for _, t := range testCompletionStreamContent {
		dataBytes = append(dataBytes, []byte("event: completion\n")...)
		dataBytes = append(dataBytes, []byte(fmt.Sprintf(`data: {"type":"completion","id":"compl_01GatBXF5t5K51mYzbVgRJfZ","completion":"%s","stop_reason":null,"model":"claude-instant-1.2","stop":null,"log_id":"compl_01GatBXF5t5K51mYzbVgRJfZ"}`, t)+"\n\n")...)
	}

	dataBytes = append(dataBytes, []byte("event: completion\n")...)
	dataBytes = append(dataBytes, []byte(`data: {"type":"completion","id":"compl_01GatBXF5t5K51mYzbVgRJfZ","completion":"","stop_reason":"stop_sequence","model":"claude-instant-1.2","stop":null,"log_id":"compl_01GatBXF5t5K51mYzbVgRJfZ"}`+"\n\n")...)

	_, _ = w.Write(dataBytes)
}

func handlerCompleteStreamEmptyMessages(numEmptyMessages int, payload string) test.Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		_, err := getCompleteRequest(r)
		if err != nil {
			http.Error(w, "request error", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")

		var dataBytes []byte

		dataBytes = append(dataBytes, []byte("event: message_start\n")...)
		dataBytes = append(dataBytes, []byte(`data: {"type":"message_start","message":{"id":"123333","type":"message","role":"assistant","model":"claude-3-opus-20240229","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":844,"output_tokens":2}}}`+"\n\n")...)

		for i := 0; i < numEmptyMessages; i++ {
			dataBytes = append(dataBytes, []byte(payload+"\n")...)
		}

		_, _ = w.Write(dataBytes)
	}
}
