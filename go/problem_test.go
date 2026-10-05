package parallelworks

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAsProblemReadsProblemDetails(t *testing.T) {
	body := `{"type":"/problems/activate/workflow_not_found","title":"Workflow not found","status":404,"detail":"ワークフローが見つかりません","code":"workflow_not_found","params":{"name":"x"}}`
	pe, ok := AsProblem(fmt.Errorf("getting workflow: %w", &APIError{StatusCode: 404, Status: "404 Not Found", Body: []byte(body)}))
	if !ok {
		t.Fatal("AsProblem reported no problem")
	}
	if got := pe.Code(); got != "workflow_not_found" {
		t.Errorf("Code() = %q", got)
	}
	if got := pe.Detail(); got != "ワークフローが見つかりません" {
		t.Errorf("Detail() = %q", got)
	}
	if got := pe.Problem.Params["name"]; got != "x" {
		t.Errorf("params name = %v", got)
	}
	if got := pe.TypeURL("https://activate.parallel.works"); got != "https://activate.parallel.works/problems/activate/workflow_not_found" {
		t.Errorf("TypeURL() = %q", got)
	}
	if !errors.Is(pe, ErrNotFound) {
		t.Error("problem does not match ErrNotFound")
	}
}

func TestAsProblemDerivesCodeForAboutBlank(t *testing.T) {
	pe, _ := AsProblem(&APIError{StatusCode: 403, Body: []byte(`{"type":"about:blank","title":"Forbidden","status":403}`)})
	if got := pe.Code(); got != "forbidden" {
		t.Errorf("Code() = %q", got)
	}
	if got := pe.Detail(); got != "Forbidden" {
		t.Errorf("Detail() = %q", got)
	}
	if got := pe.TypeURL("https://activate.parallel.works"); got != "" {
		t.Errorf("TypeURL() = %q, want none for about:blank", got)
	}
}

func TestAsProblemReadsFieldErrors(t *testing.T) {
	body := `{"type":"/problems/validation","status":422,"detail":"validation failed","code":"validation","errors":[` +
		`{"type":"/problems/too_long","code":"too_long","detail":"must be at most 3 characters","pointer":"#/items/0/name","params":{"max":3}},` +
		`{"type":"/problems/required","code":"required","parameter":"org","in":"query"},` +
		`{"type":"/problems/invalid","code":"invalid","detail":"bad body","pointer":"#"}]}`
	pe, _ := AsProblem(&APIError{StatusCode: 422, Status: "422 Unprocessable Entity", Body: []byte(body)})
	if len(pe.Problem.Errors) != 3 {
		t.Fatalf("got %d field errors", len(pe.Problem.Errors))
	}
	want := []string{"items[0].name: must be at most 3 characters", "org: required", "bad body"}
	for i, fe := range pe.Problem.Errors {
		if got := fe.String(); got != want[i] {
			t.Errorf("field error %d = %q, want %q", i, got, want[i])
		}
	}
	if got, want := pe.Error(), "API error 422 Unprocessable Entity: validation failed; items[0].name: must be at most 3 characters; org: required; bad body"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestAsProblemReadsTheOlderEnvelope(t *testing.T) {
	body := `{"error":true,"message":"Workflow not found","code":"workflow_not_found","errors":["no such workflow"],"params":{"name":"x"}}`
	pe, ok := AsProblem(&APIError{StatusCode: 404, Body: []byte(body)})
	if !ok {
		t.Fatal("AsProblem reported no problem")
	}
	if got := pe.Code(); got != "workflow_not_found" {
		t.Errorf("Code() = %q", got)
	}
	if got := pe.Detail(); got != "Workflow not found" {
		t.Errorf("Detail() = %q", got)
	}
	if len(pe.Problem.Errors) != 1 || pe.Problem.Errors[0].String() != "no such workflow" {
		t.Errorf("errors = %+v", pe.Problem.Errors)
	}
	if got := pe.TypeURL("https://activate.parallel.works"); got != "" {
		t.Errorf("TypeURL() = %q, want none for an envelope", got)
	}
}

func TestAsProblemReadsAProxyPage(t *testing.T) {
	pe, ok := AsProblem(&APIError{StatusCode: 502, Body: []byte("<html><body>Bad Gateway</body></html>")})
	if !ok {
		t.Fatal("AsProblem reported no problem")
	}
	if pe.Problem.Detail != nil {
		t.Errorf("detail = %q, want none", *pe.Problem.Detail)
	}
	if got := pe.Code(); got != "internal" {
		t.Errorf("Code() = %q", got)
	}
	if got := pe.Detail(); got != "Bad Gateway" {
		t.Errorf("Detail() = %q", got)
	}
}

func TestAsProblemIgnoresErrorsWithoutAResponse(t *testing.T) {
	if _, ok := AsProblem(errors.New("dial tcp: connection refused")); ok {
		t.Error("AsProblem read a transport error as a problem")
	}
}

func TestWithProblemDetailsSendsHeaders(t *testing.T) {
	var accept, language string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept, language = r.Header.Get("Accept"), r.Header.Get("Accept-Language")
		w.Header().Set("Content-Type", ProblemMediaType)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"about:blank","status":404,"detail":"見つかりません"}`))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, WithProblemDetails("ja-JP")).ListWorkflows(t.Context())
	if accept != "application/json, "+ProblemMediaType {
		t.Errorf("Accept = %q", accept)
	}
	if language != "ja-JP" {
		t.Errorf("Accept-Language = %q", language)
	}
	pe, ok := AsProblem(err)
	if !ok || pe.Detail() != "見つかりません" || pe.Code() != "not_found" {
		t.Errorf("AsProblem(%v) = %+v, %v", err, pe, ok)
	}
}

func TestWithProblemDetailsOmitsAnEmptyLanguage(t *testing.T) {
	var header http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header = r.Header
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL, WithProblemDetails("")).ListWorkflows(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, ok := header["Accept-Language"]; ok {
		t.Errorf("Accept-Language sent: %q", header.Get("Accept-Language"))
	}
}

func TestAcceptLanguageFromEnv(t *testing.T) {
	cases := []struct {
		lcAll, lcMessages, lang, want string
	}{
		{lang: "ja_JP.UTF-8", want: "ja-JP"},
		{lang: "zh_CN", want: "zh-CN"},
		{lang: "es", want: "es"},
		{lang: "de_DE.UTF-8@euro", want: "de-DE"},
		{lang: "C", want: ""},
		{lang: "C.UTF-8", want: ""},
		{lang: "POSIX", want: ""},
		{want: ""},
		{lcMessages: "ko_KR.UTF-8", lang: "en_US.UTF-8", want: "ko-KR"},
		{lcAll: "fr_FR.UTF-8", lcMessages: "ko_KR.UTF-8", lang: "en_US.UTF-8", want: "fr-FR"},
		{lcAll: "C", lang: "ja_JP.UTF-8", want: ""},
	}
	for _, tc := range cases {
		t.Setenv("LC_ALL", tc.lcAll)
		t.Setenv("LC_MESSAGES", tc.lcMessages)
		t.Setenv("LANG", tc.lang)
		if got := AcceptLanguageFromEnv(); got != tc.want {
			t.Errorf("LC_ALL=%q LC_MESSAGES=%q LANG=%q: got %q, want %q", tc.lcAll, tc.lcMessages, tc.lang, got, tc.want)
		}
	}
}
