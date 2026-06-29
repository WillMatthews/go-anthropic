package anthropic

import (
	"net/http"
	"testing"
)

func TestRequiresVertexRepEndpoint(t *testing.T) {
	rep := []Model{
		ModelClaudeSonnet4Dot5, ModelClaudeSonnet4Dot5V20250929,
		ModelClaudeOpus4Dot5, ModelClaudeOpus4Dot6, ModelClaudeOpus4Dot8,
		ModelClaudeSonnet4Dot6, ModelClaudeHaiku4Dot5, ModelClaudeFable5,
		Model("claude-some-future-model"), // unknown → treated as rep-required
	}
	for _, m := range rep {
		if !requiresVertexRepEndpoint(m) {
			t.Errorf("%q should require the rep/global endpoint", m)
		}
	}

	legacy := []Model{
		ModelClaude2Dot1, ModelClaude3Opus20240229, ModelClaude3Haiku20240307,
		ModelClaude3Dot5Sonnet20241022, ModelClaude3Dot7Sonnet20250219,
		ModelClaudeOpus4Dot0, ModelClaudeOpus4Dot1, ModelClaudeSonnet4Dot0,
	}
	for _, m := range legacy {
		if requiresVertexRepEndpoint(m) {
			t.Errorf("%q is legacy and should stay on the regional endpoint", m)
		}
	}
}

func TestVertexRepGeography(t *testing.T) {
	cases := map[string]string{
		"us-east1":        "us",
		"us-central1":     "us",
		"europe-west1":    "eu",
		"europe-west4":    "eu",
		"asia-southeast1": "", // no multi-region rep endpoint
		"global":          "",
		"us":              "",
		"eu":              "",
	}
	for region, want := range cases {
		if got := vertexRepGeography(region); got != want {
			t.Errorf("vertexRepGeography(%q) = %q, want %q", region, got, want)
		}
	}
}

func vertexRoutingClient(location string) *Client {
	return &Client{config: ClientConfig{
		BaseURL:    vertexBaseURL("my-project", location),
		APIVersion: APIVersionVertex20231016,
	}}
}

func TestVertexAutoRepRouting(t *testing.T) {
	cases := []struct {
		name     string
		location string
		model    Model
		suffix   string
		want     string
	}{
		{
			name:     "new model on pinned EU region auto-routes to eu rep",
			location: "europe-west1",
			model:    ModelClaudeOpus4Dot6,
			suffix:   "/messages",
			want:     "https://aiplatform.eu.rep.googleapis.com/v1/projects/my-project/locations/eu/publishers/anthropic/models/claude-opus-4-6:rawPredict",
		},
		{
			name:     "new model on pinned US region auto-routes to us rep",
			location: "us-central1",
			model:    ModelClaudeSonnet4Dot5,
			suffix:   "/messages",
			want:     "https://aiplatform.us.rep.googleapis.com/v1/projects/my-project/locations/us/publishers/anthropic/models/claude-sonnet-4-5@20250929:rawPredict",
		},
		{
			name:     "legacy model on pinned region stays regional",
			location: "europe-west1",
			model:    ModelClaude3Haiku20240307,
			suffix:   "/messages",
			want:     "https://europe-west1-aiplatform.googleapis.com/v1/projects/my-project/locations/europe-west1/publishers/anthropic/models/claude-3-haiku@20240307:rawPredict",
		},
		{
			name:     "global location is left untouched",
			location: "global",
			model:    ModelClaudeOpus4Dot6,
			suffix:   "/messages",
			want:     "https://aiplatform.googleapis.com/v1/projects/my-project/locations/global/publishers/anthropic/models/claude-opus-4-6:rawPredict",
		},
		{
			name:     "region without a rep geography is left untouched",
			location: "asia-southeast1",
			model:    ModelClaudeOpus4Dot6,
			suffix:   "/messages",
			want:     "https://asia-southeast1-aiplatform.googleapis.com/v1/projects/my-project/locations/asia-southeast1/publishers/anthropic/models/claude-opus-4-6:rawPredict",
		},
		{
			name:     "count_tokens on a new model also auto-routes",
			location: "europe-west1",
			model:    ModelClaudeOpus4Dot6,
			suffix:   "/messages/count_tokens",
			want:     "https://aiplatform.eu.rep.googleapis.com/v1/projects/my-project/locations/eu/publishers/anthropic/models/count-tokens:rawPredict",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			adapter := &VertexAdapter{location: tc.location}
			client := vertexRoutingClient(tc.location)
			req := &MessagesRequest{
				Model:     tc.model,
				MaxTokens: 10,
				Messages:  []Message{NewUserTextMessage("hi")},
			}
			got, err := adapter.PrepareRequest(client, http.MethodPost, tc.suffix, req)
			if err != nil {
				t.Fatalf("PrepareRequest: %v", err)
			}
			if got != tc.want {
				t.Errorf("url = %q, want %q", got, tc.want)
			}
		})
	}
}
