package parallelworks

import (
	"context"
	"encoding/base64"
	"net/http"
)

// WithAPIKey returns a ClientOption that configures API key authentication
// using HTTP Basic Auth. This is the recommended authentication method for
// long-running integrations with configurable expiration.
//
// API keys can be generated from your ACTIVATE account settings.
//
// Example:
//
//	client, err := NewClientWithResponses(
//	    "https://cloud.parallel.works",
//	    WithAPIKey("your-api-key"),
//	)
func WithAPIKey(apiKey string) ClientOption {
	encoded := base64.StdEncoding.EncodeToString([]byte(apiKey + ":"))
	return WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Basic "+encoded)
		return nil
	})
}

// WithToken returns a ClientOption that configures Bearer token (JWT)
// authentication. This is best for scripts and CLI tools.
//
// Tokens expire after 24 hours and can be generated from your ACTIVATE
// account settings.
//
// Example:
//
//	client, err := NewClientWithResponses(
//	    "https://cloud.parallel.works",
//	    WithToken("your-jwt-token"),
//	)
func WithToken(token string) ClientOption {
	return WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	})
}
