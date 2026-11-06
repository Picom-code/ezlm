#!/usr/bin/env bash
# connect_e2e.sh: the app-connection end-to-end harness (PLAN 9.3), Mac and
# Linux. Everything runs in a throwaway folder: HOME, the XDG folders,
# HERMES_HOME, DSH_HOME, OPENCODE_CONFIG_DIR and EZLM_HOME all point into
# it, launchers are only written (EZLM_LAUNCH_DRYRUN=1), the model is
# cmd/fakellm on a loopback port, and every process the run starts is
# stopped by its own PID afterwards. Nothing is ever stopped by name.
#
#   scripts/e2e/connect_e2e.sh [self] [opencode] [hermes] [dsh]
#
# "self" (the default) checks the harness itself: fakellm answers a scripted
# tool round trip and tuiprobe drives a small terminal loop. Each other
# name runs scripts/e2e/<name>.sh (owned by that tool's workstream): a file
# that defines an e2e_<name> function is sourced and the function called;
# any other step file runs as its own script with the environment below,
# E2E_ROOT set to a scratch folder in the run, and its exit code deciding.
# opencode.sh takes its binaries from E2E_OPENCODE_BINS (default: the
# opencode on PATH), dsh.sh its dsh folder from DSH_BIN_DIR (default: the
# folder of the dsh on PATH), hermes.sh the hermes on PATH.
#
# For the step files:
#   T              the run's folder; E2E_BIN holds ezlm, fakellm, tuiprobe
#   REAL_HOME      the user's real home (only for before/after comparisons)
#   GO             the go binary (go is not on the run's PATH)
#   FAKE_URL       http://127.0.0.1:<port> of the fake model; FAKE_KEY its key
#                  (never print it); FAKE_ALIAS the model id; FAKE_LOG its
#                  request log (JSON lines, no keys)
#   e2e_log msg    progress line
#   e2e_fail msg   fail the run (the guard still checks the real files)
#   e2e_track pid  stop this PID (and its group) when the run ends
#   e2e_probe ...  tuiprobe with the run's environment
#   E2E_TOOL_DIRS  extra folders put on PATH (colon separated), e.g. where
#                  opencode, hermes or dsh live; nothing else of the user's
#                  PATH is used
#
# Before and after the run the guard fingerprints the user's real app
# settings (not ~/.hermes/auth.json, which a running Hermes rewrites itself), the OpenCode database, the OpenCode background service record
# and the process listening on EZLM's real port, and fails loudly on any
# difference.
set -u
REPO=$(cd "$(dirname "$0")/../.." && pwd)
T=$(mktemp -d "${TMPDIR:-/tmp}/ezlm-e2e.XXXXXX")
E2E_BIN="$T/bin"
FAILED=0
PIDS=()
REAL_HOME=$HOME
REAL_PORT=${E2E_REAL_PORT:-8081}

e2e_log() { printf '[e2e] %s\n' "$*"; }
e2e_redact_line() { sed -E 's/(ezlm-[A-Za-z0-9]{16,}|sk-[A-Za-z0-9_-]{20,})/ezlm-REDACTED/g'; }
e2e_fail() { printf '[e2e] FAIL: %s\n' "$*" >&2; FAILED=1; }
e2e_track() { PIDS+=("$1"); }

