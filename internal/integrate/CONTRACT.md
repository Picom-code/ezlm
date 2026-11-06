# App connections: the contract

Seven workstreams build EZLM's app connections in parallel against this file. It is small on
purpose: the shared types and functions below are frozen for this round. If you need a change,
ask the core owner; never edit another workstream's file. Read with `PLAN.md` and `DECISIONS.md`
(DECISIONS wins where they differ).

**As built (integrated 2026-11-05):** sections 11 to 13 record what was built and every place it
differs from sections 1 to 10. Where they differ, sections 11 to 13 win.

## 1. Ownership

Every file has exactly one owner. A new file must match its owner's pattern below.

| Workstream | Owns |
|---|---|
| **core** | `internal/integrate/`: `integrate.go`, `types.go`, `registry.go`, `log.go`, `installer.go`, `atomic.go`, `cache.go`, `check.go`, `findtool*.go`, `arch*.go`, `tools.go` (+ `tools.json` if added), `yamlnode.go`, `deps.go`, `main_test.go`, `integrate_test.go`, `CONTRACT.md`, `testdata/core/**`, and any new file in this folder that matches no other row. Also `internal/app/**` (server gate, rewrite rules, Hermes < 64K, launch exit watcher, report attachments, demo), `internal/serve/**` (props and reasoning levels), `internal/proc/**`, `internal/catalog/**` (the `tools` section), `internal/paths/**`, `cmd/ezlm/**`, `go.mod`, `go.sum`. |
| **launch** | `internal/integrate/launch*.go`, `terminal*.go`, `pty*.go`, `smoke*.go` (and their `_test.go`), `testdata/launch/**`; `internal/fakellm/**`; `cmd/tuiprobe/**`, `cmd/fakellm/**`; `scripts/e2e/**` except the per-tool step files below. |
| **opencode** | `internal/integrate/opencode*.go` (incl. `opencode_test.go`), `testdata/opencode/**`, `scripts/e2e/opencode.sh`. |
| **hermes** | `internal/integrate/hermes*.go` (incl. `hermes_test.go`), `testdata/hermes/**`, `scripts/e2e/hermes.sh`. |
| **dsh** | `internal/integrate/dsh*.go`, `node*.go` (and their tests), `testdata/dsh/**`, `testdata/node/**`, `scripts/e2e/dsh.sh`. |
| **winui** | `windows/EZLM/**` (incl. `Assets/Logos/**`). |
| **macweb** | `macos/**` (incl. `Logos/**`), `internal/web/**` (incl. `static/logos/**`). |

Rules for the shared Go package `internal/integrate`:

- Tool files: every package-level name contains the tool's name (`openCode…`, `WriteOpenCodeConfig`,
  `hermes…`, `dsh…`, `node…`), test helpers too (`openCodeTestEP`). Two `init()` functions are fine.
- Launch files: only the exported names listed in section 3.9 to 3.11; unexported names start with
  `launch`, `term`, `pty` or `smoke`.
- Core takes no name with those prefixes or tool names.
- Nobody but core edits `go.mod`/`go.sum`. Ask for a module; until then, use only what is there
  (`yaml.v3`, `hujson`, `creack/pty`, `x/sys`, `gopsutil`, the standard library).
- `windows/installer`, `scripts/*.py`, `BETA.md` are not part of this round.

## 2. How a connection runs (core pipelines, `integrate.go`)

```
Targets(ep *Endpoint) []Target                  // every Status poll; cheap
Setup(ctx, id, ep, makeDefault, log Log) error  // Resolve(install) → Configure → Check → Preflight → Smoke → SaveCheck
Rewrite(id, ep) error                           // Configure(ep, false)
Preflight(ctx, id, ep, log) error               // Resolve(no install) → tool.Preflight
Open(ctx, id, mode, ep, log) (Launched, error)  // Resolve(no install) → LaunchSpec → Launch
Off() bool                                      // EZLM_NO_INTEGRATIONS
Workspace() string                              // ~/EZLM Workspace
var ErrNotInstalled, ErrNeeds64K
```

Setup's progress lines: the tool's own (`r.Step`, e.g. "Installing OpenCode…"), then core's
"Adding EZLM as an OpenCode provider…" and "Testing OpenCode with your model…". It saves a
`CheckResult` either way. The TUI smoke runs only when the main mode's `LaunchSpec.Ready` is set.

**State** (core's `deriveState`, precedence top to bottom; a tool may set `Detection.State` itself,
e.g. `needs_update` or `needs_tools`, and core still applies these overrides):

