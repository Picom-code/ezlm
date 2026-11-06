#!/bin/bash
# OpenCode step of the connections e2e (PLAN 9.3, Tier A and C): runs
# TestOpenCodeReal for each OpenCode binary given, against the fake model,
# inside the Go test's throwaway home. That test runs Setup end to end
# (probe, config, the model-and-tool check, the preflight and the TUI smoke
# in a pseudo-terminal), checks the data format the build writes, and for
# 1.x the laptop case (a 2.x history makes EZLM choose its pinned 2.2.5).
#
#   scripts/e2e/opencode.sh <opencode-2.x> [<opencode-1.x>]
#   (from connect_e2e.sh: E2E_OPENCODE_BINS, else the opencode on PATH)
#   OPENCODE_RECORD=<dir> also records the run --format json fixtures.
#   OPENCODE_ZIP=<pinned release archive> also installs EZLM's pinned copy
#   from it (served locally, checked against the pinned SHA-256).
#
# The user's own OpenCode config, key, history and background service are
# hashed before and after; any change fails the run.
set -euo pipefail
cd "$(dirname "$0")/../.."
[ $# -ge 1 ] || set -- ${E2E_OPENCODE_BINS:-$(command -v opencode || true)}
[ $# -ge 1 ] || { echo "usage: $0 <opencode-2.x> [<opencode-1.x>]" >&2; exit 2; }
GO=${GO:-go}

# Configs are hashed; the history database, which the user's own OpenCode
# may write meanwhile, is compared by inode (EZLM must never replace it).
real=${REAL_HOME:-$HOME}
watch=("$real/.config/opencode/opencode.json" "$real/.config/opencode/opencode.jsonc" "$real/.config/opencode/ezlm.key"
  "$real/.local/state/opencode/service.json" "$real/.config/opencode/service.json")
db="$real/.local/share/opencode/opencode.db"
snapshot() {
  for f in "${watch[@]}"; do
    if [ -e "$f" ]; then echo "$f $(shasum -a 256 "$f" | cut -d' ' -f1)"; else echo "$f absent"; fi
  done
  if [ -e "$db" ]; then echo "$db inode $(ls -i "$db" | awk '{print $1}')"; else echo "$db absent"; fi
}
before=$(snapshot)

status=0
for bin in "$@"; do
  echo "== OpenCode at $bin"
  EZLM_TEST_OPENCODE="$bin" EZLM_OPENCODE_RECORD="${OPENCODE_RECORD:-}" \
    "$GO" test -count=1 -timeout 15m -run 'TestOpenCodeReal$' -v ./internal/integrate/ | grep -v '^        20' || status=1
done

if [ -n "${OPENCODE_ZIP:-}" ]; then
  echo "== pinned install from $OPENCODE_ZIP"
  EZLM_TEST_OPENCODE_ZIP="$OPENCODE_ZIP" "$GO" test -count=1 -timeout 10m -run 'TestOpenCodeRealInstall$' -v ./internal/integrate/ | grep -v '^        20' || status=1
fi

after=$(snapshot)
if [ "$before" != "$after" ]; then
  echo "FAIL: the user's OpenCode files changed:" >&2
  diff <(echo "$before") <(echo "$after") >&2 || true
  exit 1
fi
exit $status
