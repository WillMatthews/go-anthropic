package anthropic

import (
	"net/http"
	"testing"
)

// fullValidRateLimitHeaders returns a header set with every rate limit header
// present and valid, so tests can omit/corrupt a single header in isolation.
func fullValidRateLimitHeaders() http.Header {
	h := http.Header{}
	h.Set("anthropic-ratelimit-requests-limit", "100")
	h.Set("anthropic-ratelimit-requests-remaining", "99")
	h.Set("anthropic-ratelimit-requests-reset", "2024-06-04T07:13:19Z")
	h.Set("anthropic-ratelimit-tokens-limit", "10000")
	h.Set("anthropic-ratelimit-tokens-remaining", "9900")
	h.Set("anthropic-ratelimit-tokens-reset", "2024-06-04T07:13:19Z")
	return h
}

func TestParseIntHeaderAbsentIsZeroNoError(t *testing.T) {
	// retry-after is absent here; it must parse to 0 with no error and never
	// a negative sentinel.
	h := fullValidRateLimitHeaders()

	headers, err := newRateLimitHeaders(h)
	if err != nil {
		t.Fatalf("expected no error for absent optional header, got: %s", err)
	}
	if headers.RetryAfter != 0 {
		t.Fatalf("expected RetryAfter 0 when absent, got %d", headers.RetryAfter)
	}
}

func TestParseIntHeaderMalformedIsError(t *testing.T) {
	// A malformed (non-numeric) optional header must produce an error rather
	// than being silently masked.
	h := fullValidRateLimitHeaders()
	h.Set("retry-after", "not-a-number")

	_, err := newRateLimitHeaders(h)
	if err == nil {
		t.Fatalf("expected error for malformed optional header, got nil")
	}
}
