package parallelworks

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

const (
	// APIKeyPrefix is the prefix for Parallel Works API keys
	APIKeyPrefix = "pwt_"
	// OpaqueAccessTokenPrefix marks an opaque OAuth access token, such as the one
	// `pw auth login` saves; it is sent as a Bearer token like a JWT.
	OpaqueAccessTokenPrefix = "pwoa_"
	// UserTokenPrefix marks an opaque user token. It names its platform host as
	// an API key does, and is sent as a Bearer token like a JWT.
	UserTokenPrefix = "pwut_"
)

// ErrInvalidCredential is returned when a credential cannot be parsed
var ErrInvalidCredential = errors.New("invalid credential format")

// ErrNoPlatformHost is returned when platform host cannot be extracted from credential
var ErrNoPlatformHost = errors.New("could not extract platform host from credential")

// IsAPIKey returns true if the credential appears to be an API key (starts with "pwt_")
func IsAPIKey(credential string) bool {
	return strings.HasPrefix(strings.TrimSpace(credential), APIKeyPrefix)
}

// IsToken returns true if the credential is sent as a Bearer token: a JWT, whose
// three base64-encoded parts are separated by dots, an opaque OAuth access token,
// or an opaque user token.
func IsToken(credential string) bool {
	credential = strings.TrimSpace(credential)
	if strings.HasPrefix(credential, OpaqueAccessTokenPrefix) || strings.HasPrefix(credential, UserTokenPrefix) {
		return true
	}
	parts := strings.Split(credential, ".")
	return len(parts) == 3 && !strings.HasPrefix(credential, APIKeyPrefix)
}

// ExtractPlatformHost extracts the platform host from an API key or token.
//
// For API keys (pwt_xxxx.yyyy) and opaque user tokens (pwut_xxxx.yyyy): decodes the first part after the prefix
// For JWT tokens: decodes the payload (second segment) and reads platform_host field
func ExtractPlatformHost(credential string) (string, error) {
	credential = strings.TrimSpace(credential)
	if IsAPIKey(credential) {
		return extractEncodedHost(credential, APIKeyPrefix)
	}
	if strings.HasPrefix(credential, UserTokenPrefix) {
		return extractEncodedHost(credential, UserTokenPrefix)
	}
	if strings.HasPrefix(credential, OpaqueAccessTokenPrefix) {
		return "", ErrNoPlatformHost
	}
	if IsToken(credential) {
		return extractHostFromToken(credential)
	}
	return "", ErrInvalidCredential
}

// extractEncodedHost extracts the platform host from a credential of the form
// <prefix><base64_host>.<key>.
func extractEncodedHost(credential, prefix string) (string, error) {
	withoutPrefix := strings.TrimPrefix(credential, prefix)

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

// NewClientFromCredential creates a new client using only a credential.
// The platform host is automatically extracted from the credential.
//
// For API keys and opaque user tokens: host is decoded from the first part after the prefix
// For JWT tokens: host is read from the platform_host claim
//
// Example:
//
//	client, err := NewClientFromCredential("pwt_Y2xvdWQucGFyYWxsZWwud29ya3M.xxxxx")
//	// Automatically connects to activate.parallel.works
func NewClientFromCredential(credential string, opts ...ClientOption) (*Client, error) {
	host, err := ExtractPlatformHost(credential)
	if err != nil {
		return nil, err
	}

	// Ensure https:// prefix
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "https://" + host
	}

	credential = strings.TrimSpace(credential)

	// Use Bearer for tokens, Basic for API keys (any format)
	var auth AuthProvider
	if IsToken(credential) {
		auth = &BearerAuth{Token: credential}
	} else {
		auth = &BasicAuth{Username: credential, Password: ""}
	}

	allOpts := append([]ClientOption{WithAuth(auth)}, opts...)
	return NewClient(host, allOpts...), nil
}
