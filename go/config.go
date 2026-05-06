package parallelworks

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CredentialConfig file constants
const (
	pwDirectoryName     = "pw"
	credentialsFileName = ".credentials"
)

// Sentinel errors for config and context operations
var (
	ErrNoContextConfigured  = errors.New("no context configured; use 'pw auth apikey' or 'pw auth token' to authenticate")
	ErrNoCredentials        = errors.New("no credential found in context")
	ErrContextNotFound      = errors.New("context not found")
	ErrContextAlreadyExists = errors.New("context already exists")
	ErrCannotDeleteCurrent  = errors.New("cannot delete current context without --force")
	ErrNoContextsAvailable  = errors.New("no contexts available")
	ErrEmptyContextName     = errors.New("context name cannot be empty")
	ErrMultipleContexts     = errors.New("multiple contexts exist for this user/server combination; use --context-name to specify which to update or to create a new one")
)

// Identity represents a saved authentication context in the credentials file.
type Identity struct {
	ApiKey        string `json:"apikey,omitempty"`
	Token         string `json:"token,omitempty"`
	Server        string `json:"server"`
	Name          string `json:"name"`
	CanonicalName string `json:"canonicalName,omitempty"` // user:<username>@<server>
	Organization  string `json:"organization"`
}

// Credential returns the API key or JWT token from this identity.
func (id *Identity) Credential() string {
	if id.ApiKey != "" {
		return id.ApiKey
	}
	return id.Token
}

// Username extracts the username from CanonicalName (format: "user:<username>@<server>").
func (id *Identity) Username() string {
	if id.CanonicalName == "" {
		return ""
	}
	atIdx := strings.Index(id.CanonicalName, "@")
	if atIdx == -1 {
		return ""
	}
	subject := id.CanonicalName[:atIdx]
	// Remove "user:" prefix if present
	return strings.TrimPrefix(subject, "user:")
}

// CredentialConfig represents the PW credentials file at ~/pw/.credentials.
type CredentialConfig struct {
	Identities      map[string]Identity `json:"identities"`
	CurrentIdentity string              `json:"currentIdentity"`

	// path is the file path this config was loaded from (not serialized)
	path string `json:"-"`
}

// ContextInfo represents information about a context for display.
type ContextInfo struct {
	Name         string `json:"name"`
	Server       string `json:"server"`
	Organization string `json:"organization"`
	IsCurrent    bool   `json:"isCurrent"`
}

// DefaultCredentialConfigPath returns the default credentials file path: ~/pw/.credentials.
// PW_CREDENTIALS_DIR overrides the directory containing .credentials; lets dev
// workflows isolate credentials without redirecting $HOME (which would also
// break software cache / ssh keys when /tmp is mounted noexec).
func DefaultCredentialConfigPath() (string, error) {
	if override := os.Getenv("PW_CREDENTIALS_DIR"); override != "" {
		return filepath.Join(override, credentialsFileName), nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("unable to get home directory: %w", err)
	}
	return filepath.Join(homeDir, pwDirectoryName, credentialsFileName), nil
}

// LoadCredentialConfig reads the credentials file from the default path.
func LoadCredentialConfig() (*CredentialConfig, error) {
	path, err := DefaultCredentialConfigPath()
	if err != nil {
		return nil, err
	}
	return LoadCredentialConfigFrom(path)
}

// LoadCredentialConfigFrom reads the credentials file from a specific path.
func LoadCredentialConfigFrom(path string) (*CredentialConfig, error) {
	cfg := &CredentialConfig{path: path}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		// No file — return empty config
		cfg.Identities = make(map[string]Identity)
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}

	// Treat empty file same as non-existent — return empty config
	if len(data) == 0 {
		cfg.Identities = make(map[string]Identity)
		return cfg, nil
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse credentials file (%s): %w\nThis may indicate a corrupted credentials file. Try deleting the file and re-authenticating with 'pw auth apikey' or 'pw auth token'.", path, err)
	}
	if cfg.Identities == nil {
		cfg.Identities = make(map[string]Identity)
	}
	return cfg, nil
}

// Save writes the config back to its original path.
func (c *CredentialConfig) Save() error {
	if c.path == "" {
		path, err := DefaultCredentialConfigPath()
		if err != nil {
			return err
		}
		c.path = path
	}
	return c.SaveTo(c.path)
}

