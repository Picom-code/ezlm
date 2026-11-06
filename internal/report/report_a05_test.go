package report

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"
)

// roundTripFunc lets a test answer Send's request without a network.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// withDefaultClient swaps the client Send uses and puts it back afterwards.
func withDefaultClient(t *testing.T, rt roundTripFunc) {
	t.Helper()
	old := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: rt}
	t.Cleanup(func() { http.DefaultClient = old })
}

// withoutToken clears both ways Send finds a token, so it takes the
// browser fallback without touching the network.
func withoutToken(t *testing.T) {
	t.Helper()
	old := Token
	Token = ""
	t.Cleanup(func() { Token = old })
	t.Setenv("EZLM_REPORT_TOKEN", "")
}

func issueResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestSendWithoutTokenReturnsPrefilledURL(t *testing.T) {
	withoutToken(t)
	withDefaultClient(t, func(*http.Request) (*http.Response, error) {
		t.Error("Send called GitHub without a token")
		return nil, errors.New("unexpected request")
	})

	const title, body = "Beta report: it stopped", "### What happened\n\nit stopped"
	got, err := Send(context.Background(), title, body)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got.Sent || got.Number != 0 {
		t.Errorf("Send without a token reported sent=%v number=%d, want false 0", got.Sent, got.Number)
	}
	if want := NewIssueURL(title, body); got.URL != want {
		t.Errorf("fallback URL = %q, want %q", got.URL, want)
	}
}

func TestSendFilesIssue(t *testing.T) {
	old := Token
	Token = "build-token"
	t.Cleanup(func() { Token = old })
	t.Setenv("EZLM_REPORT_TOKEN", "REDACTED-KEY")

	var (
		req  *http.Request
		sent map[string]any
	)
	withDefaultClient(t, func(r *http.Request) (*http.Response, error) {
		req = r
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(raw, &sent); err != nil {
			t.Fatalf("request body is not JSON: %v (%s)", err, raw)
		}
		return issueResponse(http.StatusCreated, `{"html_url":"https://github.com/picom-code/ezlm/issues/42","number":42}`), nil
	})

	got, err := Send(context.Background(), "Beta report: it stopped", "### What happened\n\nit stopped")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !got.Sent || got.Number != 42 || got.URL != "https://github.com/picom-code/ezlm/issues/42" {
		t.Errorf("Send = %+v, want sent with number 42 and the issue URL", got)
	}
	if req.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", req.Method)
	}
	if want := "https://api.github.com/repos/picom-code/ezlm/issues"; req.URL.String() != want {
		t.Errorf("URL = %s, want %s", req.URL, want)
	}
	for header, want := range map[string]string{
		"Authorization": "Bearer REDACTED-KEY",
		"Accept":        "application/vnd.github+json",
		"Content-Type":  "application/json",
	} {
		if got := req.Header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if sent["title"] != "Beta report: it stopped" || sent["body"] != "### What happened\n\nit stopped" {
		t.Errorf("payload = %v, want the title and body passed to Send", sent)
	}
	labels, ok := sent["labels"].([]any)
	if !ok || len(labels) != 1 || labels[0] != "beta-report" {
		t.Errorf("labels = %v, want [beta-report]", sent["labels"])
	}
}

func TestSendTokenPrecedence(t *testing.T) {
	cases := []struct {
		name  string
		build string
		env   string
		want  string
	}{
		{"env wins", "build-token", "REDACTED-ENV", "REDACTED-ENV"},
		{"build token used when env is empty", "build-token", "", "build-token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			old := Token
			Token = tc.build
			t.Cleanup(func() { Token = old })
			t.Setenv("EZLM_REPORT_TOKEN", tc.env)

			var auth string
			withDefaultClient(t, func(r *http.Request) (*http.Response, error) {
				auth = r.Header.Get("Authorization")
				return issueResponse(http.StatusCreated, `{"html_url":"https://github.com/picom-code/ezlm/issues/1","number":1}`), nil
			})
			if _, err := Send(context.Background(), "t", "b"); err != nil {
				t.Fatalf("Send: %v", err)
			}
			if want := "Bearer " + tc.want; auth != want {
				t.Errorf("Authorization = %q, want %q", auth, want)
			}
		})
	}
}

