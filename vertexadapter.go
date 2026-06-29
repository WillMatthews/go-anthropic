package anthropic

import (
	"encoding/json"
	"fmt"
	"net/http"
)

var _ ClientAdapter = (*VertexAdapter)(nil)

type VertexAdapter struct {
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

	// On Vertex AI anthropic_version travels in the request body rather than as
	// a header. This is set for every supported endpoint.
	support.SetAnthropicVersion(c.config.APIVersion)

	// modelSegment is the path component immediately before the ":action"
	// specifier; action is rawPredict or streamRawPredict.
	var modelSegment, action string
	switch urlSuffix {
	case "/messages":
		// The model is lifted into the URL and cleared from the body.
		modelSegment = support.GetModel().asVertexModel()
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

	return fmt.Sprintf("%s/%s:%s", c.config.BaseURL, modelSegment, action), nil
}

func (v *VertexAdapter) SetRequestHeaders(c *Client, req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+c.config.GetApiKey())
	return nil
}