1. `not_installed`: `Detection.Bin == ""`.
2. `needs_64k`: `Linked` and `ep.Ctx < Info.MinCtx`. Detail: "<Name> needs a 64K context. Connect again to give the model the room (it may run slower), or pick a smaller model."
3. the tool's own `State`, or `connected` if `Linked`, else `installed`;
4. then `broken` if that is `connected`/`installed` and `LastCheck` failed. Detail = `LastCheck.Msg`.

`Target.Connected` = (`State == connected`). `Target.Linked` (not on the wire) = "the config has
EZLM's entry": the app rewrites every Linked target on deploy, model switch, key edit and port move.

## 3. The Go API (`types.go` and friends)

### 3.1 Endpoint and reasoning

```go
type Endpoint struct {
    BaseURL, Key, ModelID, ModelName string // Key: never logged, never in a launcher or Env
    Ctx       int
    ToolCalls bool      // catalog flag
    Vision    bool      // an image projector is loaded (never, today)
    Reasoning Reasoning // from /props after each deploy (DECISIONS 6)
}
type Reasoning struct { Levels []string; Thinking bool }
func (Reasoning) Accepts(level string) bool
```

Writers' rule (DECISIONS 6): a tool's **default** request must never carry an effort level outside
`Levels`. Offer only `Levels` where the tool has per-model level lists (dsh `reasoningEfforts`,
Hermes per-model reasoning, OpenCode variants if 2.2.5 supports them); with `Levels` empty, write no
reasoning settings at all. Core fills `Reasoning` from `serve.Config.Props` (`serve.FetchProps`,
`internal/serve/props.go`); wiring the fetch into deploy is core's job.

### 3.2 Target (the wire type) and its constants

Fields: `ID Name Desc Supported Why Installed Connected` (as before) plus, all optional:
`Icon Version Bin Source State Detail Config MinCtx Launch []LaunchMode LastCheck *CheckResult`.

- States: `not_installed`, `installed`, `connected`, `needs_update`, `needs_64k`, `needs_tools`, `broken`.
- Sources: `ezlm` (EZLM's pinned copy under `~/.ezlm/tools`), `msix`, `script`, `npm`, `path`.
- Launch modes: `tui`, `web`. `LaunchMode{ID, Label}`; `Label` is the full button text.
- Check stages: `model`, `preflight`, `tui`, `launch`.
- `CheckResult{At, OK, ToolCall, Secs, Stage, Msg, LogPath}`. An OK `Msg` starts lowercase so it
  reads after "Checked 2 min ago: " (e.g. "answered and used a tool (14 s)"); a failed `Msg` is the
  full reason sentence.

### 3.3 The tool interface and registry

```go
type Tool interface {
    Info() Info                     // {ID, Name, Desc, Icon, Order, MinCtx, Launch}
    Detect(ep *Endpoint) Detection  // cheap: stat, parse configs, CachedProbe; never runs a program
    Resolve(ctx, r *Run, install bool) (Found, error)  // pick/probe the program; install pinned copy if allowed; else ErrNotInstalled
    Configure(ep Endpoint, makeDefault bool) error      // write EZLM's entry; ErrNeeds64K leaves the file untouched
    Check(ctx, f Found, ep Endpoint, w *CheckWorkspace, r *Run) (toolCall bool, err error)
    Preflight(ctx, f Found, ep Endpoint, r *Run) error  // exact launch binary, env, data dirs; no new session
    LaunchSpec(f Found, ep Endpoint, mode string) (LaunchSpec, error)
}
type Found     struct { Bin, Version, Source, Arch string; Env []string }
type Detection struct { Found; State, Detail string; Linked bool; Config, Unsupported string }
func Register(t Tool)               // from the tool's own init(); panics on a duplicate id
func Replace(t Tool) (restore func()) // tests: swap in a fake (also from other packages)
func Lookup(id string) (Tool, bool)
func All() []Tool                   // by Info.Order: opencode 10, hermes 20, dsh 30
```

Errors returned by `Check`, `Preflight`, `Resolve` and `Configure` are shown to the user as is: one
plain sentence naming the next action. `Found.Env` and `LaunchSpec.Env` never hold keys.

### 3.4 Logging and redaction (`log.go`)

```go
func LogPath(id string) string               // ~/.ezlm/logs/integrate-<id>.log (moved to .1 past 2 MB)
func LogTail(id string, n int) []string
func Redact(s string, secrets ...string) string // literal secrets → REDACTED-KEY; ezlm-…, sk-…, Bearer … shortened
func RedactKeys(s string) string             // ezlm keys → first 9 chars + "…" (reports)
type Run struct{ ID string; … }              // a nil *Run is valid and silent (tool unit tests)
func NewRun(id string, progress Log, secrets ...string) *Run
func (r *Run) Step(msg)                      // UI + log
func (r *Run) Stream(line)                   // log every line, UI at most every 500 ms
func (r *Run) Logf(format, …)                // log only
func (r *Run) Exec(ctx, timeout, cmd) (out string, err error) // check/probe command: logged, tree-killed on timeout
func (r *Run) Install(ctx, cmd, timeout) error                // installer: streamed; 0 = no timeout
func (r *Run) Redact(s) string; LogPath() string; Close() error
```

Everything a tool runs goes through `r.Exec` or `r.Install`, so it is logged and redacted.

### 3.5 Installer runner (`installer.go`)

`ShellCommand(script) *exec.Cmd` (PowerShell on Windows, `bash -lc` elsewhere, env plus `CI=1
NONINTERACTIVE=1`; append your own to `cmd.Env`), `(*Run).Install`, and
`CheckDisk(dir string, need uint64, what string) error`. On failure the error names the last
telling line and the log. Processes the installer leaves running after success are kept
(`proc.InstallLeft`, logged; `(*Run).LeftRunning`); on timeout or cancel its whole tree is stopped.

### 3.6 Config writes (`atomic.go`)

`WriteFileAtomic(path, data, perm)` (temp file in the same folder, fsync, rename; creates the
folder), `Backup(path) (string, error)` (keeps the oldest and newest 3 `*.ezlm-backup-*`),
`WriteConfig(path, data, perm)` = Backup + WriteFileAtomic. Re-read and merge right before writing.
YAML node helpers for Hermes and dsh (`yamlnode.go`): `yamlLoad yamlEncode yamlStr yamlInt yamlBool
yamlMap yamlChild yamlSet yamlDelete`.

### 3.7 Finding programs (`findtool.go`, `arch*.go`)

`FindTool(name string, extraDirs ...string) string`, `FindAllTools(…) []string` (search order:
PATH, extraDirs, the usual per-user folders; core adds the login-shell PATH, the Windows registry
PATH and WindowsApps aliases behind the same signature). `BinaryArch(path) string` reads PE /
Mach-O / ELF headers; `MachineArch() string` is the machine's own architecture (IsWow64Process2 on
Windows, Rosetta-aware on Mac). `EZLM_TOOL_PATH` replaces the whole search (tests).

