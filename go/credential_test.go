package parallelworks

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
)

func TestIsAPIKey(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"pwt_abc.def", true},
		{"  pwt_abc.def  ", true}, // trims whitespace
		{"eyJhbGciOiJIUzI1NiJ9.eyJwbGF0Zm9ybV9ob3N0IjoiZXhhbXBsZS5jb20ifQ.sig", false},
		{"", false},
		{"not-a-key", false},
	}
	for _, tt := range tests {
		if got := IsAPIKey(tt.input); got != tt.want {
			t.Errorf("IsAPIKey(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestIsToken(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"header.payload.signature", true},
		{"  header.payload.signature  ", true}, // trims whitespace
		{"pwt_abc.def.ghi", false},             // API key with 3 parts
		{"no-dots", false},
		{"one.dot", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsToken(tt.input); got != tt.want {
			t.Errorf("IsToken(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestExtractPlatformHost_APIKey(t *testing.T) {
	encoded := base64.RawURLEncoding.EncodeToString([]byte("cloud.parallel.works"))
	apiKey := APIKeyPrefix + encoded + ".secretkey"

	host, err := ExtractPlatformHost(apiKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "cloud.parallel.works" {
		t.Errorf("got %q, want %q", host, "cloud.parallel.works")
	}
}

func TestExtractPlatformHost_APIKey_StdEncoding(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("cloud.parallel.works"))
	apiKey := APIKeyPrefix + encoded + ".secretkey"

	host, err := ExtractPlatformHost(apiKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "cloud.parallel.works" {
		t.Errorf("got %q, want %q", host, "cloud.parallel.works")
	}
}

func TestExtractPlatformHost_APIKey_Whitespace(t *testing.T) {
	encoded := base64.RawURLEncoding.EncodeToString([]byte("cloud.parallel.works"))
	apiKey := "  " + APIKeyPrefix + encoded + ".secretkey\n"

	host, err := ExtractPlatformHost(apiKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "cloud.parallel.works" {
		t.Errorf("got %q, want %q", host, "cloud.parallel.works")
	}
}

func TestExtractPlatformHost_Token(t *testing.T) {
	payload, _ := json.Marshal(jwtClaims{PlatformHost: "activate.parallel.works"})
	token := "eyJhbGciOiJIUzI1NiJ9." +
		base64.RawURLEncoding.EncodeToString(payload) +
		".signature"

	host, err := ExtractPlatformHost(token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "activate.parallel.works" {
		t.Errorf("got %q, want %q", host, "activate.parallel.works")
	}
}

func TestExtractPlatformHost_InvalidCredential(t *testing.T) {
	_, err := ExtractPlatformHost("not-a-credential")
	if err != ErrInvalidCredential {
		t.Errorf("got %v, want ErrInvalidCredential", err)
	}
}

func TestExtractPlatformHost_MissingHost(t *testing.T) {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(""))
	apiKey := APIKeyPrefix + encoded + ".secretkey"

	_, err := ExtractPlatformHost(apiKey)
	if err != ErrNoPlatformHost {
		t.Errorf("got %v, want ErrNoPlatformHost", err)
	}
}

func TestExtractPlatformHost_TokenMissingHost(t *testing.T) {
	payload, _ := json.Marshal(jwtClaims{PlatformHost: ""})
	token := "eyJhbGciOiJIUzI1NiJ9." +
		base64.RawURLEncoding.EncodeToString(payload) +
		".signature"

	_, err := ExtractPlatformHost(token)
	if err != ErrNoPlatformHost {
		t.Errorf("got %v, want ErrNoPlatformHost", err)
	}
}

func TestNewClientFromCredential_APIKey(t *testing.T) {
	encoded := base64.RawURLEncoding.EncodeToString([]byte("cloud.parallel.works"))
	apiKey := APIKeyPrefix + encoded + ".secretkey"

	client, err := NewClientFromCredential(apiKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestNewClientFromCredential_Token(t *testing.T) {
	payload, _ := json.Marshal(jwtClaims{PlatformHost: "activate.parallel.works"})
	token := "eyJhbGciOiJIUzI1NiJ9." +
		base64.RawURLEncoding.EncodeToString(payload) +
		".signature"

	client, err := NewClientFromCredential(token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestNewClientFromCredential_APIKeyAuth(t *testing.T) {
	encoded := base64.RawURLEncoding.EncodeToString([]byte("cloud.parallel.works"))
	apiKey := APIKeyPrefix + encoded + ".secretkey"

	client, err := NewClientFromCredential(apiKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify auth is applied correctly
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.com", nil)
	if err := client.auth.Apply(req); err != nil {
		t.Fatalf("auth.Apply failed: %v", err)
	}

	auth := req.Header.Get("Authorization")
	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte(apiKey+":"))
	if auth != expected {
		t.Errorf("got Authorization %q, want %q", auth, expected)
	}
}

func TestNewClientFromCredential_TokenAuth(t *testing.T) {
	payload, _ := json.Marshal(jwtClaims{PlatformHost: "activate.parallel.works"})
	token := "eyJhbGciOiJIUzI1NiJ9." +
		base64.RawURLEncoding.EncodeToString(payload) +
		".signature"

	client, err := NewClientFromCredential(token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.com", nil)
	if err := client.auth.Apply(req); err != nil {
		t.Fatalf("auth.Apply failed: %v", err)
	}

	expected := "Bearer " + token
	if got := req.Header.Get("Authorization"); got != expected {
		t.Errorf("got Authorization %q, want %q", got, expected)
	}
}
