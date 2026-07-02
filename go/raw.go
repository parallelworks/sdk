package parallelworks

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// doRaw executes an HTTP request and returns the raw response body.
// Unlike do(), it does not attempt to unmarshal the response as JSON.
func (c *Client) doRaw(ctx context.Context, method string, path string, body io.Reader) ([]byte, error) {
	fullURL := c.baseURL + path

	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}

	if c.auth != nil {
		if err := c.auth.Apply(req); err != nil {
			return nil, fmt.Errorf("applying auth: %w", err)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{StatusCode: resp.StatusCode, Status: resp.Status, Body: respBody}
	}

	return respBody, nil
}

// GetRaw performs an authenticated GET request and returns the raw response body.
func (c *Client) GetRaw(ctx context.Context, path string) ([]byte, error) {
	return c.doRaw(ctx, http.MethodGet, path, nil)
}

// PostRaw performs an authenticated POST request with a raw body and returns the raw response body.
func (c *Client) PostRaw(ctx context.Context, path string, body []byte) ([]byte, error) {
	return c.doRaw(ctx, http.MethodPost, path, bytes.NewReader(body))
}

// PutRaw performs an authenticated PUT request with a raw body and returns the raw response body.
func (c *Client) PutRaw(ctx context.Context, path string, body []byte) ([]byte, error) {
	return c.doRaw(ctx, http.MethodPut, path, bytes.NewReader(body))
}

// PatchRaw performs an authenticated PATCH request with a raw body and returns the raw response body.
func (c *Client) PatchRaw(ctx context.Context, path string, body []byte) ([]byte, error) {
	return c.doRaw(ctx, http.MethodPatch, path, bytes.NewReader(body))
}

// DeleteRaw performs an authenticated DELETE request.
func (c *Client) DeleteRaw(ctx context.Context, path string) error {
	_, err := c.doRaw(ctx, http.MethodDelete, path, nil)
	return err
}

// StreamOption configures a single Stream call.
type StreamOption func(*streamConfig)

type streamConfig struct {
	retryOnNetworkError bool
}

// WithStreamRetry retries the request on transient network errors (connection
// reset, unexpected EOF, timeout) that occur before any response is received,
// using the client's RetryConfig for backoff. Only safe for side-effect-free
// streaming endpoints such as chat completions: a request the server already
// processed but whose response was lost would have its side effect repeated on
// retry, so never enable this for creates or other mutations.
func WithStreamRetry() StreamOption {
	return func(c *streamConfig) { c.retryOnNetworkError = true }
}

// Stream performs an authenticated HTTP request and returns the raw *http.Response
// without reading the body. The caller is responsible for closing resp.Body.
// This is useful for Server-Sent Events (SSE) and other streaming responses.
func (c *Client) Stream(ctx context.Context, method, path string, body io.Reader, headers map[string]string, opts ...StreamOption) (*http.Response, error) {
	var cfg streamConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	fullURL := c.baseURL + path
	retryEnabled := cfg.retryOnNetworkError && c.retryConfig != nil

	// Buffer the body once so it can be replayed on each retry attempt.
	var bodyBytes []byte
	if retryEnabled && body != nil {
		var err error
		bodyBytes, err = io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("reading request body: %w", err)
		}
	}

	maxAttempts := 1
	if retryEnabled {
		maxAttempts = 1 + c.retryConfig.MaxRetries
	}

	for attempt := 0; ; attempt++ {
		var reqBody io.Reader = body
		if retryEnabled && bodyBytes != nil {
			reqBody = bytes.NewReader(bodyBytes)
		}

		req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		if c.userAgent != "" {
			req.Header.Set("User-Agent", c.userAgent)
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}

		if c.auth != nil {
			if err := c.auth.Apply(req); err != nil {
				return nil, fmt.Errorf("applying auth: %w", err)
			}
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if retryEnabled && attempt < maxAttempts-1 && isRetryableNetworkError(err) {
				timer := time.NewTimer(retryDelay(attempt, *c.retryConfig, nil))
				select {
				case <-ctx.Done():
					timer.Stop()
					return nil, ctx.Err()
				case <-timer.C:
				}
				continue
			}
			return nil, fmt.Errorf("executing request: %w", err)
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, &APIError{StatusCode: resp.StatusCode, Status: resp.Status, Body: respBody}
		}

		return resp, nil
	}
}

// AuthHeader returns the HTTP headers that would be set by this client's auth provider.
// This is useful for passing auth to WebSocket connections or other non-SDK HTTP clients.
func (c *Client) AuthHeader() (http.Header, error) {
	if c.auth == nil {
		return http.Header{}, nil
	}
	req, err := http.NewRequest(http.MethodGet, c.baseURL, nil)
	if err != nil {
		return nil, err
	}
	if err := c.auth.Apply(req); err != nil {
		return nil, err
	}
	return req.Header, nil
}
