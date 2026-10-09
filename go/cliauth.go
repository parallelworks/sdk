package parallelworks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultCLICommand is the pw executable CLIAuth runs, looked up on PATH.
	DefaultCLICommand = "pw"
	// DefaultCLITimeout bounds one run of the CLI, as the AWS SDK for Go's
	// credential_process provider does by default.
	DefaultCLITimeout = time.Minute
	// Inside the CLI's two-minute renewal window, so the CLI always renews a token the SDK asks it for.
	cliRenewMargin = time.Minute
)

// ErrSignInExpired is returned when the pw CLI cannot provide an access token
// for a `pw auth` sign-in and no token it provided earlier is still valid.
var ErrSignInExpired = errors.New("the pw CLI could not provide a token for the pw auth sign-in; run pw auth again, or install pw")

// CLIAuth authenticates as a `pw auth` sign-in. It runs
// `pw auth token --print -o json` for a current access token instead of
// rotating the refresh token itself: the platform signs a device out when two
// clients rotate the same refresh token, and the CLI serializes its rotations
// across processes. Like a client-go exec credential plugin or an AWS
// credential_process, it reads the token and its lifetime from the command's
// stdout, an RFC 6749 section 5.1 token response, and caches the token in
// memory until it nears expiry.
type CLIAuth struct {
	// Command is the pw executable, looked up on PATH unless it is a path.
	// Empty means DefaultCLICommand.
	Command string
	// Context is the credentials-file context to print a token for. Empty
	// means the one the CLI picks: PW_CONTEXT, then the current context.
	Context string
	// Timeout bounds one run of the CLI. Zero means DefaultCLITimeout.
	Timeout time.Duration

	mu    sync.Mutex
	token string
	// Zero when the CLI did not say, so the token is used for the life of the client.
	expiresAt time.Time
	// A token the CLI could not renew is used until it expires rather than asking on every request.
	settled bool
}

// Apply sets the Bearer token, running the CLI first when it is due.
func (a *CLIAuth) Apply(req *http.Request) error {
	token, err := a.Token(req.Context())
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}

// Token returns a current access token for the sign-in.
func (a *CLIAuth) Token(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.usable() {
		return a.token, nil
	}
	token, expiresAt, err := a.run(ctx)
	if err != nil {
		if a.token != "" && time.Now().Before(a.expiresAt) {
			a.settled = true
			return a.token, nil
		}
		return "", fmt.Errorf("%w: %w", ErrSignInExpired, err)
	}
	a.token, a.expiresAt = token, expiresAt
	a.settled = !expiresAt.IsZero() && time.Until(expiresAt) <= cliRenewMargin
	return a.token, nil
}

func (a *CLIAuth) usable() bool {
	switch {
	case a.token == "":
		return false
	case a.expiresAt.IsZero():
		return true
	case a.settled:
		return time.Now().Before(a.expiresAt)
	}
	return time.Until(a.expiresAt) > cliRenewMargin
}

// cliTokenResponse is the RFC 6749 section 5.1 token response `pw auth token --print -o json` prints.
type cliTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   *int64 `json:"expires_in"`
}

func (a *CLIAuth) run(ctx context.Context) (string, time.Time, error) {
	command := a.Command
	if command == "" {
		command = DefaultCLICommand
	}
	timeout := a.Timeout
	if timeout <= 0 {
		timeout = DefaultCLITimeout
	}
	// A canceled request must not kill the CLI before it saves the rotated refresh token.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()

	args := []string{"auth", "token", "--print", "-o", "json"}
	if a.Context != "" {
		args = append(args, "--context", a.Context)
	}
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = cliEnv()
	// Bounds the wait for a child the CLI left holding its output open.
	cmd.WaitDelay = time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	describe := command + " " + strings.Join(args, " ")
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", time.Time{}, fmt.Errorf("%s did not finish within %s", describe, timeout)
		}
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			return "", time.Time{}, fmt.Errorf("%s: %w: %s", describe, err, detail)
		}
		return "", time.Time{}, fmt.Errorf("%s: %w", describe, err)
	}
	received := time.Now()
	var response cliTokenResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return "", time.Time{}, fmt.Errorf("%s printed no token response: %w", describe, err)
	}
	if response.AccessToken == "" || strings.ContainsAny(response.AccessToken, " \t\r\n") {
		return "", time.Time{}, fmt.Errorf("%s printed no single access_token", describe)
	}
	if !strings.EqualFold(response.TokenType, "Bearer") {
		return "", time.Time{}, fmt.Errorf("%s printed token_type %q, want Bearer", describe, response.TokenType)
	}
	var expiresAt time.Time
	if response.ExpiresIn != nil {
		expiresAt = received.Add(time.Duration(*response.ExpiresIn) * time.Second)
	}
	return response.AccessToken, expiresAt, nil
}

// cliEnv drops PW_API_KEY, which would make the CLI print it rather than the sign-in's token.
func cliEnv() []string {
	environ := os.Environ()
	env := make([]string, 0, len(environ))
	for _, kv := range environ {
		if !strings.HasPrefix(kv, "PW_API_KEY=") {
			env = append(env, kv)
		}
	}
	return env
}
