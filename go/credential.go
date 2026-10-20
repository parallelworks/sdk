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
	// WorkflowRunTokenPrefix marks a workflow run's opaque token. It names its
	// platform host like an API key and is sent as a Bearer token.
	WorkflowRunTokenPrefix = "pwwr_"
	// InstanceTokenPrefix marks an instance agent's opaque token, named and sent
	// like a workflow run's.
	InstanceTokenPrefix = "pwin_"
	// ClusterSessionTokenPrefix marks a cluster session's opaque token, named and
	// sent like a workflow run's.
	ClusterSessionTokenPrefix = "pwcs_"
)

// hostNamingTokenPrefixes mark opaque tokens that name their platform host like an API key and are sent as Bearer tokens.
var hostNamingTokenPrefixes = []string{WorkflowRunTokenPrefix, InstanceTokenPrefix, ClusterSessionTokenPrefix}

func hostNamingTokenPrefix(credential string) (string, bool) {
	for _, prefix := range hostNamingTokenPrefixes {
		if strings.HasPrefix(credential, prefix) {
			return prefix, true
		}
	}
	return "", false
}

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
// or a workflow run's, instance's or cluster session's token.
func IsToken(credential string) bool {
	credential = strings.TrimSpace(credential)
	if _, ok := hostNamingTokenPrefix(credential); ok || strings.HasPrefix(credential, OpaqueAccessTokenPrefix) {
		return true
	}
	parts := strings.Split(credential, ".")
	return len(parts) == 3 && !strings.HasPrefix(credential, APIKeyPrefix)
}

// ExtractPlatformHost extracts the platform host from an API key or JWT token.
//
// For API keys (pwt_xxxx.yyyy) and workflow run, instance and cluster session
// tokens (pwwr_, pwin_, pwcs_ followed by xxxx.yyyy): decodes
// the first part after the prefix to get the host
// For JWT tokens: decodes the payload (second segment) and reads platform_host field
func ExtractPlatformHost(credential string) (string, error) {
	credential = strings.TrimSpace(credential)
	if IsAPIKey(credential) {
		return extractHostFromAPIKey(credential, APIKeyPrefix)
	}
	if prefix, ok := hostNamingTokenPrefix(credential); ok {
		return extractHostFromAPIKey(credential, prefix)
	}
	if strings.HasPrefix(credential, OpaqueAccessTokenPrefix) {
		return "", ErrNoPlatformHost
	}
	if IsToken(credential) {
		return extractHostFromToken(credential)
	}
	return "", ErrInvalidCredential
}

// extractHostFromAPIKey extracts platform host from a credential of the form
// <prefix><base64_host>.<key>, such as an API key.
func extractHostFromAPIKey(apiKey, prefix string) (string, error) {
	withoutPrefix := strings.TrimPrefix(apiKey, prefix)

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
// For API keys and workflow run, instance and cluster session tokens: host is decoded from the first part after the prefix
// For JWT tokens: host is read from the platform_host claim
// For access tokens (pwoa_), which name no host: PW_PLATFORM_HOST, else the
// server of the credentials file's selected context
//
// Example:
//
//	client, err := NewClientFromCredential("pwt_Y2xvdWQucGFyYWxsZWwud29ya3M.xxxxx")
//	// Automatically connects to activate.parallel.works
func NewClientFromCredential(credential string, opts ...ClientOption) (*Client, error) {
	host, err := ExtractPlatformHost(credential)
	if errors.Is(err, ErrNoPlatformHost) {
		host, err = unnamedCredentialHost(nil, "")
	}
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
