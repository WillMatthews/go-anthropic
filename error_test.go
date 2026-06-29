package anthropic_test

import (
	"strings"
	"testing"

	"github.com/liushuangls/go-anthropic/v2"
)

func TestRequestErrorNilErrFormat(t *testing.T) {
	// Valid JSON body without an "error" field leaves Err nil.
	reqErr := &anthropic.RequestError{
		StatusCode: 400,
		Err:        nil,
		Body:       []byte("{}"),
	}

	got := reqErr.Error()
	if strings.Contains(got, "<nil>") || strings.Contains(got, "err:") {
		t.Fatalf("expected clean format without nil err segment, got: %q", got)
	}
	if !strings.Contains(got, "status code: 400") || !strings.Contains(got, "body: {}") {
		t.Fatalf("unexpected error format: %q", got)
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
