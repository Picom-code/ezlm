# lib.sh: helpers for the e2e step files (source it). They expect the
# harness's isolated environment (connect_e2e.sh) or an equally isolated one:
# HOME, the XDG folders, HERMES_HOME, DSH_HOME, OPENCODE_CONFIG_DIR and
# EZLM_HOME all in a scratch folder, and EZLM set to the ezlm binary.
#
#   e2e_launch <id> [mode]      App.Launch through the engine's HTTP API, the
#                               way the Mac app and the web page call it;
#                               prints the job's events (keys removed)
#   e2e_tui_drive <id> <ready> <exit-keys> <out-dir> [timeout] [tuiprobe flags...]
#                               runs the launcher EZLM wrote for <id> in a
#                               120x40 pseudo-terminal: waits for <ready>,
#                               types the check prompt, waits for the nonce of
#                               a fresh ezlm-check.txt in the workspace, sends
#                               the exit keys and checks the exit file says 0
#   e2e_redact                  filter: replaces key-shaped words
#
# Nothing here prints a key: engine output goes through e2e_redact.

e2e_redact() { sed -E 's/(ezlm-[A-Za-z0-9]{16,}|sk-[A-Za-z0-9_-]{20,})/ezlm-REDACTED/g'; }

e2e_workspace() { printf '%s\n' "$HOME/EZLM Workspace"; }

# e2e_engine_start starts `ezlm engine` on a fifo (it runs until its stdin
# closes) and sets ENGINE_URL, ENGINE_TOKEN and ENGINE_PID.
e2e_engine_start() {
	local dir
	dir=$(mktemp -d "${TMPDIR:-/tmp}/ezlm-engine.XXXXXX")
	mkfifo "$dir/in"
	"$EZLM" engine <"$dir/in" >"$dir/out" 2>"$dir/err" &
	ENGINE_PID=$!
	exec 7>"$dir/in"
	ENGINE_DIR=$dir
	local i
	for i in $(seq 1 100); do
		[ -s "$dir/out" ] && break
		sleep 0.1
	done
	ENGINE_URL=$(sed -n 's/.*"url":"\([^"]*\)".*/\1/p' "$dir/out" | head -1)
	ENGINE_TOKEN=$(sed -n 's/.*"token":"\([^"]*\)".*/\1/p' "$dir/out" | head -1)
	[ -n "$ENGINE_URL" ] && [ -n "$ENGINE_TOKEN" ]
}

e2e_engine_stop() {
	[ -n "${ENGINE_PID:-}" ] || return 0
	exec 7>&-
	local i
	for i in $(seq 1 50); do
		kill -0 "$ENGINE_PID" 2>/dev/null || break
		sleep 0.1
	done
	kill -TERM "$ENGINE_PID" 2>/dev/null
	wait "$ENGINE_PID" 2>/dev/null
	rm -rf "$ENGINE_DIR"
	ENGINE_PID=
}

e2e_engine_call() { # method path [json]
	curl -sS -X "$1" -H "X-EZLM-Token: $ENGINE_TOKEN" -H 'Content-Type: application/json' ${3:+-d "$3"} "$ENGINE_URL$2"
}

# e2e_job <job> <method> <path> <json>: starts a job and prints its events
# until it is done; returns 0 when it ended ok.
e2e_job() {
	local job=$1 events state=1
	events=$(mktemp "${TMPDIR:-/tmp}/ezlm-events.XXXXXX")
	curl -sSN --max-time "${E2E_JOB_TIMEOUT:-900}" "$ENGINE_URL/api/events?t=$ENGINE_TOKEN" >"$events" 2>/dev/null &
	local cpid=$!
	sleep 0.3
	e2e_engine_call "$2" "$3" "$4" | e2e_redact
	echo
	local i
	for i in $(seq 1 $((${E2E_JOB_TIMEOUT:-900} * 5))); do
		if grep -q "\"job\":\"$job\",\"step\":\"done\"" "$events"; then
			break
		fi
		kill -0 "$cpid" 2>/dev/null || break
		sleep 0.2
	done
	kill "$cpid" 2>/dev/null
	wait "$cpid" 2>/dev/null
	grep '^data:' "$events" | grep "\"job\":\"$job\"" | grep -v '"state":"progress"' | sed 's/^data: //' |
		sed -E 's/"data":\{.*\}\}$/"data":"(status)"}/' | e2e_redact
	grep -q "\"job\":\"$job\",\"step\":\"done\",\"state\":\"ok\"" "$events" && state=0
	rm -f "$events"
	return $state
}

