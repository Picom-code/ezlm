#!/bin/bash
# Packages build/EZLM.app into a disk image with an Applications shortcut, so
# installing is a drag. Run scripts/build-app.sh first.
#
#   scripts/make-dmg.sh         -> build/EZLM.dmg        (the name releases use)
#   scripts/make-dmg.sh 0.1.1   -> build/EZLM-0.1.1.dmg
#
# The in-app updater takes any release file named EZLM*.dmg and expects
# EZLM.app at the top of the disk image.
set -euo pipefail
cd "$(dirname "$0")/.."
APP="build/EZLM.app"
[ -d "$APP" ] || { echo "build/EZLM.app is missing: run scripts/build-app.sh first" >&2; exit 1; }
OUT="build/EZLM${1:+-$1}.dmg"
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT
cp -R "$APP" "$STAGE/"
ln -s /Applications "$STAGE/Applications"
rm -f "$OUT"
hdiutil create -volname "EZLM" -srcfolder "$STAGE" -ov -format UDZO -fs HFS+ "$OUT" >/dev/null
echo "$OUT ($(du -h "$OUT" | cut -f1))"
