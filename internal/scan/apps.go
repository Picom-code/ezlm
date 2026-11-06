package scan

// Model mode: the programs using the most memory, grouped by app, and a safe
// way to close them that the user confirms one at a time.
//
// Safety rules (the same for listing and closing, re-checked at close time):
//   - only the current user's own processes;
//   - never the operating system (anything under C:\Windows, /System, ...),
//     security software, remote access (SSH, Tailscale) or EZLM itself;
//   - closing is gentle by default (the same as clicking the window's X, so
//     browsers restore tabs and apps can ask to save); force-closing happens
//     only when the user asks for it after a gentle close didn't work.

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/picom-code/ezlm/internal/paths"
)

// App is a program using memory, with all of its processes.
type App struct {
	ID      string  `json:"id"`      // stable while it runs: its install folder
	Name    string  `json:"name"`    // what people call it ("Google Chrome")
	Exe     string  `json:"exe"`     // its main executable (for the icon)
	PIDs    []int32 `json:"pids"`    //
	RAM     uint64  `json:"ram"`     // system memory it would free
	GPU     uint64  `json:"gpu"`     // graphics memory it holds
	Windows bool    `json:"windows"` // has a window, so it can be closed gently
	Kind    string  `json:"kind"`    // "app", "background" or "ai" (another model server)
	Note    string  `json:"note"`    // what to know before closing it
}

// CloseResult says what happened when closing an app.
type CloseResult struct {
	Closed    bool   `json:"closed"`     // every process of it exited
	Remaining int    `json:"remaining"`  // processes still running
	Freed     uint64 `json:"freed"`      // memory released
	NeedForce bool   `json:"need_force"` // no window to close, or it kept running
}

var osGetenv = os.Getenv

// proc is one process as the platform layer reports it.
type proc struct {
	pid  int32
	exe  string
	ram  uint64
	mine bool // owned by the current user
}

// MemoryApps lists apps using at least minBytes of memory (system plus
// graphics), biggest first.
func MemoryApps(ctx context.Context, minBytes uint64) []App {
	procs := listProcs(ctx)
	gpu := gpuUsage(ctx)
	wins := windowPIDs()
	groups := map[string]*App{}
	biggest := map[string]uint64{}
	for _, p := range procs {
		if !closableProc(p) {
			continue
		}
		key := installRoot(p.exe, os.Getenv)
		a := groups[key]
		if a == nil {
			a = &App{ID: key, Exe: p.exe}
			groups[key] = a
		}
		a.PIDs = append(a.PIDs, p.pid)
		a.RAM += p.ram
		a.GPU += gpu[p.pid]
		if wins[p.pid] {
			a.Windows = true
		}
		// The process that names the app: one with a window beats one whose
		// name matches its folder, which beats the biggest helper.
		score := p.ram
		if strings.Contains(strings.ToLower(filepath.Base(key)), baseName(p.exe)) || strings.Contains(baseName(p.exe), strings.ToLower(filepath.Base(key))) {
			score += 1 << 50
		}
		if wins[p.pid] {
			score += 1 << 60
		}
		if score >= biggest[key] {
			biggest[key], a.Exe = score, p.exe
		}
	}
	var out []App
	for _, a := range groups {
		if a.RAM+a.GPU < minBytes {
			continue
		}
		a.Name = friendlyName(a.Exe)
		a.Kind, a.Note = describe(a)
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RAM+2*out[i].GPU > out[j].RAM+2*out[j].GPU })
	return out
}

