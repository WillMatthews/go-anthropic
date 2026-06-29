package anthropic

import (
	"fmt"
	"net/http"
)

var _ ClientAdapter = (*BedrockAdapter)(nil)

// BedrockAdapter targets Claude on Amazon Bedrock through the bedrock-runtime
// InvokeModel REST API.
//
// The request shape differs from the direct Anthropic API in the same way as
// Vertex AI: the model is carried in the URL (not the body) and anthropic_version
// is sent in the body. Streaming is selected by the endpoint
// (invoke-with-response-stream) rather than a "stream" body field.
//
// Authentication (AWS SigV4) is the caller's responsibility — provide a signing
// *http.Client via WithHTTPClient. The adapter deliberately sets no auth headers,
// keeping the library free of an AWS SDK dependency.
type BedrockAdapter struct{}

func (b *BedrockAdapter) TranslateError(resp *http.Response, body []byte) (error, bool) {
	// Defer to the default error handling: model-level errors come back in the
	// Anthropic error envelope, and AWS-level errors fall through to RequestError
	// carrying the raw body.
	return nil, false
}

func (b *BedrockAdapter) PrepareRequest(
	c *Client,
	method string,
	urlSuffix string,
	body any,
) (string, error) {
	support, ok := body.(VertexAISupport)
	if !ok {
		return "", fmt.Errorf("this call is not supported by the Amazon Bedrock API")
	}

	// The Bedrock InvokeModel API only exposes message generation; count_tokens,
	// batches, and the other endpoints are not available on this path.
	if urlSuffix != "/messages" {
		return "", fmt.Errorf("unsupported Amazon Bedrock endpoint: %s", urlSuffix)
	}

	// anthropic_version travels in the body on Bedrock; the model is lifted into
	// the URL and cleared from the body.
	model := support.GetModel()
	support.SetAnthropicVersion(c.config.APIVersion)
	support.SetModel("")

	action := "invoke"
	if support.IsStreaming() {
		action = "invoke-with-response-stream"
		// The stream is implied by the endpoint; Bedrock rejects a "stream" field
		// in the InvokeModel body, so clear it.
		if s, ok := body.(interface{ SetStream(bool) }); ok {
			s.SetStream(false)
		}
	}

	return fmt.Sprintf("%s/model/%s/%s", c.config.BaseURL, model, action), nil
}

func (b *BedrockAdapter) SetRequestHeaders(c *Client, req *http.Request) error {
	// No auth headers: the caller's signing http.Client adds AWS SigV4. The
	// Content-Type/Accept headers set by newRequest are sufficient for signing.
	return nil
}
