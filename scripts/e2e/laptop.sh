#!/bin/bash
# OpenCode decision table, end to end through the ezlm CLI and the real
# binaries (PLAN 4.2, the RTX Spark laptop's failure). Run it from
# connect_e2e.sh (it needs the fake model and the run's isolated folders):
#
#   E2E_OC1=<opencode 1.19.5> [E2E_OC2=<a user's opencode 2.x>] scripts/e2e/connect_e2e.sh laptop
#
# Cases, each in a home of its own under $E2E_ROOT:
#   laptop  1.19.5 is the opencode on PATH and the history is a v2 database
#           (what OpenBot's OpenCode 2.x leaves): EZLM must download its
#           pinned 2.2.5 from GitHub, check its SHA-256, connect it, and both
#           the check and the TUI must work against that history.
#   user2   the user's own 2.x is on PATH: reused, nothing downloaded.
#   v1      1.19.5 on PATH with its own v1 history: reused, nothing
#           downloaded, and no 2.x ever opens (and converts) that history.
# E2E_LAPTOP_CASES picks some ("laptop user2 v1" by default).
set -u -o pipefail
here=$(cd "$(dirname "$0")" && pwd)
. "$here/lib.sh"
: "${EZLM:?run this from connect_e2e.sh}" "${E2E_ROOT:?}" "${FAKE_LOG:?}" "${E2E_BIN:?}"
OC1=${E2E_OC1:?set E2E_OC1 to an opencode 1.19.5}
OC2=${E2E_OC2:-$(command -v opencode)}
base_ezlm=$EZLM_HOME
failed=0
fail() { echo "laptop.sh: FAIL ($case): $*" >&2; failed=1; }

case_env() { # name
	local r="$E2E_ROOT/oc-$1"
	mkdir -p "$r/home" "$r/ezlm" "$r/bin"
	export HOME="$r/home" XDG_CONFIG_HOME="$r/home/.config" XDG_DATA_HOME="$r/home/.local/share"
	export XDG_STATE_HOME="$r/home/.local/state" XDG_CACHE_HOME="$r/home/.cache"
	export HERMES_HOME="$r/home/.hermes" DSH_HOME="$r/home/.dsh" OPENCODE_CONFIG_DIR="$r/home/.config/opencode" EZLM_HOME="$r/ezlm"
	cp "$base_ezlm/config.json" "$EZLM_HOME/config.json"
	# Only this case's opencode is on PATH (nothing of the user's).
	export PATH="$r/bin:/usr/bin:/bin:/usr/sbin:/sbin"
	unset EZLM_TOOL_PATH
	CASE_DIR=$r
}

# make_history <opencode> <expected format>: another app's OpenCode (OpenBot
# runs 2.x as `opencode acp`) chats once in this home's data folder, with a
# config of its own, so the history is one that build really wrote (the
# testdata databases have their migration rows removed; OpenCode can't open
# them).
make_history() {
	local bin=$1 want=$2 cfg="$CASE_DIR/other-app-config" ws="$CASE_DIR/other-app-ws" flags=
	mkdir -p "$cfg" "$ws"
	cat >"$cfg/opencode.json" <<EOF
{"\$schema": "https://opencode.ai/config.json", "model": "other/$FAKE_ALIAS",
 "provider": {"other": {"npm": "@ai-sdk/openai-compatible", "name": "Other app",
  "options": {"baseURL": "$FAKE_URL/v1", "apiKey": "{env:OTHER_APP_KEY}"},
  "models": {"$FAKE_ALIAS": {"name": "Other", "tool_call": true}}}}}
EOF
	"$bin" run --help 2>/dev/null | grep -q -- --standalone && flags=--standalone
	(cd "$ws" && PWD=$ws OPENCODE_CONFIG_DIR=$cfg OTHER_APP_KEY=$FAKE_KEY "$bin" run $flags --model "other/$FAKE_ALIAS" "Reply with exactly: hi" </dev/null >"$CASE_DIR/other-app-run.txt" 2>&1)
	local f
	f=$(db_format)
	echo "history written by $("$bin" --version 2>&1 | tail -1): $f, sessions $(sqlite3 -readonly "$XDG_DATA_HOME/opencode/opencode.db" "select count(*) from $([ "$want" = v2 ] && echo session_v2 || echo session)" 2>&1)"
	[ "$f" = "$want" ] || fail "the other app's OpenCode wrote a $f history: $(tail -3 "$CASE_DIR/other-app-run.txt")"
}

