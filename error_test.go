package anthropic_test

import (
	"encoding/json"
	"testing"

	"github.com/liushuangls/go-anthropic/v2"
)

func TestErrorResponseRequestID(t *testing.T) {
	body := `{"type":"error","request_id":"req_123","error":{"type":"invalid_request_error","message":"bad"}}`
	var resp anthropic.ErrorResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if resp.RequestID != "req_123" {
		t.Fatalf("unexpected request_id: %q", resp.RequestID)
	}
	if resp.Error == nil || resp.Error.Type != anthropic.ErrTypeInvalidRequest {
		t.Fatalf("unexpected error payload: %+v", resp.Error)
	}
}

func TestIsXError(t *testing.T) {
	countBool := func(bools []bool) int {
		count := 0
		for _, b := range bools {
			if b {
				count++
			}
		}
		return count
	}

	errTypes := []anthropic.ErrType{
		anthropic.ErrTypeInvalidRequest,
		anthropic.ErrTypeAuthentication,
		anthropic.ErrTypePermission,
		anthropic.ErrTypeNotFound,
		anthropic.ErrTypeTooLarge,
		anthropic.ErrTypeRateLimit,
		anthropic.ErrTypeApi,
		anthropic.ErrTypeOverloaded,
	}
	isErrFuncs := func(e anthropic.APIError) []bool {
		return []bool{
			e.IsInvalidRequestErr(),
			e.IsAuthenticationErr(),
			e.IsPermissionErr(),
			e.IsNotFoundErr(),
			e.IsTooLargeErr(),
			e.IsRateLimitErr(),
			e.IsApiErr(),
			e.IsOverloadedErr(),
		}
	}

	apiErrors := []anthropic.APIError{}
	for _, errType := range errTypes {
		apiErrors = append(apiErrors, anthropic.APIError{
			Type:    errType,
			Message: "fake message",
		})
	}

	for i, e := range apiErrors {
		isErrorType := isErrFuncs(e)

		// Expect only one error type to be true for each error
		numErrorType := countBool(isErrorType)
		if numErrorType != 1 {
			t.Errorf("Expected 1 error type to be true, got %d, for error %d", numErrorType, i)
		}

		// Expect the error type to be true for the correct error
		if !isErrorType[i] {
			t.Errorf("Expected error type %T to be true, got false", e)
		}
	}
}
