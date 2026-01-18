package parallelworks

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

const (
	// APIKeyPrefix is the prefix for Parallel Works API keys
	APIKeyPrefix = "pwt_"
)

// ErrInvalidCredential is returned when a credential cannot be parsed
var ErrInvalidCredential = errors.New("invalid credential format")

// ErrNoPlatformHost is returned when platform host cannot be extracted from credential
var ErrNoPlatformHost = errors.New("could not extract platform host from credential")

// IsAPIKey returns true if the credential appears to be an API key (starts with "pwt_")
func IsAPIKey(credential string) bool {
	return strings.HasPrefix(strings.TrimSpace(credential), APIKeyPrefix)
}

// IsToken returns true if the credential appears to be a JWT token.
// JWTs have three base64-encoded parts separated by dots.
func IsToken(credential string) bool {
	credential = strings.TrimSpace(credential)
	parts := strings.Split(credential, ".")
	return len(parts) == 3 && !strings.HasPrefix(credential, APIKeyPrefix)
}

// ExtractPlatformHost extracts the platform host from an API key or JWT token.
//
// For API keys (pwt_xxxx.yyyy): decodes the first part after pwt_ to get the host
// For JWT tokens: decodes the payload (second segment) and reads platform_host field
func ExtractPlatformHost(credential string) (string, error) {
	credential = strings.TrimSpace(credential)
	if IsAPIKey(credential) {
		return extractHostFromAPIKey(credential)
	}
	if IsToken(credential) {
		return extractHostFromToken(credential)
	}
	return "", ErrInvalidCredential
}

// extractHostFromAPIKey extracts platform host from an API key.
// API key format: pwt_<base64_host>.<key>
func extractHostFromAPIKey(apiKey string) (string, error) {
	// Remove pwt_ prefix
	withoutPrefix := strings.TrimPrefix(apiKey, APIKeyPrefix)

	// Split by dot
	parts := strings.SplitN(withoutPrefix, ".", 2)
	if len(parts) < 2 {
		return "", ErrInvalidCredential
	}

	// Decode the first part (host)
	hostBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		// Try standard encoding
		hostBytes, err = base64.StdEncoding.DecodeString(parts[0])
		if err != nil {
			return "", ErrInvalidCredential
		}
	}

	host := string(hostBytes)
	if host == "" {
		return "", ErrNoPlatformHost
	}

	return host, nil
}

// jwtClaims represents the JWT payload with platform_host
type jwtClaims struct {
	PlatformHost string `json:"platform_host"`
}

// extractHostFromToken extracts platform host from a JWT token.
// JWT format: header.payload.signature (all base64 encoded)
func extractHostFromToken(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", ErrInvalidCredential
	}

	// Decode the payload (second part)
	// JWT uses base64url encoding without padding
	payload := parts[1]
	// Add padding if needed
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}

	payloadBytes, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		// Try RawURLEncoding
		payloadBytes, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return "", ErrInvalidCredential
		}
	}

	var claims jwtClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return "", ErrInvalidCredential
	}

	if claims.PlatformHost == "" {
		return "", ErrNoPlatformHost
	}

	return claims.PlatformHost, nil
}

// WithAPIKey returns a ClientOption that configures API key authentication
// using HTTP Basic Auth. This is the recommended authentication method for
// long-running integrations with configurable expiration.
//
// API keys can be generated from your ACTIVATE account settings and start
// with the prefix "pwt_".
//
// Example:
//
//	client, err := NewClientWithResponses(
//	    "https://activate.parallel.works",
//	    WithAPIKey("pwt_..."),
//	)
func WithAPIKey(apiKey string) ClientOption {
	// Trim whitespace to handle env vars with trailing newlines
	apiKey = strings.TrimSpace(apiKey)
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
//	    "https://activate.parallel.works",
//	    WithToken("eyJhbGciOiJIUzI1NiIs..."),
//	)
func WithToken(token string) ClientOption {
	// Trim whitespace to handle env vars with trailing newlines
	token = strings.TrimSpace(token)
	return WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	})
}

// WithCredential returns a ClientOption that automatically detects the
// credential type and configures the appropriate authentication method.
//
// - If the credential starts with "pwt_", it's treated as an API key (Basic Auth)
// - Otherwise, it's treated as a JWT token (Bearer Auth)
//
// Example:
//
//	credential := os.Getenv("PW_CREDENTIAL")
//	client, err := NewClientWithResponses(
//	    "https://activate.parallel.works",
//	    WithCredential(credential),
//	)
func WithCredential(credential string) ClientOption {
	if IsAPIKey(credential) {
		return WithAPIKey(credential)
	}
	return WithToken(credential)
}

// NewClientFromCredential creates a new client using only a credential.
// The platform host is automatically extracted from the credential.
//
// For API keys: host is decoded from the first part after pwt_
// For JWT tokens: host is read from the platform_host claim
//
// Example:
//
//	client, err := NewClientFromCredential("pwt_Y2xvdWQucGFyYWxsZWwud29ya3M.xxxxx")
//	// Automatically connects to activate.parallel.works
func NewClientFromCredential(credential string, opts ...ClientOption) (*ClientWithResponses, error) {
	host, err := ExtractPlatformHost(credential)
	if err != nil {
		return nil, err
	}

	// Ensure https:// prefix
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "https://" + host
	}

	// Prepend the credential option
	allOpts := append([]ClientOption{WithCredential(credential)}, opts...)

	return NewClientWithResponses(host, allOpts...)
}