db_format() {
	local db="$XDG_DATA_HOME/opencode/opencode.db" v1=0 v2=0
	grep -aq 'CREATE TABLE `session_v2`' "$db" "$db-wal" 2>/dev/null && v2=1
	grep -aq 'CREATE TABLE `session`' "$db" "$db-wal" 2>/dev/null && v1=1
	case "$v1$v2" in 11) echo both ;; 01) echo v2 ;; 10) echo v1 ;; *) echo none ;; esac
}

doctor() { # field
	"$EZLM" doctor apps opencode --json | e2e_redact
}

tui() { # out-dir
	e2e_launch opencode >"$1.launch.txt" 2>&1 || { fail "launch (dry run) failed: $(tail -3 "$1.launch.txt")"; return; }
	e2e_tui_drive opencode "EZLM (local)" '{ctrl-c}{wait:1s}{ctrl-c}' "$1" 120s >"$1.drive.txt" 2>&1 || fail "the TUI didn't answer with the nonce: $(tail -3 "$1.drive.txt")"
	cat "$1.drive.txt"
}

for case in ${E2E_LAPTOP_CASES:-laptop user2 v1}; do
	echo "== $case"
	case $case in
	laptop)
		case_env laptop
		ln -sf "$OC1" "$CASE_DIR/bin/opencode"
		make_history "$OC2" v2
		# The trap is armed: the user's 1.19.5 can't open this history.
		trap_out=$(cd "$HOME" && PWD="$HOME" opencode models ezlm 2>&1 </dev/null)
		case $trap_out in *"no session table"*) echo "1.19.5 on this history: $(grep -o 'Database is not empty and has no session table' <<<"$trap_out" | head -1)" ;;
		*) fail "1.19.5 opened the v2 history: $trap_out" ;; esac
		"$EZLM" connect opencode --default 2>&1 | e2e_redact || fail "ezlm connect opencode"
		pinned="$EZLM_HOME/tools/opencode/2.2.5/opencode"
		[ -x "$pinned" ] || fail "no pinned OpenCode at $pinned"
		echo "pinned: $("$pinned" --version 2>&1)"
		grep -q 'checksum ok for opencode-darwin-arm64.zip' "$EZLM_HOME/logs/integrate-opencode.log" || fail "no checksum line in the log"
		grep -E 'Downloading OpenCode|checksum ok|TUI smoke: ok=|EZLM uses OpenCode|choice pinned' "$EZLM_HOME/logs/integrate-opencode.log" | cut -c1-200
		d=$(doctor)
		echo "$d" | grep -E '"(state|version|source|bin|preflight)"'
		grep -q '"state": "connected"' <<<"$d" && grep -q '"source": "ezlm"' <<<"$d" && grep -q '"preflight": "ok"' <<<"$d" || fail "doctor: $d"
		tui "$CASE_DIR/tui"
		f=$(db_format)
		echo "history after the check and the TUI: $f"
		[ "$f" = v2 ] || fail "history is $f"
		;;
	user2)
		case_env user2
		ln -sf "$OC2" "$CASE_DIR/bin/opencode"
		"$EZLM" connect opencode --default 2>&1 | e2e_redact || fail "ezlm connect opencode"
		[ ! -e "$EZLM_HOME/tools/opencode" ] || fail "EZLM installed its own OpenCode next to the user's 2.x"
		d=$(doctor)
		echo "$d" | grep -E '"(state|version|source|bin|preflight)"'
		grep -q '"state": "connected"' <<<"$d" && grep -q "\"bin\": \"$CASE_DIR/bin/opencode\"" <<<"$d" && grep -q '"preflight": "ok"' <<<"$d" || fail "doctor: $d"
		tui "$CASE_DIR/tui"
		;;
	v1)
		case_env v1
		ln -sf "$OC1" "$CASE_DIR/bin/opencode"
		make_history "$OC1" v1
		"$EZLM" connect opencode --default 2>&1 | e2e_redact || fail "ezlm connect opencode"
		[ ! -e "$EZLM_HOME/tools/opencode" ] || fail "EZLM installed its own OpenCode next to a working 1.x"
		d=$(doctor)
		echo "$d" | grep -E '"(state|version|source|bin|preflight)"'
		grep -q '"state": "connected"' <<<"$d" && grep -q '"version": "1.19.5"' <<<"$d" && grep -q '"preflight": "ok"' <<<"$d" || fail "doctor: $d"
		tui "$CASE_DIR/tui"
		f=$(db_format)
		echo "history after the check and the TUI: $f"
		[ "$f" = v1 ] || fail "a 2.x converted the user's 1.x history ($f)"
		;;
	*) fail "unknown case" ;;
	esac
done
echo "fake model requests with a tool result: $(grep -c '"tool_result"' "$FAKE_LOG")"
[ "$failed" = 0 ] && echo "laptop.sh: ok"
exit $failed
