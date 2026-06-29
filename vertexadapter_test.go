package anthropic_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/liushuangls/go-anthropic/v2"
)

func vertexTestClient(location string) *anthropic.Client {
	return anthropic.NewClient(
		"test-token",
		anthropic.WithVertexAI("my-project", location),
	)
}

func vertexTestRequest(stream bool) *anthropic.MessagesRequest {
	return &anthropic.MessagesRequest{
		Model:     anthropic.ModelClaudeOpus4Dot8,
		MaxTokens: 10,
		Stream:    stream,
		Messages: []anthropic.Message{
			anthropic.NewUserTextMessage("hi"),
		},
	}
}

func TestVertexAdapterMessagesURL(t *testing.T) {
	cases := []struct {
		name     string
		location string
		stream   bool
		want     string
	}{
		{
			name:     "global",
			location: "global",
			want:     "https://aiplatform.googleapis.com/v1/projects/my-project/locations/global/publishers/anthropic/models/claude-opus-4-8:rawPredict",
		},
		{
			name:     "global streaming",
			location: "global",
			stream:   true,
			want:     "https://aiplatform.googleapis.com/v1/projects/my-project/locations/global/publishers/anthropic/models/claude-opus-4-8:streamRawPredict",
		},
		{
			name:     "multi-region us",
			location: "us",
			want:     "https://aiplatform.us.rep.googleapis.com/v1/projects/my-project/locations/us/publishers/anthropic/models/claude-opus-4-8:rawPredict",
		},
		{
			name:     "multi-region eu",
			location: "eu",
			want:     "https://aiplatform.eu.rep.googleapis.com/v1/projects/my-project/locations/eu/publishers/anthropic/models/claude-opus-4-8:rawPredict",
		},
		{
			name:     "specific region",
			location: "us-east1",
			want:     "https://us-east1-aiplatform.googleapis.com/v1/projects/my-project/locations/us-east1/publishers/anthropic/models/claude-opus-4-8:rawPredict",
		},
	}

	adapter := &anthropic.VertexAdapter{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := vertexTestClient(tc.location)
			req := vertexTestRequest(tc.stream)

			got, err := adapter.PrepareRequest(client, http.MethodPost, "/messages", req)
			if err != nil {
				t.Fatalf("PrepareRequest: %v", err)
			}
			if got != tc.want {
				t.Errorf("url = %q, want %q", got, tc.want)
			}

			// The model is lifted into the URL and cleared from the body, and
			// anthropic_version is injected into the body.
			if req.Model != "" {
				t.Errorf("model should be cleared from body, got %q", req.Model)
			}
			if req.AnthropicVersion != string(anthropic.APIVersionVertex20231016) {
				t.Errorf("anthropic_version = %q", req.AnthropicVersion)
			}

			body, err := json.Marshal(req)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if strings.Contains(string(body), `"model"`) {
				t.Errorf("messages body must omit model entirely, got %s", body)
			}
			if !strings.Contains(string(body), `"anthropic_version":"vertex-2023-10-16"`) {
				t.Errorf("messages body must carry anthropic_version, got %s", body)
			}
		})
	}
}

func TestVertexAdapterCountTokensURL(t *testing.T) {
	adapter := &anthropic.VertexAdapter{}
	client := vertexTestClient("global")
	req := vertexTestRequest(false)

	got, err := adapter.PrepareRequest(client, http.MethodPost, "/messages/count_tokens", req)
	if err != nil {
		t.Fatalf("PrepareRequest: %v", err)
	}
	want := "https://aiplatform.googleapis.com/v1/projects/my-project/locations/global/publishers/anthropic/models/count-tokens:rawPredict"
	if got != want {
		t.Errorf("url = %q, want %q", got, want)
	}

	// Unlike /messages, count_tokens keeps the real model in the request body —
	// the "count-tokens" pseudo-model only routes the request.
	if req.Model != anthropic.ModelClaudeOpus4Dot8 {
		t.Errorf("count_tokens must keep the model in the body, got %q", req.Model)
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(body), `"model":"claude-opus-4-8"`) {
		t.Errorf("count_tokens body must include model, got %s", body)
	}
	if !strings.Contains(string(body), `"anthropic_version":"vertex-2023-10-16"`) {
		t.Errorf("count_tokens body must carry anthropic_version, got %s", body)
	}
}

func TestVertexAdapterUnsupportedSuffix(t *testing.T) {
	adapter := &anthropic.VertexAdapter{}
	client := vertexTestClient("global")
	req := vertexTestRequest(false)

	if _, err := adapter.PrepareRequest(client, http.MethodPost, "/messages/batches", req); err == nil {
		t.Error("expected an error for an unsupported Vertex suffix")
	}
}

func TestVertexAdapterRejectsUnsupportedBody(t *testing.T) {
	adapter := &anthropic.VertexAdapter{}
	client := vertexTestClient("global")

	// A non-MessagesRequest body (e.g. the legacy Complete API) is not
	// supported on Vertex and must be rejected rather than silently mis-routed.
	if _, err := adapter.PrepareRequest(client, http.MethodPost, "/complete", &anthropic.CompleteRequest{}); err == nil {
		t.Error("expected an error for an unsupported request body")
	}
}

// TestMessagesRequestMarshalDirectKeepsModel guards the direct (non-cloud) path:
// when AnthropicVersion is unset the model must always be serialized, even when
// empty, so a missing model surfaces a clear server-side error (see #117).
func TestMessagesRequestMarshalDirectKeepsModel(t *testing.T) {
	req := anthropic.MessagesRequest{Model: anthropic.ModelClaudeOpus4Dot8, MaxTokens: 10}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(body), `"model":"claude-opus-4-8"`) {
		t.Errorf("direct request must serialize model, got %s", body)
	}

	empty := anthropic.MessagesRequest{MaxTokens: 10}
	body, err = json.Marshal(empty)
	if err != nil {
		t.Fatalf("marshal empty: %v", err)
	}
	if !strings.Contains(string(body), `"model":""`) {
		t.Errorf("direct request must keep an (empty) model field, got %s", body)
	}
}
