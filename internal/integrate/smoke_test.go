package integrate

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

func TestStripANSI(t *testing.T) {
	for in, want := range map[string]string{
		"\x1b[1mREADY\x1b[0m":                      "READY",
		"Ask\x1b[1Canything\x1b[3C>":               "Ask anything   >",
		"\x1b[2;1Hrow two\x1b[3;1Hrow three":       "\nrow two\nrow three",
		"\x1b]0;OpenCode (EZLM)\x07hello":          "hello",
		"\x1b]8;;http://x\x1b\\link\x1b]8;;\x1b\\": "link",
		"a\r\nb\rc\x08d\x07":                       "a\nb\ncd",
		"\x1b(Bplain\x1b=\x1b>":                    "plain",
		"\x1b[?25l\x1b[?1049hscreen":               "screen",
		"\x1bP+q544e\x1b\\after":                   "after",
		"Jörg\x1b[38;2;255;0;0m!":                  "Jörg!",
		"cut\x1b[":                                 "cut",
	} {
		if got := StripANSI(in); got != want {
			t.Errorf("StripANSI(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSmokeHelperReady(t *testing.T) {
	exe, lead := launchTestHelper(t)
	res, err := Smoke(context.Background(), LaunchSpec{Bin: exe, Args: lead, Env: []string{"EZLM_LAUNCH_HELPER=ready"}, Dir: t.TempDir(), Ready: []string{"READY to go"}}, 20*time.Second)
	if err != nil || !res.OK || !res.Ready || !res.Alive || res.Fatal != "" {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestSmokeHelperFatal(t *testing.T) {
	exe, lead := launchTestHelper(t)
	res, err := Smoke(context.Background(), LaunchSpec{Bin: exe, Args: lead, Env: []string{"EZLM_LAUNCH_HELPER=fatal"}, Dir: t.TempDir(), Ready: []string{"READY"}}, 20*time.Second)
	if err != nil || res.OK || res.Fatal != "Database is not empty" {
		t.Fatalf("%+v %v", res, err)
	}
	if !strings.Contains(res.Screen, "no session table") {
		t.Errorf("screen: %q", res.Screen)
	}
}

func TestSmokeNoReadyMarker(t *testing.T) {
	exe, lead := launchTestHelper(t)
	start := time.Now()
	res, err := Smoke(context.Background(), LaunchSpec{Bin: exe, Args: lead, Env: []string{"EZLM_LAUNCH_HELPER=ready"}, Dir: t.TempDir(), Ready: []string{"never shown"}}, 3*time.Second)
	if err != nil || res.OK || res.Ready || !res.Alive {
		t.Fatalf("%+v %v", res, err)
	}
	if d := time.Since(start); d < 3*time.Second || d > 15*time.Second {
		t.Errorf("waited %v", d)
	}
}

func TestSmokeRefusesKeys(t *testing.T) {
	for _, spec := range []LaunchSpec{
		{Bin: "/bin/true", Env: []string{"EZLM_API_KEY=whatever"}},
		{Bin: "/bin/true", Env: []string{"OTHER=ezlm-AbCdEf1234567890xy"}},
		{Bin: "/bin/true", Args: []string{"--key", "sk-Ab3dEf6hIj9kLmN0pQrStU"}},
	} {
		if _, err := Smoke(context.Background(), spec, time.Second); err == nil {
			t.Errorf("%+v: a key must be refused", spec)
		}
	}
}

// TestPTYCloseKillsTree: Close stops the program and what it started.
func TestPTYCloseKillsTree(t *testing.T) {
	exe, lead := launchTestHelper(t)
	cmd := exec.Command(exe, lead...)
	cmd.Env = append(os.Environ(), "EZLM_LAUNCH_HELPER=tree")
	lt := launchStartTerm(t, cmd)
	lt.waitFor("READY", 20*time.Second)
	m := regexp.MustCompile(`CHILD:(\d+)`).FindStringSubmatch(lt.text())
	if m == nil {
		t.Fatalf("no child pid:\n%s", lt.text())
	}
	child, _ := strconv.Atoi(m[1])
	parent := lt.p.Pid()
	if ok, _ := process.PidExists(int32(child)); !ok {
		t.Fatal("the child isn't running")
	}
	lt.p.Close()
	if !lt.p.Exited() {
		t.Error("Close must wait for the program to end")
	}
	for end := time.Now().Add(5 * time.Second); ; time.Sleep(100 * time.Millisecond) {
		c, _ := process.PidExists(int32(child))
		p, _ := process.PidExists(int32(parent))
		if !c && !p {
			break
		}
		if time.Now().After(end) {
			t.Fatalf("still running after Close: child %v, program %v", c, p)
		}
	}
}

func TestPTYExitCode(t *testing.T) {
	exe, lead := launchTestHelper(t)
	cmd := exec.Command(exe, append(lead, "x")...)
	cmd.Env = append(os.Environ(), "EZLM_LAUNCH_HELPER=7", "EZLM_LAUNCH_PROBE=p")
	lt := launchStartTerm(t, cmd)
	code, err := lt.p.Wait()
	if err != nil || code != 7 {
		t.Fatalf("code %d, %v", code, err)
	}
	lt.waitFor("PROBE:p", 5*time.Second)
}