### 3.8 Cache, pins, checks (`cache.go`, `tools.go`, `check.go`)

```go
type Probe struct { Version, Arch string; Facts map[string]string; … } // e.g. opencode Facts["data"] = "v1"|"v2"|"none"
func CachedProbe(bin string) (Probe, bool)  // valid while size and mtime match
func SaveProbe(bin string, p Probe) error
func LastCheck(id string) *CheckResult;  func SaveCheck(id string, c CheckResult) error
func NotePort(port int) error;           func KnownPorts() []int  // every port EZLM served on
type Pin struct { Tool, Version, Package string; Tested []string; Assets map[string]Asset }
type Asset struct { URL, SHA256 string; Size uint64 }
func RegisterPin(p Pin)                  // overrides tools.json (tests); the shipped pins are all in tools.json
func PinFor(tool string) (Pin, bool)
func AssetKey(goos, goarch string, baseline bool) string // "windows-arm64", "darwin-x64-baseline"
func ToolsDir() string; ToolDir(tool, version string) string; IsEZLMTool(bin string) bool
func FetchAsset(ctx, r *Run, a Asset, dst string) error  // download + SHA-256; refuses an asset without one
const CheckFile = "ezlm-check.txt"; const CheckPrompt = "Use your file-reading tool to open …"
type CheckWorkspace struct { Dir, File, Nonce string } // Nonce = EZLM-CHECK-<8 base32>
func NewCheckWorkspace() (*CheckWorkspace, error); (w) Prompt() string; Passed(out) bool; Close()
type ErrorText struct { Match, Say, State string } // Say may use {app}
func RegisterErrorText(tool string, texts ...ErrorText) // tool "" = all; from init()
func Explain(tool, out string) (ErrorText, bool)       // tool texts first, then common (401, refused, model not found)
func ErrLine(out string) string
```

Check rules (PLAN 3.6, DECISIONS 8): run the app with `cmd.Dir = w.Dir`, prompt `w.Prompt()`, pass
only when `w.Passed(output)`; return `toolCall` true when the app's output shows a tool event.

