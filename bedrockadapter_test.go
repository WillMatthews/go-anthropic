package anthropic_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/liushuangls/go-anthropic/v2"
)

const bedrockModel = anthropic.Model("global.anthropic.claude-opus-4-6-v1")

func bedrockTestClient() *anthropic.Client {
	return anthropic.NewClient("", anthropic.WithBedrock("us-west-2"))
}

func bedrockTestRequest(stream bool) *anthropic.MessagesRequest {
	return &anthropic.MessagesRequest{
		Model:     bedrockModel,
		MaxTokens: 10,
		Stream:    stream,
		Messages: []anthropic.Message{
			anthropic.NewUserTextMessage("hi"),
		},
	}
}

func TestBedrockAdapterInvokeURL(t *testing.T) {
	cases := []struct {
		name   string
		stream bool
		want   string
	}{
		{
			name: "invoke",
			want: "https://bedrock-runtime.us-west-2.amazonaws.com/model/global.anthropic.claude-opus-4-6-v1/invoke",
		},
		{
			name:   "streaming invoke",
			stream: true,
			want:   "https://bedrock-runtime.us-west-2.amazonaws.com/model/global.anthropic.claude-opus-4-6-v1/invoke-with-response-stream",
		},
	}

	adapter := &anthropic.BedrockAdapter{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := bedrockTestClient()
			req := bedrockTestRequest(tc.stream)

			got, err := adapter.PrepareRequest(client, http.MethodPost, "/messages", req)
			if err != nil {
				t.Fatalf("PrepareRequest: %v", err)
			}
			if got != tc.want {
				t.Errorf("url = %q, want %q", got, tc.want)
			}

			// Model is lifted into the URL and cleared from the body;
			// anthropic_version is injected.
			if req.Model != "" {
				t.Errorf("model should be cleared from body, got %q", req.Model)
			}
			if req.AnthropicVersion != string(anthropic.APIVersionBedrock20230531) {
				t.Errorf("anthropic_version = %q", req.AnthropicVersion)
			}
			// Streaming is implied by the endpoint and must be absent from the body.
			if tc.stream && req.Stream {
				t.Error("stream should be cleared from the body for streaming invoke")
			}

			body, err := json.Marshal(req)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if strings.Contains(string(body), `"model"`) {
				t.Errorf("body must omit model entirely, got %s", body)
			}
			if !strings.Contains(string(body), `"anthropic_version":"bedrock-2023-05-31"`) {
				t.Errorf("body must carry anthropic_version, got %s", body)
			}
			if strings.Contains(string(body), `"stream"`) {
				t.Errorf("body must not carry a stream field, got %s", body)
			}
		})
	}
}

func TestBedrockAdapterUnsupportedEndpoint(t *testing.T) {
	adapter := &anthropic.BedrockAdapter{}
	client := bedrockTestClient()
	req := bedrockTestRequest(false)

	// count_tokens / batches are not reachable via the InvokeModel path.
	if _, err := adapter.PrepareRequest(client, http.MethodPost, "/messages/count_tokens", req); err == nil {
		t.Error("expected an error for an unsupported Bedrock endpoint")
	}
}

func TestBedrockAdapterRejectsUnsupportedBody(t *testing.T) {
	adapter := &anthropic.BedrockAdapter{}
	client := bedrockTestClient()

	if _, err := adapter.PrepareRequest(client, http.MethodPost, "/messages", &anthropic.CompleteRequest{}); err == nil {
		t.Error("expected an error for an unsupported request body")
	}
}

// TestBedrockAdapterSetsNoAuthHeaders verifies the adapter leaves authentication
// to the caller's signing http.Client (no x-api-key / Authorization header).
func TestBedrockAdapterSetsNoAuthHeaders(t *testing.T) {
	adapter := &anthropic.BedrockAdapter{}
	client := anthropic.NewClient("should-not-be-used", anthropic.WithBedrock("us-west-2"))

	req := httptest.NewRequest(http.MethodPost, "https://bedrock-runtime.us-west-2.amazonaws.com/model/m/invoke", nil)
	req.Header.Del("Authorization")
	if err := adapter.SetRequestHeaders(client, req); err != nil {
		t.Fatalf("SetRequestHeaders: %v", err)
	}
	if got := req.Header.Get("X-Api-Key"); got != "" {
		t.Errorf("X-Api-Key should not be set, got %q", got)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization should be left to the signing client, got %q", got)
	}
}
