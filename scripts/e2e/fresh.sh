#!/bin/bash
# Fresh installs through `ezlm connect`, each in an empty home of its own
# under $E2E_ROOT (run it from connect_e2e.sh; it needs the fake model):
#
#   [E2E_FRESH_CASES="opencode dsh hermes"] scripts/e2e/connect_e2e.sh fresh
#
#   opencode  no opencode anywhere: EZLM downloads its pinned 2.2.5 from
#             GitHub, checks the SHA-256 and connects it; the TUI answers.
#   dsh       no node and no dsh (EZLM_TOOL_PATH is an empty folder, so even
#             Homebrew's node isn't seen): EZLM installs its pinned Node and
#             dsh 0.2.1-rc.5 into its tools folder; the lock file npm wrote
#             must carry the integrity tools.json pins, even with the home's
#             .npmrc saying package-lock=false.
#   hermes    no hermes anywhere: EZLM runs Hermes's install.sh
#             --non-interactive. The home's .bash_profile (the installer runs
#             in a login bash) puts stand-ins for sudo, brew, launchctl,
#             xcode-select, softwareupdate, tccutil, osascript and open first
#             on PATH: each records that it was called and refuses, so the
#             installer can't reach anything outside this home through them.
#             The run fails if any was called. Expect a few minutes and about
#             2 GB.
set -u -o pipefail
here=$(cd "$(dirname "$0")" && pwd)
. "$here/lib.sh"
: "${EZLM:?run this from connect_e2e.sh}" "${E2E_ROOT:?}" "${FAKE_LOG:?}" "${E2E_BIN:?}"
tools_json="$here/../../internal/integrate/tools.json"
base_ezlm=$EZLM_HOME
failed=0
fail() { echo "fresh.sh: FAIL ($case): $*" >&2; failed=1; }

case_env() { # name
	local r="$E2E_ROOT/fresh-$1"
	mkdir -p "$r/home" "$r/ezlm" "$r/empty"
	export HOME="$r/home" XDG_CONFIG_HOME="$r/home/.config" XDG_DATA_HOME="$r/home/.local/share"
	export XDG_STATE_HOME="$r/home/.local/state" XDG_CACHE_HOME="$r/home/.cache"
	export HERMES_HOME="$r/home/.hermes" DSH_HOME="$r/home/.dsh" OPENCODE_CONFIG_DIR="$r/home/.config/opencode" EZLM_HOME="$r/ezlm"
	cp "$base_ezlm/config.json" "$EZLM_HOME/config.json"
	export PATH="/usr/bin:/bin:/usr/sbin:/sbin"
	unset EZLM_TOOL_PATH
	CASE_DIR=$r
}

doctor() { "$EZLM" doctor apps "$1" --json | e2e_redact; }

