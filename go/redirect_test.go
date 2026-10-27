package parallelworks

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func redirectRequest(t *testing.T, method, from, to string) (*http.Request, []*http.Request) {
	t.Helper()
	first, err := http.NewRequestWithContext(t.Context(), method, from, nil)
	if err != nil {
		t.Fatal(err)
	}
	next, err := http.NewRequestWithContext(t.Context(), method, to, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, req := range []*http.Request{first, next} {
		req.Header.Set("Authorization", "Bearer pwoa_token")
		req.Header.Set("X-Api-Key", "pwt_key")
		req.Header.Set("Cookie", "session=secret")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "pw")
	}
	next.Header.Set("Referer", from+"?token=secret")
	return next, []*http.Request{first}
}

func TestARedirectToPlainHTTPIsAlwaysRefused(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost} {
		for _, to := range []string{"http://platform.example/x", "http://elsewhere.example/x"} {
			req, via := redirectRequest(t, method, "https://platform.example/api/x", to)
			if err := CheckRedirect(req, via); !errors.Is(err, ErrInsecureRedirect) {
				t.Errorf("%s to %s: got %v, want ErrInsecureRedirect", method, to, err)
			}
		}
	}
}

func TestARedirectOnTheOriginKeepsItsHeaders(t *testing.T) {
	for from, to := range map[string]string{
		"https://platform.example/x":      "https://PLATFORM.example:443/y",
		"http://platform.example/x":       "https://platform.example/y",
		"https://platform.example:8443/x": "https://platform.example:8443/y",
	} {
		req, via := redirectRequest(t, http.MethodPost, from, to)
		if err := CheckRedirect(req, via); err != nil {
			t.Fatalf("%s to %s: %v", from, to, err)
		}
		if req.Header.Get("Authorization") == "" || req.Header.Get("X-Api-Key") == "" {
			t.Errorf("%s to %s dropped the credential", from, to)
		}
	}
}

func TestAGetFollowsARedirectElsewhereWithoutItsCredentials(t *testing.T) {
	for _, to := range []string{"https://gravatar.example/avatar", "https://platform.example:8443/x", "https://sub.platform.example/x", "https://user@platform.example/x"} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			req, via := redirectRequest(t, method, "https://platform.example/api/avatar", to)
			if err := CheckRedirect(req, via); err != nil {
				t.Fatalf("%s to %s: %v", method, to, err)
			}
			for _, name := range []string{"Authorization", "X-Api-Key", "Cookie", "Referer"} {
				if req.Header.Get(name) != "" {
					t.Errorf("%s to %s carried %s", method, to, name)
				}
			}
			if req.Header.Get("Accept") == "" || req.Header.Get("User-Agent") == "" {
				t.Errorf("%s to %s dropped Accept or User-Agent", method, to)
			}
		}
	}
}

func TestARedirectElsewhereDropsAHeaderSetWithoutCanonicalCase(t *testing.T) {
	req, via := redirectRequest(t, http.MethodGet, "https://platform.example/api/avatar", "https://gravatar.example/avatar")
	nonCanonical := "x-api-key"
	req.Header[nonCanonical] = []string{"pwt_key"}
	if err := CheckRedirect(req, via); err != nil {
		t.Fatal(err)
	}
	if _, carried := req.Header[nonCanonical]; carried {
		t.Fatal("the redirect carried a credential header set without canonical case")
	}
}

func TestARequestThatIsNotAGetStopsAtARedirectElsewhere(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		req, via := redirectRequest(t, method, "https://platform.example/api/x", "https://elsewhere.example/x")
		if err := CheckRedirect(req, via); !errors.Is(err, http.ErrUseLastResponse) {
			t.Errorf("%s: got %v, want the redirect handed back", method, err)
		}
	}
}

func TestRedirectsStopAfterTen(t *testing.T) {
	req, via := redirectRequest(t, http.MethodGet, "https://platform.example/x", "https://platform.example/y")
	for len(via) < 10 {
		via = append(via, via[0])
	}
	if err := CheckRedirect(req, via); !errors.Is(err, ErrTooManyRedirects) {
		t.Fatalf("got %v, want the redirect limit", err)
	}
}

func apiKeyFor(server *httptest.Server) string {
	host := strings.TrimPrefix(server.URL, "https://")
	return APIKeyPrefix + base64.RawURLEncoding.EncodeToString([]byte(host)) + ".secret"
}

func TestAClientFollowsAnAvatarRedirectWithoutItsCredential(t *testing.T) {
	var carried atomic.Value
	carried.Store("unset")
	elsewhere := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		carried.Store(r.Header.Get("Authorization"))
		_, _ = w.Write([]byte("png"))
	}))
	defer elsewhere.Close()
	platform := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Another port on the same host, to which Go itself would carry Authorization.
		http.Redirect(w, r, elsewhere.URL+"/avatar/abc", http.StatusTemporaryRedirect)
	}))
	defer platform.Close()

	client, err := NewClientFromCredential(apiKeyFor(platform), WithHTTPClient(platform.Client()))
	if err != nil {
		t.Fatal(err)
	}
	body, err := client.GetRaw(t.Context(), "/api/avatar/ada")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "png" {
		t.Fatalf("got %q", body)
	}
	if got := carried.Load(); got != "" {
		t.Fatalf("the avatar host received Authorization %q", got)
	}
}

