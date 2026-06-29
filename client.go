package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// retryBaseDelay is the base delay used for exponential backoff between
// retries when no positive Retry-After header is provided.
const retryBaseDelay = 500 * time.Millisecond

// isRetryableStatus reports whether a response with the given status code
// should be retried.
func isRetryableStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests, // 429
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		529:                            // overloaded
		return true
	default:
		return false
	}
}

// backoffDelay returns the exponential backoff delay for the given attempt
// (0-based): retryBaseDelay * 2^attempt.
func backoffDelay(attempt int) time.Duration {
	return retryBaseDelay * time.Duration(1<<uint(attempt))
}

// parseRetryAfterSeconds parses the Retry-After header as a number of seconds.
// It returns a non-positive duration when the header is absent, malformed, or
// not positive, so callers fall back to exponential backoff.
func parseRetryAfterSeconds(h http.Header) time.Duration {
	v := h.Get("Retry-After")
	if v == "" {
		return 0
	}
	secs, err := strconv.Atoi(v)
	if err != nil || secs <= 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}

// waitForRetry sleeps for delay, returning early if the context is cancelled.
func waitForRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type Client struct {
	config ClientConfig
}

type Response interface {
	SetHeader(http.Header)
}

type httpHeader http.Header

func (h *httpHeader) SetHeader(header http.Header) {
	*h = httpHeader(header)
}

func (h *httpHeader) Header() http.Header {
	return http.Header(*h)
}

func (h *httpHeader) GetRateLimitHeaders() (RateLimitHeaders, error) {
	return newRateLimitHeaders(h.Header())
}

// NewClient create new Anthropic API client
func NewClient(apiKey string, opts ...ClientOption) *Client {
	return &Client{
		config: newConfig(apiKey, opts...),
	}
}

func (c *Client) sendRequest(req *http.Request, v Response) error {
	ctx := req.Context()

	var res *http.Response
	for attempt := 0; ; attempt++ {
		// Rewind the request body so it can be replayed on retries.
		if attempt > 0 && req.GetBody != nil {
			body, gerr := req.GetBody()
			if gerr != nil {
				return gerr
			}
			req.Body = body
		}

		var err error
		res, err = c.config.HTTPClient.Do(req)
		if err != nil {
			// Transport error: retry if attempts remain and the context is
			// still live, otherwise return the error.
			if attempt < c.config.MaxRetries && ctx.Err() == nil {
				if werr := waitForRetry(ctx, backoffDelay(attempt)); werr != nil {
					return werr
				}
				continue
			}
			return err
		}

		if attempt < c.config.MaxRetries && isRetryableStatus(res.StatusCode) {
			// Honor a positive Retry-After, otherwise exponential backoff.
			delay := parseRetryAfterSeconds(res.Header)
			if delay <= 0 {
				delay = backoffDelay(attempt)
			}
			// Drain and close the body so the connection can be reused.
			_, _ = io.Copy(io.Discard, res.Body)
			res.Body.Close()
			if werr := waitForRetry(ctx, delay); werr != nil {
				return werr
			}
			continue
		}

		break
	}

	defer res.Body.Close()

	v.SetHeader(res.Header)

	if err := c.handlerRequestError(res); err != nil {
		return err
	}

	if err := json.NewDecoder(res.Body).Decode(v); err != nil {
		return err
	}

	return nil
}

func (c *Client) handlerRequestError(resp *http.Response) error {
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("error, reading response body: %w", err)
		}

		// use the adapter to translate the error, if it can
		if err, handled := c.config.Adapter.TranslateError(resp, body); handled {
			return err
		}

		var errRes ErrorResponse
		err = json.Unmarshal(body, &errRes)
		if err != nil || errRes.Error == nil {
			reqErr := RequestError{
				StatusCode: resp.StatusCode,
				Err:        err,
				Body:       body,
			}
			return &reqErr
		}

		return fmt.Errorf("error, status code: %d, message: %w", resp.StatusCode, errRes.Error)
	}
	return nil
}

type requestSetter func(req *http.Request)

func withBetaVersion(betaVersion ...BetaVersion) requestSetter {
	version := ""
	for i, v := range betaVersion {
		version += string(v)
		if i < len(betaVersion)-1 {
			version += ","
		}
	}

	return func(req *http.Request) {
		req.Header.Set("anthropic-beta", version)
	}
}

func (c *Client) newRequest(
	ctx context.Context,
	method, urlSuffix string,
	body any,
	requestSetters ...requestSetter,
) (req *http.Request, err error) {

	// prepare the request
	var fullURL string
	fullURL, err = c.config.Adapter.PrepareRequest(c, method, urlSuffix, body)
	if err != nil {
		return nil, err
	}

	var reqBody []byte
	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}

	req, err = http.NewRequestWithContext(
		ctx,
		method,
		fullURL,
		bytes.NewBuffer(reqBody),
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "application/json; charset=utf-8")

	// set any provider-specific headers (including Authorization)
	if err := c.config.Adapter.SetRequestHeaders(c, req); err != nil {
		return nil, err
	}

	for _, setter := range requestSetters {
		setter(req)
	}

	return req, nil
}

func (c *Client) newStreamRequest(
	ctx context.Context,
	method, urlSuffix string,
	body any,
	requestSetters ...requestSetter,
) (req *http.Request,
	err error) {
	req, err = c.newRequest(ctx, method, urlSuffix, body, requestSetters...)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")

	return req, nil
}
