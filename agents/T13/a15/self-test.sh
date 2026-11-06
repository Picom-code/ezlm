#!/bin/sh
# Self-test for agents/T13/assemble.py. Builds a fixture tree in a temp
# directory, runs the assembler, and checks the merged output. Nothing outside
# the temp directory is touched.
#
#   sh agents/T13/a15/self-test.sh
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
script="$here/../assemble.py"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

mkdir -p "$tmp/agents/T13/sections"
cd "$tmp/agents/T13/sections"

cat > a01.md <<'EOF'
# Install on Windows

1. Download **EZLM-Setup.exe** and open it.

## If Windows blocks it

Click **More info**, then **Run anyway**.
EOF

for n in 02 03 04 05 06 07 08; do
    printf '# Section %s\n\nText for %s.\n' "$n" "$n" > "a$n.md"
done

cat > a09.md <<'EOF'
# Errors from the engine

## "Cannot reach the engine"

**Message:** Cannot reach the engine
**What it means:** EZLM started the server but it did not answer in time.
**What to do:** Press Start server again.

## "Port 8080 is in use"

**What it means:** Another program holds the port.
EOF

cat > a10.md <<'EOF'
# Errors from scan

### "Not enough memory"

**What you see:** Not enough memory
**Why it happens:** The model needs more RAM than is free.
**Next step:** Close a few apps and scan again.

### "No model fits"

**What it means:** Nothing in the catalog fits.
**What to do:** Choose a smaller model.
EOF

cat > a11.md <<'EOF'
# Other errors

| Message | What it means | What to do |
| --- | --- | --- |
| "Key rejected" | The key does not match. | Copy the key again. |
| "Update failed" | The download broke. | Try the update again. |
EOF

cat > a12.md <<'EOF'
# App messages

## "EZLM can't verify the download"

**Message:**
The download was incomplete.

**What it means:**
The file did not arrive whole.

**What to do:**
Download it again.
EOF

cat > a13.md <<'EOF'
# FAQ

**Does it work offline?**

Yes, after the model downloads.
EOF

fail() {
    echo "self-test: FAIL: $1" >&2
    exit 1
}

# 1. The only gap is the entry with no action; everything else passes.
set +e
output=$(python3 "$script" --root "$tmp" --check 2>&1)
status=$?
set -e
[ "$status" -eq 1 ] || fail "--check exit was $status, expected 1"
printf '%s\n' "$output" | grep -q 'Port 8080 is in use' || fail "missing the expected gap"
[ "$(printf '%s\n' "$output" | grep -c 'missing:')" -eq 1 ] || fail "expected exactly one gap"
printf '%s\n' "$output" | grep -q 'missing section a14' && fail "a14 must not be required"

# 2. Writing produces the three documents in the right order.
python3 "$script" --root "$tmp" --report "$tmp/report.md" >/dev/null
for doc in BETA.md TROUBLESHOOTING.md FAQ.md; do
    [ -s "$tmp/agents/T13/$doc" ] || fail "$doc was not written"
done
grep -q '^# EZLM beta tester guide' "$tmp/agents/T13/BETA.md" || fail "BETA.md title"
grep -q '^## Install on Windows' "$tmp/agents/T13/BETA.md" || fail "BETA.md section heading"
grep -q '^### If Windows blocks it' "$tmp/agents/T13/BETA.md" || fail "BETA.md heading demotion"
grep -q '^### "Cannot reach the engine"' "$tmp/agents/T13/TROUBLESHOOTING.md" || fail "entry heading"
grep -q 'Key rejected' "$tmp/agents/T13/TROUBLESHOOTING.md" || fail "table entry"
grep -q 'Does it work offline' "$tmp/agents/T13/FAQ.md" || fail "FAQ content"
grep -q 'what to do' "$tmp/report.md" || fail "report gap"

# 3. A rerun changes nothing (no empty commits).
before=$(cat "$tmp/agents/T13/BETA.md" "$tmp/agents/T13/TROUBLESHOOTING.md" "$tmp/agents/T13/FAQ.md" | cksum)
python3 "$script" --root "$tmp" >/dev/null
after=$(cat "$tmp/agents/T13/BETA.md" "$tmp/agents/T13/TROUBLESHOOTING.md" "$tmp/agents/T13/FAQ.md" | cksum)
[ "$before" = "$after" ] || fail "rerun changed the output"

echo "self-test: ok"
