package serve

import (
	"crypto/sha1"
	"fmt"
	"os"
)

// testSuffix is "" for the real install. A test instance with its own
// EZLM_HOME gets its own service name ("-test-1a2b3c4d"), so it can never stop
// or replace the real install's server: launchd jobs and systemd units are
// per user, not per EZLM folder.
func testSuffix() string {
	h := os.Getenv("EZLM_HOME")
	if h == "" {
		return ""
	}
	return fmt.Sprintf("-test-%x", sha1.Sum([]byte(h)))[:14]
}
