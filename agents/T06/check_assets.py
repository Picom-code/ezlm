#!/usr/bin/env python3
"""Print the llama.cpp asset EZLM would pick for each platform, and verify it.

Given one or more llama.cpp release tags, this mirrors the asset selection in
internal/engine/engine.go (assetPatterns, matches, PickAssets) and prints the
archive EZLM would download for every OS/arch/backend combination it can
produce. Each pick is then checked against the release's asset list and, with
--head, against the download URL.

Standard library only. GET and HEAD only; nothing is downloaded.

Examples:
    python3 agents/T06/check_assets.py b11960 latest
    python3 agents/T06/check_assets.py --head b12144
    python3 agents/T06/check_assets.py --json latest > /tmp/assets.json

Exit status is 0 when every platform has a pick that exists in the release,
1 when a pick is missing or a companion runtime is absent, and 2 on a fetch
failure.
"""

from __future__ import annotations

import argparse
import html
import json
import os
import re
import sys
import urllib.error
import urllib.parse
import urllib.request

REPO = "ggml-org/llama.cpp"
API = "https://api.github.com/repos/" + REPO + "/releases"
DOWNLOAD = "https://github.com/" + REPO + "/releases/download"
WEB = "https://github.com/" + REPO + "/releases"
UA = "ezlm-check-assets"

# (goos, arch, backend, cuda13, note) rows mirror what internal/scan and
# Install can produce. darwin ignores the backend: it always installs the
# macos build. arm64 only has CUDA 13 builds, so cuda13 is forced there.
PLATFORMS = [
    ("darwin", "arm64", "metal", False, "Apple Silicon"),
    ("darwin", "amd64", "cpu", False, "Intel Mac"),
    ("linux", "amd64", "cuda", False, "CUDA 12.4+ driver"),
    ("linux", "amd64", "cuda", True, "Blackwell / CUDA 13"),
    ("linux", "amd64", "rocm", False, "AMD: Vulkan build"),
    ("linux", "amd64", "vulkan", False, ""),
    ("linux", "amd64", "cpu", False, ""),
    ("linux", "arm64", "cuda", True, "arm64 only has CUDA 13"),
    ("linux", "arm64", "vulkan", False, ""),
    ("linux", "arm64", "cpu", False, ""),
    ("windows", "amd64", "cuda", False, "CUDA 12.4+ driver"),
    ("windows", "amd64", "cuda", True, "Blackwell / CUDA 13"),
    ("windows", "amd64", "rocm", False, "AMD: Vulkan build"),
    ("windows", "amd64", "vulkan", False, ""),
    ("windows", "amd64", "cpu", False, ""),
    ("windows", "arm64", "cuda", True, "arm64 only has CUDA 13"),
    ("windows", "arm64", "vulkan", False, ""),
    ("windows", "arm64", "cpu", False, ""),
]


class FetchError(Exception):
    pass


