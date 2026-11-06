# T16 a14: `*_darwin.go` review

Scope: the Go side of the Mac integration outside the frozen areas:
`internal/scan/scan_darwin.go`, `internal/serve/service_darwin.go`,
`internal/serve/birth_darwin.go`, `internal/awake/awake_darwin.go`.
Read `internal/scan/scan.go`, `internal/serve/config.go`, `procs.go`,
`run.go`, `client.go` and the `internal/app` callers for context.
All work is on `agent/T16-a14`.

## Findings

### 1. [major] A user-set `iogpu.wired_limit_mb` can exceed physical RAM
`internal/scan/scan_darwin.go:43` (baseline): the sysctl value was used as-is
(`usable = mb << 20`). `sysctl -w iogpu.wired_limit_mb=32768` on a 16 GB Mac
reports 32768, so the fit engine planned 32 GB of "GPU" memory on a 16 GB
machine and picked a model that could not fit. The shift also overflows for a
very large value. Fixed in `gpuUsable`: a limit above `RAMTotal` is clamped to
`RAMTotal`; the 2/3 and 3/4 defaults are unchanged.
Evidence: `TestPlatformScanClampsWiredLimitToRAM` fails before the fix with
"GPU usable memory 34359738368 is more than the machine's RAM 17179869184".

### 2. [minor] `Install` could leave launchd unable to start the job
`internal/serve/service_darwin.go:81` (baseline): only the plist's folder was
created. The plist's `StandardOutPath` is `~/.ezlm/logs/server.log`, and
launchd fails to spawn a job whose log folder is missing. This is reachable on
a home where the config was written by hand or by an older build. Fixed by
calling `paths.Ensure()` before writing the plist.
Evidence: `TestInstallWritesPlistAndLogFolder` fails before the fix with
"the server log folder was not created".

### 3. [minor] `Install` unloaded the working agent before the new one was written
`internal/serve/service_darwin.go:80-82` (baseline): `Uninstall()` ran first and
the `MkdirAll` error was discarded, so a failure to write the plist left the
user with no login service at all. Fixed by creating the folders first, then
unloading, then writing; the `MkdirAll` error is returned now.

### 4. [minor] `plistPath` ignores `os.UserHomeDir` errors
`internal/serve/service_darwin.go:16`: if `HOME` is unset the path becomes a
relative `Library/LaunchAgents/...`. Low impact: the app always has a home.
Left as is, noted here.

### 5. [minor] `bootout` can give up while the job is still loaded
`internal/serve/service_darwin.go:25`: after 50 polls (10 s) it returns without
reporting failure; the following `bootstrap` then retries three times and
returns only the last launchctl message. A slow shutdown therefore shows a
confusing "Bootstrap failed: 5" instead of "the old server did not stop".
Left as is: changing the error text affects the app's deploy messages, and I
had no way to make launchd slow on this machine.

### 6. [minor, open question] Rosetta is treated as an Intel Mac
`internal/scan/scan_darwin.go:9`: `platformScan` keys off `runtime.GOARCH`, so
an amd64 build on Apple Silicon takes the Intel branch: "Intel Mac detected"
and the CPU backend, although Metal is available to the process. Only matters
if the amd64 build is ever shipped to Apple Silicon; the app ships arm64.
Not fixed.

### 7. [minor] `appleBandwidth` matches future chip names loosely
`internal/scan/scan_darwin.go:96`: generation matching uses `strings.Contains`
over `m5..m1`, so a future "M10" matches "m1", and an unknown chip silently
gets 100 GB/s. Harmless today; noted so it is not forgotten when a new chip
ships. Not fixed.

### 8. [info] Memory-pressure heuristic can overstate free memory
`internal/scan/scan_darwin.go:20-29`: `RAMAvail` becomes
`max(gopsutil available, free percentage * RAMTotal)`. gopsutil's darwin
"available" is `free + inactive`, and `memory_pressure`'s percentage also
counts speculative and purgeable pages, so the max is defensible. On a machine
already under pressure the percentage is still a snapshot; the fit engine's
safety margin covers it. No change.

### 9. [info] Checked and found correct
`birth_darwin.go` (`Stat_t.Birthtimespec`), `awake_darwin.go`
(`caffeinate -i -w <pid>` ends with the process and the kill path cannot leak),
the plist XML escaping of `&`, `<`, `>` and quotes, `KeepAlive` with
`SuccessfulExit=false`, `ThrottleInterval`, the `testSuffix` label isolation,
and `Start`/`Stop`/`Uninstall` behaviour.

## Changes

- `internal/scan/scan_darwin.go`: new `gpuUsable(ramTotal, wiredMB)`, clamps
  the sysctl limit to RAM and keeps the default fractions.
- `internal/serve/service_darwin.go`: `plistContents(c)` extracted from
  `Install`; `Install` ensures the EZLM folders and the LaunchAgents folder,
  checks the `MkdirAll` error, unloads only after the folders are ready, and
  then writes the plist.
- Tests: `internal/scan/a14_darwin_test.go`,
  `internal/serve/a14_darwin_test.go`. They use fake `sysctl`,
  `memory_pressure` and `launchctl` scripts on `PATH` and temp `HOME` /
  `EZLM_HOME`, so they never touch the real launchd or the real home.
  Two files rather than one because they are in two packages; no other agent
  has Go test files, so there is no collision.
- `agents/T16/a14/REPORT.md`: this file.

## Verification

- `go test -p 2 ./internal/scan ./internal/serve`: pass.
- Both new behavioural tests were run with the fix reverted and failed as
  quoted above.
- Full `gofmt -l .`, `go build ./...`, `go vet ./...`, `go test -p 2 ./...`,
  `GOOS=windows GOARCH=amd64 go build ./...`, `GOOS=windows GOARCH=arm64
  go build ./...`, `GOOS=linux go build ./...`: see the final commit.
- Not run: `swift build && swift test`; this slice changes no Swift.

## Next steps

- Decide whether the amd64-on-Apple-Silicon case (finding 6) needs
  `sysctl -n hw.optional.arm64` detection.
- When a new Apple chip ships, update the bandwidth table and tighten the
  generation match (finding 7).
- Give `bootout` a real failure (finding 5) if the deploy messages can absorb
  it.
