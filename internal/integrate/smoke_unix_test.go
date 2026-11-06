//go:build !windows

package integrate

import (
	"context"
	"testing"
	"time"
)

func TestSmokeReadyAndFatal(t *testing.T) {
	ok, err := Smoke(context.Background(), LaunchSpec{Bin: "/bin/sh", Args: []string{"-c", "printf '\\033[1mREADY\\033[0m\\n'; sleep 30"}, Ready: []string{"READY"}}, 10*time.Second)
	if err != nil || !ok.OK || !ok.Ready || !ok.Alive {
		t.Fatalf("ready app: %+v %v", ok, err)
	}
	bad, err := Smoke(context.Background(), LaunchSpec{Bin: "/bin/sh", Args: []string{"-c", "echo 'Error: Database is not empty and has no session table'; exit 1"}, Ready: []string{"READY"}}, 10*time.Second)
	if err != nil || bad.OK || bad.Fatal == "" {
		t.Fatalf("failing app: %+v %v", bad, err)
	}
}
