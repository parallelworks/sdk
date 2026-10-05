package parallelworks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// ProblemMediaType is the RFC 9457 media type for a problem details object.
const ProblemMediaType = "application/problem+json"

// BlankProblemType is the type of a problem that means nothing beyond its
// HTTP status.
const BlankProblemType = "about:blank"

// WithProblemDetails asks the API for errors as RFC 9457 problem details, with
// their detail in acceptLanguage (an Accept-Language value such as "ja-JP")
// when it is not empty. Read them with AsProblem.
func WithProblemDetails(acceptLanguage string) ClientOption {
	return WithMiddleware(func(req *http.Request, next RoundTripFunc) (*http.Response, error) {
		if accept := req.Header.Get("Accept"); !strings.Contains(accept, ProblemMediaType) {
			if accept == "" {
				accept = "application/json"
			}
			req.Header.Set("Accept", accept+", "+ProblemMediaType)
		}
		if acceptLanguage != "" && req.Header.Get("Accept-Language") == "" {
			req.Header.Set("Accept-Language", acceptLanguage)
		}
		return next(req)
	})
}

// AcceptLanguageFromEnv returns the language of the POSIX locale environment
// (LC_ALL, then LC_MESSAGES, then LANG) as a language tag: "ja_JP.UTF-8" is
// "ja-JP". It is empty when none is set or the locale is C or POSIX.
func AcceptLanguageFromEnv() string {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(name); v != "" {
			return localeLanguageTag(v)
		}
	}
	return ""
}

func localeLanguageTag(locale string) string {
	tag, _, _ := strings.Cut(locale, ".")
	tag, _, _ = strings.Cut(tag, "@")
	if tag == "" || tag == "C" || tag == "POSIX" {
		return ""
	}
	for _, r := range tag {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return ""
		}
	}
	return strings.ReplaceAll(tag, "_", "-")
}

// ProblemError is a failed request read as RFC 9457 problem details. A server
// that sends the older {error, message, code, errors} envelope, or a proxy that
// sends no JSON at all, is read into the same shape, with type about:blank.
type ProblemError struct {
	*APIError
	Problem Problem
}

func (e *ProblemError) Error() string {
	msg := e.Detail()
	for _, fe := range e.Problem.Errors {
		msg += "; " + fe.String()
	}
	return fmt.Sprintf("API error %s: %s", e.statusLabel(), msg)
}

func (e *ProblemError) Unwrap() error {
	return e.APIError
}

// Code is the problem's stable code, derived from the status for about:blank.
func (e *ProblemError) Code() string {
	if e.Problem.Code != nil && *e.Problem.Code != "" {
		return *e.Problem.Code
	}
	return codeForStatus(e.StatusCode)
}

// Detail is the server's explanation, in the language the request asked for
// when the server has it, falling back to the title and then the status text.
func (e *ProblemError) Detail() string {
	if e.Problem.Detail != nil && *e.Problem.Detail != "" {
		return *e.Problem.Detail
	}
	if e.Problem.Title != nil && *e.Problem.Title != "" {
		return *e.Problem.Title
	}
	return http.StatusText(e.StatusCode)
}

// TypeURL is the absolute URL documenting the problem's type, resolved against
// the API's base URL, or "" for about:blank.
func (e *ProblemError) TypeURL(baseURL string) string {
	if e.Problem.Type == "" || e.Problem.Type == BlankProblemType {
		return ""
	}
	ref, err := url.Parse(e.Problem.Type)
	if err != nil {
		return ""
	}
	if ref.IsAbs() {
		return ref.String()
	}
	base, err := url.Parse(baseURL)
	if err != nil || !base.IsAbs() {
		return ""
	}
	return base.ResolveReference(ref).String()
}

// Field is the invalid field as a path such as items[0].name, or the name of
// the invalid parameter. It is empty for an error that names neither.
func (fe FieldError) Field() string {
	if fe.Pointer != nil && *fe.Pointer != "" {
		return pointerFieldPath(*fe.Pointer)
	}
	if fe.Parameter != nil {
		return *fe.Parameter
	}
	return ""
}