### 3.9 Launch (`launch.go`, `terminal_*.go`; launch workstream)

```go
type LaunchSpec struct { ID, Mode, Title, Bin string; Args, Env []string; Dir, URL string; Ready, Fatal []string }
type Launched   struct { Launcher, ExitFile string; DryRun bool }
func Launch(spec LaunchSpec) (Launched, error)
func LaunchDir() string                    // ~/.ezlm/launch
func ExitFile(id string) string            // ~/.ezlm/launch/<id>.exit
func ReadExit(path string) (code int, at time.Time, ok bool)
func DryRun() bool                         // EZLM_LAUNCH_DRYRUN
func OpenURL(url string) error
```

- Core fills `ID`, `Mode`, `Title` ("<Name> (EZLM)") and `Dir` (Workspace) when a tool leaves them empty.
- Launcher files: `<LaunchDir>/<id>.command` (Mac, run with `open -a Terminal`), `<id>.cmd` (Windows,
  UTF-8 with `chcp 65001`, `%` → `%%`, `call` for `.cmd`/`.bat`, wt → `cmd /c start` fallback),
  `<id>.sh` (Linux). Exact bodies: PLAN 4.5.
- Exit file: the launcher writes `"<exit code> <unix seconds>\n"` when the app ends. `Launch`
  deletes a stale one first and returns its path in `Launched.ExitFile` only when the launcher
  writes one; core then watches it for 20 s (`internal/app/launch.go watchExit`).
- `EZLM_LAUNCH_DRYRUN=1`: write the launcher, return `Launched{Launcher, ExitFile, DryRun: true}`,
  open nothing (also no browser).
- `Bin == "" && URL != ""`: only open `URL` in the default browser (a web UI already running).
- Never put keys in a launcher, argv or `Env`.

### 3.10 Pseudo-terminal and smoke (`pty_*.go`, `smoke.go`)

```go
func StartPTY(cmd *exec.Cmd, cols, rows int) (*PTY, error) // creack/pty on Unix, ConPTY on Windows
func (p *PTY) Read(b []byte) (int, error); Write(b []byte) (int, error)
func (p *PTY) Pid() int; Exited() bool; Wait() (code int, err error)
func (p *PTY) Close() error  // closes the terminal and kills OUR process tree only
var CommonFatal []string
func StripANSI(s string) string
func Smoke(ctx, spec LaunchSpec, wait time.Duration) (SmokeResult, error) // 120x40; OK = alive, ready, no fatal
type SmokeResult struct { OK, Alive, Ready bool; Fatal string; Code int; Screen string }
```

### 3.11 Fake model server (`internal/fakellm`, `cmd/fakellm`; PLAN 9.3 Tier A)

`fakellm.New(fakellm.Options{Key, Alias, Ctx, Levels, Thinking, NoTools, Log}) *Server` is an
`http.Handler` (use `httptest.NewServer`); `(*Server).Requests() []Request`. It serves `/health`,
`/v1/models` (Bearer key and alias), `/props` and `/v1/props` (`n_ctx`,
`chat_template_caps.reasoning_effort_levels`), and `/v1/chat/completions` streamed in llama-server
b11960's chunk shape (content null first, `reasoning_content` when Thinking, `tool_calls` deltas,
a finish chunk with `timings`, optional usage chunk, `[DONE]`) or not streamed. Script: if a
tool named `*read*` with a path-like parameter is offered and the last message isn't a tool result,
call it on `ezlm-check.txt` (absolute when the prompt names a working directory); a tool result is
echoed back as the answer; "Reply with exactly: X" answers X. A `reasoning_effort` outside
`Levels` gets HTTP 500 "Jinja Exception: Unexpected reasoning effort <x>". `cmd/fakellm` prints
its base URL on the first stdout line; stop it by PID.

## 4. App layer (`internal/app`, core)

- `App.Launch(id, mode string) error` (mode "" = the main mode). Job `launch`, steps `connect`
  (only when not Linked or not installed) and `open`; the `open` fail message is the reason.
- `App.Integrate(ids, makeDefault)`: job `integrate`, steps `integrate:<id>` with `info` lines.
- `App.IntegrationLog(id, n) []string`, `App.Preflight(ctx, id) error`.
- `endpointFor(cfg)` is the one place an `Endpoint` is built.

## 5. Wire format (HTTP API the three UIs use)

- `GET /api/status` → `targets: [Target]`. Example:

