# T10 severity table (agent a16)

Snapshot: baseline commit `5213370` (EZLM source snapshot 2026-11-06), branch `agent/T10-a16`.
Scope: `internal/proc/**`, `internal/awake/**`, and the service parts of `internal/serve/**`
(`service_*.go`, `procs.go`, `birth_*.go`, `hide_*.go`, `run.go`, `client.go` lifecycle paths,
`config.go` port/stop paths). Auth, LAN and key code in `internal/serve` is T07's and is only
touched where a lifecycle finding crosses it.

Method: every row below was read in this worktree at the snapshot commit. Rows marked
**code-read** were not executed here; **Windows-only** rows can only be code-read on this Mac
(the Windows build is compiled, not run). `go test -p 2 ./internal/proc/ ./internal/awake/
./internal/serve/` passes at this commit (0.83 s, 0.49 s, 5.33 s), so none of these findings is
already caught by an existing test.

Severity: **blocker** = data loss, security, or EZLM stops working for many users; **major** =
breaks a stated T10 goal (survives reboot/sleep/port conflict, no orphans, never kills the wrong
process) on a realistic machine; **minor** = edge case, stale message, or test-quality issue.

Status: **open** = no guard in this snapshot; **mitigated** = a guard exists but does not remove
the failure, or removes it only on one platform/path; **fixed** = the snapshot already contains a
guard and a regression test or a clearly correct check. a16 changed no product code, so "fixed"
rows are fixes that landed before this branch; they are listed so the merge does not regress them.

Platform column: `all`, `mac`, `linux`, `win`. Windows-only rows are marked `win (code-read)`.

## A. Process tree: internal/proc

| ID | Finding | Severity | Status | Evidence (file:line) | Fix / next step |
|---|---|---|---|---|---|
| P1 | Unix stop kills the whole process group by pgid (`kill(-pid)`) without checking that the group still belongs to the process EZLM started. After the leader exits and is reaped, the kernel may reuse the pgid, so a later stop or the deferred `close()` can kill an unrelated group. This is the "never kill the wrong process" invariant. | major | open | `internal/proc/proc_unix.go:25-27` (`kill`/`close` = `syscall.Kill(-t.pid, ...)`); deferred call after `Wait` at `internal/proc/proc.go:66-75` | Record the leader's start time (and pgid) when tracking; before `Kill(-pgid)`, re-read `/proc/<pid>/stat` (Linux) or `syscall.Getpgid(pid)` plus start time and skip if it changed. On macOS, fall back to killing only the tracked PIDs from `pids()`. |
| P2 | Windows containment is assigned after the process has started. A grandchild spawned between `cmd.Start()` and `AssignProcessToJobObject` is outside the job and survives cancel. The `AssignProcessToJobObject` error is ignored (line 33), and `OpenProcess` failure silently returns an empty job, so `kill()`/`close()` then kill nothing and the tree leaks. | major | open | `internal/proc/proc_windows.go:24-37`, `39-43`; `prepare` at `16-22` does not set `CREATE_SUSPENDED` | Create the process suspended (`CREATE_SUSPENDED`), create/assign the job, then `ResumeThread`; check both `OpenProcess` and `AssignProcessToJobObject` and fall back to enumerating and terminating the recorded PIDs by PID (never by name). |
| P3 | `pids()` hard-codes the `JOBOBJECT_BASIC_PROCESS_ID_LIST` layout for 64-bit Windows (count at byte offset 4, ids starting at `buf[1]`), and caps the list at 256 ids with no indication of truncation. A 32-bit build (or a job with more than 256 processes) reports wrong or missing PIDs. | minor | open | `internal/proc/proc_windows.go:66-83` | Use the documented struct (`NumberOfAssignedProcesses`, `NumberOfProcessIdsInList`, then `ULONG_PTR` ids) or retry with the size the API returns; treat `n == len(buf)-1` as truncated. |
| P4 | On macOS a helper tree does not die when EZLM is killed hard: `setPdeathsig` is a no-op outside Linux and `Setpgid` alone only makes the group killable while EZLM is alive to call it. An installer or app check killed with SIGKILL can leave its children running. | minor | mitigated | `internal/proc/proc_unix.go:18-20`; `internal/proc/pdeath_other.go:5`; `internal/proc/pdeath_linux.go:5` | Keep the current Linux behaviour; on macOS either accept it (documented) or use a kqueue `EVFILT_PROC` watcher. `InstallLeft` already reports leftovers so callers can log them. |
| P5 | `waitDelay` is a package-level variable mutated by a test. Tests in this package that run in parallel (or any future `t.Parallel`) race on it. | minor | open | `internal/proc/proc.go:47-50`; `internal/proc/leftover_unix_test.go:18-20` | Make it a field on an options struct or pass it through `runTree`; keep the default 5 s. |
| P6 | `Output`/`Install` with `killLeftovers=true` kills the command's whole group after a *successful* exit. A command that deliberately daemonizes into the same group (a wrapper that `exec`s a server) is killed even though it exited 0. This is intended for installers but is surprising for `Output`. | minor | mitigated | `internal/proc/proc.go:52-54`, `66-75`; `internal/proc/proc_unix.go:27` | Document it on `Output`, or add an `OutputKeep` variant; combine with the P1 ownership check. |
| P7 | On Windows the job object is closed (`close()`) or has its kill-on-close flag cleared (`release()`) but a handle leak is possible when `track` creates a job and `OpenProcess`/assign fails: the empty job handle is still closed later, which is fine, but the process was never in it, so `close()` kills nothing. | minor | open | `internal/proc/proc_windows.go:24-37`, `45-63` | Same fix as P2; log or flag when containment is not established. |

