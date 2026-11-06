package app

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestReportLive files a real issue (EZLM_LIVE_REPORT=1 plus EZLM_REPORT_TOKEN).
// It runs in the temp EZLM_HOME set up by TestMain.
func TestReportLive(t *testing.T) {
	if os.Getenv("EZLM_LIVE_REPORT") == "" || os.Getenv("EZLM_REPORT_TOKEN") == "" {
		t.Skip("set EZLM_LIVE_REPORT=1 and EZLM_REPORT_TOKEN to file a real test issue")
	}
	r, err := New().Report(context.Background(), "Test report from EZLM's own test suite (safe to close).\nIt checks that reports reach GitHub Issues with the beta-report label.", true, "app log line 1\napp log line 2")
	if err != nil {
		t.Fatal(err)
	}
	if !r.Sent || r.Number == 0 || !strings.Contains(r.URL, "/issues/") {
		t.Fatalf("not sent: %+v", r)
	}
	t.Logf("filed %s", r.URL)
}
