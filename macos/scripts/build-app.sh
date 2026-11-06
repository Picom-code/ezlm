#!/bin/bash
# Builds EZLM.app: SwiftUI front end + universal Go engine in Resources.
set -euo pipefail
cd "$(dirname "$0")/.."
ROOT=$(cd .. && pwd)
OUT=${OUT:-build}
APP="$OUT/EZLM.app"
# The app's own version (Info.plist). The engine's version, which update
# checks compare, is const Version in internal/version/version.go.
VERSION=${VERSION:-0.1.11}

swift build -c release --arch arm64 --arch x86_64
BIN=$(swift build -c release --arch arm64 --arch x86_64 --show-bin-path)/EZLM

# Problem reports file GitHub issues with a token kept outside the repo
# (~/.config/ezlm/report.token). Without it, reports open GitHub in the browser.
LDFLAGS="-s -w"
TOKEN_FILE="$HOME/.config/ezlm/report.token"
if [ -f "$TOKEN_FILE" ]; then
  REPORT_TOKEN=$(tr -d '[:space:]' < "$TOKEN_FILE")
  if [ -n "$REPORT_TOKEN" ]; then
    LDFLAGS="$LDFLAGS -X github.com/picom-code/ezlm/internal/report.Token=$REPORT_TOKEN"
    echo "Problem reports: filed directly (token from $TOKEN_FILE)" >&2
  fi
  unset REPORT_TOKEN
fi

ENGINE_TMP="$(pwd)/$OUT.engine" # next to the app build, not in the repo root
( cd "$ROOT" && GOOS=darwin GOARCH=arm64 go build -ldflags "$LDFLAGS" -o "$ENGINE_TMP-arm64" ./cmd/ezlm \
             && GOOS=darwin GOARCH=amd64 go build -ldflags "$LDFLAGS" -o "$ENGINE_TMP-amd64" ./cmd/ezlm )
unset LDFLAGS

rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp "$BIN" "$APP/Contents/MacOS/EZLM"
lipo -create "$ENGINE_TMP-arm64" "$ENGINE_TMP-amd64" -output "$APP/Contents/Resources/ezlm-engine"
rm -f "$ENGINE_TMP-arm64" "$ENGINE_TMP-amd64"

# Vendor logos (see Logos/NOTICE.md)
cp -R Logos "$APP/Contents/Resources/Logos"

# Icon
ICON=$(mktemp -d)/EZLM.iconset
mkdir -p "$ICON"
swift scripts/icon.swift "$ICON/base.png"
for s in 16 32 128 256 512; do
  sips -z $s $s "$ICON/base.png" --out "$ICON/icon_${s}x${s}.png" >/dev/null
  sips -z $((s*2)) $((s*2)) "$ICON/base.png" --out "$ICON/icon_${s}x${s}@2x.png" >/dev/null
done
rm "$ICON/base.png"
iconutil -c icns "$ICON" -o "$APP/Contents/Resources/AppIcon.icns"

cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>EZLM</string>
  <key>CFBundleDisplayName</key><string>EZLM</string>
  <key>CFBundleIdentifier</key><string>com.ezlm.app</string>
  <key>CFBundleExecutable</key><string>EZLM</string>
  <key>CFBundleIconFile</key><string>AppIcon</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>$VERSION</string>
  <key>CFBundleVersion</key><string>$VERSION</string>
  <key>LSMinimumSystemVersion</key><string>14.0</string>
  <key>NSHighResolutionCapable</key><true/>
  <key>LSApplicationCategoryType</key><string>public.app-category.developer-tools</string>
  <key>NSAppleEventsUsageDescription</key><string>Model mode asks the apps you choose to quit, so your AI model gets more memory.</string>
</dict></plist>
PLIST

# Ad-hoc signature for local runs; releases are signed with a Developer ID and notarized.
codesign --force --deep --sign "${SIGN_ID:--}" "$APP" >/dev/null
echo "$APP"
