#!/usr/bin/env bash
set +x
set -euo pipefail

fail() {
  printf '%s\n' "$1" >&2
  exit 1
}

[[ $(uname -s) == Darwin && $# -eq 2 ]] || fail 'Usage on macOS: notarize-macos-app.sh <AppRivet.app> <new-output-dir>'
source_app=$1
destination=$2
[[ ! -e $destination && ! -L $destination ]] || fail 'Use a new output directory for the notarized assets.'

for variable in APPLE_TEAM_ID APPLE_DEVELOPER_ID_P12_BASE64 APPLE_DEVELOPER_ID_P12_PASSWORD \
  APPLE_NOTARY_KEY APPLE_NOTARY_KEY_ID APPLE_NOTARY_ISSUER_ID; do
  [[ -n ${!variable:-} ]] || fail "Missing required Apple release credential: $variable"
done
[[ $APPLE_TEAM_ID =~ ^[A-Z0-9]{10}$ ]] || fail 'APPLE_TEAM_ID must be the Apple Developer Team ID.'

info="$source_app/Contents/Info.plist"
[[ $(plutil -extract CFBundleIdentifier raw -o - "$info") == io.github.fpokrzywa.apprivet.desktop ]] \
  || fail 'Expected the AppRivet desktop bundle.'
version=$(plutil -extract CFBundleShortVersionString raw -o - "$info")
[[ $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail 'Expected an AppRivet version such as 0.4.10.'
codesign --verify --deep --strict "$source_app"

umask 077
scratch=$(mktemp -d "${TMPDIR:-/tmp}/apprivet-notary.XXXXXX")
keychain="$scratch/signing.keychain-db"
notary_key="$scratch/AuthKey.p8"
cleanup() {
  security delete-keychain "$keychain" >/dev/null 2>&1 || true
  rm -rf -- "$scratch"
}
trap cleanup EXIT

keychain_password=$(openssl rand -hex 32)
printf '%s' "$APPLE_DEVELOPER_ID_P12_BASE64" | base64 -D > "$scratch/certificate.p12"
printf '%s\n' "$APPLE_NOTARY_KEY" > "$notary_key"
unset APPLE_DEVELOPER_ID_P12_BASE64 APPLE_NOTARY_KEY
security create-keychain -p "$keychain_password" "$keychain"
security set-keychain-settings -lut 3600 "$keychain"
security unlock-keychain -p "$keychain_password" "$keychain"
security import "$scratch/certificate.p12" -k "$keychain" -f pkcs12 \
  -P "$APPLE_DEVELOPER_ID_P12_PASSWORD" -T /usr/bin/codesign
security set-key-partition-list -S apple-tool:,apple: -s -k "$keychain_password" "$keychain" >/dev/null
rm -f -- "$scratch/certificate.p12"
unset keychain_password APPLE_DEVELOPER_ID_P12_PASSWORD

identity=$(security find-identity -v -p codesigning "$keychain" | awk -v team="$APPLE_TEAM_ID" '
  length($2) == 40 && /"Developer ID Application: / && index($0, "(" team ")\"") { print $2; exit }
')
[[ -n $identity ]] || fail 'No valid Developer ID Application identity for this Apple team was found in the certificate export.'
sign=(codesign --force --timestamp --sign "$identity" --keychain "$keychain")

submit_and_staple() {
  local upload=$1 target=$2 report="$scratch/notary-result.json" submit_exit=0 status id
  local credentials=(--key "$notary_key" --key-id "$APPLE_NOTARY_KEY_ID" --issuer "$APPLE_NOTARY_ISSUER_ID")
  xcrun notarytool submit "$upload" "${credentials[@]}" --wait --timeout 15m --output-format json > "$report" || submit_exit=$?
  status=$(plutil -extract status raw -o - "$report" 2>/dev/null || true)
  id=$(plutil -extract id raw -o - "$report" 2>/dev/null || true)
  printf 'Apple notarization for %s ended with status %s. Submission %s.\n' "${upload##*/}" "${status:-unknown}" "${id:-unknown}"
  if [[ $submit_exit -ne 0 || $status != Accepted ]]; then
    if [[ $id =~ ^[a-fA-F0-9-]{36}$ ]]; then
      xcrun notarytool log "$id" "${credentials[@]}" || true
    fi
    fail 'Apple did not confirm acceptance. No release assets were created.'
  fi
  xcrun stapler staple "$target"
  xcrun stapler validate "$target"
}

app="$scratch/AppRivet.app"
ditto "$source_app" "$app"
sparkle="$app/Contents/Frameworks/Sparkle.framework/Versions/Current"
# Sparkle's Downloader must retain the entitlements from the pinned framework.
"${sign[@]}" --options runtime --preserve-metadata=entitlements "$sparkle/XPCServices/Downloader.xpc"
for component in "$sparkle/Autoupdate" "$sparkle/Updater.app" "$sparkle/XPCServices/Installer.xpc"; do
  "${sign[@]}" --options runtime "$component"
done
"${sign[@]}" --options runtime "$app/Contents/Frameworks/Sparkle.framework"
"${sign[@]}" --options runtime "$app/Contents/MacOS/apprivet"
"${sign[@]}" --options runtime "$app"
codesign --verify --deep --strict "$app"

ditto -c -k --sequesterRsrc --keepParent "$app" "$scratch/AppRivet-for-notary.zip"
submit_and_staple "$scratch/AppRivet-for-notary.zip" "$app"

archive="$scratch/AppRivet-macOS-$version.zip"
disk_image="$scratch/AppRivet-macOS.dmg"
ditto -c -k --sequesterRsrc --keepParent "$app" "$archive"
mkdir -m 755 "$scratch/disk-image"
ditto "$app" "$scratch/disk-image/AppRivet.app"
ln -s /Applications "$scratch/disk-image/Applications"
hdiutil create -quiet -volname AppRivet -fs HFS+ -format UDZO -srcfolder "$scratch/disk-image" "$disk_image"
"${sign[@]}" "$disk_image"
submit_and_staple "$disk_image" "$disk_image"

hdiutil verify "$disk_image"
codesign --verify --strict "$disk_image"
spctl --assess --type open --context context:primary-signature --verbose=2 "$disk_image"
ditto -x -k "$archive" "$scratch/unpacked"
unpacked="$scratch/unpacked/AppRivet.app"
codesign --verify --deep --strict "$unpacked"
xcrun stapler validate "$unpacked"
spctl --assess --type execute --verbose=2 "$unpacked"

mkdir "$destination"
cp "$archive" "$disk_image" "$destination/"
printf 'Verified notarized release assets in %s\n' "$destination"
