# T14 a08 report: LocalAI

**Product:** LocalAI (mudler/LocalAI), version 4.12.1 (2026-10-23).
**Output:** `agents/T14/products/a08.md`.
**Method:** desk study only. No product installed, no container run, no model loaded. Docs read with
`web_fetch`, GitHub data through the read-only REST/search API (`api.github.com`). All access dates
**2026-11-06**.

## What I read

Primary sources (all accessed 2026-11-06):

- Quickstart - https://localai.io/docs/basics/getting_started/
- Install methods - https://localai.io/docs/installation/
- Containers, volumes, upgrade warning - https://localai.io/docs/installation/containers/
- macOS launcher - https://localai.io/docs/getting-started/macos/
- Setting up models, gallery fit rule, import preview gap -
  https://localai.io/docs/getting-started/models/
- GPU acceleration, auto backend detection, ROCm/SYCL/Vulkan limits -
  https://localai.io/docs/features/gpu-acceleration/
- Authentication and keys - https://localai.io/docs/features/authentication/
- Integrations (OpenCode, Claude Code, Crush) - https://localai.io/docs/integrations/
- README - https://github.com/mudler/LocalAI#readme
- Releases 4.9.0, 4.10.0, 4.11.0, 4.12.0, 4.12.1 - https://github.com/mudler/LocalAI/releases
  (release bodies via `api.github.com/repos/mudler/LocalAI/releases`)
- Issues: #2368, #1592, #6268, #9540, #10485, #11022, #12058, #12081 (incl. all 6 comments),
  #12807, #12893, #11995, #11814 - https://github.com/mudler/LocalAI/issues/<n>
- EZLM code for comparison: `internal/fit/fit.go`, `internal/keys/keys.go`,
  `internal/serve/config.go`, `internal/scan/`, `BETA.md` (read-only).

## How the complaint counts were made

`GET https://api.github.com/search/issues?q=repo:mudler/LocalAI+is:issue+<term>&sort=reactions&order=desc`
and `GET /repos/mudler/LocalAI/issues/<n>`. `rN` is the issue's `reactions.total_count`, `cM` its
`comments`; both are per issue, not summed across duplicates. Repo state at that moment: 50,229
stars, 181 open issues+PRs, 103 open issues matching `is:issue is:open` (search API). GitHub only;
Reddit/Discord not counted, which likely under-samples non-developer users.

## Findings that mattered

1. No native Windows build, and it is the most-reacted open issue (#2368, r16/c7, open since
   2024-05-22). The next most-reacted open issue has 7 reactions. Windows = Docker Desktop/WSL2.
2. Fit is advisory, not enforced. Gallery recommendation uses "95% of the detected budget at 4096
   context", but the load path's hardware auto-tuning emits `n_gpu_layers=99999999`, `context=8192`
   (log quoted in #12081). That is "offload everything", not "offload what fits", and it is the
   mechanism behind the OOM and "recommended model will not run" reports.
3. Auth off by default, `-p 8080:8080` publishes on all interfaces, legacy keys grant full admin.
   4.9.0 had to make auth deny-by-default; 4.12.1 was a security-only SSRF fix.
4. Upgrade trap: container recreate loses anything outside four volumes, and a mounted `/backends`
   keeps a stale engine (`core/gallery/backends.go:197-210`, per the maintainer bot in #12081). The
   reporter recovered only by deleting all persistent state.
5. Docs drift: macOS page says signed+notarized; README says not signed and gives the `xattr`
   workaround (#6268). README news stops at 4.11.0 while 4.12.1 is out.
6. The backend-per-model-OCI design is genuinely good and worth copying in spirit: install only the
   engine the model needs, detect the vendor and pull the right image. EZLM's single bundled engine
   is simpler but cannot serve vLLM/MLX/whisper-class breadth later.

## What I could not verify

- Real-world first-run time and failure rate on Windows/WSL2 (no install allowed; desk study).
- Whether the "hardware auto-tuning" always emits `n_gpu_layers=99999999` or clamps per model; I
  have one log line from one model (#12081) and no source read of the tuning function.
- Current state of the macOS signing claim: the docs page (23 Aug 2026) and the README (undated,
  but citing #6268 from Sep 2025) disagree; I did not download the DMG to check with `codesign`.
- Whether a Qualcomm Hexagon/NPU backend exists outside the docs list. The backend list shows RKLLM
  (Rockchip) only; absence in docs is not proof of absence in the repo.
- Discord/Reddit complaint volume (no counts obtainable without an account).

## Open questions

- Does LocalAI's fit check ever block an install, or is it purely a UI filter? The docs describe
  only the recommendation list being hidden.
- Should EZLM adopt per-model engine images (LocalAI's model) when it adds a second backend, or keep
  one engine? That is a T14-level architecture question, not mine to settle.
- Is "no Docker" worth a dedicated onboarding screen in EZLM, or just site copy? I recommend the
  latter (R5) because the installer already carries the claim.

## Deliverables and state

- `agents/T14/products/a08.md` - full section in the brief's shape, 5 recommendations.
- No code changes. `gofmt -l .` and `go build ./...` unaffected; run as sanity before the final
  commit.
- Commit: `17f3b51` (docs only, `agent/T14-a08`).
