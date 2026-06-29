package anthropic

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

var _ ClientAdapter = (*VertexAdapter)(nil)

type VertexAdapter struct {
	// location is the Vertex location configured via WithVertexAI. It is used to
	// auto-route models that Google only serves from its multi-region endpoints.
	location string
}

// vertexLegacyModels are the models that predate Google's multi-region and
// global Vertex endpoints. Per Google, "Claude Sonnet 4.5 and all future models"
// are served from the multi-region ("us"/"eu") and global endpoints, while
// earlier models remain on the per-region endpoints. Any model NOT in this set
// is treated as requiring the newer endpoints.
//
// This list necessarily tracks Google's rollout; new model constants are treated
// as requiring the multi-region/global endpoints by default.
var vertexLegacyModels = map[Model]bool{
	ModelClaude2Dot0:               true,
	ModelClaude2Dot1:               true,
	ModelClaude3Opus20240229:       true,
	ModelClaude3Sonnet20240229:     true,
	ModelClaude3Dot5Sonnet20240620: true,
	ModelClaude3Dot5Sonnet20241022: true,
	ModelClaude3Dot5SonnetLatest:   true,
	ModelClaude3Haiku20240307:      true,
	ModelClaude3Dot5HaikuLatest:    true,
	ModelClaude3Dot5Haiku20241022:  true,
	ModelClaude3Dot7SonnetLatest:   true,
	ModelClaude3Dot7Sonnet20250219: true,
	ModelClaudeOpus4Dot0:           true,
	ModelClaudeOpus4V20250514:      true,
	ModelClaudeOpus4Dot1:           true,
	ModelClaudeOpus4Dot1V20250805:  true,
	ModelClaudeSonnet4Dot0:         true,
	ModelClaudeSonnet4V20250514:    true,
}

// requiresVertexRepEndpoint reports whether a model must be served from Vertex's
// multi-region/global endpoints rather than a single-region endpoint. New models
// (Sonnet 4.5 and later) are only available on the newer endpoints, so anything
// not known to be legacy is treated as requiring them.
func requiresVertexRepEndpoint(model Model) bool {
	return !vertexLegacyModels[model]
}

// vertexRepGeography maps a single Vertex region to its multi-region endpoint
// geography ("us" or "eu"). It returns "" when the region has no multi-region
// endpoint (e.g. an Asia-Pacific region) or is already a multi-region/global
// location, in which case no rewrite is applied.
func vertexRepGeography(region string) string {
	switch {
	case strings.HasPrefix(region, "us-"):
		return "us"
	case strings.HasPrefix(region, "europe-"):
		return "eu"
	default:
		return ""
	}
}

// effectiveBaseURL rewrites the configured single-region base URL to the matching
// multi-region endpoint when the model requires it — mirroring the URL rewrite
// callers previously had to do with an HTTP roundtripper. Requests on models that
// are available regionally, and clients already configured for a multi-region or
// global location, are left untouched.
//
// Note: a single-region endpoint is required for provisioned throughput. If you
// pin a region for that reason, drive those models on a separate client.
func (v *VertexAdapter) effectiveBaseURL(baseURL string, model Model) string {
	if !requiresVertexRepEndpoint(model) {
		return baseURL
	}
	geo := vertexRepGeography(v.location)
	if geo == "" || geo == v.location {
		return baseURL
	}
	baseURL = strings.Replace(
		baseURL,
		v.location+"-aiplatform.googleapis.com",
		"aiplatform."+geo+".rep.googleapis.com",
		1,
	)
	baseURL = strings.Replace(
		baseURL,
		"/locations/"+v.location+"/",
		"/locations/"+geo+"/",
		1,
	)
	return baseURL
}

func (v *VertexAdapter) TranslateError(resp *http.Response, body []byte) (error, bool) {
	switch resp.StatusCode {
	case http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusTooManyRequests:
		var errRes VertexAIErrorResponse
		err := json.Unmarshal(body, &errRes)
		if err != nil {
			// it could be an array
			var errResArr []VertexAIErrorResponse
			err = json.Unmarshal(body, &errResArr)
			if err == nil && len(errResArr) > 0 {
				errRes = errResArr[0]
			}
		}

		if err != nil || errRes.Error == nil {
			reqErr := RequestError{
				StatusCode: resp.StatusCode,
				Err:        err,
				Body:       body,
			}
			return &reqErr, true
		}
		return fmt.Errorf(
			"error, status code: %d, message: %w",
			resp.StatusCode,
			errRes.Error,
		), true
	}
	return nil, false
}

func (v *VertexAdapter) PrepareRequest(
	c *Client,
	method string,
	urlSuffix string,
	body any,
) (string, error) {
	support, ok := body.(VertexAISupport)
	if !ok {
		return "", fmt.Errorf("this call is not supported by the Vertex AI API")
	}

	// Capture the model before it is cleared; it drives both URL placement and
	// multi-region endpoint routing.
	routingModel := support.GetModel()

	// On Vertex AI anthropic_version travels in the request body rather than as
	// a header. This is set for every supported endpoint.
	support.SetAnthropicVersion(c.config.APIVersion)

	// modelSegment is the path component immediately before the ":action"
	// specifier; action is rawPredict or streamRawPredict.
	var modelSegment, action string
	switch urlSuffix {
	case "/messages":
		// The model is lifted into the URL and cleared from the body.
		modelSegment = routingModel.asVertexModel()
		support.SetModel("")
		if support.IsStreaming() {
			action = "streamRawPredict"
		} else {
			action = "rawPredict"
		}
	case "/messages/count_tokens":
		// "count-tokens" is a fixed routing pseudo-model; the real model stays
		// in the request body so the endpoint knows what to count.
		modelSegment = "count-tokens"
		action = "rawPredict"
	default:
		return "", fmt.Errorf("unknown suffix: %s", urlSuffix)
	}

	// New models (Sonnet 4.5+) are only served from Vertex's multi-region/global
	// endpoints, so auto-route them there when a single region was configured.
	baseURL := v.effectiveBaseURL(c.config.BaseURL, routingModel)

	return fmt.Sprintf("%s/%s:%s", baseURL, modelSegment, action), nil
}

func (v *VertexAdapter) SetRequestHeaders(c *Client, req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+c.config.GetApiKey())
	return nil
}