## B. Sleep/wake: internal/awake

| ID | Finding | Severity | Status | Evidence (file:line) | Fix / next step |
|---|---|---|---|---|---|
| A1 | Linux hold is silent when it does not hold. If `systemd-inhibit` exists but there is no user session, the command exits immediately; if `tail` is busybox (no `--pid`), `tail` exits immediately. `hold` only checks `cmd.Start()`, so EZLM believes the machine is kept awake while the inhibitor is gone. | minor | open | `internal/awake/awake_linux.go:11-24` | After start, wait ~200 ms and check `cmd.ProcessState`/`done`; if it exited, return a no-op (and optionally fall back to `caffeinate`-style polling). Log once when the hold is not effective. |
| A2 | The Linux inhibitor is tied to `tail --pid=<EZLM pid>`. If EZLM is SIGKILLed, `tail` notices on its polling interval and the inhibitor ends, which is correct, but a container without `systemd-inhibit` or with a read-only `/dev/null` silently holds nothing. | minor | mitigated | `internal/awake/awake_linux.go:12-23` | Same detection as A1; keep the "does nothing where that isn't possible" contract in the doc comment. |
| A3 | Windows `reasonContext` is a hand-rolled struct whose trailing `[2]uintptr` padding stands in for the larger union arm. If the Windows SDK layout or pointer size changes, `PowerCreateRequest` reads past/garbage; the code also ignores the `SetInformation`/error detail when `PowerCreateRequest` is not found. | minor | open | `internal/awake/awake_windows.go:16-46` | Declare the full union (`SimpleReasonString` and the detailed arm) as a Go struct with a `uintptr`-sized field, or use the `x/sys/windows` definition if one exists. Keep the silent no-op only for "API missing". |
| A4 | Windows test matches the reason string anywhere in `powercfg /requests` output, so another program using the same reason makes the test pass even if EZLM's request is absent. | minor | open | `internal/awake/awake_test.go:62-83` | Match the PID or a unique per-test reason (`fmt.Sprintf("ezlm-awake-test-%d", os.Getpid())`). |
| A5 | The macOS test's `ours()` runs `pgrep -P` and `pmset` on every poll (30 × 100 ms), which is fine, but `pgrep` matches by name (`caffeinate`). This is test-only and does not violate the "never kill by name" rule because nothing is killed. | minor | mitigated | `internal/awake/awake_test.go:19-31` | No change needed; note that the production code starts `/usr/bin/caffeinate` by absolute path. |

## C. Service registration

### macOS (launchd)