// SaveTo writes the config to a specific path.
func (c *CredentialConfig) SaveTo(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// resolveOptions holds options for identity resolution.
type resolveOptions struct {
	context      string
	platformHost string
}

// ResolveOption configures identity resolution.
type ResolveOption func(*resolveOptions)

// WithContext overrides the context name (equivalent to --context flag).
func WithContext(name string) ResolveOption {
	return func(o *resolveOptions) {
		o.context = name
	}
}

// WithPlatformHost overrides the platform host (equivalent to --platform-host flag).
func WithPlatformHost(host string) ResolveOption {
	return func(o *resolveOptions) {
		o.platformHost = host
	}
}

// ResolveIdentity returns the active identity using this priority:
//  1. PW_API_KEY env var → construct identity from credential
//  2. context option (from WithContext / --context flag)
//  3. PW_CONTEXT env var
//  4. CredentialConfig.CurrentIdentity
func (c *CredentialConfig) ResolveIdentity(opts ...ResolveOption) (*Identity, error) {
	var o resolveOptions
	for _, opt := range opts {
		opt(&o)
	}

	// Priority 1: PW_API_KEY env var
	if credential := os.Getenv("PW_API_KEY"); credential != "" {
		return c.identityFromCredential(credential, o.platformHost)
	}

	// Determine context name: flag → env → config default
	contextName := c.CurrentIdentity
	if envCtx := os.Getenv("PW_CONTEXT"); envCtx != "" {
		contextName = envCtx
	}
	if o.context != "" {
		contextName = o.context
	}

	if contextName == "" {
		return nil, ErrNoContextConfigured
	}

	identity, exists := c.Identities[contextName]
	if !exists {
		return nil, fmt.Errorf("context %q not found", contextName)
	}

	// Apply platform host override
	if o.platformHost != "" {
		identity.Server = o.platformHost
	}

	return &identity, nil
}

// identityFromCredential constructs an identity from a raw credential string.
func (c *CredentialConfig) identityFromCredential(credential string, platformHostOverride string) (*Identity, error) {
	host, err := ExtractPlatformHost(credential)
	if err != nil {
		// If we can't extract the host, use the override or return error
		if platformHostOverride != "" {
			host = platformHostOverride
		} else {
			return nil, err
		}
	}

	if platformHostOverride != "" {
		host = platformHostOverride
	}

	id := &Identity{
		Server: host,
	}
	if IsToken(credential) {
		id.Token = credential
	} else {
		id.ApiKey = credential
	}
	return id, nil
}

// Contexts returns a sorted list of all contexts.
func (c *CredentialConfig) Contexts() []ContextInfo {
	if len(c.Identities) == 0 {
		return nil
	}

	contexts := make([]ContextInfo, 0, len(c.Identities))
	for name, identity := range c.Identities {
		contexts = append(contexts, ContextInfo{
			Name:         name,
			Server:       identity.Server,
			Organization: identity.Organization,
			IsCurrent:    name == c.CurrentIdentity,
		})
	}

	sort.Slice(contexts, func(i, j int) bool {
		return contexts[i].Name < contexts[j].Name
	})

	return contexts
}

// ContextNames returns just the context names, sorted (useful for tab completion).
func (c *CredentialConfig) ContextNames() []string {
	contexts := c.Contexts()
	names := make([]string, len(contexts))
	for i, ctx := range contexts {
		names[i] = ctx.Name
	}
	return names
}

// SetCurrentContext switches the active context.
func (c *CredentialConfig) SetCurrentContext(name string) error {
	if name == "" {
		return ErrEmptyContextName
	}
	if _, exists := c.Identities[name]; !exists {
		return ErrContextNotFound
	}
	c.CurrentIdentity = name
	return nil
}

// RenameContext renames a context from oldName to newName.
func (c *CredentialConfig) RenameContext(oldName, newName string) error {
	if oldName == "" || newName == "" {
		return ErrEmptyContextName
	}

	identity, exists := c.Identities[oldName]
	if !exists {
		return ErrContextNotFound
	}
	if _, exists := c.Identities[newName]; exists {
		return ErrContextAlreadyExists
	}

	// Set canonical name if empty (handles legacy contexts)
	if identity.CanonicalName == "" {
		identity.CanonicalName = oldName
	}
	identity.Name = newName

	c.Identities[newName] = identity
	delete(c.Identities, oldName)

	if c.CurrentIdentity == oldName {
		c.CurrentIdentity = newName
	}
	return nil
}

// DeleteContext removes a context by name.
// If force is false and it's the current context, returns ErrCannotDeleteCurrent.
// Returns the new current context name (empty if none remain or deleted wasn't current).
func (c *CredentialConfig) DeleteContext(name string, force bool) (newCurrent string, err error) {
	if name == "" {
		return "", ErrEmptyContextName
	}
	if _, exists := c.Identities[name]; !exists {
		return "", ErrContextNotFound
	}
	if c.CurrentIdentity == name && !force {
		return "", ErrCannotDeleteCurrent
	}

	wasCurrent := c.CurrentIdentity == name
	delete(c.Identities, name)

	if wasCurrent {
		c.CurrentIdentity = ""
		if len(c.Identities) > 0 {
			names := make([]string, 0, len(c.Identities))
			for n := range c.Identities {
				names = append(names, n)
			}
			sort.Strings(names)
			c.CurrentIdentity = names[0]
			newCurrent = c.CurrentIdentity
		}
	}
	return newCurrent, nil
}

// UpsertContext creates or updates a context.
// If contextName is empty, uses canonicalName matching to find the right context.
func (c *CredentialConfig) UpsertContext(contextName, canonicalName, server, org, credential string) error {
	if c.Identities == nil {
		c.Identities = make(map[string]Identity)
	}

	var apiKey, token string
	if IsAPIKey(credential) {
		apiKey = credential
	} else {
		token = credential
	}

	if contextName != "" {
		if existing, exists := c.Identities[contextName]; exists {
			existing.ApiKey = apiKey
			existing.Token = token
			existing.Server = server
			existing.Organization = org
			existing.Name = contextName
			if existing.CanonicalName == "" {
				existing.CanonicalName = canonicalName
			}
			c.Identities[contextName] = existing
		} else {
			c.Identities[contextName] = Identity{
				ApiKey:        apiKey,
				Token:         token,
				Server:        server,
				Name:          contextName,
				CanonicalName: canonicalName,
				Organization:  org,
			}
		}
		c.CurrentIdentity = contextName
		return nil
	}

	// No explicit name — use canonical name matching
	count := c.countContextsByCanonicalName(canonicalName)
	if count > 1 {
		return ErrMultipleContexts
	}

	existingName := c.findContextByCanonicalName(canonicalName)
	if existingName != "" {
		existing := c.Identities[existingName]
		existing.ApiKey = apiKey
		existing.Token = token
		existing.Server = server
		existing.Organization = org
		c.Identities[existingName] = existing
		c.CurrentIdentity = existingName
	} else {
		c.Identities[canonicalName] = Identity{
			ApiKey:        apiKey,
			Token:         token,
			Server:        server,
			Name:          canonicalName,
			CanonicalName: canonicalName,
			Organization:  org,
		}
		c.CurrentIdentity = canonicalName
	}
	return nil
}

func (c *CredentialConfig) findContextByCanonicalName(canonicalName string) string {
	for name, identity := range c.Identities {
		if identity.CanonicalName == canonicalName {
			return name
		}
	}
	return ""
}

func (c *CredentialConfig) countContextsByCanonicalName(canonicalName string) int {
	count := 0
	for _, identity := range c.Identities {
		if identity.CanonicalName == canonicalName {
			count++
		}
	}
	return count
}

// NewClientFromCredentialConfig creates an authenticated client from the credentials file.
// Respects PW_API_KEY, PW_CONTEXT env vars, and flag overrides via ResolveOption.
//
// ResolveOption values (WithContext, WithPlatformHost) are extracted from opts
// and used for identity resolution. Remaining ClientOption values configure the client.
func NewClientFromCredentialConfig(opts ...any) (*Client, error) {
	var resolveOpts []ResolveOption
	var clientOpts []ClientOption

	for _, opt := range opts {
		switch v := opt.(type) {
		case ResolveOption:
			resolveOpts = append(resolveOpts, v)
		case ClientOption:
			clientOpts = append(clientOpts, v)
		}
	}

	cfg, err := LoadCredentialConfig()
	if err != nil {
		return nil, err
	}

	identity, err := cfg.ResolveIdentity(resolveOpts...)
	if err != nil {
		return nil, err
	}

	credential := identity.Credential()
	if credential == "" {
		return nil, ErrNoCredentials
	}

	// Use Bearer for JWT tokens, Basic for API keys (any format)
	var auth AuthProvider
	if IsToken(credential) {
		auth = &BearerAuth{Token: credential}
	} else {
		auth = &BasicAuth{Username: credential, Password: ""}
	}

	// Use identity.Server as base URL (respects platform host overrides)
	host := identity.Server
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "https://" + host
	}

	return NewClient(host, append([]ClientOption{WithAuth(auth)}, clientOpts...)...), nil
}

// PlatformHost returns the hostname from the client's base URL (without protocol prefix).
func (c *Client) PlatformHost() string {
	host := c.baseURL
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	return host
}