// String renders the error as "field: detail", or the code when the server
// sent no detail.
func (fe FieldError) String() string {
	msg := fe.Code
	if fe.Detail != nil && *fe.Detail != "" {
		msg = *fe.Detail
	}
	if field := fe.Field(); field != "" {
		return field + ": " + msg
	}
	return msg
}

func pointerFieldPath(pointer string) string {
	raw := strings.TrimPrefix(pointer, "#")
	if raw == "" {
		return ""
	}
	var b strings.Builder
	for seg := range strings.SplitSeq(strings.TrimPrefix(raw, "/"), "/") {
		if s, err := url.PathUnescape(seg); err == nil {
			seg = s
		}
		seg = strings.ReplaceAll(strings.ReplaceAll(seg, "~1", "/"), "~0", "~")
		if _, err := strconv.Atoi(seg); err == nil && b.Len() > 0 {
			b.WriteString("[" + seg + "]")
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(seg)
	}
	return b.String()
}

// AsProblem reads any error a request returned as problem details. It reports
// false only when err did not come from an HTTP response.
func AsProblem(err error) (*ProblemError, bool) {
	var pe *ProblemError
	if errors.As(err, &pe) {
		return pe, true
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return nil, false
	}
	return &ProblemError{APIError: apiErr, Problem: parseProblemBody(apiErr.Body, apiErr.StatusCode)}, true
}

func parseProblemBody(body []byte, status int) Problem {
	status64 := int64(status)
	blank := Problem{Type: BlankProblemType, Status: &status64}

	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		return blank
	}
	if _, ok := fields["type"]; ok {
		var p Problem
		if json.Unmarshal(body, &p) != nil {
			return blank
		}
		if p.Type == "" {
			p.Type = BlankProblemType
		}
		if p.Status == nil {
			p.Status = &status64
		}
		return p
	}

	var envelope struct {
		Message string         `json:"message"`
		Code    string         `json:"code"`
		Errors  []string       `json:"errors"`
		Params  map[string]any `json:"params"`
	}
	if json.Unmarshal(body, &envelope) != nil || envelope.Message == "" {
		return blank
	}
	blank.Detail = &envelope.Message
	if envelope.Code != "" {
		blank.Code = &envelope.Code
	}
	blank.Params = envelope.Params
	for _, e := range envelope.Errors {
		blank.Errors = append(blank.Errors, FieldError{Detail: &e})
	}
	return blank
}

func codeForStatus(status int) string {
	switch {
	case status == http.StatusUnauthorized:
		return "unauthenticated"
	case status == http.StatusForbidden:
		return "forbidden"
	case status == http.StatusNotFound:
		return "not_found"
	case status == http.StatusConflict:
		return "conflict"
	case status == http.StatusTooManyRequests:
		return "rate_limited"
	case status == http.StatusServiceUnavailable:
		return "unavailable"
	case status >= 400 && status < 500:
		return "invalid_request"
	default:
		return "internal"
	}
}

// BaseURL is the URL every request path is resolved against.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// ProblemGuide is a problem type's documentation as data: what it means, why
// it happens and how to fix it, in the language the server chose.
type ProblemGuide struct {
	Type     string             `json:"type"`
	Code     string             `json:"code"`
	Status   int                `json:"status"`
	Title    string             `json:"title"`
	Message  string             `json:"message,omitempty"`
	Why      string             `json:"why,omitempty"`
	Fix      []string           `json:"fix,omitempty"`
	Links    []ProblemGuideLink `json:"links,omitempty"`
	Language string             `json:"language"`
}

// ProblemGuideLink is further reading on a problem type's page.
type ProblemGuideLink struct {
	Title string `json:"title"`
	Href  string `json:"href"`
}

// GetProblemGuide fetches the guidance for a problem type, such as a
// ProblemError's Problem.Type, from the server's /problems/ pages.
func (c *Client) GetProblemGuide(ctx context.Context, problemType string) (*ProblemGuide, error) {
	if !strings.HasPrefix(problemType, "/problems/") {
		return nil, fmt.Errorf("not a problem type on this server: %q", problemType)
	}
	var guide ProblemGuide
	if err := c.do(ctx, http.MethodGet, problemType, nil, "", &guide, "application/json", false); err != nil {
		return nil, err
	}
	return &guide, nil
}
