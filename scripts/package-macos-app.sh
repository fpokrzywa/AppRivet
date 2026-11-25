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
sparkle_version=2.10.1
sparkle_sha256=1a856d2a9edb8e628de88121d7be04cf0e2167f8641650b78fca00b14244cf27
curl --fail --location --silent --show-error --retry 3 \
  "https://github.com/sparkle-project/Sparkle/releases/download/$sparkle_version/Sparkle-$sparkle_version.tar.xz" \
  -o "$scratch/sparkle.tar.xz"
(cd "$scratch" && printf '%s  sparkle.tar.xz\n' "$sparkle_sha256" | shasum -a 256 -c -)
mkdir "$scratch/sparkle"
tar -xf "$scratch/sparkle.tar.xz" -C "$scratch/sparkle" ./Sparkle.framework ./LICENSE
sdk=$(xcrun --sdk macosx --show-sdk-path)
for arch in arm64 x86_64; do
  go_arch=$arch
  if [[ $arch == x86_64 ]]; then go_arch=amd64; fi
  CGO_ENABLED=0 GOOS=darwin GOARCH=$go_arch go build -trimpath -o "$scratch/cli-$arch" ./cmd/apprivet
  xcrun swiftc -parse-as-library -swift-version 5 -O -whole-module-optimization \
    -sdk "$sdk" -target "$arch-apple-macosx13.0" \
    -F "$scratch/sparkle" -framework Sparkle -Xlinker -rpath -Xlinker @executable_path/../Frameworks \
    desktop/Workflow.swift desktop/AppRivetDesktop.swift -o "$scratch/desktop-$arch"
done

staged="$scratch/AppRivet.app"
mkdir -p "$staged/Contents/MacOS" "$staged/Contents/Resources" "$staged/Contents/Frameworks"
ditto "$scratch/sparkle/Sparkle.framework" "$staged/Contents/Frameworks/Sparkle.framework"
cp "$scratch/sparkle/LICENSE" "$staged/Contents/Resources/Sparkle-LICENSE.txt"
cp desktop/Info.plist "$staged/Contents/Info.plist"
cp internal/studio/ui/apprivet-mark-light.png "$staged/Contents/Resources/AppRivetMarkLight.png"
cp internal/studio/ui/apprivet-mark-dark.png "$staged/Contents/Resources/AppRivetMarkDark.png"
iconset="$scratch/AppRivet.iconset"
mkdir "$iconset"
logo="$staged/Contents/Resources/AppRivetMarkDark.png"
for points in 16 32 128 256 512; do
  pixels=$((points * 2))
  icon="$iconset/icon_${points}x${points}"
  sips -Z "$points" "$logo" --out "$icon.png" >/dev/null
  sips -Z "$pixels" "$logo" --out "${icon}@2x.png" >/dev/null
done
iconutil -c icns -o "$staged/Contents/Resources/AppRivet.icns" "$iconset"
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