# e2e_guard prints one line per watched file or process, for diffing.
e2e_guard() {
	local f
	for f in "$REAL_HOME/.config/opencode/opencode.json" "$REAL_HOME/.config/opencode/opencode.jsonc" \
		"$REAL_HOME/.config/opencode/ezlm.key" "$REAL_HOME/.hermes/config.yaml" \
		"$REAL_HOME/.dsh/.credentials.yaml" "$REAL_HOME/.dsh/.env" "$REAL_HOME/.dsh/cordis.patch.yml" \
		"$REAL_HOME/.ezlm/config.json" "$REAL_HOME/.local/state/opencode/service.json" \
		"$REAL_HOME"/.dsh/profiles/*/cordis.patch.yml; do
		[ -f "$f" ] && printf '%s %s\n' "$(shasum -a 256 "$f" | cut -c1-64)" "$f"
	done
	for f in "$REAL_HOME/.local/share/opencode/opencode.db" "$REAL_HOME/.local/share/opencode/opencode.db-wal"; do
		[ -f "$f" ] && printf '%s %s\n' "$(stat -f '%z %m' "$f" 2>/dev/null || stat -c '%s %Y' "$f")" "$f"
	done
	if command -v lsof >/dev/null; then
		printf 'listen %s %s\n' "$REAL_PORT" "$(lsof -nP -iTCP@127.0.0.1:"$REAL_PORT" -sTCP:LISTEN -t 2>/dev/null | sort | tr '\n' ' ')"
	fi
}

# e2e_leftovers lists the PIDs of processes whose command line names the
# run's folder: started by this run, by construction.
e2e_leftovers() {
	# The folder goes to awk through its environment, so awk's own command
	# line doesn't name it.
	ps -ax -o pid=,ppid=,etime=,command= | E2E_T="$T" E2E_ME="$$" awk 'index($0, ENVIRON["E2E_T"]) && $1 != ENVIRON["E2E_ME"]'
}

e2e_cleanup() {
	local pid
	for pid in "${PIDS[@]+"${PIDS[@]}"}"; do
		kill -TERM "-$pid" 2>/dev/null || kill -TERM "$pid" 2>/dev/null
	done
	sleep 0.3
	for pid in "${PIDS[@]+"${PIDS[@]}"}"; do
		kill -KILL "-$pid" 2>/dev/null || kill -KILL "$pid" 2>/dev/null
	done
	# Anything else still running from the run's folder (an app's helper, a
	# Hermes gateway for the test home) is ours: say so and stop it by PID.
	local left
	e2e_leftovers >"$T/leftovers.txt"
	left=$(awk '{print $1}' "$T/leftovers.txt")
	if [ -n "$left" ]; then
		e2e_log "left running from the run's folder (stopping): $(echo $left)"
		cut -c1-200 "$T/leftovers.txt" | e2e_redact_line
		kill -TERM $left 2>/dev/null
		sleep 1
		kill -KILL $left 2>/dev/null
	fi
	# A server gate that fell through to StartSync would have registered a
	# launchd job (a systemd unit on Linux) for this run's EZLM_HOME.
	if [ -n "${EZLM_HOME:-}" ]; then
		local label
		label="com.ezlm.server-test-$(printf '%s' "$EZLM_HOME" | shasum -a 1 | cut -c1-8)"
		if ls "$HOME"/Library/LaunchAgents/com.ezlm.server* >/dev/null 2>&1 || ls "$HOME"/.config/systemd/user/ezlm* >/dev/null 2>&1; then
			e2e_fail "EZLM registered a server service in the run's home (the server gate fell through to StartSync)"
		fi
		if command -v launchctl >/dev/null && launchctl print "gui/$(id -u)/$label" >/dev/null 2>&1; then
			e2e_fail "launchd has $label: the server gate fell through to StartSync"
			launchctl bootout "gui/$(id -u)/$label" 2>/dev/null
		fi
	fi
	e2e_guard >"$T/guard-after.txt"
	if ! diff -u "$T/guard-before.txt" "$T/guard-after.txt"; then
		printf '[e2e] FAIL: the run changed real files or processes (above)\n' >&2
		FAILED=1
	fi
	if [ "$FAILED" = 0 ]; then
		e2e_log "passed"
		[ -z "${E2E_KEEP:-}" ] && rm -rf "$T"
	else
		e2e_log "failed; the run's files are in $T"
	fi
	exit "$FAILED"
}

e2e_guard >"$T/guard-before.txt"
trap e2e_cleanup EXIT
trap 'exit 130' INT TERM

# Built before HOME moves, so Go keeps using its own caches.
e2e_log "building ezlm, fakellm and tuiprobe"
mkdir -p "$E2E_BIN"
GO=${GO:-$(command -v go || ls /usr/local/go/bin/go /opt/homebrew/bin/go 2>/dev/null | head -1)}
if [ -z "$GO" ]; then
	e2e_fail "go was not found; set GO to the go binary"
	exit 1
fi
(cd "$REPO" && "$GO" build -o "$E2E_BIN/" ./cmd/ezlm ./cmd/fakellm ./cmd/tuiprobe) || { e2e_fail "the build failed"; exit 1; }
# Steps that run go test keep Go's own caches and module downloads.
export GOCACHE=$("$GO" env GOCACHE) GOMODCACHE=$("$GO" env GOMODCACHE) GOPATH=$("$GO" env GOPATH) GO REAL_HOME

# The run's environment: nothing points at the user's folders.
export HOME="$T/home"
export XDG_CONFIG_HOME="$HOME/.config" XDG_DATA_HOME="$HOME/.local/share" XDG_STATE_HOME="$HOME/.local/state" XDG_CACHE_HOME="$HOME/.cache"
export HERMES_HOME="$HOME/.hermes" DSH_HOME="$HOME/.dsh" OPENCODE_CONFIG_DIR="$HOME/.config/opencode" EZLM_HOME="$T/ezlm"
export DSH_TELEMETRY_MODE=DISABLED OPENCODE_DISABLE_MODELS_FETCH=1 OPENCODE_DISABLE_AUTOUPDATE=1 EZLM_LAUNCH_DRYRUN=1
export HERMES_NONINTERACTIVE=1 CI=1
unset EZLM_NO_INTEGRATIONS
mkdir -p "$HOME" "$XDG_CONFIG_HOME" "$XDG_DATA_HOME" "$XDG_STATE_HOME" "$XDG_CACHE_HOME" "$EZLM_HOME" "$E2E_BIN"
export PATH="$E2E_BIN${E2E_TOOL_DIRS:+:$E2E_TOOL_DIRS}:/usr/bin:/bin:/usr/sbin:/sbin"
e2e_log "run folder $T"

FAKE_KEY="ezlm-e2e$(LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 24)"
FAKE_ALIAS=${FAKE_ALIAS:-qwen3.5-4b}
FAKE_LOG="$T/fakellm.jsonl"
mkfifo "$T/fake.url"
"$E2E_BIN/fakellm" -key "$FAKE_KEY" -alias "$FAKE_ALIAS" -log "$FAKE_LOG" >"$T/fake.url" 2>"$T/fakellm.err" &
e2e_track $!
read -r FAKE_URL <"$T/fake.url"
export FAKE_URL FAKE_KEY FAKE_ALIAS FAKE_LOG T E2E_BIN
e2e_log "fake model at $FAKE_URL"

# EZLM's config points at the fake model, as a real deploy would.
FAKE_PORT=${FAKE_URL##*:}
cat >"$EZLM_HOME/config.json" <<EOF
{"key": "$FAKE_KEY", "port": $FAKE_PORT, "model_id": "$FAKE_ALIAS", "model_name": "Qwen3.5 4B (fake)", "ctx": 65536, "tool_calls": true, "server": "fakellm", "engine_tag": "fakellm"}
EOF
chmod 600 "$EZLM_HOME/config.json"

e2e_probe() { "$E2E_BIN/tuiprobe" "$@"; }

e2e_self() {
	e2e_log "self: fake model tool round trip"
	local ws="$T/self ws"
	mkdir -p "$ws"
	printf 'EZLM-CHECK-SELFTEST\n' >"$ws/ezlm-check.txt"
	local tools='[{"type":"function","function":{"name":"read","parameters":{"type":"object","properties":{"filePath":{"type":"string"}}}}}]'
	local first
	first=$(curl -sS -H "Authorization: Bearer $FAKE_KEY" -H 'Content-Type: application/json' "$FAKE_URL/v1/chat/completions" \
		-d "{\"messages\":[{\"role\":\"system\",\"content\":\"Working directory: $ws\"},{\"role\":\"user\",\"content\":\"open ezlm-check.txt\"}],\"tools\":$tools}")
	case $first in
	*'"name":"read"'*) ;;
	*) e2e_fail "turn 1 was not a call to the read tool: $first" ;;
	esac
	case $first in
	*"$ws/ezlm-check.txt"*) ;;
	*) e2e_fail "turn 1 didn't ask for the check file: $first" ;;
	esac
	local second
	second=$(curl -sS -N -H "Authorization: Bearer $FAKE_KEY" -H 'Content-Type: application/json' "$FAKE_URL/v1/chat/completions" \
		-d '{"stream":true,"messages":[{"role":"user","content":"x"},{"role":"tool","tool_call_id":"c1","content":"EZLM-CHECK-SELFTEST"}]}')
	case $second in
	*'"content":"EZLM-CHECK-SELFTEST"'*'"timings"'*'[DONE]'*) ;;
	*) e2e_fail "turn 2 didn't stream the tool result back: $second" ;;
	esac
	grep -q "$FAKE_KEY" "$FAKE_LOG" && e2e_fail "the fake model's log holds the key"

	e2e_log "self: tuiprobe drives a terminal loop"
	e2e_probe -cmd /bin/sh -ready 'ready>' -type 'hello' -expect 'you said: hel+o' -exit '/exit{enter}' -code 7 -timeout 20s -out "$T/self-screen.txt" -- \
		-c 'printf "ready> "; while read l; do [ "$l" = "/exit" ] && exit 7; echo "you said: $l"; printf "ready> "; done' \
		|| e2e_fail "tuiprobe did not drive the loop (screen in $T/self-screen.txt)"
}

STEPS=("$@")
[ ${#STEPS[@]} = 0 ] && STEPS=(self)
for step in "${STEPS[@]}"; do
	case $step in
	self) e2e_self ;;
	*)
		f="$REPO/scripts/e2e/$step.sh"
		if [ ! -f "$f" ]; then
			e2e_fail "no step file $f"
			continue
		fi
		if grep -q "^e2e_$step()" "$f"; then
			# shellcheck source=/dev/null
			. "$f"
			"e2e_$step" || e2e_fail "$step failed"
		else
			mkdir -p "$T/steps"
			e2e_log "$step: running $f"
			E2E_ROOT="$T/steps" EZLM="$E2E_BIN/ezlm" bash "$f" || e2e_fail "$step failed"
		fi
		;;
	esac
done
