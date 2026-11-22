# Project status and handoff

Updated: 2026-11-22. Milestone 2 read-only discovery and portable source snapshot creation/verification are implemented. The physical-device workflow and remote worker are not complete.

Repository: [fpokrzywa/AppRivet](https://github.com/fpokrzywa/AppRivet). The owner changed visibility from private to public during this session; the change was verified. Standard hosted CI now starts, resolving the earlier billing admission failure. License selection remains open; no license, versioned binary release, payments or activation were added.

## Implemented

- Portable CLI: doctor, init, inspect, build, test, devices, version and help; schema-1 JSON, human output and stable failures.
- Original SwiftUI/Xcode template, shared scheme, XCTest target and example; trusted simulator build/test adapters.
- Read-only CoreDevice discovery via devicectl, separate simulator inventory via simctl, and offline JSON inspection on every CLI platform.
- Legacy/modern device normalization, conservative unknown states, explicit source/live provenance, privacy filtering, bounded reads/capture, private report cleanup, cancellation and deadlines.
- Duplicate-key/identifier rejection, null-precedence protection, original synthetic fixtures, adapter/CLI/security tests and parser fuzz target.
- Explicit source file selection, deterministic bounded archives, canonical integrity verification and optional expected SHA-256 checks on every CLI host. No extraction, execution, upload or authorization is implied.
- Research, parity matrix, architecture, threat model, ADRs, roadmap, contribution/security documentation and CI.

## Evidence

| Check | Result |
| --- | --- |
| Current Windows tests, seven packages, and go vet | Passed, including snapshot security/CLI regressions; snapshot coverage 83.5% |
| Linux runtime tests and six cross-builds for snapshots | All seven Linux package suites and macOS/Linux/Windows amd64/arm64 client builds passed |
| Linux parser fuzz smoke | 274,120 executions over 10 seconds without failure; no coverage guidance in this precompiled binary |
| Native Windows/Linux coverage-guided fuzz | 10,000 inputs passed on each; Linux CI uses a 60-second outer deadline |
| Client cross-builds | macOS/Linux/Windows, amd64 and arm64 passed |
| Actual Windows CLI | Offline CoreDevice/simulator reports succeed with live=false; live discovery refuses with exit 3 |
| Prior milestone hosted CI | [Run 45053200343](https://github.com/fpokrzywa/AppRivet/actions/runs/45053200343), commit 7c3d011: all four jobs passed, including Windows/Linux/macOS Go tests, vet, Linux/macOS race checks and 10,000 native Linux device fuzz inputs |
| Snapshot local CLI/fuzz | Real Windows example roundtrip and independent SHA-256 passed; final native Windows verifier fuzz completed 10,000 inputs |
| Snapshot hosted CI | Pending new push; workflow adds native verification fuzzing and real CLI roundtrips on Windows/Linux/macOS |
| Mac discovery smoke | devicectl 506.7 / JSON v3 returned zero devices; simctl returned 132 shutdown simulators across iOS/tvOS/watchOS/xrOS |
| Original SwiftUI app build | Passed with Xcode 16.4 (16F6), iOS Simulator SDK 18.5, hosted macOS arm64 |
| Simulator XCTest / module-collision build | One XCTest passed on iPhone 16 Pro / iOS 18.5; generated SwiftUI-named app built successfully |
| Physical USB/Wi-Fi devices, pairing states and modern physical-device schema | Unverified; requires live hardware |
| Signing, install, launch, Apple account APIs, remote execution | Not implemented; no validation claim |

Toolchain: Go 1.27.4, installed portably at %LOCALAPPDATA%\AppRivet\toolchains\go1.27.4\go; global PATH unchanged. Windows executable: bin/apprivet.exe. Reproduction commands: [VERIFICATION](VERIFICATION.md). Device format, privacy rules and full live matrix: [DEVICES](DEVICES.md).

## Review and decisions

Independent review found two M2 parser defects, fixed with failing-then-passing tests: duplicate JSON keys could overwrite inventories, and explicit modern nulls could revive stale legacy states. Duplicate case aliases, including Unicode simple-fold aliases, are rejected too. The suite also exposed a test-only coverage-runtime stderr warning; the assertion now checks separation of stdout payload rather than assuming empty stderr. No deferred minor review findings.

Independent snapshot review found one cancellation-at-final-EOF defect, reproduced with a failing test and corrected before push. All public success paths now check cancellation before accepting a result. Review found no other material archive, path, quota, CLI or output-handling defects. Hostile same-user filesystem races, content-level secret detection, buildability and live host validation were explicitly outside that review; these remain documented boundaries, not inferred guarantees.

The reviewer could not establish physical-device compatibility, Mac file/cancellation behavior or physical state mappings from fixtures. Hosted discovery smoke now covers basic Mac tool invocation; the detailed physical-device matrix remains open. Local Linux testing supplies Unix behavior evidence, not Apple hardware evidence.

Engineering decisions: continue in the clean dedicated repository; add simulator discovery and offline inspection as useful credential-free work; reject ambiguous reports rather than return partial or misleading inventory. Trade-off: explicit null known fields and future schema versions require an adapter update, and install/launch remain unavailable until their own authorization/isolation validation.

CI reliability observation: [run 45052339395](https://github.com/fpokrzywa/AppRivet/actions/runs/45052339395), attempt 1, passed all Go jobs but timed out during simctl discovery at 60 seconds. An unchanged Mac retry passed discovery, then was superseded by the final CI/documentation update while XCTest was running. This is an observed intermittent timing failure, not a parser rejection or a proven Apple-service root cause. No discovery retry or timeout increase was added. Preserve this evidence and investigate if it recurs.

## Next work

1. Complete snapshot CI and record its exact commit/run. Preserve all prior Mac checks. Investigate recurrent simulator timeouts before claiming reliable unattended Mac operation; no billing workaround is required.
2. Validate USB/Wi-Fi/disconnected/unpaired/locked/Developer Mode-disabled and older devices against Apple's actual reports. Add carefully sanitized captures with provenance; current committed fixtures are synthetic.
3. Design and test the credential-free worker/signing boundary and trusted human approvals before enabling signing or device mutations. Discovery cannot authorize installation. Keep Apple passwords and private keys out of the CLI.
4. Continue M2 installation/launch only with the required hardware/signing evidence. Remote builds, MCP and release distribution retain their separate acceptance gates.
5. Build authenticated worker admission/extraction and grants around verified snapshot digests only with a concrete isolation design. Snapshot integrity alone is not execution permission. See [SNAPSHOTS](SNAPSHOTS.md) for format limits and unvalidated worker/signing gates.
