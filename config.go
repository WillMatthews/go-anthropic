package anthropic

import (
	"fmt"
	"net/http"
)

const (
	anthropicAPIURLv1              = "https://api.anthropic.com/v1"
	defaultEmptyMessagesLimit uint = 300
)

type APIVersion string

const (
	APIVersion20230601        APIVersion = "2023-06-01"
	APIVersionVertex20231016  APIVersion = "vertex-2023-10-16"
	APIVersionBedrock20230531 APIVersion = "bedrock-2023-05-31"
)

type BetaVersion string

const (
	BetaTools20240404               BetaVersion = "tools-2024-04-04"
	BetaTools20240516               BetaVersion = "tools-2024-05-16"
	BetaPromptCaching20240731       BetaVersion = "prompt-caching-2024-07-31"
	BetaMessageBatches20240924      BetaVersion = "message-batches-2024-09-24"
	BetaTokenCounting20241101       BetaVersion = "token-counting-2024-11-01"
	BetaMaxTokens35Sonnet20240715   BetaVersion = "max-tokens-3-5-sonnet-2024-07-15"
	BetaComputerUse20241022         BetaVersion = "computer-use-2024-10-22"
	BetaOutput128k20250219          BetaVersion = "output-128k-2025-02-19"
	BetaInterleavedThinking20250514 BetaVersion = "interleaved-thinking-2025-05-14"
	BetaComputerUse20250124         BetaVersion = "computer-use-2025-01-24"
	BetaStructuredOutputs20251113   BetaVersion = "structured-outputs-2025-11-13"
)

type ApiKeyFunc func() string

// ClientConfig is a configuration of a client.
type ClientConfig struct {
	apiKeyFunc ApiKeyFunc
	apiKey     string

	BaseURL     string
	APIVersion  APIVersion
	BetaVersion []BetaVersion
	HTTPClient  *http.Client

	EmptyMessagesLimit uint

	Adapter ClientAdapter
}

type ClientOption func(c *ClientConfig)

func newConfig(apiKey string, opts ...ClientOption) ClientConfig {
	c := ClientConfig{
		apiKey: apiKey,

		BaseURL:    anthropicAPIURLv1,
		APIVersion: APIVersion20230601,
		HTTPClient: &http.Client{},

		EmptyMessagesLimit: defaultEmptyMessagesLimit,
		Adapter:            &DefaultAdapter{},
	}

	for _, opt := range opts {
		opt(&c)
	}

	return c
}

func (c *ClientConfig) GetApiKey() string {
	if c.apiKeyFunc != nil {
		return c.apiKeyFunc()
	}
	return c.apiKey
}

func WithBaseURL(baseUrl string) ClientOption {
	return func(c *ClientConfig) {
		c.BaseURL = baseUrl
	}
}

func WithAPIVersion(apiVersion APIVersion) ClientOption {
	return func(c *ClientConfig) {
		c.APIVersion = apiVersion
	}
}

func WithHTTPClient(cli *http.Client) ClientOption {
	return func(c *ClientConfig) {
		c.HTTPClient = cli
	}
}

func WithEmptyMessagesLimit(limit uint) ClientOption {
	return func(c *ClientConfig) {
		c.EmptyMessagesLimit = limit
	}
}

func WithBetaVersion(betaVersion ...BetaVersion) ClientOption {
	return func(c *ClientConfig) {
		c.BetaVersion = betaVersion
	}
}

func WithVertexAI(projectID string, location string) ClientOption {
	return func(c *ClientConfig) {
		c.BaseURL = fmt.Sprintf(
			"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/anthropic/models",
			location,
			projectID,
			location,
		)
		c.APIVersion = APIVersionVertex20231016
		c.Adapter = &VertexAdapter{}
	}
}

// WithBedrock configures the client to talk to Claude on Amazon Bedrock via the
// bedrock-runtime InvokeModel REST API in the given AWS region.
//
// Authentication is intentionally left to the caller: Bedrock requires AWS
// SigV4-signed requests, so supply an *http.Client whose transport signs
// outbound requests (e.g. built on aws-sdk-go-v2) via WithHTTPClient. The
// adapter sets no auth headers of its own, keeping this library dependency-free.
//
// Pass full Bedrock model IDs as the request Model (e.g.
// "global.anthropic.claude-opus-4-6-v1" or
// "us.anthropic.claude-sonnet-4-5-20250929-v1:0"); they are placed directly in
// the request URL. To target a custom endpoint (e.g. a VPC endpoint), apply
// WithBaseURL after WithBedrock.
func WithBedrock(region string) ClientOption {
	return func(c *ClientConfig) {
		c.BaseURL = fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com", region)
		c.APIVersion = APIVersionBedrock20230531
		c.Adapter = &BedrockAdapter{}
	}
}

func WithApiKeyFunc(apiKeyFunc ApiKeyFunc) ClientOption {
	return func(c *ClientConfig) {
		c.apiKeyFunc = apiKeyFunc
	}
}
