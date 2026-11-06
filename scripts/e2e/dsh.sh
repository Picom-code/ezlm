#!/bin/bash
# DeepSeek Harness step of the connections e2e run (PLAN 9.3).
#
# Run it from connect_e2e.sh after the isolated environment is set up and
# $EZLM_HOME/config.json points at the fake (or test) model server:
#
#   EZLM=<built ezlm> DSH_BIN_DIR=<folder with dsh 0.2.1-rc.5 and node> scripts/e2e/dsh.sh
#
# (DSH_BIN_DIR defaults to the folder of the dsh on PATH.)
#
# It seeds DSH_HOME with the fixture patch files (a copy of a real setup
# with a Spark route), runs `ezlm connect dsh`, then proves with dsh itself
# that the composed web profile has both the Spark route and EZLM's, that the
# key sits in $DSH_HOME/.env, and that the home-level patch was never
# written. It refuses to run against a real home.
set -euo pipefail

here=$(cd "$(dirname "$0")/../.." && pwd)
fx="$here/internal/integrate/testdata/dsh"
: "${EZLM:?set EZLM to the ezlm binary}"
if [ -z "${DSH_BIN_DIR:-}" ] && command -v dsh >/dev/null; then DSH_BIN_DIR=$(dirname "$(command -v dsh)"); fi
: "${DSH_BIN_DIR:?set DSH_BIN_DIR to the folder holding dsh and node}"
: "${DSH_HOME:?set DSH_HOME to a temp folder}"
: "${EZLM_HOME:?set EZLM_HOME to a temp folder}"

real_home=$(eval echo "~$(id -un)")
case "$DSH_HOME" in
"$real_home/.dsh" | "$real_home/.dsh/"*) echo "dsh.sh: DSH_HOME is the real one; refusing" >&2; exit 2 ;;
esac
case "$HOME" in
"$real_home") echo "dsh.sh: HOME is the real one; refusing" >&2; exit 2 ;;
esac
export DSH_TELEMETRY_MODE=DISABLED DSH_TELEMETRY_DISABLED=1 EZLM_LAUNCH_DRYRUN=1
export EZLM_TOOL_PATH="$DSH_BIN_DIR"

fail() { echo "dsh.sh: FAIL: $*" >&2; exit 1; }

mkdir -p "$DSH_HOME/profiles/web" "$DSH_HOME/profiles/headless"
cp "$fx/web-spark.cordis.patch.yml" "$DSH_HOME/profiles/web/cordis.patch.yml"
cp "$fx/headless-empty.cordis.patch.yml" "$DSH_HOME/profiles/headless/cordis.patch.yml"
spark_before=$(sed -n '/^      spark:/,/^- id: subagent/p' "$DSH_HOME/profiles/web/cordis.patch.yml")

"$EZLM" connect dsh || fail "ezlm connect dsh"

grep -q '^EZLM_API_KEY=' "$DSH_HOME/.env" || fail "no key in $DSH_HOME/.env"
[ "$(stat -f %Lp "$DSH_HOME/.env" 2>/dev/null || stat -c %a "$DSH_HOME/.env")" = 600 ] || fail ".env is not 0600"
[ ! -e "$DSH_HOME/cordis.patch.yml" ] || fail "the home-level patch was written"
spark_after=$(sed -n '/^      spark:/,/^      ezlm:/p' "$DSH_HOME/profiles/web/cordis.patch.yml" | sed '$d')
[ "$spark_after" = "$(printf '%s\n' "$spark_before" | sed '$d')" ] || fail "the spark route changed"

node="$DSH_BIN_DIR/node"
dsh_js=$(cd "$DSH_BIN_DIR" && readlink -f dsh 2>/dev/null || echo "$DSH_BIN_DIR/dsh")
dump=$("$node" "$dsh_js" --profile web --dump-config)
grep -q '^      spark:' <<<"$dump" || fail "composed web profile lost spark"
grep -q '^      ezlm:' <<<"$dump" || fail "composed web profile has no ezlm"

"$EZLM" doctor apps dsh || fail "doctor apps dsh"
echo "dsh.sh: ok"
