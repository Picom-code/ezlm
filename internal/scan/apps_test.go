package scan

import (
	"strings"
	"testing"
)

func winEnv(k string) string {
	return map[string]string{
		"ProgramFiles":      `C:\Program Files`,
		"ProgramFiles(x86)": `C:\Program Files (x86)`,
		"ProgramW6432":      `C:\Program Files`,
		"LOCALAPPDATA":      `C:\Users\a\AppData\Local`,
		"APPDATA":           `C:\Users\a\AppData\Roaming`,
	}[k]
}

func TestInstallRootGroupsHelpers(t *testing.T) {
	same := [][2]string{
		{`C:\Program Files (x86)\Steam\steam.exe`, `C:\Program Files (x86)\Steam\bin\cef\cef.win7x64\steamwebhelper.exe`},
		{`C:\Program Files\Google\Chrome\Application\chrome.exe`, `C:\Program Files\Google\Chrome\Application\131.0\chrome_crashpad_handler.exe`},
		{`C:\Users\a\AppData\Local\Discord\app-1.0.9\Discord.exe`, `C:\Users\a\AppData\Local\Discord\app-1.0.9\modules\x\Discord.exe`},
		{"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "/Applications/Google Chrome.app/Contents/Frameworks/X.framework/Helpers/Google Chrome Helper.app/Contents/MacOS/Google Chrome Helper"},
	}
	for _, p := range same {
		if a, b := installRoot(p[0], winEnv), installRoot(p[1], winEnv); a != b {
			t.Errorf("%s and %s grouped apart: %q vs %q", p[0], p[1], a, b)
		}
	}
	apart := [][2]string{
		{`C:\Users\a\AppData\Local\Programs\Microsoft VS Code\Code.exe`, `C:\Users\a\AppData\Local\Programs\Obsidian\Obsidian.exe`},
		{`C:\Program Files\WindowsApps\Microsoft.WindowsTerminal_1.21\WindowsTerminal.exe`, `C:\Program Files\WindowsApps\SpotifyAB.Spotify_1.2\Spotify.exe`},
		{"/usr/bin/firefox", "/usr/bin/thunderbird"},
	}
	for _, p := range apart {
		if a, b := installRoot(p[0], winEnv), installRoot(p[1], winEnv); a == b {
			t.Errorf("%s and %s grouped together as %q", p[0], p[1], a)
		}
	}
}

func TestClosableSafetyRules(t *testing.T) {
	ok := proc{pid: 1234, exe: `C:\Program Files\Google\Chrome\Application\chrome.exe`, mine: true, ram: 1 << 30}
	if !closableProc(ok) {
		t.Fatal("a user's own Chrome should be closable")
	}
	for name, p := range map[string]proc{
		"another user's":  {pid: 1234, exe: ok.exe, mine: false},
		"explorer":        {pid: 1234, exe: `C:\Windows\explorer.exe`, mine: true},
		"svchost":         {pid: 1234, exe: `C:\Windows\System32\svchost.exe`, mine: true},
		"sshd":            {pid: 1234, exe: `C:\Program Files\OpenSSH\sshd.exe`, mine: true},
		"tailscale":       {pid: 1234, exe: `C:\Program Files\Tailscale\tailscale-ipn.exe`, mine: true},
		"ezlm app":        {pid: 1234, exe: `C:\Users\a\AppData\Local\Programs\EZLM\EZLM.exe`, mine: true},
		"ezlm engine":     {pid: 1234, exe: `C:\Users\a\AppData\Local\Programs\EZLM\Engine\ezlm.exe`, mine: true},
		"system pid":      {pid: 4, exe: `C:\x.exe`, mine: true},
		"no exe":          {pid: 1234, exe: "", mine: true},
		"macOS WindowSrv": {pid: 1234, exe: "/System/Library/PrivateFrameworks/SkyLight.framework/Resources/WindowServer", mine: true},
	} {
		if closableProc(p) {
			t.Errorf("%s must never be offered for closing", name)
		}
	}
}

func TestDescribe(t *testing.T) {
	a := &App{Exe: `C:\x\llama-server.exe`, Name: "llama-server", Windows: false}
	if k, n := describe(a); k != "ai" || !strings.Contains(n, "not EZLM's") {
		t.Errorf("other AI servers should be labelled: %s %s", k, n)
	}
	a = &App{Exe: `C:\x\worker.exe`, Name: "Worker", Windows: false}
	if k, _ := describe(a); k != "background" {
		t.Errorf("windowless apps are background: %s", k)
	}
	if friendlyName(`C:\Program Files\Google\Chrome\Application\chrome.exe`) != "Google Chrome" {
		t.Error("friendly name for Chrome")
	}
}

func TestRuntimeAndVendorNaming(t *testing.T) {
	old := osGetenv
	osGetenv = winEnv
	defer func() { osGetenv = old }()
	if n := friendlyName(`C:\Users\a\AppData\Local\hermes\hermes-agent\venv\Scripts\python.exe`); n != "Hermes (Python)" {
		t.Errorf("python runtime named %q", n)
	}
	a, b := installRoot(`C:\Program Files\NVIDIA Corporation\NVIDIA app\CEF\NVIDIA Overlay.exe`, winEnv), installRoot(`C:\Program Files\NVIDIA Corporation\NvBroadcast\NVIDIA Broadcast.exe`, winEnv)
	if a == b {
		t.Errorf("NVIDIA products grouped together: %q", a)
	}
}
