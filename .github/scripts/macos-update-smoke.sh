#!/usr/bin/env bash
set -euo pipefail

if [[ $(uname -s) != Darwin || ${RUNNER_ENVIRONMENT:-} != github-hosted || -z ${RUNNER_TEMP:-} ]]; then
  echo 'Run this smoke test on a fresh GitHub-hosted macOS runner.' >&2
  exit 1
fi

scripts=$(cd "$(dirname "$0")" && pwd)
work=$(mktemp -d "$RUNNER_TEMP/apprivet-update-smoke.XXXXXX")
work=$(cd "$work" && pwd -P)
archive="$work/0.4.8.zip"
app="$work/AppRivet.app"
info="$app/Contents/Info.plist"
executable="$app/Contents/MacOS/AppRivetDesktop"
frameworks="$app/Contents/Frameworks"
fixture_log="$work/fixture.log"
feed=https://raw.githubusercontent.com/fpokrzywa/AppRivet/main/updates/appcast.xml
public_key='H0hoVJciH/vY/oz64yWgXm6UL8S+tgBf6ow0cwKY6gE='
release_download=https://github.com/fpokrzywa/AppRivet/releases/download
old_archive_sha256=275c1a3c1132f8ce6237d1a108687d6aa372befd172c733e78aa5d74ec704fed
target_version=0.4.9
target_build=13
target_url="$release_download/v$target_version/AppRivet-macOS-$target_version.zip"

show_diagnostics() {
  local status=$?
  if [[ -s "$fixture_log" ]]; then cat "$fixture_log"; fi
  if [[ $status -ne 0 ]]; then
    /usr/bin/log show --last 6m --style compact --predicate 'category == "Sparkle"' || true
  fi
}
trap show_diagnostics EXIT

fail() {
  printf '%s\n' "$1" >&2
  exit 1
}

plist_value() {
  plutil -extract "$1" raw -o - "${2:-$info}"
}

verify_bundle() {
  [[ $(plist_value CFBundleShortVersionString) == "$1" ]] || fail "Unexpected app version, expected $1."
  [[ $(plist_value CFBundleVersion) == "$2" ]] || fail "Unexpected app build number, expected $2."
  [[ $(plist_value CFBundleIdentifier) == io.github.fpokrzywa.apprivet.desktop ]] || fail 'Unexpected bundle identifier.'
  [[ $(plist_value CFBundleExecutable) == AppRivetDesktop ]] || fail 'Unexpected bundle executable.'
  [[ $(plist_value SUFeedURL) == "$feed" ]] || fail 'The bundle must use the production appcast.'
  [[ $(plist_value SUPublicEDKey) == "$public_key" ]] || fail 'The bundle must pin the production update key.'
  [[ $(plist_value SUVerifyUpdateBeforeExtraction) == true ]] || fail 'Archive signature verification must precede extraction.'
  codesign --verify --deep --strict "$app"
}

if pgrep -x AppRivetDesktop >/dev/null; then
  fail 'An AppRivet process is already running on this runner.'
fi

echo 'Extracting the published 0.4.8 fixture.'
curl --fail --location --silent --show-error --proto '=https' --proto-redir '=https' \
  --connect-timeout 15 --max-time 60 --retry 2 --retry-max-time 90 \
  "$release_download/v0.4.8/AppRivet-macOS-0.4.8.zip" -o "$archive"
printf '%s  %s\n' "$old_archive_sha256" "$archive" | shasum -a 256 -c -
ditto -x -k "$archive" "$work"
verify_bundle 0.4.8 12
[[ $(plist_value CFBundleShortVersionString "$frameworks/Sparkle.framework/Resources/Info.plist") == 2.10.1 ]] \
  || fail 'Unexpected Sparkle framework version in the published fixture.'

# Replace only the host executable. Keep the release bundle, Sparkle framework, and update settings intact.
cp "$info" "$work/original-info.plist"
xcrun --sdk macosx clang -fobjc-arc -Wall -Wextra -Werror -mmacosx-version-min=13.0 \
  -F "$frameworks" -framework AppKit -framework Sparkle \
  -Wl,-rpath,@executable_path/../Frameworks "$scripts/macos-update-smoke.m" -o "$work/driver"
mv -f "$work/driver" "$executable"
codesign --force --sign - "$app"
cmp "$info" "$work/original-info.plist"
verify_bundle 0.4.8 12
driver_sha256=$(shasum -a 256 "$executable" | cut -d ' ' -f 1)

echo "Starting Sparkle with its packaged HTTPS feed. Expecting $target_version (build $target_build)."
touch "$fixture_log"
open -n -g --stderr "$fixture_log" "$app" --args \
  -SparkleSmokeVersion "$target_version" -SparkleSmokeBuild "$target_build" -SparkleSmokeURL "$target_url" \
  -SUEnableAutomaticChecks NO

# Wait for the old PID to exit and a new process to map the executable at the fixture path.
deadline=$((SECONDS + 240))
while (( SECONDS < deadline )); do
  if grep -q '^Sparkle smoke failure:' "$fixture_log"; then
    fail 'The Sparkle driver reported a failure.'
  fi
  old_pid=$(awk '/^Sparkle smoke fixture PID [0-9]+$/ { print $5; exit }' "$fixture_log")
  if [[ -n "$old_pid" ]] && ! kill -0 "$old_pid" 2>/dev/null; then
    version=$(plist_value CFBundleShortVersionString 2>/dev/null || true)
    if [[ "$version" == "$target_version" ]]; then
      for pid in $(pgrep -x AppRivetDesktop || true); do
        if /usr/sbin/lsof -a -d txt -p "$pid" -Fn 2>/dev/null | grep -Fx "n$executable" >/dev/null; then
          verify_bundle "$target_version" "$target_build"
          installed_sha256=$(shasum -a 256 "$executable" | cut -d ' ' -f 1)
          [[ "$installed_sha256" != "$driver_sha256" ]] || fail 'The fixture driver is still installed.'
          printf 'Sparkle replaced 0.4.8 with %s and relaunched %s (PID %s).\n' "$target_version" "$executable" "$pid"
          exit 0
        fi
      done
    fi
  fi
  sleep 2
done

fail "Timed out waiting for $target_version to replace 0.4.8 and relaunch from $app."
