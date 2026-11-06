package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunServesAndLogs(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "requests.jsonl")
	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, []string{"-key", "ezlm-secretkey123456", "-alias", "qwen3.5-4b", "-log", logPath}, pw)
	}()
	base, err := bufio.NewReader(pr).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	base = strings.TrimSpace(base)
	if !strings.HasPrefix(base, "http://127.0.0.1:") {
		t.Fatalf("first line %q", base)
	}
	req, _ := http.NewRequest("GET", base+"/v1/models", nil)
	req.Header.Set("Authorization", "Bearer ezlm-secretkey123456")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var m struct{ Data []struct{ ID string } }
	json.NewDecoder(resp.Body).Decode(&m)
	resp.Body.Close()
	if len(m.Data) != 1 || m.Data[0].ID != "qwen3.5-4b" {
		t.Fatalf("models %+v", m)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(logPath)
	if !strings.Contains(string(b), `"auth":"ok"`) || strings.Contains(string(b), "secretkey") {
		t.Fatalf("log %s", b)
	}
}

func TestRunRefusesOtherAddresses(t *testing.T) {
	if err := run(context.Background(), []string{"-addr", "0.0.0.0:0"}, io.Discard); err == nil {
		t.Fatal("fakellm must only listen on loopback")
	}
}
