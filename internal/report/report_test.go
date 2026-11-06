package report

import (
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"
)

// A report with long, symbol-heavy logs still makes a link GitHub accepts,
// keeps what the person wrote, and never splits a character.
func TestNewIssueURLStaysShort(t *testing.T) {
	body := "## What happened\n\nThe model stopped — again ✗\n\n<details>\n" + strings.Repeat("{\"k\":\"ü<>&%\"}\n", 3000) + "</details>"
	link := NewIssueURL("Beta report: it stopped", body)
	u, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	got := u.Query().Get("body")
	if enc := len(url.QueryEscape(got)); enc > maxBodyURL {
		t.Errorf("encoded body is %d bytes, want <= %d", enc, maxBodyURL)
	}
	if !utf8.ValidString(got) {
		t.Error("trimmed body isn't valid UTF-8")
	}
	if !strings.HasPrefix(got, "## What happened\n\nThe model stopped — again ✗") || !strings.HasSuffix(got, "(trimmed)") {
		t.Errorf("trimmed body lost the start or the marker: %q…%q", got[:40], got[len(got)-20:])
	}
	if short := NewIssueURL("t", "short"); !strings.Contains(short, "body=short") {
		t.Errorf("short body changed: %s", short)
	}
}
