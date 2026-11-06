// Package report files beta testers' problem reports as GitHub issues.
package report

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"
	"unicode/utf8"
)

// Repo is where reports go.
var Repo = "picom-code/ezlm"

// Token can file issues on Repo. It's set at build time (never committed):
//
//	go build -ldflags "-X github.com/picom-code/ezlm/internal/report.Token=…"
//
// and should be a fine-grained token limited to Issues on that one repo.
// EZLM_REPORT_TOKEN overrides it for testing. Without one, reports open
// GitHub's new-issue page in the browser instead.
var Token = ""

type Result struct {
	Sent   bool   `json:"sent"`   // filed directly
	URL    string `json:"url"`    // the issue, or the pre-filled page to finish in the browser
	Number int    `json:"number"` // issue number when sent
}

func token() string {
	if t := os.Getenv("EZLM_REPORT_TOKEN"); t != "" {
		return t
	}
	return Token
}

// Send files the report, or returns a pre-filled new-issue link if it can't.
func Send(ctx context.Context, title, body string) (Result, error) {
	fallback := Result{URL: NewIssueURL(title, body)}
	tok := token()
	if tok == "" {
		return fallback, nil
	}
	payload, _ := json.Marshal(map[string]any{"title": title, "body": body, "labels": []string{"beta-report"}})
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.github.com/repos/"+Repo+"/issues", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fallback, nil // offline or blocked: let them send it from the browser
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return fallback, nil
	}
	var out struct {
		URL    string `json:"html_url"`
		Number int    `json:"number"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fallback, nil
	}
	return Result{Sent: true, URL: out.URL, Number: out.Number}, nil
}

// maxBodyURL caps the encoded body: GitHub answers 414 to much longer links,
// and logs full of symbols triple in size when encoded.
const maxBodyURL = 6000

// NewIssueURL is GitHub's new-issue page with the report filled in. Long
// reports are cut from the end (the logs), keeping what the person wrote.
func NewIssueURL(title, body string) string {
	if len(url.QueryEscape(body)) > maxBodyURL {
		const suffix = "\n\n…(trimmed)"
		for len(url.QueryEscape(body+suffix)) > maxBodyURL {
			cut := len(body) * 9 / 10
			for cut > 0 && !utf8.RuneStart(body[cut]) {
				cut--
			}
			body = body[:cut]
		}
		body += suffix
	}
	return fmt.Sprintf("https://github.com/%s/issues/new?labels=beta-report&title=%s&body=%s", Repo, url.QueryEscape(title), url.QueryEscape(body))
}
