# Project status and handoff

Updated: 2026-11-22. Milestone 2 read-only device discovery is implemented. The physical-device signing/install/launch workflow is not complete.

Repository: [fpokrzywa/AppRivet](https://github.com/fpokrzywa/AppRivet). The owner changed visibility from private to public during this session; the change was verified. Standard hosted CI now starts, resolving the earlier billing admission failure. License selection remains open; no license, versioned binary release, payments or activation were added.

## Implemented

- Portable CLI: doctor, init, inspect, build, test, devices, version and help; schema-1 JSON, human output and stable failures.
- Original SwiftUI/Xcode template, shared scheme, XCTest target and example; trusted simulator build/test adapters.
- Read-only CoreDevice discovery via devicectl, separate simulator inventory via simctl, and offline JSON inspection on every CLI platform.
- Legacy/modern device normalization, conservative unknown states, explicit source/live provenance, privacy filtering, bounded reads/capture, private report cleanup, cancellation and deadlines.
- Duplicate-key/identifier rejection, null-precedence protection, original synthetic fixtures, adapter/CLI/security tests and parser fuzz target.
- Research, parity matrix, architecture, threat model, ADRs, roadmap, contribution/security documentation and CI.

## Evidence

| Check | Result |
| --- | --- |
| Current Windows tests, six packages, and go vet | Passed |
| Current Linux runtime tests, six packages in restricted local container | Passed |
| Linux parser fuzz smoke | 274,120 executions over 10 seconds without failure; no coverage guidance in this precompiled binary |
| Native Windows coverage-guided fuzz | 10,000 inputs passed; CI uses the same execution count with a 60-second outer deadline |
| Client cross-builds | macOS/Linux/Windows, amd64 and arm64 passed |
| Actual Windows CLI | Offline CoreDevice/simulator reports succeed with live=false; live discovery refuses with exit 3 |
| Hosted CI after public visibility change | [Run 45051791524](https://github.com/fpokrzywa/AppRivet/actions/runs/45051791524), commit 4f1ab4b: all four jobs passed, including Windows/Linux/macOS Go tests and Linux/macOS race checks |
| Mac discovery smoke | devicectl 506.7 / JSON v3 returned zero devices; simctl returned 132 shutdown simulators across iOS/tvOS/watchOS/xrOS |
| Original SwiftUI app build | Passed with Xcode 16.4 (16F6), iOS Simulator SDK 18.5, hosted macOS arm64 |
| Simulator XCTest / module-collision build | One XCTest passed on iPhone 16 Pro / iOS 18.5; generated SwiftUI-named app built successfully |
| Physical USB/Wi-Fi devices, pairing states and modern physical-device schema | Unverified; requires live hardware |
| Signing, install, launch, Apple account APIs, remote execution | Not implemented; no validation claim |

Toolchain: Go 1.27.4, installed portably at %LOCALAPPDATA%\AppRivet\toolchains\go1.27.4\go; global PATH unchanged. Windows executable: bin/apprivet.exe. Reproduction commands: [VERIFICATION](VERIFICATION.md). Device format, privacy rules and full live matrix: [DEVICES](DEVICES.md).

## Review and decisions

Independent review found two M2 parser defects, fixed with failing-then-passing tests: duplicate JSON keys could overwrite inventories, and explicit modern nulls could revive stale legacy states. Duplicate case aliases, including Unicode simple-fold aliases, are rejected too. The suite also exposed a test-only coverage-runtime stderr warning; the assertion now checks separation of stdout payload rather than assuming empty stderr. No deferred minor review findings.

The reviewer could not establish physical-device compatibility, Mac file/cancellation behavior or physical state mappings from fixtures. Hosted discovery smoke now covers basic Mac tool invocation; the detailed physical-device matrix remains open. Local Linux testing supplies Unix behavior evidence, not Apple hardware evidence.

Engineering decisions: continue in the clean dedicated repository; add simulator discovery and offline inspection as useful credential-free work; reject ambiguous reports rather than return partial or misleading inventory. Trade-off: explicit null known fields and future schema versions require an adapter update, and install/launch remain unavailable until their own authorization/isolation validation.

CI reliability observation: [run 45052339395](https://github.com/fpokrzywa/AppRivet/actions/runs/45052339395), attempt 1, passed all Go jobs but timed out during simctl discovery at 60 seconds. An unchanged Mac retry passed discovery, then was superseded by the final CI/documentation update while XCTest was running. This is an observed intermittent timing failure, not a parser rejection or a proven Apple-service root cause. No discovery retry or timeout increase was added. Preserve this evidence and investigate if it recurs.

## Next work

1. Keep all CI jobs green for subsequent code changes, including the bounded native Linux parser fuzz step. The first successful integration run above predates the final key-alias hardening and fuzz-step addition; the final follow-up run must also pass. Investigate recurrent simulator timeouts before claiming reliable unattended Mac operation. No billing workaround is currently required.
2. Validate USB/Wi-Fi/disconnected/unpaired/locked/Developer Mode-disabled and older devices against Apple's actual reports. Add carefully sanitized captures with provenance; current committed fixtures are synthetic.
3. Design and test the credential-free worker/signing boundary and trusted human approvals before enabling signing or device mutations. Discovery cannot authorize installation. Keep Apple passwords and private keys out of the CLI.
4. Continue M2 installation/launch only with the required hardware/signing evidence. Remote builds, MCP and release distribution retain their separate acceptance gates.
