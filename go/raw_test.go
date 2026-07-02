package parallelworks

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func okResponse(req *http.Request) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader("data: {}\n\n")),
		Request:    req,
	}
}

func fastRetryClient(t *testing.T, rt http.RoundTripper) *Client {
	t.Helper()
	return NewClient("https://example.test",
		WithHTTPClient(&http.Client{Transport: rt}),
		WithRetry(RetryConfig{MaxRetries: 3, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond, Multiplier: 2}),
	)
}

func TestStreamRetriesTransientNetworkErrorWhenOptedIn(t *testing.T) {
	var attempts int
	rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		if attempts < 3 {
			return nil, io.ErrUnexpectedEOF
		}
		return okResponse(req), nil
	})

	resp, err := fastRetryClient(t, rt).Stream(t.Context(), "POST", "/x", strings.NewReader(`{"a":1}`), nil, WithStreamRetry())
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	resp.Body.Close()
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestStreamDoesNotRetryWithoutOptIn(t *testing.T) {
	var attempts int
	rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		return nil, io.ErrUnexpectedEOF
	})

	if _, err := fastRetryClient(t, rt).Stream(t.Context(), "POST", "/x", strings.NewReader(`{}`), nil); err == nil {
		t.Fatal("expected error without opt-in")
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt without opt-in, got %d", attempts)
	}
}

func TestStreamReplaysBodyOnRetry(t *testing.T) {
	const payload = `{"hello":"world"}`
	var bodies []string
	rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(req.Body)
		bodies = append(bodies, string(b))
		if len(bodies) < 2 {
			return nil, io.ErrUnexpectedEOF
		}
		return okResponse(req), nil
	})

	resp, err := fastRetryClient(t, rt).Stream(t.Context(), "POST", "/x", strings.NewReader(payload), nil, WithStreamRetry())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
	if len(bodies) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(bodies))
	}
	for i, b := range bodies {
		if b != payload {
			t.Errorf("attempt %d sent body %q, want %q", i, b, payload)
		}
	}
}

func TestStreamDoesNotRetryNonNetworkError(t *testing.T) {
	var attempts int
	rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		return nil, errors.New("boom")
	})

	if _, err := fastRetryClient(t, rt).Stream(t.Context(), "POST", "/x", strings.NewReader(`{}`), nil, WithStreamRetry()); err == nil {
		t.Fatal("expected error")
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt for non-network error, got %d", attempts)
	}
}