func TestAClientDoesNotRetryARedirectToPlainHTTP(t *testing.T) {
	var asked atomic.Int32
	platform := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		http.Redirect(w, r, "http://"+r.Host+r.URL.Path, http.StatusFound)
	}))
	defer platform.Close()

	client, err := NewClientFromCredential(apiKeyFor(platform), WithHTTPClient(platform.Client()),
		WithRetry(RetryConfig{MaxRetries: 3, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond, Multiplier: 2}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.GetWhoami(t.Context())
	if !errors.Is(err, ErrInsecureRedirect) {
		t.Fatalf("got %v, want ErrInsecureRedirect", err)
	}
	if asked.Load() != 1 {
		t.Fatalf("the platform was asked %d times", asked.Load())
	}
}

func TestAClientWithItsOwnRedirectPolicyKeepsIt(t *testing.T) {
	platform := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/moved" {
			// On the origin, which CheckRedirect would follow.
			http.Redirect(w, r, "/moved", http.StatusTemporaryRedirect)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`"ada"`))
	}))
	defer platform.Close()
	own := &http.Client{Transport: platform.Client().Transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	client, err := NewClientFromCredential(apiKeyFor(platform), WithHTTPClient(own))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.GetWhoami(t.Context())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("got %v, want the 307 the client's own policy hands back", err)
	}
}

func TestAPostSeeOtherElsewhereFollowsAsAGetWithoutItsCredentials(t *testing.T) {
	req, via := redirectRequest(t, http.MethodGet, "https://platform.example/api/x", "https://elsewhere.example/x")
	via[0].Method = http.MethodPost
	req.Response = &http.Response{StatusCode: http.StatusSeeOther}
	if err := CheckRedirect(req, via); err != nil {
		t.Fatalf("got %v, want the 303's GET followed", err)
	}
	if req.Header.Get("Authorization") != "" || req.Header.Get("X-Api-Key") != "" {
		t.Fatal("the GET carried the POST's credentials")
	}
}

func TestAWriteRedirectedElsewhereBecomesAGetOnlyForASeeOther(t *testing.T) {
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html>sign in</html>"))
	}))
	defer elsewhere.Close()
	for _, status := range []int{http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther} {
		platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, elsewhere.URL+"/login", status)
		}))
		for _, method := range []string{http.MethodPost, http.MethodDelete} {
			req, err := http.NewRequestWithContext(t.Context(), method, platform.URL+"/api/x", strings.NewReader("{}"))
			if err != nil {
				t.Fatal(err)
			}
			resp, err := (&http.Client{CheckRedirect: CheckRedirect}).Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			want := status
			if status == http.StatusSeeOther {
				want = http.StatusOK
			}
			if resp.StatusCode != want {
				t.Errorf("%s answered %d: got %d, want %d", method, status, resp.StatusCode, want)
			}
		}
		platform.Close()
	}
}

func TestAGetWithABodyStopsAtARedirectElsewhere(t *testing.T) {
	req, via := redirectRequest(t, http.MethodGet, "https://platform.example/api/x", "https://elsewhere.example/x")
	req.Body = io.NopCloser(strings.NewReader("secret"))
	if err := CheckRedirect(req, via); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("got %v, want the redirect handed back", err)
	}
}

func TestARangedGetKeepsItsRangeElsewhere(t *testing.T) {
	req, via := redirectRequest(t, http.MethodGet, "https://platform.example/api/x", "https://cdn.example/x")
	want := map[string]string{"Range": "bytes=0-1", "If-Range": `"v1"`, "If-None-Match": `"v0"`, "If-Modified-Since": "Sat, 24 Oct 2026 00:00:00 GMT", "Accept-Encoding": "gzip"}
	for name, value := range want {
		req.Header.Set(name, value)
	}
	if err := CheckRedirect(req, via); err != nil {
		t.Fatal(err)
	}
	for name, value := range want {
		if got := req.Header.Get(name); got != value {
			t.Errorf("%s = %q, want %q", name, got, value)
		}
	}
	if req.Header.Get("Authorization") != "" {
		t.Error("the redirect carried Authorization")
	}
}

func TestAnUpgradeToHTTPSStaysOnTheOriginOnlyFromTheDefaultPorts(t *testing.T) {
	for from, stays := range map[string]bool{
		"http://platform.example/x":      true,
		"http://platform.example:8080/x": false,
	} {
		for _, to := range []string{"https://platform.example/y", "https://platform.example:8443/y"} {
			req, via := redirectRequest(t, http.MethodGet, from, to)
			if err := CheckRedirect(req, via); err != nil {
				t.Fatal(err)
			}
			want := stays && to == "https://platform.example/y"
			if kept := req.Header.Get("Authorization") != ""; kept != want {
				t.Errorf("%s to %s kept Authorization = %t, want %t", from, to, kept, want)
			}
		}
	}
}

func TestARedirectWithTheRequestsOwnUserinfoStaysOnTheOrigin(t *testing.T) {
	req, via := redirectRequest(t, http.MethodPost, "https://user:pass@platform.example/x", "https://user:pass@platform.example/y")
	if err := CheckRedirect(req, via); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("Authorization") == "" {
		t.Fatal("a redirect on the origin dropped Authorization")
	}
}

func TestAPlainClientFollowsARedirectElsewhereWithoutItsCredential(t *testing.T) {
	var followed atomic.Int32
	var carried atomic.Bool
	elsewhere := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		followed.Add(1)
		if r.Header.Get("Authorization") != "" {
			carried.Store(true)
		}
	}))
	defer elsewhere.Close()
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/avatar", http.StatusTemporaryRedirect)
	}))
	defer platform.Close()
	client := NewClient(platform.URL, WithAuth(&BearerAuth{Token: "pwoa_token"}), WithHTTPClient(&http.Client{}))

	if _, err := client.GetWhoami(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetRaw(t.Context(), "/api/avatar/ada"); err != nil {
		t.Fatal(err)
	}
	resp, err := client.Stream(t.Context(), http.MethodGet, "/api/avatar/ada", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if followed.Load() != 3 || carried.Load() {
		t.Fatalf("the other origin was asked %d times, and received Authorization: %t", followed.Load(), carried.Load())
	}
}
