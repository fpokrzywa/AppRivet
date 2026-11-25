#!/usr/bin/env bash
set -euo pipefail

if [[ $(uname -s) != Darwin ]]; then
  echo 'Packaging the desktop app requires macOS with full Xcode and Go.' >&2
  exit 1
fi

repo=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo"
destination=${1:-"$repo/dist/macos"}
mkdir -p "$destination"
destination=$(cd "$destination" && pwd)
app="$destination/AppRivet.app"
archive="$destination/AppRivet-macOS.zip"
if [[ -e "$app" || -L "$app" || -e "$archive" || -L "$archive" ]]; then
  echo "Choose a new output folder. AppRivet packaging does not overwrite $app or $archive." >&2
  exit 1
fi

scratch=$(mktemp -d "${TMPDIR:-/tmp}/apprivet-package.XXXXXX")
trap 'rm -rf -- "$scratch"' EXIT
sdk=$(xcrun --sdk macosx --show-sdk-path)
for arch in arm64 x86_64; do
  go_arch=$arch
  if [[ $arch == x86_64 ]]; then go_arch=amd64; fi
  CGO_ENABLED=0 GOOS=darwin GOARCH=$go_arch go build -trimpath -o "$scratch/cli-$arch" ./cmd/apprivet
  xcrun swiftc -parse-as-library -swift-version 5 -O -whole-module-optimization \
    -sdk "$sdk" -target "$arch-apple-macosx13.0" \
    desktop/Workflow.swift desktop/AppRivetDesktop.swift -o "$scratch/desktop-$arch"
done

staged="$scratch/AppRivet.app"
mkdir -p "$staged/Contents/MacOS"
cp desktop/Info.plist "$staged/Contents/Info.plist"
plutil -lint "$staged/Contents/Info.plist"
xcrun lipo -create "$scratch/desktop-arm64" "$scratch/desktop-x86_64" -output "$staged/Contents/MacOS/AppRivetDesktop"
xcrun lipo -create "$scratch/cli-arm64" "$scratch/cli-x86_64" -output "$staged/Contents/MacOS/apprivet"
chmod 755 "$staged/Contents/MacOS/AppRivetDesktop" "$staged/Contents/MacOS/apprivet"
codesign --force --sign - "$staged/Contents/MacOS/apprivet"
codesign --force --sign - "$staged"
codesign --verify --deep --strict "$staged"
ditto -c -k --sequesterRsrc --keepParent "$staged" "$scratch/AppRivet-macOS.zip"
ditto "$staged" "$app"
cp "$scratch/AppRivet-macOS.zip" "$archive"
printf 'Packaged %s\nArchive: %s\n' "$app" "$archive"