func TestSendFallsBackWhenGitHubIsUnavailable(t *testing.T) {
	const title, body = "Beta report: it stopped", "### What happened\n\nit stopped"
	cases := []struct {
		name string
		rt   roundTripFunc
	}{
		{"transport error", func(*http.Request) (*http.Response, error) {
			return nil, errors.New("offline")
		}},
		{"server error", func(*http.Request) (*http.Response, error) {
			return issueResponse(http.StatusInternalServerError, `{"message":"boom"}`), nil
		}},
		{"unauthorized", func(*http.Request) (*http.Response, error) {
			return issueResponse(http.StatusUnauthorized, `{"message":"bad credentials"}`), nil
		}},
		{"created with an unreadable body", func(*http.Request) (*http.Response, error) {
			return issueResponse(http.StatusCreated, "not json"), nil
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			old := Token
			Token = ""
			t.Cleanup(func() { Token = old })
			t.Setenv("EZLM_REPORT_TOKEN", "REDACTED-KEY")
			withDefaultClient(t, tc.rt)

			got, err := Send(context.Background(), title, body)
			if err != nil {
				t.Fatalf("Send: %v", err)
			}
			if got.Sent || got.Number != 0 {
				t.Errorf("Send = %+v, want the browser fallback", got)
			}
			if want := NewIssueURL(title, body); got.URL != want {
				t.Errorf("fallback URL = %q, want %q", got.URL, want)
			}
			if strings.Contains(got.URL, "REDACTED-KEY") {
				t.Error("fallback URL leaks the token")
			}
		})
	}
}

// The pre-filled link must round-trip every character, including the ones
// that mean something in a query string.
func TestNewIssueURLRoundTrip(t *testing.T) {
	cases := []struct {
		name        string
		title, body string
	}{
		{"empty", "", ""},
		{"plain", "Beta report: it stopped", "### What happened\n\nit stopped"},
		{"reserved characters", "a&b=c?d#e", `100% + 50% = <ok> & "quoted"`},
		{"unicode", "Über ✗", "模型 stopped — again 🚀"},
		{"redacted key", "Beta report: it stopped", "key REDACTED-KEY, ezlm-AbCd… and sk-…"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			link := NewIssueURL(tc.title, tc.body)
			u, err := url.Parse(link)
			if err != nil {
				t.Fatalf("url.Parse(%q): %v", link, err)
			}
			if u.Scheme != "https" || u.Host != "github.com" || u.Path != "/picom-code/ezlm/issues/new" {
				t.Errorf("link = %q, want a github.com/picom-code/ezlm new-issue page", link)
			}
			q := u.Query()
			if got := q.Get("title"); got != tc.title {
				t.Errorf("title = %q, want %q", got, tc.title)
			}
			if got := q.Get("body"); got != tc.body {
				t.Errorf("body = %q, want %q", got, tc.body)
			}
			if got := q.Get("labels"); got != "beta-report" {
				t.Errorf("labels = %q, want beta-report", got)
			}
		})
	}
}

// Long reports keep their start, stay under the encoded-body cap and never
// end in half a character.
func TestNewIssueURLTrimsLongBodies(t *testing.T) {
	const prefix = "## What happened\n\nThe model stopped — again ✗\n\n"
	cases := []struct {
		name string
		body string
	}{
		{"ascii logs", prefix + strings.Repeat("line of log text\n", 4000)},
		{"symbol heavy", prefix + strings.Repeat("{\"k\":\"ü<>&%\"}\n", 3000)},
		{"three-byte runes", prefix + strings.Repeat("模型停止", 3000)},
		{"emoji", prefix + strings.Repeat("🚀", 3000)},
		{"rune at the cut", prefix + strings.Repeat("a", 9000) + "模" + strings.Repeat("a", 1000)},
		{"one megabyte", prefix + strings.Repeat("0123456789abcdef\n", 65536)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			link := NewIssueURL("Beta report: it stopped", tc.body)
			u, err := url.Parse(link)
			if err != nil {
				t.Fatalf("url.Parse(%q): %v", link, err)
			}
			got := u.Query().Get("body")
			if enc := len(url.QueryEscape(got)); enc > maxBodyURL {
				t.Errorf("encoded body is %d bytes, want <= %d", enc, maxBodyURL)
			}
			if !utf8.ValidString(got) {
				t.Error("trimmed body isn't valid UTF-8")
			}
			if !strings.HasPrefix(got, prefix) {
				t.Errorf("trimmed body lost the start: %q", got[:40])
			}
			const marker = "\n\n…(trimmed)"
			if !strings.HasSuffix(got, marker) {
				t.Errorf("trimmed body lost the marker: %q", got[len(got)-20:])
			}
			if kept := strings.TrimSuffix(got, marker); !strings.HasPrefix(tc.body, kept) {
				t.Error("trimmed body is not a prefix of the original")
			}
		})
	}
}

func TestNewIssueURLKeepsBodiesUnderTheCap(t *testing.T) {
	for _, body := range []string{
		"short",
		strings.Repeat("a", maxBodyURL),    // exactly at the cap
		strings.Repeat("a", maxBodyURL-10), // just under it
		"## What happened\n\n" + strings.Repeat("ok\n", 1000),
	} {
		got, err := url.Parse(NewIssueURL("t", body))
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}
		if got := got.Query().Get("body"); got != body {
			t.Errorf("body of %d bytes changed: got %d bytes", len(body), len(got))
		}
	}
}