def http_get(url, method="GET", timeout=30):
    req = urllib.request.Request(
        url,
        headers={"User-Agent": UA, "Accept": "application/vnd.github+json"},
        method=method,
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return resp.read(), resp.status, resp.geturl()
    except urllib.error.HTTPError as err:
        raise FetchError("%s %s: HTTP %d" % (method, url, err.code))
    except (urllib.error.URLError, OSError) as err:
        raise FetchError("%s %s: %s" % (method, url, err))


def api_release(tag, timeout):
    body, _, _ = http_get(API + "/tags/" + urllib.parse.quote(tag), timeout=timeout)
    data = json.loads(body)
    assets = {}
    for a in data.get("assets") or []:
        name = a.get("name") or ""
        if not name:
            continue
        assets[name] = {
            "name": name,
            "url": a.get("browser_download_url")
            or DOWNLOAD + "/" + tag + "/" + urllib.parse.quote(name),
            "size": a.get("size"),
            "digest": a.get("digest") or "",
        }
    if not assets:
        raise FetchError("release %s has no assets in the API listing" % tag)
    return {"tag": tag, "source": "api", "assets": assets}


def html_release(tag, timeout):
    """Read the release's asset list from GitHub's expanded-assets fragment.

    The REST API allows 60 requests an hour per address, which a room full of
    agents exhausts quickly. This page is plain HTML and is not rate limited.
    """
    url = WEB + "/expanded_assets/" + urllib.parse.quote(tag)
    body, _, _ = http_get(url, timeout=timeout)
    page = body.decode("utf-8", "replace")
    pattern = re.compile(
        r"/releases/download/" + re.escape(tag) + r"/([^\"'?#<>\s]+)"
    )
    assets = {}
    for raw in pattern.findall(page):
        name = html.unescape(raw)
        if name not in assets:
            assets[name] = {
                "name": name,
                "url": DOWNLOAD + "/" + tag + "/" + urllib.parse.quote(name),
                "size": None,
                "digest": "",
            }
    if not assets:
        raise FetchError("release %s has no assets on its release page" % tag)
    return {"tag": tag, "source": "html", "assets": assets}


def has_binaries(release):
    return any("-bin-" in name for name in release["assets"])


def resolve_latest(timeout):
    """Return the build tag that llama.cpp's latest stable release points at.

    Stable v0.x releases carry no binaries, only nightly-tag.txt naming the
    bNNNNN build that does. This mirrors engine.LatestRelease.
    """
    try:
        body, _, _ = http_get(API + "/latest", timeout=timeout)
        stable = json.loads(body)
        if any("-bin-" in (a.get("name") or "") for a in stable.get("assets") or []):
            return stable["tag_name"]
        for a in stable.get("assets") or []:
            if a.get("name") == "nightly-tag.txt":
                text, _, _ = http_get(a["browser_download_url"], timeout=timeout)
                tag = text.decode("utf-8", "replace").strip()
                if tag:
                    return tag
    except (FetchError, ValueError):
        pass
    # HTML fallback: follow /releases/latest to the stable tag, read its asset
    # list, then fetch nightly-tag.txt from it.
    _, _, final = http_get(WEB + "/latest", timeout=timeout)
    stable_tag = final.rstrip("/").rsplit("/", 1)[-1]
    if not stable_tag or stable_tag == "latest":
        raise FetchError("could not resolve the latest stable release tag")
    body, _, _ = http_get(
        WEB + "/expanded_assets/" + urllib.parse.quote(stable_tag), timeout=timeout
    )
    match = re.search(r"href=\"([^\"]*nightly-tag\.txt)\"", body.decode("utf-8", "replace"))
    if not match:
        raise FetchError("latest release %s has no nightly-tag.txt" % stable_tag)
    url = html.unescape(match.group(1))
    if url.startswith("/"):
        url = "https://github.com" + url
    text, _, _ = http_get(url, timeout=timeout)
    tag = text.decode("utf-8", "replace").strip()
    if not tag:
        raise FetchError("nightly-tag.txt in %s is empty" % stable_tag)
    return tag


def load_release(tag, timeout, cache_dir=None):
    if tag == "latest":
        tag = resolve_latest(timeout)
    if cache_dir:
        path = os.path.join(cache_dir, tag + ".json")
        if os.path.exists(path):
            with open(path, "r", encoding="utf-8") as fh:
                data = json.load(fh)
            assets = {
                a["name"]: {
                    "name": a["name"],
                    "url": a.get("url") or DOWNLOAD + "/" + tag + "/" + a["name"],
                    "size": a.get("size"),
                    "digest": a.get("digest") or "",
                }
                for a in data.get("assets") or []
            }
            if assets:
                return {"tag": tag, "source": "cache", "assets": assets}
    try:
        release = api_release(tag, timeout)
    except (FetchError, ValueError):
        release = html_release(tag, timeout)
    if cache_dir and release["source"] == "api":
        os.makedirs(cache_dir, exist_ok=True)
        payload = {
            "tag": release["tag"],
            "assets": [
                {
                    "name": a["name"],
                    "url": a["url"],
                    "size": a["size"],
                    "digest": a["digest"],
                }
                for a in release["assets"].values()
            ],
        }
        with open(os.path.join(cache_dir, tag + ".json"), "w", encoding="utf-8") as fh:
            json.dump(payload, fh, indent=1)
    return release


def asset_patterns(goos, arch, backend, cuda13):
    """Mirror engine.assetPatterns: substrings, in order of preference."""
    a = {"amd64": "x64", "arm64": "arm64"}.get(arch)
    if a is None:
        return []
    if backend == "cuda" and cuda13:
        pre = {"linux": "-bin-ubuntu-cuda-13", "windows": "-bin-win-cuda-13"}.get(goos)
        if pre is None:
            return []
        vk = {"linux": "-bin-ubuntu-vulkan-", "windows": "-bin-win-vulkan-"}[goos]
        return [[pre, a], [vk + a]]
    if goos == "darwin":
        return [["-bin-macos-" + a]]
    if goos == "linux":
        if backend == "cuda":
            return [
                ["-bin-ubuntu-cuda-12", a],
                ["-bin-ubuntu-cuda-", a],
                ["-bin-ubuntu-vulkan-" + a],
                ["-bin-ubuntu-" + a],
            ]
        if backend in ("rocm", "vulkan"):
            return [["-bin-ubuntu-vulkan-" + a], ["-bin-ubuntu-" + a]]
        return [["-bin-ubuntu-" + a]]
    if goos == "windows":
        if backend == "cuda":
            return [
                ["-bin-win-cuda-12", a],
                ["-bin-win-cuda-", a],
                ["-bin-win-vulkan-" + a],
                ["-bin-win-cpu-" + a],
            ]
        if backend in ("rocm", "vulkan"):
            return [["-bin-win-vulkan-" + a], ["-bin-win-cpu-" + a]]
        return [["-bin-win-cpu-" + a]]
    return []


def matches(name, parts):
    if not (name.endswith(".zip") or name.endswith(".tar.gz")):
        return False
    return all(p in name for p in parts)


def cuda_flavor(name):
    i = name.find("-cuda-")
    if i < 0:
        return ""
    rest = name[i:]
    for suffix in (".zip", ".tar.gz"):
        if rest.endswith(suffix):
            rest = rest[: -len(suffix)]
            break
    return rest


def pick_assets(assets, goos, arch, backend, cuda13):
    """Mirror engine.PickAssets: primary archive plus matching cudart."""
    for pat in asset_patterns(goos, arch, backend, cuda13):
        for name in assets:
            if not name.startswith("llama-") or not matches(name, pat):
                continue
            out = [assets[name]]
            flavor = cuda_flavor(name)
            if flavor:
                os_part = {"windows": "-bin-win-", "linux": "-bin-ubuntu-"}.get(goos, "")
                if os_part:
                    for cname in assets:
                        if not cname.startswith("cudart-") or os_part not in cname:
                            continue
                        if cname.endswith(flavor) or (
                            cname.endswith(flavor + ".zip")
                            or cname.endswith(flavor + ".tar.gz")
                        ):
                            out.append(assets[cname])
                            break
            return out
    return []


def pinned_assets(path):
    """Read the tag and asset names out of internal/engine/pinned.go."""
    with open(path, "r", encoding="utf-8") as fh:
        text = fh.read()
    tag_match = re.search(r'var pinnedRelease = Release\{Tag: "([^"]+)"', text)
    names = re.findall(r'\{Name: "([^"]+)"', text)
    return (tag_match.group(1) if tag_match else None), set(names)


def check_tag(tag, timeout, cache_dir, head, pinned):
    release = load_release(tag, timeout, cache_dir)
    assets = release["assets"]
    pinned_names = None
    if pinned and pinned[0] == release["tag"]:
        pinned_names = pinned[1]
    rows = []
    for goos, arch, backend, cuda13, note in PLATFORMS:
        picks = pick_assets(assets, goos, arch, backend, cuda13)
        row = {
            "platform": goos + "/" + arch,
            "backend": backend,
            "cuda13": cuda13,
            "note": note,
            "pick": picks[0]["name"] if picks else "",
            "pick_url": picks[0]["url"] if picks else "",
            "cudart": picks[1]["name"] if len(picks) > 1 else "",
            "cudart_url": picks[1]["url"] if len(picks) > 1 else "",
            "status": "ok",
        }
        if not picks:
            row["status"] = "no match"
        elif pinned_names is not None and row["pick"] not in pinned_names:
            row["status"] = "not in pinned.go"
        elif backend == "cuda" and not row["cudart"]:
            row["status"] = "ok, no cudart"
        rows.append(row)

    head_checked = 0
    head_failed = []
    if head:
        seen = set()
        for row in rows:
            if not row["pick"]:
                continue
            for key in ("pick_url", "cudart_url"):
                url = row[key]
                if not url or url in seen:
                    continue
                seen.add(url)
                head_checked += 1
                try:
                    _, status, _ = http_get(url, method="HEAD", timeout=timeout)
                    if status >= 400:
                        head_failed.append("%s HTTP %d" % (url, status))
                except FetchError as err:
                    head_failed.append(str(err))

    summary = {
        "tag": release["tag"],
        "source": release["source"],
        "assets": len(assets),
        "rows": len(rows),
        "ok": sum(1 for r in rows if r["status"].startswith("ok")),
        "no_match": sum(1 for r in rows if r["status"] == "no match"),
        "no_cudart": sum(1 for r in rows if r["status"] == "ok, no cudart"),
        "not_pinned": sum(1 for r in rows if r["status"] == "not in pinned.go"),
        "pinned_checked": pinned_names is not None,
        "head_checked": head_checked,
        "head_failed": head_failed,
    }
    return summary, rows


def print_table(summary, rows):
    print(
        "tag %s  source=%s  assets=%d"
        % (summary["tag"], summary["source"], summary["assets"])
    )
    headers = ["platform", "backend", "cuda13", "pick", "cudart", "status"]
    body = []
    for row in rows:
        body.append(
            [
                row["platform"],
                row["backend"],
                "yes" if row["cuda13"] else "no",
                row["pick"] or "-",
                row["cudart"] or "-",
                row["status"],
            ]
        )
    widths = [len(h) for h in headers]
    for cells in body:
        widths = [max(w, len(c)) for w, c in zip(widths, cells)]
    line = "  ".join(h.ljust(w) for h, w in zip(headers, widths)).rstrip()
    print(line)
    print("  ".join("-" * w for w in widths))
    for cells in body:
        print("  ".join(c.ljust(w) for c, w in zip(cells, widths)).rstrip())
    print(
        "summary: %d rows, %d ok, %d no match, %d without cudart"
        % (summary["rows"], summary["ok"], summary["no_match"], summary["no_cudart"])
    )
    if summary["pinned_checked"]:
        print("pinned.go: every pick present" if not summary["not_pinned"]
              else "pinned.go: %d picks missing" % summary["not_pinned"])
    if summary["head_checked"]:
        if summary["head_failed"]:
            print("head: %d urls checked, %d failed" % (summary["head_checked"], len(summary["head_failed"])))
            for failure in summary["head_failed"]:
                print("  " + failure)
        else:
            print("head: %d urls checked, all present" % summary["head_checked"])


def main(argv=None):
    parser = argparse.ArgumentParser(
        description="Print the llama.cpp asset EZLM would pick for each platform."
    )
    parser.add_argument("tags", nargs="*", help="llama.cpp tags, or 'latest'")
    parser.add_argument(
        "--pinned",
        default="internal/engine/pinned.go",
        help="pinned.go to read the fallback tag and names from (default: %(default)s)",
    )
    parser.add_argument(
        "--cache", metavar="DIR", help="cache API release JSON in DIR"
    )
    parser.add_argument(
        "--head", action="store_true", help="HEAD each chosen download URL"
    )
    parser.add_argument("--json", action="store_true", dest="as_json")
    parser.add_argument("--timeout", type=float, default=30.0)
    args = parser.parse_args(argv)

    pinned = None
    if args.pinned and os.path.exists(args.pinned):
        try:
            pinned = pinned_assets(args.pinned)
        except (OSError, ValueError) as err:
            print("warning: could not read %s: %s" % (args.pinned, err), file=sys.stderr)

    tags = list(dict.fromkeys(args.tags))
    if not tags:
        tags = [(pinned[0] if pinned and pinned[0] else "b11960"), "latest"]
        tags = list(dict.fromkeys(tags))

    results = []
    failed = False
    for tag in tags:
        try:
            summary, rows = check_tag(
                tag, args.timeout, args.cache, args.head, pinned
            )
        except (FetchError, ValueError) as err:
            print("%s: %s" % (tag, err), file=sys.stderr)
            failed = True
            continue
        results.append({"summary": summary, "rows": rows})
        if summary["no_match"] or summary["no_cudart"] or summary["not_pinned"] or summary["head_failed"]:
            failed = True
        if not args.as_json:
            if results and len(results) > 1:
                print()
            print_table(summary, rows)

    if args.as_json:
        json.dump({"releases": results}, sys.stdout, indent=2)
        sys.stdout.write("\n")

    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