e2e_launch() { # id [mode]
	local body="{\"id\":\"$1\"${2:+,\"mode\":\"$2\"}}"
	e2e_engine_start || { echo "the engine didn't start" >&2; return 1; }
	e2e_job launch POST /api/launch "$body"
	local rc=$?
	e2e_engine_stop
	return $rc
}

# e2e_tui_drive: see the header.
e2e_tui_drive() {
	local id=$1 ready=$2 keys=$3 out=$4 timeout=${5:-180s}
	shift 5 2>/dev/null || shift $#
	local launcher
	launcher=$(ls "$EZLM_HOME/launch/$id".command "$EZLM_HOME/launch/$id".sh 2>/dev/null | head -1)
	[ -n "$launcher" ] || { echo "no launcher for $id in $EZLM_HOME/launch" >&2; return 1; }
	mkdir -p "$out"
	cp "$launcher" "$out/"
	local ws nonce
	ws=$(e2e_workspace)
	mkdir -p "$ws"
	nonce="EZLM-CHECK-$(LC_ALL=C tr -dc 'A-Z2-7' </dev/urandom | head -c 8)"
	printf '%s\n' "$nonce" >"$ws/ezlm-check.txt"
	rm -f "$EZLM_HOME/launch/$id.exit"
	local prompt="Use your file-reading tool to open ezlm-check.txt in the current folder. Reply with exactly the text inside it and nothing else."
	"${TUIPROBE:-$E2E_BIN/tuiprobe}" -cmd "$launcher" -cols 120 -rows 40 -ready "$ready" -wait "${E2E_READY_WAIT:-60s}" \
		-type "$prompt" -type-delay "${E2E_TYPE_DELAY:-5ms}" -expect "$nonce" -timeout "$timeout" -settle "${E2E_SETTLE:-10s}" \
		-exit "$keys" -exit-wait 20s -json -out "$out/screen.txt" "$@" >"$out/probe.json" 2>"$out/probe.err"
	local rc=$?
	rm -f "$ws/ezlm-check.txt"
	sed -i.bak -E 's/(ezlm-[A-Za-z0-9]{16,}|sk-[A-Za-z0-9_-]{20,})/ezlm-REDACTED/g' "$out/screen.txt" "$out/probe.json" 2>/dev/null
	rm -f "$out"/*.bak
	cat "$out/probe.err"
	if [ -f "$EZLM_HOME/launch/$id.exit" ]; then
		cp "$EZLM_HOME/launch/$id.exit" "$out/"
		echo "exit file: $(cat "$EZLM_HOME/launch/$id.exit")"
		read -r code _ <"$EZLM_HOME/launch/$id.exit"
		[ "$code" = 0 ] || rc=1
	else
		echo "no exit file was written"
		rc=1
	fi
	echo "nonce $nonce: $(grep -o "$nonce" "$out/screen.txt" | wc -l | tr -d ' ') times on screen"
	# The screen can show the nonce in the tool's output before the answer:
	# the app's own record of the turn says whether the answer had it.
	if type "e2e_answer_$id" >/dev/null 2>&1; then
		if "e2e_answer_$id" "$nonce" >"$out/answer.txt" 2>&1; then
			echo "answer recorded by the app: $(head -c 300 "$out/answer.txt" | tr '\n' ' ')"
		else
			echo "the app recorded no answer with the nonce: $(head -c 300 "$out/answer.txt")"
			rc=1
		fi
	fi
	return $rc
}

# e2e_answer_opencode: the session database has a text part (the model's
# answer, not the tool's output) with the nonce. 1.x and 2.x store parts as
# JSON in different tables; the dump covers both and the WAL.
e2e_answer_opencode() {
	local db="${XDG_DATA_HOME:-$HOME/.local/share}/opencode/opencode.db"
	sqlite3 -readonly "$db" .dump 2>/dev/null | grep -o "\"type\":\"text\",\"text\":\"[^\"]*$1[^\"]*\"" | head -3 | grep .
}