// CloseApp closes one app from MemoryApps, gently unless force is set.
func CloseApp(ctx context.Context, id string, force bool) (CloseResult, error) {
	var app *App
	for _, a := range MemoryApps(ctx, 0) {
		if a.ID == id {
			a := a
			app = &a
			break
		}
	}
	if app == nil {
		return CloseResult{Closed: true}, nil // already gone
	}
	if !force && !app.Windows && !gracefulWithoutWindow() {
		return CloseResult{Remaining: len(app.PIDs), NeedForce: true}, nil
	}
	ram := map[int32]uint64{}
	for _, p := range listProcs(ctx) {
		ram[p.pid] = p.ram
	}
	if force {
		for _, pid := range app.PIDs {
			killPID(pid, app.Exe)
		}
	} else {
		closeGently(app)
	}
	// Wait for it to go: apps may take a few seconds to save and exit.
	limit := 12 * time.Second
	if force {
		limit = 5 * time.Second
	}
	deadline := time.Now().Add(limit)
	var alive []int32
	for {
		alive = alivePIDs(ctx, app.PIDs)
		if len(alive) == 0 || time.Now().After(deadline) || ctx.Err() != nil {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	var freed uint64
	for _, pid := range app.PIDs {
		if !contains(alive, pid) {
			freed += ram[pid]
		}
	}
	return CloseResult{Closed: len(alive) == 0, Remaining: len(alive), Freed: freed, NeedForce: len(alive) > 0}, nil
}

func contains(xs []int32, x int32) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// alivePIDs returns which of pids still run (with the same owner rules, so a
// reused PID belonging to something else doesn't count as the app).
func alivePIDs(ctx context.Context, pids []int32) []int32 {
	live := map[int32]bool{}
	for _, p := range listProcs(ctx) {
		live[p.pid] = true
	}
	var out []int32
	for _, pid := range pids {
		if live[pid] {
			out = append(out, pid)
		}
	}
	return out
}

// closableProc applies the safety rules to one process.
func closableProc(p proc) bool {
	if !p.mine || p.pid <= 4 || p.pid == int32(os.Getpid()) || p.exe == "" {
		return false
	}
	if protectedName(baseName(p.exe)) || systemPath(p.exe) {
		return false
	}
	// EZLM itself: the app, its engine, and the model server it runs.
	norm := func(s string) string { return strings.ToLower(strings.ReplaceAll(filepath.Clean(s), `\`, "/")) }
	l := norm(p.exe)
	if strings.HasPrefix(l, norm(paths.Home())+"/") || strings.Contains(l, "/programs/ezlm/") || strings.Contains(l, "/ezlm.app/") {
		return false
	}
	return true
}

// baseName is the lowercased file name without ".exe".
func baseName(exe string) string {
	b := strings.ToLower(filepath.Base(strings.ReplaceAll(exe, `\`, "/")))
	return strings.TrimSuffix(b, ".exe")
}

var protected = map[string]bool{
	// Windows (most live under C:\Windows and are skipped by path too)
	"system": true, "idle": true, "registry": true, "memory compression": true, "secure system": true,
	"smss": true, "csrss": true, "wininit": true, "winlogon": true, "services": true, "lsass": true, "lsaiso": true,
	"svchost": true, "dwm": true, "explorer": true, "fontdrvhost": true, "sihost": true, "taskhostw": true, "ctfmon": true,
	"runtimebroker": true, "searchhost": true, "searchindexer": true, "searchapp": true, "startmenuexperiencehost": true,
	"shellexperiencehost": true, "textinputhost": true, "lockapp": true, "msmpeng": true, "nissrv": true,
	"mpdefendercoreservice": true, "securityhealthservice": true, "securityhealthsystray": true, "smartscreen": true,
	"audiodg": true, "spoolsv": true, "wmiprvse": true, "conhost": true, "dllhost": true, "sgrmbroker": true,
	"applicationframehost": true, "vmmem": true, "vmmemwsl": true, "vmcompute": true, "wslservice": true, "wslhost": true,
	"nvcontainer": true, "nvdisplay.container": true,
	"msedgewebview2": true, // runs inside other apps; closing it breaks them
	// remote access: closing these could cut someone off from the machine
	"sshd": true, "ssh-agent": true, "tailscale": true, "tailscaled": true, "tailscale-ipn": true, "anydesk": true, "teamviewer": true, "rustdesk": true,
	// macOS
	"kernel_task": true, "launchd": true, "windowserver": true, "loginwindow": true, "finder": true, "dock": true,
	"systemuiserver": true, "controlcenter": true, "coreaudiod": true, "mds": true, "mds_stores": true, "mdworker": true,
	"notificationcenter": true, "spotlight": true,
	// Linux desktops
	"systemd": true, "init": true, "xorg": true, "xwayland": true, "gnome-shell": true, "plasmashell": true,
	"kwin_wayland": true, "kwin_x11": true, "pipewire": true, "wireplumber": true, "pulseaudio": true,
	"dbus-daemon": true, "gdm": true, "sddm": true, "lightdm": true, "networkmanager": true,
	// EZLM
	"ezlm": true,
}

func protectedName(base string) bool { return protected[base] }

// systemPath is true for operating-system locations.
func systemPath(exe string) bool {
	l := strings.ToLower(filepath.Clean(exe))
	if runtime.GOOS == "windows" {
		root := strings.ToLower(os.Getenv("SystemRoot"))
		if root == "" {
			root = `c:\windows`
		}
		return strings.HasPrefix(l, root+`\`)
	}
	for _, p := range []string{"/system/", "/usr/libexec/", "/usr/sbin/", "/sbin/", "/library/apple/", "/usr/lib/systemd/"} {
		if strings.HasPrefix(l, p) {
			return true
		}
	}
	return false
}

// installRoot is the folder that groups an app's processes: the first folder
// under the usual install locations (Program Files\Steam, AppData\Local\Discord),
// the outermost .app bundle on macOS, else the program's own folder.
func installRoot(exe string, getenv func(string) string) string {
	if i := strings.Index(exe, ".app/"); i >= 0 {
		return strings.ToLower(exe[:i+4])
	}
	if strings.Contains(exe, `\`) || (len(exe) > 1 && exe[1] == ':') {
		l := strings.ToLower(strings.ReplaceAll(exe, "/", `\`))
		var roots []string
		for _, v := range []string{"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432", "LOCALAPPDATA", "APPDATA"} {
			if r := getenv(v); r != "" {
				roots = append(roots, strings.ToLower(strings.TrimRight(r, `\`))+`\`)
			}
		}
		if la := getenv("LOCALAPPDATA"); la != "" {
			roots = append(roots, strings.ToLower(strings.TrimRight(la, `\`))+`\programs\`)
		}
		for _, r := range append([]string(nil), roots...) {
			roots = append(roots, r+`windowsapps\`)
		}
		best := ""
		for _, r := range roots {
			if strings.HasPrefix(l, r) && len(r) > len(best) {
				best = r
			}
		}
		if best != "" {
			parts := strings.Split(l[len(best):], `\`)
			if len(parts) > 1 {
				// Vendors install several products under one folder
				// (Google\Chrome, NVIDIA Corporation\NVIDIA app): go one deeper.
				if vendorFolders[parts[0]] && len(parts) > 2 {
					return best + parts[0] + `\` + parts[1]
				}
				return best + parts[0]
			}
		}
		if i := strings.LastIndex(l, `\`); i > 0 {
			return l[:i]
		}
		return l
	}
	dir := filepath.Dir(exe)
	switch dir {
	case "/usr/bin", "/bin", "/usr/local/bin", "/usr/sbin", "/opt/homebrew/bin", "/usr/games":
		return strings.ToLower(exe) // shared folders: each program is its own app
	}
	return strings.ToLower(dir)
}

var vendorFolders = map[string]bool{
	"google": true, "microsoft": true, "nvidia corporation": true, "adobe": true, "common files": true, "amd": true,
	"intel": true, "jetbrains": true, "logitech": true, "razer": true, "corsair": true, "elgato": true, "alienware": true,
	"dell": true, "hp": true, "lenovo": true, "asus": true, "msi": true, "microsoft office": true, "programs": true,
}

// runtimes host other programs, so they're named after the folder they run from.
var runtimes = map[string]bool{"python": true, "pythonw": true, "python3": true, "node": true, "java": true, "javaw": true, "dotnet": true, "ruby": true, "deno": true, "bun": true, "electron": true}

var aiServers = map[string]bool{
	"llama-server": true, "ollama": true, "ollama app": true, "lms": true, "lm studio": true, "koboldcpp": true,
	"llamafile": true, "jan": true, "gpt4all": true, "msty": true, "text-generation-launcher": true, "vllm": true,
}

var names = map[string]string{
	"chrome": "Google Chrome", "msedge": "Microsoft Edge", "firefox": "Firefox", "brave": "Brave", "opera": "Opera",
	"vivaldi": "Vivaldi", "arc": "Arc", "discord": "Discord", "spotify": "Spotify", "steam": "Steam",
	"steamwebhelper": "Steam", "code": "Visual Studio Code", "cursor": "Cursor", "slack": "Slack",
	"ms-teams": "Microsoft Teams", "teams": "Microsoft Teams", "obs64": "OBS Studio", "obs": "OBS Studio",
	"whatsapp": "WhatsApp", "telegram": "Telegram", "zoom": "Zoom", "photoshop": "Photoshop", "notion": "Notion",
	"figma": "Figma", "olk": "Outlook", "outlook": "Outlook", "winword": "Word", "excel": "Excel",
	"powerpnt": "PowerPoint", "onedrive": "OneDrive", "epicgameslauncher": "Epic Games Launcher",
	"battle.net": "Battle.net", "ea desktop": "EA app", "eadesktop": "EA app", "riotclientservices": "Riot Client",
	"leagueclient": "League of Legends", "medal": "Medal", "nvidia overlay": "NVIDIA Overlay",
	"nvidia app": "NVIDIA App", "llama-server": "llama-server", "ollama": "Ollama", "ollama app": "Ollama",
	"lm studio": "LM Studio", "lms": "LM Studio", "opencode": "OpenCode", "claude": "Claude", "chatgpt": "ChatGPT",
}

// friendlyName is what people call the program.
func friendlyName(exe string) string {
	b := baseName(exe)
	if n, ok := names[b]; ok {
		return n
	}
	if runtimes[b] {
		folder := filepath.Base(strings.ReplaceAll(installRoot(exe, osGetenv), `\`, "/"))
		lang := map[string]string{"pythonw": "python", "python3": "python", "javaw": "java"}[b]
		if lang == "" {
			lang = b
		}
		return upperFirst(folder) + " (" + upperFirst(lang) + ")"
	}
	if n := strings.TrimSuffix(strings.TrimSuffix(fileDescription(exe), ".exe"), ".EXE"); n != "" {
		return n
	}
	if i := strings.Index(exe, ".app/"); i >= 0 {
		if n := strings.TrimSuffix(filepath.Base(exe[:i+4]), ".app"); n != "" {
			return n
		}
	}
	if b == "" {
		return "Unknown app"
	}
	return upperFirst(b)
}

// upperFirst capitalises the first letter and leaves the rest alone, including
// names that start outside ASCII.
func upperFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}

// describe picks the app's kind and the one thing to know before closing it.
func describe(a *App) (kind, note string) {
	b := baseName(a.Exe)
	switch {
	case aiServers[b]:
		return "ai", "Another AI model server, not EZLM's. Close it only if you're not using it."
	case b == "chrome" || b == "msedge" || b == "brave" || b == "vivaldi" || b == "opera" || b == "arc":
		return "app", a.Name + " reopens your tabs next time."
	case b == "firefox":
		return "app", "Firefox can restore your tabs next time."
	case b == "discord":
		return "app", "Discord keeps running in the background after its window closes; EZLM will offer to close it fully."
	case b == "steam" || b == "steamwebhelper":
		return "app", "Closing Steam also pauses downloads and updates."
	case b == "obs64" || b == "obs":
		return "app", "Stop any recording or stream first."
	case b == "code" || b == "cursor":
		return "app", "It reopens your files; save your work first."
	case vpns[b]:
		return kindFor(a), "Closing it disconnects your VPN."
	case runtimes[b]:
		return kindFor(a), "A program running in the background (" + a.Name + "). Close it only if you know you don't need it right now."
	case !a.Windows:
		return "background", "It has no window, so closing it means stopping it; unsaved data in it may be lost."
	}
	return "app", "It may ask you to save your work first."
}

var vpns = map[string]bool{"nordvpn": true, "expressvpn": true, "protonvpn": true, "mullvad vpn": true, "surfshark": true, "openvpn": true, "openvpn-gui": true, "wireguard": true, "windscribe": true}

func kindFor(a *App) string {
	if a.Windows {
		return "app"
	}
	return "background"
}