| ID | Finding | Severity | Status | Evidence (file:line) | Fix / next step |
|---|---|---|---|---|---|
| D1 | `Install` destroys the old job before the new one is known-good: it calls `Uninstall()` (bootout + remove plist), then writes the plist, then bootstraps. A write or bootstrap failure leaves no autostart and no running server, with no rollback. | major | open | `internal/serve/service_darwin.go:80-86`, `107-114` | Write a temp plist, validate it (`plutil -lint`), then bootout and atomically rename it into place; bootstrap and restore the previous plist on failure. |
| D2 | `bootout` gives up after 10 s even if launchd still has the job loaded, and `bootstrap` then retries three times. If all fail, the error text is only the last launchctl output, and the plist has already been removed by D1. | minor | open | `internal/serve/service_darwin.go:25-45` | Return a wrapped error that names the label and plist path; retry with a longer backoff; do not remove the plist until bootstrap succeeds. |
| D3 | `Start` rewrites the plist from config, but if `Load` fails it bootstraps whatever plist is on disk, which may point at a server binary that has moved (an update). launchd then fails to start it with no visible message. | minor | open | `internal/serve/service_darwin.go:98-105` | Check `os.Stat(c.Server)` in `Start`/`Install`; on failure return a message naming the missing binary. |
| D4 | `Install` truncates the log with `os.Truncate` and ignores the error; when the log does not exist yet, launchd creates it, so this is harmless, but a permission error is hidden. | minor | open | `internal/serve/service_darwin.go:85` | `os.MkdirAll(filepath.Dir(LogPath()))` and handle the error, or ignore `IsNotExist` explicitly. |
| D5 | `domain()` uses `gui/<uid>`; from an SSH-only session with no GUI login, launchd may not have the `gui` domain, so install fails. The code surfaces the launchctl error, but the message is not actionable. | minor | open | `internal/serve/service_darwin.go:21`, `25-45` | Wrap with "log in to the desktop once and retry" when the `gui` domain is unavailable. |

### Linux (systemd user unit; code-read on this Mac, `GOOS=linux` build checked)

