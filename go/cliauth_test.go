package parallelworks

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type fakeCLI struct {
	dir     string
	log     string
	credDir string
}

// installFakeCLI puts a pw on PATH that logs its arguments and PW_API_KEY,
// then runs body, and points the SDK at a credentials file in a temp dir.
func installFakeCLI(t *testing.T, body string) *fakeCLI {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake pw is a shell script")
	}
	f := &fakeCLI{dir: t.TempDir(), credDir: t.TempDir()}
	f.log = filepath.Join(f.dir, "calls")
	f.script(t, body)
	t.Setenv("PATH", f.dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PW_CREDENTIALS_DIR", f.credDir)
	t.Setenv("PW_API_KEY", "")
	t.Setenv("PW_CONTEXT", "")
	t.Setenv("PW_PLATFORM_HOST", "")
	return f
}

func (f *fakeCLI) script(t *testing.T, body string) {
	t.Helper()
	script := "#!/bin/sh\necho \"$* key=${PW_API_KEY:-unset}\" >> " + f.log + "\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(f.dir, "pw"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func (f *fakeCLI) credentialsPath() string {
	return filepath.Join(f.credDir, credentialsFileName)
}

func (f *fakeCLI) calls(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(f.log)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func writeSignIn(t *testing.T, path, token string, expiresAt time.Time) {
	t.Helper()
	oauth, err := json.Marshal(map[string]any{"clientId": "pw-cli", "expiresAt": expiresAt, "keychainAccount": "work"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &CredentialConfig{
		CurrentIdentity: "work",
		Identities: map[string]Identity{
			"work": {Token: token, Server: "work.example.com", Name: "work", OAuth: oauth},
			"api":  {ApiKey: "pwt_aG9zdA==.key", Server: "api.example.com", Name: "api"},
		},
	}
	if err := cfg.SaveTo(path); err != nil {
		t.Fatal(err)
	}
}

func authorization(t *testing.T, auth AuthProvider) (string, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.Apply(req); err != nil {
		return "", err
	}
	return req.Header.Get("Authorization"), nil
}

func signedInClient(t *testing.T, opts ...any) *Client {
	t.Helper()
	client, err := NewClientFromCredentialConfig(opts...)
	if err != nil {
		t.Fatalf("NewClientFromCredentialConfig: %v", err)
	}
	if _, ok := client.auth.(*CLIAuth); !ok {
		t.Fatalf("auth = %T, want *CLIAuth", client.auth)
	}
	return client
}

// printScript prints a token response for token; expiresIn < 0 leaves expires_in out.
func printScript(token string, expiresIn int) string {
	if expiresIn < 0 {
		return `echo '{"access_token":"` + token + `","token_type":"Bearer"}'`
	}
	return fmt.Sprintf(`echo '{"access_token":"%s","token_type":"Bearer","expires_in":%d}'`, token, expiresIn)
}

func authorizeTimes(t *testing.T, auth AuthProvider, n int, want string) {
	t.Helper()
	for range n {
		if got, err := authorization(t, auth); err != nil || got != want {
			t.Fatalf("Authorization = %q, %v; want %q", got, err, want)
		}
	}
}

func TestCLIAuth_CachesTheTokenUntilItNearsExpiry(t *testing.T) {
	f := installFakeCLI(t, printScript("pwoa_new", 3600))
	writeSignIn(t, f.credentialsPath(), "pwoa_stored", time.Now().Add(time.Hour))

	client := signedInClient(t)
	if client.PlatformHost() != "work.example.com" {
		t.Errorf("host = %q", client.PlatformHost())
	}
	// Set after the client chose the sign-in, so only the CLI could see it.
	t.Setenv("PW_API_KEY", "pwt_leaked")
	authorizeTimes(t, client.auth, 3, "Bearer pwoa_new")
	calls := f.calls(t)
	if len(calls) != 1 {
		t.Fatalf("CLI ran %d times, want once: %v", len(calls), calls)
	}
	if calls[0] != "auth token --print -o json --context work key=unset" {
		t.Errorf("CLI call = %q", calls[0])
	}
}

func TestCLIAuth_RenewsBeforeTheTokenExpires(t *testing.T) {
	f := installFakeCLI(t, printScript("pwoa_new", 3600))
	writeSignIn(t, f.credentialsPath(), "pwoa_stored", time.Now().Add(time.Hour))

	auth := signedInClient(t).auth.(*CLIAuth)
	authorizeTimes(t, auth, 1, "Bearer pwoa_new")

	auth.expiresAt = time.Now().Add(30 * time.Second)
	f.script(t, printScript("pwoa_newer", 3600))
	authorizeTimes(t, auth, 2, "Bearer pwoa_newer")
	if calls := f.calls(t); len(calls) != 2 {
		t.Errorf("CLI ran %d times, want twice: %v", len(calls), calls)
	}
}

func TestCLIAuth_UsesATokenWithNoExpiryForTheLifeOfTheClient(t *testing.T) {
	f := installFakeCLI(t, printScript("pwoa_new", -1))
	writeSignIn(t, f.credentialsPath(), "pwoa_stored", time.Now().Add(time.Hour))

	authorizeTimes(t, signedInClient(t).auth, 3, "Bearer pwoa_new")
	if calls := f.calls(t); len(calls) != 1 {
		t.Errorf("CLI ran %d times, want once: %v", len(calls), calls)
	}
}

func TestCLIAuth_KeepsATokenTheCLICouldNotRenew(t *testing.T) {
	f := installFakeCLI(t, printScript("pwoa_stored", 30))
	writeSignIn(t, f.credentialsPath(), "pwoa_stored", time.Now().Add(30*time.Second))

	authorizeTimes(t, signedInClient(t).auth, 3, "Bearer pwoa_stored")
	if calls := f.calls(t); len(calls) != 1 {
		t.Errorf("CLI ran %d times, want once until the token expires: %v", len(calls), calls)
	}
}

func TestCLIAuth_PrintsForTheContextTheSDKSelected(t *testing.T) {
	f := installFakeCLI(t, printScript("pwoa_printed", 3600))
	writeSignIn(t, f.credentialsPath(), "pwoa_old", time.Now().Add(time.Hour))
	cfg, err := LoadCredentialConfigFrom(f.credentialsPath())
	if err != nil {
		t.Fatal(err)
	}
	cfg.Identities["other"] = Identity{Token: "pwoa_x", Server: "other.example.com", Name: "other", OAuth: cfg.Identities["work"].OAuth}
	if err := cfg.SaveTo(f.credentialsPath()); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PW_CONTEXT", "other")

	authorizeTimes(t, signedInClient(t, WithContext("work")).auth, 1, "Bearer pwoa_printed")
	if calls := f.calls(t); len(calls) != 1 || !strings.Contains(calls[0], "--context work") {
		t.Errorf("CLI calls = %v", calls)
	}
}

func TestCLIAuth_UsesTheLastTokenUntilItExpiresWhenTheCLIFails(t *testing.T) {
	f := installFakeCLI(t, printScript("pwoa_new", 3600))
	writeSignIn(t, f.credentialsPath(), "pwoa_stored", time.Now().Add(time.Hour))

	auth := signedInClient(t).auth.(*CLIAuth)
	authorizeTimes(t, auth, 1, "Bearer pwoa_new")

	auth.Command = filepath.Join(t.TempDir(), "missing-pw")
	auth.expiresAt = time.Now().Add(30 * time.Second)
	authorizeTimes(t, auth, 1, "Bearer pwoa_new")

	auth.expiresAt = time.Now().Add(-time.Second)
	if _, err := authorization(t, auth); !errors.Is(err, ErrSignInExpired) {
		t.Fatalf("err = %v, want ErrSignInExpired", err)
	}
}

func TestCLIAuth_FailsWithoutTheCLI(t *testing.T) {
	f := installFakeCLI(t, "")
	writeSignIn(t, f.credentialsPath(), "pwoa_stored", time.Now().Add(time.Hour))

	_, err := authorization(t, signedInClient(t, WithCLICommand(filepath.Join(t.TempDir(), "missing-pw"))).auth)
	if !errors.Is(err, ErrSignInExpired) {
		t.Fatalf("err = %v, want ErrSignInExpired", err)
	}
}

func TestCLIAuth_SurfacesTheCLIsError(t *testing.T) {
	f := installFakeCLI(t, "echo 'sign-in revoked' >&2\nexit 1")
	writeSignIn(t, f.credentialsPath(), "pwoa_stored", time.Now().Add(time.Hour))

	_, err := authorization(t, signedInClient(t).auth)
	if !errors.Is(err, ErrSignInExpired) || !strings.Contains(err.Error(), "sign-in revoked") {
		t.Fatalf("err = %v, want ErrSignInExpired with the CLI's stderr", err)
	}
}

func TestCLIAuth_RejectsOutputThatIsNotABearerTokenResponse(t *testing.T) {
	for name, body := range map[string]string{
		"bare token":   "echo pwoa_bare",
		"no token":     `echo '{"token_type":"Bearer"}'`,
		"not bearer":   `echo '{"access_token":"pwoa_x","token_type":"mac"}'`,
		"spaced token": `echo '{"access_token":"pwoa x","token_type":"Bearer"}'`,
	} {
		t.Run(name, func(t *testing.T) {
			f := installFakeCLI(t, body)
			writeSignIn(t, f.credentialsPath(), "pwoa_stored", time.Now().Add(time.Hour))
			if _, err := authorization(t, signedInClient(t).auth); !errors.Is(err, ErrSignInExpired) {
				t.Fatalf("err = %v, want ErrSignInExpired", err)
			}
		})
	}
}

func TestCLIAuth_Timeout(t *testing.T) {
	f := installFakeCLI(t, "exec sleep 5")
	writeSignIn(t, f.credentialsPath(), "pwoa_stored", time.Now().Add(time.Hour))

	_, err := authorization(t, signedInClient(t, WithCLITimeout(100*time.Millisecond)).auth)
	if err == nil || !strings.Contains(err.Error(), "did not finish within 100ms") {
		t.Fatalf("err = %v", err)
	}
}

func TestNewClientFromCredentialConfig_APIKeyAndExplicitTokensSkipTheCLI(t *testing.T) {
	f := installFakeCLI(t, "echo pwoa_unexpected")
	writeSignIn(t, f.credentialsPath(), "pwoa_stored", time.Now().Add(time.Hour))

	client, err := NewClientFromCredentialConfig(WithContext("api"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := client.auth.(*BasicAuth); !ok {
		t.Errorf("auth = %T, want *BasicAuth", client.auth)
	}

	t.Setenv("PW_API_KEY", "pwoa_explicit")
	client, err = NewClientFromCredentialConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := authorization(t, client.auth); err != nil || got != "Bearer pwoa_explicit" {
		t.Fatalf("Authorization = %q, %v", got, err)
	}
	if client.PlatformHost() != "work.example.com" {
		t.Errorf("host = %q, want the selected context's server", client.PlatformHost())
	}
	if calls := f.calls(t); len(calls) != 0 {
		t.Errorf("ran the CLI: %v", calls)
	}
}

func TestNewClientFromCredential_AccessTokenHost(t *testing.T) {
	f := installFakeCLI(t, "")
	writeSignIn(t, f.credentialsPath(), "pwoa_stored", time.Now().Add(time.Hour))

	client, err := NewClientFromCredential("pwoa_abc")
	if err != nil || client.PlatformHost() != "work.example.com" {
		t.Fatalf("host = %v, %v; want the selected context's server", client, err)
	}

	t.Setenv("PW_PLATFORM_HOST", "env.example.com")
	client, err = NewClientFromCredential("pwoa_abc")
	if err != nil || client.PlatformHost() != "env.example.com" {
		t.Fatalf("got %v, %v; want PW_PLATFORM_HOST", client, err)
	}

	t.Run("PW_PLATFORM_HOST needs no credentials file", func(t *testing.T) {
		t.Setenv("PW_CREDENTIALS_DIR", "")
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", "")
		client, err := NewClientFromCredential("pwoa_abc")
		if err != nil || client.PlatformHost() != "env.example.com" {
			t.Fatalf("got %v, %v; want PW_PLATFORM_HOST", client, err)
		}
	})

	t.Setenv("PW_PLATFORM_HOST", "")
	t.Setenv("PW_CREDENTIALS_DIR", t.TempDir())
	if _, err := NewClientFromCredential("pwoa_abc"); !errors.Is(err, ErrNoPlatformHost) {
		t.Fatalf("err = %v, want ErrNoPlatformHost", err)
	}
}
