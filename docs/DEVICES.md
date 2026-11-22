# Device discovery: Milestone 2

Implemented scope: read-only discovery adapters, offline inventory inspection, normalized output and tests. Signing, pairing, device installation and launch are not implemented. No Apple account credentials are read or requested.

## Commands

```sh
# On a Mac with full Xcode:
apprivet devices --json
apprivet devices --simulators --json
apprivet devices --timeout 45s

# On any CLI host; these are explicitly offline, not live discovery:
apprivet devices --from-json coredevices.json --json
apprivet devices --from-json simulators.json --simulators --json
```

Default discovery invokes `/usr/bin/xcrun devicectl list devices --json-output <private-temp-file>`. Simulator discovery invokes `/usr/bin/xcrun simctl list devices --json`. No shell, arbitrary arguments, project scripts, account APIs, pairing commands or device mutations are invoked. Full Xcode is required for live operation; Windows/Linux return `unsupported_platform` unless an offline report is provided. Xcode selection follows the system setting, not an inherited DEVELOPER_DIR override.

The default timeout is 30 seconds, with a positive maximum of 5 minutes. Cancellation and deadlines use the existing runner. Temporary devicectl reports are precreated with mode 0600 inside an exclusively created private directory and removed on every normal success/error/cancellation path. Abrupt termination of AppRivet itself can leave OS temporary files. POSIX permission semantics apply on macOS; Windows offline reads rely on existing filesystem ACLs.

## Inventory contract

The existing schema-1 envelope contains `data.tool`, `tool_version` / `tool_json_version` when supplied, `source`, `live`, and `devices`.

- Live commands set `source: "local"`, `live: true`; offline inspection sets `source: "file"`, `live: false`. An offline file cannot become authorization for a later operation.
- `id` is the CoreDevice identifier for devicectl, or the simulator UDID for simctl. A hardware `udid` is separate when present. Do not interchange IDs or identify a device by its name.
- `kind` is `physical`, `simulator`, or `unknown`. CoreDevice classification uses explicit `reality`, never an identifier-shape guess. simctl records are simulators.
- `name`, `platform`, `model`, `os_version`, `boot_state`, `connection_state`, `pairing_state`, `transport`, and `developer_mode` preserve the reported values; missing values become `unknown`. Unknown future states remain visible. Display text is redacted for recognized secrets, control/format characters removed, whitespace collapsed, and length capped at 512 characters.
- simctl also supplies `runtime` and optional `available`; missing availability is omitted rather than inferred false. A booted simulator is not labeled physically connected.
- Inventories include remembered/disconnected, unavailable and non-iOS records. They are sorted by ID. This is what the selected tool reports, not proof that devices are physically attached, paired, reachable or installable.
- An empty valid inventory succeeds. Missing/invalid structures, duplicate IDs, failed outcomes or malformed records fail the whole inventory; no silently partial list is returned. Unknown additive fields are ignored.

devicectl's legacy property dictionaries and the modern `properties.hardware/state/software/connection` layout are supported provisionally. Modern values take precedence, with fallback for omitted values. Declared JSON versions 1-5 are accepted; missing version metadata is allowed for legacy reports; other declared versions produce `unsupported_schema`. This is a parser policy, not a claim of live testing every Xcode release.

Only selected fields are emitted. Raw environment metadata, serial numbers, network addresses, paths, capability dumps and arbitrary error payloads are not exported. IDs and user-assigned names still identify devices: review normalized output before sharing. Redaction cannot guarantee removal of arbitrary secrets embedded in names.

File reports are limited to 4 MiB and 4096 records. Nonregular files and final-component symlinks are rejected; a bounded read checks file identity again after opening. The parent filesystem and installed Apple tools remain trusted; this is not protection from a hostile process running as the same OS user. Live simctl stdout has the runner's 256 KiB cap; truncation is an error, never a partial success. Larger complete simctl files can be inspected offline up to the file limit.

Structured stdout is captured separately from redacted diagnostic stderr and is never serialized through runner diagnostics. Parsing and field selection happen before normalized output is rendered. Raw tool JSON/logs are not included in discovery error responses.

## Errors