| ID | Finding | Severity | Status | Evidence (file:line) | Fix / next step |
|---|---|---|---|---|---|
| L1 | `quote` escapes `\`, `"` and `%` but not `$`. systemd expands `$VAR`/`${VAR}` in `ExecStart` arguments even inside double quotes, so a model path or extra arg containing `$` is corrupted. | major | open | `internal/serve/service_linux.go:25-27`, `49-53` | Escape `$` as `$$` in `quote` (and add a unit test with `$HOME`, `%i`, spaces and a quote in one path). |
| L2 | `Environment=LD_LIBRARY_PATH=<libPath>` and `StandardOutput=append:<LogPath>` are interpolated unquoted and unescaped. A home or install folder with a space splits the environment assignment (systemd's `Environment=` is whitespace-separated); a `%` in the path is treated as a specifier. The engine then cannot find the CUDA libraries or the log. | major | open | `internal/serve/service_linux.go:58-68` | Apply `quote` to the environment value and to the log paths, and escape `%` as `%%` in every unit value. Add a test that builds a unit for `/home/John Smith/.ezlm` and parses it. |
| L3 | `libPath` walks up while the *parent's* base is not `bin`. For the standard layout `~/.ezlm/bin/llama-<tag>/llama-server` it stops at the tag dir (bounded walk), but for a server directly under `bin` (or any path not two levels below a `bin` dir) the loop climbs to `/` and `WalkDir("/")` walks the whole filesystem on every Install/Start. | minor | mitigated | `internal/serve/service_linux.go:31-46`; standard layout at `internal/engine/engine.go:210`, `305-315` | Stop at the first ancestor whose own base is `bin`, or at `paths.Home()`; also check the `WalkDir` error. |
| L4 | `libPath` includes every directory under the engine tree that contains any `*.so`-named file, without deduplication order guarantees beyond WalkDir's lexical order, and it never checks that the resulting `LD_LIBRARY_PATH` entries are colon-safe. A path containing `:` breaks the variable. | minor | open | `internal/serve/service_linux.go:36-45` | Keep the walk; document the `:` limitation or reject such install paths with a clear message. |
| L5 | `Stop` returns `systemctl stop`'s error when the unit is not loaded ("Unit ezlm.service not loaded"), so a no-op stop is reported as a failure to callers. | minor | open | `internal/serve/service_linux.go:86`, `17-23` | Treat "not loaded"/"not found" as success. |
| L6 | `Uninstall` ignores every `systemctl disable --now` and `daemon-reload` error and always returns nil, so it can report success while the unit is still enabled. | minor | open | `internal/serve/service_linux.go:98-103` | Collect and join errors; still remove the file. |
| L7 | `Install` leaves a half-written unit on disk if `daemon-reload` fails, and `os.MkdirAll`/`os.Truncate` errors are ignored. | minor | open | `internal/serve/service_linux.go:70-84` | Handle `MkdirAll`; write the unit to a temp file and rename; remove it on `daemon-reload` failure. |

### Windows (code-read; Windows build compiles)

| ID | Finding | Severity | Status | Evidence (file:line) | Fix / next step |
|---|---|---|---|---|---|
| W1 | `Start` returns success as soon as *any* `OwnPIDs` entry exists, without checking `/health` or that the port answers. A wedged or half-started leftover therefore blocks every later Start, and the user has a dead server with no error. | major | win (code-read) | `internal/serve/service_windows.go:159-162` | If `OwnPIDs > 0` and `!Healthy(ctx, c)`, call `StopAndWait` and start again; surface `ErrStillRunning` if the old one will not die. |
| W2 | Start is check-then-act: `OwnPIDs` then spawn, with no lock or pid file. The app's Start and the Run-key `--autostart` entry (or two app instances) can both pass the check and spawn two llama-servers; the second `startLogged` truncates the log the first is writing. | major | win (code-read) | `internal/serve/service_windows.go:159-168`, `177-203`; `internal/serve/procs.go:13-30` | Take a named mutex (or an exclusive pid file) around check+spawn; a second starter should wait briefly and then re-check health. |
| W3 | `startLogged` first tries `CREATE_BREAKAWAY_FROM_JOB`, then silently falls back without it. If EZLM itself is in a kill-on-close job (some terminals, CI, or a parent installer), the server dies with the app even though Start returned success. | minor | win (code-read) | `internal/serve/service_windows.go:186-201` | Record/flag when the fallback is used; retry with a fresh job-detach or report that the server will not outlive the app. |
| W4 | `Stop` always returns nil, even if `killOwn` failed for every PID (access denied on an elevated server). Direct callers cannot distinguish "stopped" from "still running"; only `StopAndWait` notices. | minor | win (code-read) | `internal/serve/service_windows.go:139-154` | Collect `KillWithContext` errors and return them joined; keep `StopAndWait` as the waiter. |
| W5 | `killOwn` stops every `llama-server*` process whose exe is under `paths.Home()`, including one the user started by hand from EZLM's folder. The path filter prevents killing *other apps'* servers, so this is a documented trade-off, not a name-only kill. | minor | mitigated | `internal/serve/procs.go:13-41`; `internal/serve/service_windows.go:147-154` | Keep; document in the Stop comment that a user-started server from EZLM's own folder is stopped too. |
| W6 | The `Start` comment says it "runs the task now", but the code no longer uses Task Scheduler: it spawns llama-server directly and the login entry is the Run key. `taskXMLPath`/`activeTaskPath` are legacy leftovers. | minor | win (code-read) | `internal/serve/service_windows.go:37-38`, `55`, `156-168`, `205-216` | Update the comments; remove the dead XML path once older installs have migrated. |
| W7 | `EnsureAutostart` ignores the `setRunKey()` error, so a failed repair is silent and `NoLoginTask` stays false. The next boot has no server and no explanation. | minor | win (code-read) | `internal/serve/service_windows.go:107-125` | Set `NoLoginTask = setRunKey() != nil` here, as `Install` does at line 77. |
| W8 | Legacy task cleanup acts on fixed names (`EZLM Server`, `EZLM Server (<user>)`). Another EZLM install for the same user (a beta beside a stable build) would have its login task disabled/removed. `testInstance()` only guards EZLM_HOME/EZLM_NO_AUTOSTART. | minor | win (code-read) | `internal/serve/service_windows.go:16-29`, `69-75`, `122-124` | Keep the guard; if two stable installs become supported, write an install id into the task name. |
| W9 | `hidden()` replaces `SysProcAttr` wholesale; harmless for its own `schtasks` calls, but any future caller that sets attributes first loses them. | minor | win (code-read) | `internal/serve/service_windows.go:44-47` | Mutate the existing struct instead of replacing it. |

## D. Orphans, ownership and ports (internal/serve lifecycle)

| ID | Finding | Severity | Status | Evidence (file:line) | Fix / next step |
|---|---|---|---|---|---|
| O1 | `OwnPIDs` compares the *resolved* executable path from gopsutil against the *unresolved* `paths.Home()`. If `~/.ezlm` (or `EZLM_HOME`) is reached through a symlink, EZLM's own server is not recognized: Windows `killOwn` misses it, `StopAndWait` can time out, and `WaitHealthy`'s "stopped" heuristic never fires. | major | open at baseline; fixed on `agent/T10-a14` (`95ad68a`, `realPath`) and again on `agent/T10-a10` (`d661b23`, `ownHomes`) | `internal/serve/procs.go:25`, `35-41`; `paths.Home()` at `internal/paths/paths.go:8-17` | Merge one of the two sibling fixes (they touch the same lines); `filepath.EvalSymlinks` both sides before comparing. Add a test with a temp dir and a symlinked home. |
| O2 | Ownership needs both a name prefix (`llama-server`) and a successful `ExeWithContext`. An elevated server (Windows: access denied) or a renamed binary is invisible to `OwnPIDs`; the name prefix alone is never used to kill, so this is a miss, not a wrong kill. | minor | mitigated | `internal/serve/procs.go:20-28` | Keep the double check; on Windows, fall back to a per-PID open with `PROCESS_QUERY_LIMITED_INFORMATION` if the full query fails. |
| O3 | `OwnPIDs` enumerates every process on the machine on every call. `WaitHealthy` calls it each second after the 8 s grace and `StopAndWait` every 250 ms, so a slow process table delays recovery and the UI. | minor | open | `internal/serve/client.go:90-99`, `201-213`; `internal/serve/procs.go:14-30` | Cache the scan for ~1 s, or have StopAndWait use the known PID set plus a single re-scan. |
| O4 | `OwnRSS` opens each PID by number and sums RSS. A PID reused between the scan and the read attributes another process's memory to EZLM (display only). | minor | open | `internal/serve/procs.go:43-53` | Check the process start time against the scan, or accept and document. |
| R1 | `FreePort` returns `start` when all 100 candidates are busy, so the caller deploys onto a busy port and the failure appears later as llama-server's "address already in use". | major | open | `internal/serve/config.go:173-181`; callers at `internal/app/app.go:796-797`, `1431-1439` | Return `(port, ok)` or 0, and surface a plain "no free port in 8080-8179" message before deploying. |
| R2 | `PortFree` is a check, not a reservation. Another program can bind between `PortFree` and llama-server's bind; there is no retry-on-bind-failure path in the service layer. | major | open | `internal/serve/config.go:140-159`; retry only on the pre-check at `internal/app/app.go:796` | After a bind failure in the log, pick the next free port, rewrite config, and restart once; or hold a listener until the server binds (not possible with llama-server). |
| R3 | `RunForeground` kills only the direct child on SIGTERM (not the tree), and leaves the signal goroutine blocked on `sig` after a normal exit. On Windows the scheduled-task entry point receives no SIGTERM anyway. | minor | open | `internal/serve/run.go:23-35` | Use `signal.NotifyContext` and `proc`-style tree kill; stop the notify when `Wait` returns. |
| R4 | `WaitHealthy` treats a foreign listener on the port as "still loading": if EZLM's server died and another program holds the port, `listening` stays true, so the wait runs the full 10 minutes instead of reporting `ErrStopped`. | minor | open | `internal/serve/client.go:83-99` | After the grace period, treat `OwnPIDs == 0 && !Ours(ctx, c)` as gone even when the port is occupied. |
| R5 | `StopAndWait` waits up to 10 s but its success condition requires `PortFree` *or* `!Ours`; if the port is held by a foreign program and EZLM's server is still alive but invisible to `OwnPIDs` (O1), it returns `ErrStillRunning` with a Task Manager message on macOS/Linux too. | minor | open | `internal/serve/config.go:198-213` | Fix O1 first; make the message platform-specific. |

## E. Already fixed in the snapshot (keep these; regression guards)

| ID | Fix | Evidence | Guard |
|---|---|---|---|
| F1 | A test instance (own `EZLM_HOME`) gets its own launchd label / systemd unit name, so it cannot stop the real install's server. | `internal/serve/service_name.go:9-19`; `internal/serve/port_test.go:58-69` | `TestTestInstanceServiceName` |
| F2 | On Windows, a test instance never ends/removes the real install's legacy scheduled tasks; the comment records that this once stopped the real server. | `internal/serve/service_windows.go:20-29`, `69-75` | `legacyTasks()` + `testInstance()` |
| F3 | A foreign program on the port is not adopted: `Healthy`/`Ours` require EZLM's key and model, and `StopAndWait` treats a foreign listener as "not ours". | `internal/serve/client.go:26-67`; `internal/serve/config.go:198-213`; `internal/serve/port_test.go:31-56` | `TestOursChecksKeyAndModel` |
| F4 | A server that quits while loading is reported in seconds with the log tail, not after the full timeout; a server that is still loading is waited for even when `OwnPIDs` cannot see it. | `internal/serve/client.go:83-99`; `internal/serve/stopped_test.go:17-66` | `TestWaitHealthyNoticesAStoppedServer`, `TestWaitHealthyKeepsWaitingForALoadingServer` |
| F5 | A port something is listening on is never reported free, including on Windows where a second bind can succeed. | `internal/serve/config.go:140-159`; `internal/serve/port_test.go:14-29` | `TestPortFreeSeesListener` |
| F6 | The Windows server log is handed over as an open handle, so a user profile with spaces and non-ANSI letters still gets a log. | `internal/serve/service_windows.go:170-203`; `internal/serve/startlog_windows_test.go:13-29` | `TestStartLoggedUnicodeFolder` (Windows only) |
| F7 | A Windows process tree is put in a kill-on-close job object, so a cancelled helper does not leave a client talking to the model (the mechanism is right; see P2 for the assignment race). | `internal/proc/proc_windows.go:11-52` | none (code-read) |

## F. Cross-agent reconciliation

Sibling branches that landed while this table was being written, and what they change here. The
statuses in sections A-D are still the baseline status; the merge should apply this table.

| Sibling commit | Touches | Effect on this table |
|---|---|---|
| a01 `8d4f385` launchd plist builder | `service_darwin.go`, new `service_plist.go` | Re-check D1/D4/D5 against the builder; plist content is now testable. |
| a02 `dbcbd81` do not load a job whose program is missing | `service_darwin.go` | Fixes D3 (missing binary on Start). |
| a03 `ff9e89f` drop a stale plist whose program is gone | `service_darwin.go` | Also covers D3; a02 and a03 may overlap. |
| a05 `7a805d1` unicode/moved/redirected Windows profiles | `service_windows.go`, new `service_windows_cmd.go` | Check W6-W8 against the new command builder; W1/W2 are untouched. |
| a07 `019a30e` never adopt another program's port | `client.go` | May fix R4 (foreign listener mistaken for "still loading"); verify. |
| a10 `d661b23` find a crashed engine's server through links | `procs.go` | Fixes O1, but duplicates a14's change to the same function: expect a merge conflict. |
| a11 `95fe790` sleep/wake detection | `awake/*` | Refactors `hold` into `holdCmd`; A1/A2 (silent no-op) are still open unless the new state machine checks the child. |
| a12 `27ebeb9` fake-process helper | new `internal/proc/fakeproc` | The harness a08/a09/a10/a14 need for P1/P2/O1 tests; announce it. |
| a14 `95ad68a` find EZLM's server through linked paths | `procs.go` | Fixes O1 with `realPath`; same lines as a10. Pick one, keep the other's test. |
| a15 `bf289ef` hide/show report | docs only | No lifecycle change. |

No sibling had landed a fix for P1, P2, D1, L1, L2, W1 or W2 at the time of writing. Those remain
the first merge-time priorities.

When the sibling branches land, the merge should:

1. Change P1/P2/P3 status to **fixed** only if a08/a12's tests actually exercise the reused-PID and
   job-assignment cases; a passing test that only checks the happy path is not enough.
2. Add every a01-a05 launchd/Task Scheduler finding that is not already D1-D5/W6-W8, and re-check
   D1/D2 against whatever a03 did to unload/reload.
3. Add a06/a07 port findings; R1/R2 are the lifecycle half and should not be duplicated.
4. Add a08-a10 PID-tree/orphan findings; P1/P2/O1 are the shared root causes.
5. Add a11 awake findings; A1/A2 are the shared root causes.
6. Add a13 Linux unit findings; L1/L2 are the shared root causes.
7. Add a14 registry findings; W2 is the shared root cause.
8. Add a15 hide/show findings if any of them affect stop/start.

## G. Open questions

- Is `EZLM_HOME` ever set in a real (non-test) install? If yes, every production launchd label and
  systemd unit name gains a `-test-<hash>` suffix, and an update that stops setting it leaves the
  old job behind (`service_name.go:13-19`).
- Can `Config.Server` point at a binary directly under a `bin` directory (legacy layout)? That is
  the only realistic path that makes L3 walk `/`.
- On macOS, is a SIGKILLed EZLM with a live helper tree (P4) acceptable, or should the installer
  leftovers be reaped on next start? `InstallLeft` already reports them, so a reaper is possible.
- For W1, should a wedged server be restarted automatically, or should Start return an error and
  let the UI offer "stop and start"? Automatic restart risks killing a server the user started by
  hand from EZLM's folder (W5).
