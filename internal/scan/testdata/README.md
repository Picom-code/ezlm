# Parser fixtures

The scan parsers read text from other people's machines: `nvidia-smi`,
`rocm-smi`, PowerShell, `sysctl`, `system_profiler`, `/proc`. The fixtures here
are small captured outputs, so a test can check a parser without a GPU, a
Windows PC or a Mac.

## Layout

    testdata/
      <group>/
        <case>.<ext>
        SOURCES.md

`<group>` is the parser family, not the agent that added it: `nvidia`,
`nvidia-xml`, `rocm`, `wmi`, `registry`, `sysctl`, `spdisplays`, `cpuinfo`,
`lscpu`, `locale`, `edge`. Add a directory for a new family. Fixtures belong
in a group; the top level holds only this README and the worked example in
`example/`.

Names are lowercase ASCII with hyphens between words and say which machine or
case the file is: `a100-mig-on.csv`, `rocm-6.1-rx7900.txt`,
`win11-de-rtx-3060.txt`.

| extension | contents |
|---|---|
| `.txt` | plain command output |
| `.csv` | `nvidia-smi --query-gpu=... --format=csv` output |
| `.json` | `rocm-smi --json` or other structured output |
| `.xml` | `nvidia-smi --xml` output |
| `.log` | captured console output, including errors |
| `.reg` | exported registry values |

Any other extension is ignored by the loader and rejected by the corpus test.

## Format

- **Exact bytes.** The loader returns the file unchanged, so keep what the tool
  wrote: CRLF, `[N/A]`, decimal commas, a BOM and odd spacing are all part of
  the case. When a file keeps a non-UTF-8 encoding or a BOM, say so in
  `SOURCES.md`.
- **One command, one file.** No concatenated outputs, no comment lines, no
  invented headers: the parsers read the raw text.
- **Small.** A few KB. Anything over 64 KB fails the corpus test.
- **Committed.** No download at test time, no binary, no GPU dump.
- **Anonymised.** Replace real user names, host names and serial numbers with
  neutral values. Never a key or token; test keys look like `REDACTED-KEY`.

## Provenance

Every group has a `SOURCES.md` with one entry per fixture:

```markdown
- `a100-mig-on.csv`: GitHub issue #1234, https://github.com/... (accessed
  2026-11-06). `nvidia-smi --query-gpu=name,memory.total,memory.free,driver_version`
  on an A100 with MIG enabled.
- `rocm-6.1-rx7900.txt`: synthetic, built from https://rocm.docs.amd.com/...
  (accessed 2026-11-06). Two cards, one iGPU without VRAM fields.
```

A public source (GitHub issue, vendor doc, forum post, gist) with its URL and
the date it was read, or "synthetic" plus the format document it was built
from. Name the file exactly as it appears on disk. `TestFixtureCorpus` fails
when a fixture is empty, over 64 KB, has an unknown extension, or is not named
in its group's `SOURCES.md`.

## Loading a fixture

```go
func TestROCmJSON(t *testing.T) {
	out := loadFixture(t, "rocm/rocm-6.1-rx7900.json")
	var cards map[string]map[string]string
	if err := json.Unmarshal([]byte(out), &cards); err != nil {
		t.Fatal(err)
	}
	// check the parsed cards
}
```

`loadFixture` returns the file as a string, `loadFixtureBytes` the raw bytes
when a BOM or CRLF matters, and `listFixtures(t, "rocm")` the group's fixture
names in sorted order for table tests. A missing fixture fails the test and
lists the names that do exist. Fixtures are read-only; a test that writes
uses `t.TempDir()`.