```json
{"id":"opencode","name":"OpenCode","desc":"Coding agent for your terminal","supported":true,"why":"",
 "installed":true,"connected":true,"icon":"opencode","version":"2.2.5","bin":"C:\\Users\\Sam\\.ezlm\\tools\\opencode\\2.2.5\\opencode.exe",
 "source":"ezlm","state":"connected","config":"C:\\Users\\Sam\\.config\\opencode\\opencode.json",
 "launch":[{"id":"tui","label":"Open OpenCode"}],
 "last_check":{"at":"2026-11-05T21:02:26Z","ok":true,"tool_call":true,"secs":14,"stage":"model","msg":"answered and used a tool (14 s)","log_path":"C:\\Users\\Sam\\.ezlm\\logs\\integrate-opencode.log"}}
```

  Decode every field after `connected` as optional (C# nullable, Swift optional); with no `state`,
  fall back to today's installed/connected logic.
- `POST /api/launch {"id","mode"?}`, `POST /api/integrate {"ids","default"}` (unchanged).
- `GET /api/integrate/log?id=<id>` → `{"lines": [...]}` (last 200 lines, keys removed) for "Show details".
- `GET /logos/app-<icon>.png` (web page; embedded `internal/web/static/logos/`).

## 6. UI rules (winui and macweb, same release, same words)

- Logo: `app-<icon>.png` from `Assets/Logos/` (Windows, `Marks.AppLogo(icon, size)`) and `Logos/` (Mac,
  `Logos.app(icon)` with `Bundle … subdirectory: "Logos"`; not the vendor table, so no key clash),
  36 px on tiles, 16 px on launch buttons, 20 px in Overview rows; the dsh whale drawn at 80 % of
  its box, centred. Hidden from accessibility (the name sits next to it). Missing PNG → today's
  pixel wordmark, switched on id (`dsh` gets its own white wordmark, never "hermes").
- Version line, dim: `"<version> · installed by EZLM"` for source `ezlm`, else `"<version> · yours"`; nothing when version is empty.
- State text: `connected` "Connected" (today's check mark may stay); `installed` "Installed, not connected yet"; `not_installed`
  "Not installed: EZLM will install it"; `needs_update` "Points at an old address or key: EZLM fixes
  it when you open it"; `needs_64k`, `needs_tools`: `detail`; `broken`: "Couldn't open last time:
  <detail>" when `last_check.stage == "launch"`, else "The last check failed: <detail>", with
  "Show details" (the log) and "Report a problem". Unsupported: `why`, tile at 50 % opacity.
- Last check line: "Checked <relative time>: <last_check.msg>" when `ok`.
- Launch buttons: one per `launch` entry, `label` as given, first one primary.
- Settings › About carries the attribution line from `Logos/NOTICE.md`.

## 7. CLI (core)

- `ezlm connect <ids…> [--default]` (flags are no longer taken as ids).
- `ezlm doctor apps [ids…] [--json]`: each target's state, version, program, config, last check,
  and its preflight; opens no window, starts no chat. (`ezlm apps` already lists Model mode's
  memory users, so it stays as it is.)

## 8. Environment knobs

| Variable | Effect |
|---|---|
| `EZLM_NO_INTEGRATIONS` | everything off: targets unsupported, Setup/Open refuse, Rewrite no-ops |
| `EZLM_LAUNCH_DRYRUN=1` | write launchers, open no window or browser |
| `EZLM_TOOL_PATH` | `FindTool` searches only these folders (set but empty finds nothing) |
| `EZLM_HOME` | `~/.ezlm` (logs, launch, tools, `integrations.json`) |
| `EZLM_DEMO_APPS` | demo only: starting states, e.g. `opencode=connected,hermes=broken,dsh=needs_tools`; `+fail` makes the next launch fail |
| `HOME`/`USERPROFILE`, `XDG_*`, `OPENCODE_CONFIG_DIR`, `HERMES_HOME`, `DSH_HOME`, `LOCALAPPDATA`, `APPDATA` | redirect the apps' folders (tests and e2e) |

## 9. Tests

- `internal/integrate/main_test.go` points every folder above into one temp dir, sets
  `EZLM_TOOL_PATH=""` and `EZLM_LAUNCH_DRYRUN=1` for the whole package. Put fake programs in
  `t.TempDir()` and `t.Setenv("EZLM_TOOL_PATH", dir)`. Use `Replace` for fake tools and
  `httptest.NewServer(fakellm.New(…))` for the model. Fixtures: keys replaced with `REDACTED-KEY`.
- Anything that runs opencode, hermes, dsh or ezlm by hand uses a temp HOME/XDG/HERMES_HOME/DSH_HOME/
  OPENCODE_CONFIG_DIR/EZLM_HOME; stop only PIDs you started.
- Before handing in: `gofmt -l`, `go build ./... && go vet ./... && go test ./...`, and
  `GOOS=windows GOARCH=amd64|arm64` and `GOOS=linux` builds.

## 10. Demo (`internal/app/demo.go`, `demo_apps.go`)

Starts with OpenCode `needs_update` (2.2.5, ezlm), Hermes `needs_64k` (0.22.5, msix), DeepSeek
Harness `not_installed`. Connect makes any of them `connected`. Opening Hermes while it needs 64K
fails with the 64K sentence; DeepSeek Harness's first launch fails (exit code 1) and the tile
turns `broken`; the next launch works. `EZLM_DEMO_APPS` sets other states for snapshots.

## 11. Status as built

Every workstream's part is real; nothing in section 11 of the skeleton remains.

- **core**: pipelines, state derivation, registry, Run/log/redaction, installer runner, atomic
  writes, cache, check harness, pins (`tools.json`), `FindTool` (PATH, login-shell PATH on
  Mac/Linux, registry PATH and WindowsApps aliases on Windows, `EZLM_TOOL_PATH`), arch detection,
  `serve.FetchProps` and `Config.Props` (fetched after each deploy and by the server gate), and
  the app-layer rules in section 4 (server gate, `MinCtx` and needs_64k, Rewrite before Open, the
  20 s exit watcher, per-app rewrite errors, the reopen note, report attachments). Additions are
  in section 12.
- **launch**: real launchers on Mac (`.command`, `open -a Terminal`), Windows (`.cmd`, wt with a
  `cmd /c start` fallback) and Linux (`.sh`), exit files, dry run, ConPTY on Windows, `Smoke`,
  `StripANSI`, `fakellm`, `cmd/fakellm`, `cmd/tuiprobe` and `scripts/e2e/connect_e2e.sh`.
- **opencode**: probe, history-format scan, the PLAN 4.2 decision table, pinned 2.2.5 install
  into `~/.ezlm/tools/opencode/2.2.5`, hujson config edits, per-level variants, check, preflight,
  launch and TUI smoke. Passed against the real 2.2.5 and 1.19.5.
- **hermes**: finding (incl. MSIX), probe, `providers:` and legacy writers with migration,
  per-model reasoning override, check, preflight, launch, non-interactive install with the
  gateway/login-item guard. Passed against the real 0.22.5.
- **dsh**: finding the user's tested dsh or EZLM's copy, pinned Node 24.22.2 and dsh 0.2.1-rc.5
  installs, byte-preserving profile edits for headless, web and (if present) desktop, `.env` and
  stored-ref keys, headless check, `--dump-config` preflight, `dsh web` launch. Passed against
  the real 0.2.1-rc.5 on Node 22.22.3.
- **winui**, **macweb**: tiles, logos, version and check lines, launch buttons per mode, the
  failure UI with Show details and Report a problem, Overview's connected apps and Settings ›
  About, with the words in section 13.5. The web page got Open buttons for the first time.

## 12. Core additions (additive; nothing above changed)

- `tools.json` (embedded) pins OpenCode 2.2.5 (all GitHub assets incl. linux and musl, with
  sizes), Node 24.22.2 (win-x64, win-arm64, darwin-arm64, darwin-x64, linux-x64, linux-arm64,
  from SHASUMS256.txt) and dsh 0.2.1-rc.5 (`Package`, npm `Integrity`). `PinFor` order: the
  remote catalog's `tools` list (`UseCatalogPins`, applied by the server gate) › `RegisterPin` ›
  `tools.json`. `Pin.Integrity` is new (npm `dist.integrity`). A catalog asset without a
  SHA-256 or an https URL is dropped.
- `LoginEnv(key) string`: a variable as the user's login shell (Mac/Linux, cached 10 min) or the
  registry user environment (Windows) sets it, e.g. a `HERMES_HOME` only in `~/.zshrc`.
  `RefreshToolSearch()` re-reads it after an install. Both do nothing under `EZLM_TOOL_PATH`.
- `SearchDirs(extraDirs...) []string` (for logs), `IsAppAlias(bin) bool` (an MSIX alias in
  WindowsApps: run it through CreateProcess; its header can't be read).
- `(*Run).LeftRunning() []int`: PIDs the last successful `Install` left running (already
  logged with their names); `proc.InstallLeft` underneath. Installers get `WaitDelay`, so a
  leftover that holds stdout can't hang the install.
- `NeedsCtxDetail(name, minCtx) string`: the needs_64k sentence. `Setup` turns `ErrNeeds64K`
  into it.
- `ClearLaunchFailure(id)`: after a good launch, the check from before a failed launch comes
  back (the app layer calls it).
- `WriteFileAtomic` writes through a symlink to its target (the link stays) and retries the
  rename for about 2 s on Windows; `Backup` goes next to the real file.
- `ErrLine` prefers the last line that reports an error and skips progress bars and "see the
  log" lines. Common `ErrorText`s now also cover "Unexpected reasoning effort" (needs_update),
  "below the minimum 64,000" (needs_64k), context overflow, a full disk and permissions.
- `Status.hermes64` (the 64K trade-off) is unchanged.

## 13. As built: what changed from sections 1 to 10

### 13.1 Pins and installs

- `tools.json` is the only source of shipped pins. The tool files no longer call `RegisterPin`
  (OpenCode's own copy dropped the linux-musl assets; dsh's had no `Integrity`).
- dsh install: after `npm install`, EZLM compares the `node_modules/@deepseek-ai/dsh` entry of
  the prefix's `package-lock.json` with the pin's version and `Integrity`; a mismatch removes the
  install and fails ("The DeepSeek Harness download didn't match the version EZLM tested; press
  Connect to try again …"). A missing lock file passes (npm checked the tarball itself).
- dsh declares no `engines`, so its Node floor is 22.19.0 (its strictest dependencies). The
  user's node is reused at or above that; otherwise EZLM's pinned Node 24.22.2.
- OpenCode installs only for this machine's own architecture (x64-baseline without AVX2); Linux
  pins are glibc (musl assets are listed but `AssetKey` has no musl variant).

### 13.2 Launch (3.9, 3.10)

- `LaunchSpec.Env` values may refer to the terminal's own variable as `${NAME}` (e.g.
  `PATH=<dir>` + `string(os.PathListSeparator)` + `${PATH}`). The launchers write the shell's own
  form; `Smoke` expands it in-process.
- `Launch` and `Smoke` refuse an Env name with KEY, KEYS, APIKEY, TOKEN, SECRET, PASSWORD,
  PASSWD, PASS, CREDENTIAL(S), AUTH or BEARER as an underscore-separated word, and an Env value or
  argument with a word shaped like a key: `ezlm-` plus 16 or more letters and digits, `sk-` plus
  20 or more characters including a digit, or `Bearer <8+>`. Words are split at spaces, quotes,
  `=:;,` and path separators, so a folder such as `ezlm-integrate-test-123` or `sk-learn-projects`
  is not a key. Core's `Open` and Setup's smoke also refuse a spec that contains the endpoint's
  actual key anywhere (`holdsKey`).
- `Bin` set: the launcher runs it (a `URL` is ignored; dsh web opens its own page with its
  one-time token). `Bin == ""` with `URL`: only the URL opens.
- Windows: the exit file holds the code alone (`3\r\n`); `ReadExit` takes the time from the
  file's mtime. Windows Terminal runs `wt -d <launch dir> cmd.exe /k .\<id>.cmd` (the launcher
  cds into the workspace itself, so no user path passes through wt's quoting); the `cmd /c start`
  fallback has the same shape. Everything from the app's start sits in one parenthesised block,
  because cmd.exe rereads a batch file by name. The Windows failure line has no exit code
  ("OpenCode stopped. EZLM shows what went wrong."); Mac and Linux include it.
- Setup creates the workspace before the TUI smoke (a first Connect may come before any Open).
- `fakellm.Options.Delay`; `Request` gained `Auth`, `ToolCall`, `ToolResult`, `Text`. The
  working folder a prompt names is trimmed of quotes and trailing punctuation (dsh writes
  `Working directory: "/x".`). `cmd/fakellm` listens on loopback only and takes `-delay`.

### 13.3 Tools

- **OpenCode**: 2.2.5's TUI rejects `--model`, so a 2.x launch is `opencode --standalone` in the
  workspace and the workspace `opencode.json` picks the model; 1.x gets `--model`. Every command
  and the launch set `PWD` (OpenCode reads its folder from `$PWD`), `opencode run` gets no stdin,
  and everything EZLM and the tests run is `--standalone` (an isolated 2.x without it still
  reaches the fixed service port 49374). Preflight: `models --standalone` (2.x), `models ezlm`
  (1.x). Ready marker: `EZLM (local)`. One `variants` entry per accepted level, none with `Levels`
  empty. Probe facts: `standalone`, `tui_model`, `variant` (not `data`). `opencode reload` after a
  rewrite (PLAN 3.8) is not done.
- **Hermes**: reasoning goes in `agent.reasoning_overrides.<model id>` (marked `# EZLM:`), written
  only when the level Hermes sends by default (medium) is one the template rejects. Probe order
  `config path`, `--version`, `chat --help`, cached by program and environment, redone daily.
  `HERMES_HOME` also counts when only the login shell (or the Windows user environment) sets it
  (`LoginEnv`); EZLM then passes it in `Found.Env` to every hermes it runs. Hermes uses core's
  `FindTool` and `IsAppAlias` (its own registry-PATH and WindowsApps helpers are gone). Check:
  `chat -q <prompt> -Q --provider custom:ezlm -m <id> -t file --ignore-rules --source tool
  --max-turns 6 --format stream-json`. The gateway guard reports a new login item and never
  removes it; leftover installer processes are logged by core.
- **dsh**: EZLM sets both `DSH_TELEMETRY_MODE=DISABLED` and `DSH_TELEMETRY_DISABLED=1`. A user's
  dsh is used only when its version is in the pin's tested list; EZLM's copy counts only once its
  install marker exists. With levels: `reasoningEfforts` lists them plus a valueless `off:` (the
  default request then carries none); without: `reasoningEfforts: false`,
  `supportsReasoningEffort: false`. A server without a key gets the placeholder `no-key`. The
  desktop profile is checked by reading the file (dsh won't dump it). `dsh web` uses port 3080
  when free; a running one is recognised by its 401 "dsh web authentication required" and
  EZLM's `tools/dsh/web.json`. Only the `web` mode exists; `Info.MinCtx` is 0. Setup's call to
  `LaunchSpec` (to look for ready markers) probes ports and writes `web.json` for dsh.

### 13.4 App layer and engine

- Launch-job failures (the `open`/`connect` step message, the `done` message and the saved
  launch check) start with a capital letter and end with a period.
- needs_tools reads the same for all three apps: "This model can't use tools, so <Name> can't
  edit files with it. Pick a model marked Tools, or use Chat."
- Demo: the broken detail ends with a period; the demo log uses the real Setup lines and is
  empty for an app that was never set up.
- Wire: `last_check.at` is Go's RFC 3339 with the local offset and up to nine fraction digits
  (e.g. `2026-11-05T23:18:22.836563-06:00`). Windows, the Mac and the web page decode it
  (tests in `SelfTest.cs` and `TargetTests.testDecodesTheEngine`).

### 13.5 UI words (same in the Windows app, the Mac app and the web page)

- Version line: on its own dim row under the logo and name.
- needs_64k tile: "Connect again" (connects that app again), or "Pick a smaller model" (Models;
  on the web page the model picker) when Hermes's 64K can't fit. The same button comes first
  after a failed launch of a needs_64k app.
- needs_tools tiles, and Hermes when 64K can't fit, can't be picked (60 % opacity);
  unsupported tiles 50 %.
- broken with no detail: "Couldn't open last time." or "The last check failed."
- Relative time (the chat list's words): "just now", "N min ago", "N h ago", "yesterday", then
  "Nov 1".
- Show details: title "<Name>: connection log", subtitle "What EZLM did and what <Name> said,
  newest last. Keys are never written here.", buttons Copy, Report a problem, Close.
- Report a problem from an app: "<Name>: <reason>", a blank line, "<Name> log (last N lines):"
  and up to 40 lines, keys shortened.
- Overview: "Connected to" followed by each connected app's 20 px logo and name, or "Not
  connected to any apps yet."
- Launch buttons: an app's first mode in the normal style (sends mode ""), extra modes small
  (send their id). Like Connect, they are enabled only while the server is live in all three
  UIs, so the server gate (section 4) is reached from the CLI or when the server stops mid-way.
- Settings › About carries the attribution line (the web page: an About section at the bottom).

### 13.6 Tests and e2e

- `TestRunLogsRedacted` clears its log, so `go test -count=N` passes.
- `TestDshReal` uses the shared `fakellm`.
- `scripts/e2e/connect_e2e.sh`: a step file that defines `e2e_<name>()` is sourced; any other
  step file runs as its own script with `E2E_ROOT`, `EZLM`, `REAL_HOME` and `GO` set, and Go's
  cache and module folders kept, since go is not on the run's PATH. `opencode.sh` takes
  `E2E_OPENCODE_BINS` (default: the opencode on PATH), `dsh.sh` defaults `DSH_BIN_DIR` to the
  dsh on PATH. The guard no longer hashes `~/.hermes/auth.json` (a running Hermes rewrites it).
