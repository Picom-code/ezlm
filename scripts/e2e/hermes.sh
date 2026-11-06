#!/usr/bin/env bash
# Hermes step of the connections e2e (PLAN 9.3, Tier A): EZLM's Setup
# (find, configure, model-and-tool check, preflight, TUI smoke) against the
# real Hermes on this machine and a fake model server, with every Hermes and
# EZLM folder in a scratch home. Mac and Linux.
#
#   scripts/e2e/hermes.sh [scratch-dir]
#
# HERMES_BIN_DIR picks the folder holding hermes (default: the one on PATH).
# EZLM_REAL_HERMES_SLOW=1 adds the run without EZLM's reasoning override
# (about five minutes). Reuse the scratch folder: a Hermes data folder it
# hasn't seen sets itself up first (a few hundred MB).
set -euo pipefail
cd "$(dirname "$0")/../.."

# E2E_HERMES_ROOT keeps one scratch Hermes home across runs (its first run
# sets Hermes up there, about 1.2 GB).
root="${E2E_HERMES_ROOT:-${1:-${E2E_ROOT:-$(mktemp -d)}}/hermes}"
mkdir -p "$root"
bin_dir="${HERMES_BIN_DIR:-}"
if [ -z "$bin_dir" ]; then
	hermes=$(command -v hermes) || { echo "hermes isn't on PATH; set HERMES_BIN_DIR" >&2; exit 2; }
	bin_dir=$(dirname "$hermes")
fi

# The user's own Hermes config must come out unchanged. auth.json is not
# compared: a running Hermes refreshes its credential pool there by itself.
real="${REAL_HOME:-$HOME}/.hermes/config.yaml"
sum() { [ -f "$1" ] && shasum -a 256 "$1" | cut -d' ' -f1 || echo none; }
before=$(sum "$real")

EZLM_REAL_HERMES="$bin_dir" EZLM_REAL_HERMES_ROOT="$root" \
	"${GO:-go}" test ./internal/integrate/ -run '^TestHermesRealSetup$' -count=1 -v -timeout 30m

if [ "$(sum "$real")" != "$before" ]; then
	echo "the real Hermes config changed during the run: $real" >&2
	exit 1
fi
echo "hermes e2e passed (scratch home: $root)"