for case in ${E2E_FRESH_CASES:-opencode dsh hermes}; do
	echo "== $case"
	case $case in
	opencode)
		case_env opencode
		export EZLM_TOOL_PATH="$CASE_DIR/empty"
		"$EZLM" connect opencode --default 2>&1 | e2e_redact || fail "ezlm connect opencode"
		bin="$EZLM_HOME/tools/opencode/2.2.5/opencode"
		echo "installed: $("$bin" --version 2>&1) at $bin ($(file -b "$bin" | cut -c1-60))"
		grep -E 'checksum ok|installed OpenCode|TUI smoke: ok=' "$EZLM_HOME/logs/integrate-opencode.log" | cut -c1-160
		grep -q 'checksum ok for opencode-darwin-arm64.zip\|checksum ok for opencode-' "$EZLM_HOME/logs/integrate-opencode.log" || fail "no checksum line"
		d=$(doctor opencode)
		echo "$d" | grep -E '"(state|version|source|preflight)"'
		grep -q '"source": "ezlm"' <<<"$d" && grep -q '"preflight": "ok"' <<<"$d" || fail "doctor: $d"
		e2e_launch opencode >"$CASE_DIR/launch.txt" 2>&1 || fail "launch: $(tail -3 "$CASE_DIR/launch.txt")"
		e2e_tui_drive opencode "EZLM (local)" '{ctrl-c}{wait:1s}{ctrl-c}' "$CASE_DIR/tui" 120s || fail "the TUI didn't answer"
		;;
	dsh)
		case_env dsh
		export EZLM_TOOL_PATH="$CASE_DIR/empty"
		printf 'package-lock=false\n' >"$HOME/.npmrc"
		"$EZLM" connect dsh 2>&1 | e2e_redact || fail "ezlm connect dsh"
		node=$(ls "$EZLM_HOME"/tools/node/*/bin/node 2>/dev/null | head -1)
		echo "node: $node $("$node" --version 2>&1)"
		[ -n "$node" ] || fail "EZLM didn't install its Node"
		grep -E 'node|Node' "$EZLM_HOME/logs/integrate-dsh.log" | grep -iE 'sha|checksum|install|download' | cut -c1-200 | head -8
		prefix=$(ls -d "$EZLM_HOME"/tools/dsh/0.2.1-rc.5 2>/dev/null)
		lock="$prefix/package-lock.json"
		[ -f "$lock" ] || fail "npm wrote no package-lock.json (the home's .npmrc says package-lock=false)"
		want=$(sed -n '/"tool": *"dsh"/,/}/p' "$tools_json" | sed -n 's/.*"integrity": *"\([^"]*\)".*/\1/p' | head -1)
		got=$(awk '/"node_modules\/@deepseek-ai\/dsh": \{/{f=1} f&&/"integrity"/{print; exit}' "$lock" | sed 's/.*"integrity": *"\([^"]*\)".*/\1/')
		gotv=$(awk '/"node_modules\/@deepseek-ai\/dsh": \{/{f=1} f&&/"version"/{print; exit}' "$lock" | sed 's/.*"version": *"\([^"]*\)".*/\1/')
		echo "lock: @deepseek-ai/dsh $gotv $got"
		echo "pin:  @deepseek-ai/dsh 0.2.1-rc.5 $want"
		[ -n "$want" ] && [ "$got" = "$want" ] && [ "$gotv" = 0.2.1-rc.5 ] || fail "the lock doesn't match tools.json"
		cat "$prefix/.ezlm-install.json" 2>/dev/null
		d=$(doctor dsh)
		echo "$d" | grep -E '"(state|version|source|bin|preflight)"'
		grep -q '"source": "ezlm"' <<<"$d" && grep -q '"preflight": "ok"' <<<"$d" || fail "doctor: $d"
		du -sh "$EZLM_HOME/tools/node" "$EZLM_HOME/tools/dsh" 2>/dev/null
		;;
	hermes)
		case_env hermes
		shims="$CASE_DIR/shims"
		mkdir -p "$shims"
		for s in sudo brew launchctl xcode-select softwareupdate tccutil osascript open; do
			printf '#!/bin/sh\necho "$(date +%%s) %s $*" >> "%s/called.log"\necho "%s: refused by the EZLM e2e run" >&2\nexit 1\n' "$s" "$CASE_DIR" "$s" >"$shims/$s"
			chmod +x "$shims/$s"
		done
		printf 'export PATH="%s:$PATH"\n' "$shims" >"$HOME/.bash_profile"
		"$EZLM" connect hermes --default 2>&1 | e2e_redact || fail "ezlm connect hermes"
		if [ -s "$CASE_DIR/called.log" ]; then
			echo "the installer called these (all refused):"
			cat "$CASE_DIR/called.log"
			grep -v ' sudo -n -v$' "$CASE_DIR/called.log" | grep -q . && fail "the installer reached for something outside its home"
		else
			echo "the installer called none of: sudo brew launchctl xcode-select softwareupdate tccutil osascript open"
		fi
		grep -E '^.{20} (install:|install exit|installer|the installer|left running|gateway)' "$EZLM_HOME/logs/integrate-hermes.log" | cut -c1-200 | head -12
		ls -la "$HOME/.local/bin" 2>&1 | head
		echo "rc files the installer wrote in the test home:"
		for f in "$HOME"/.zshrc "$HOME"/.zprofile "$HOME"/.bashrc "$HOME"/.profile; do [ -f "$f" ] && echo "  $f: $(grep -c . "$f") lines"; done
		ls "$HOME/Library/LaunchAgents" 2>/dev/null && fail "a LaunchAgent was written in the test home"
		d=$(doctor hermes)
		echo "$d" | grep -E '"(state|version|source|bin|preflight)"'
		grep -q '"state": "connected"' <<<"$d" && grep -q '"preflight": "ok"' <<<"$d" || fail "doctor: $d"
		du -sh "$HERMES_HOME" 2>/dev/null
		# A hermes chat leaves an idle gateway for this home: stop it by the
		# PID it recorded, only when that record names this home (never by
		# name). connect_e2e.sh also stops anything left running from the
		# run's folder.
		if [ -f "$HERMES_HOME/gateway.pid" ]; then
			gp=$(/usr/bin/python3 -c 'import json,os,sys; d=json.load(open(sys.argv[1])); print(d.get("pid","") if os.path.realpath(d.get("hermes_home","")) == os.path.realpath(sys.argv[2]) else "")' "$HERMES_HOME/gateway.pid" "$HERMES_HOME" 2>/dev/null)
			if [ -n "$gp" ] && kill -0 "$gp" 2>/dev/null; then
				echo "stopping this home's gateway, pid $gp"
				kill -TERM "$gp" 2>/dev/null
			fi
		fi
		;;
	*) fail "unknown case" ;;
	esac
done
[ "$failed" = 0 ] && echo "fresh.sh: ok"
exit $failed