| Code | Exit | Meaning / next action |
| --- | --- | --- |
| invalid_arguments | 2 | Correct flags or timeout |
| unsupported_platform | 3 | Use a Mac for live discovery or an offline report |
| unsupported_schema | 3 | New declared devicectl JSON version; update adapter using sanitized evidence |
| report_read_failed / report_io_error | 4 | Inspect file type/permissions or temporary directory access |
| invalid_report / report_too_large | 4 | Report was rejected; no inventory accepted |
| tool_failed | 5 | Verify full Xcode setup/selection with doctor and tool help |
| discovery_failed | 5 | devicectl did not report success; check tool/device setup |
| timeout / cancelled | 124 / 130 | No inventory accepted; retry deliberately |

## Research and fixture provenance

Reviewed 2026-11-22:

- [Apple command-line reference](https://developer.apple.com/documentation/xcode/xcode-command-line-tool-reference) identifies devicectl and simctl as full-Xcode tools. Installed `xcrun devicectl help list devices` and `xcrun simctl help list` remain the authority for the selected version's command syntax.
- [Apple Xcode 27 release notes](https://developer.apple.com/documentation/xcode-release-notes/xcode-27-release-notes) describe JSON v5's deprecated property dictionaries, replacement `properties` layout, and newer stdout support. AppRivet retains file-based devicectl output for older tooling compatibility.
- [First-hand tool observations in auto-mobile issue 8623](https://github.com/kaeawc/auto-mobile/issues/8623) describe the modern field mapping and simulator records in CoreDevice output. This informed an original parser, not imported implementation code. AppRivet does not assume devicectl enumerates all simulators; use simctl for that inventory.
- [Flutter's own CoreDevice tests](https://github.com/flutter/flutter/blob/master/packages/flutter_tools/test/general.shard/ios/core_devices_test.dart) document the legacy commandType/outcome/result envelope and older reports without jsonVersion. No fixture or source implementation was copied.
- [dotnet/macios-devtools device-discovery report](https://github.com/dotnet/macios-devtools/issues/195) reports sparse older-device records and pairing state under the connection dictionary, reinforcing preservation of unknown values.

All committed fixtures are original synthetic examples. No AppRivet live device capture exists yet. Public reports inform assumptions; they do not validate AppRivet on hardware.

## Live validation still required

| Scenario | Required evidence | Current status |
| --- | --- | --- |
| Selected full Xcode with no physical device | Tool/Xcode versions, successful empty or mixed listing; human and JSON output agree | Unverified |
| Physical iPhone/iPad over USB | Compare CoreDevice ID, UDID, name, reality, platform and state with Apple's output | Unverified |
| Wi-Fi device / disconnected remembered device | Verify actual transport/connection/pairing values; no assumed attachment | Unverified |
| Unpaired/locked/Developer Mode disabled device | Read-only listing behavior and clear unknown/unavailable states; no silent pairing | Unverified |
| Older unsupported device with sparse metadata | Listing remains useful and does not manufacture installation readiness | Unverified |
| Mixed simulator and physical entries in devicectl | Correct explicit reality mapping; no duplicate-ID guesswork | Unverified |
| Legacy and current modern-only JSON on real Xcode | Capture sanitized fixtures, verify declared version and actual mappings | Unverified |
| simctl booted/shutdown/unavailable inventory | Compare IDs/runtime/availability, use a listed UUID with test adapter | Unverified |
| Mac cancellation/deadline | No lingering owned processes or temporary report files | Unix behavior tested on Linux; Mac unverified |

Capture locally on a Mac without account credentials:

```sh
xcodebuild -version
xcrun devicectl help list devices
xcrun simctl help list
xcrun devicectl list devices --json-output "$TMPDIR/coredevices-private.json"
xcrun simctl list devices --json > "$TMPDIR/simulators-private.json"
apprivet devices --json
apprivet devices --simulators --json
apprivet devices --from-json "$TMPDIR/coredevices-private.json" --json
```

Raw captures contain sensitive metadata. Keep them outside Git; replace IDs/names and remove serials, hostnames, addresses, account/environment data and paths before proposing a fixture. Preserve field types and record the exact tool versions and transformations. Do not call a sanitized synthetic reconstruction a live capture.

Mac CI now contains read-only discovery smoke checks. A hosted runner with no physical hardware can validate tool invocation and an empty/simulator response only; it cannot establish physical-device support, signing or installation. M2 is not fully accepted until the relevant hardware matrix is exercised.
