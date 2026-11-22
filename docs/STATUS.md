# Project status and handoff

Updated: 2026-11-22. Read-only discovery, portable snapshots, digest-bound extraction, offline worker admission persistent local replay/capacity reservations, and local ownership/recovery checks are implemented. The physical-device workflow and authenticated/isolated remote worker are not complete.

Repository: [fpokrzywa/AppRivet](https://github.com/fpokrzywa/AppRivet). The owner changed visibility from private to public during this session; the change was verified. Standard hosted CI now starts, resolving the earlier billing admission failure. License selection remains open; no license, versioned binary release, payments or activation were added.

## Implemented

- Portable CLI: doctor, init, inspect, build, test, devices, version and help; schema-1 JSON, human output and stable failures.
- Original SwiftUI/Xcode template, shared scheme, XCTest target and example; trusted simulator build/test adapters.
- Read-only CoreDevice discovery via devicectl, separate simulator inventory via simctl, and offline JSON inspection on every CLI platform.
- Legacy/modern device normalization, conservative unknown states, explicit source/live provenance, privacy filtering, bounded reads/capture, private report cleanup, cancellation and deadlines.
- Duplicate-key/identifier rejection, null-precedence protection, original synthetic fixtures, adapter/CLI/security tests and parser fuzz target.
- Explicit source selection, deterministic bounded archives, canonical verification and digest-required extraction into a new owned directory. No execution, upload or authorization is implied.
- Read-only offline admission checks for exact grant/principal/project/operation/digest bindings, time windows and per-snapshot quotas; decisions explicitly do not authenticate or authorize execution.
- Persistent local replay protection and bounded worker reservations with init/reserve/status/release, kernel process locks, policy rechecking and retained request/nonce history. No jobs execute. See [WORKER_RESERVATIONS](WORKER_RESERVATIONS.md).
- Exclusive local claims, exact worker/claim/fence checks, retained capacity after claim expiry, explicit accounting recovery and read-only journal recovery inspection. No process execution or termination is implied. See [WORKER_RECOVERY](WORKER_RECOVERY.md).
- Research, parity matrix, architecture, threat model, ADRs, roadmap, contribution/security documentation and CI.

## Evidence

| Check | Result |
| --- | --- |
| Current Windows tests, eight packages, and go vet | Passed; admission coverage 86.2%, snapshot 81.0%, CLI 89.0% |
| Linux runtime tests and six cross-builds | All eight Linux package suites and macOS/Linux/Windows amd64/arm64 client builds passed |
| Linux parser fuzz smoke | 274,120 executions over 10 seconds without failure; no coverage guidance in this precompiled binary |
| Native Windows/Linux coverage-guided fuzz | 10,000 inputs passed on each; Linux CI uses a 60-second outer deadline |
| Client cross-builds | macOS/Linux/Windows, amd64 and arm64 passed |
| Actual Windows CLI | Offline CoreDevice/simulator reports succeed with live=false; live discovery refuses with exit 3 |
| Prior milestone hosted CI | [Run 45053200343](https://github.com/fpokrzywa/AppRivet/actions/runs/45053200343), commit 7c3d011: all four jobs passed, including Windows/Linux/macOS Go tests, vet, Linux/macOS race checks and 10,000 native Linux device fuzz inputs |
| Snapshot local CLI/fuzz | Real Windows example roundtrip and independent SHA-256 passed; final native Windows verifier fuzz completed 10,000 inputs |
| Snapshot hosted CI | [Run 45055091739](https://github.com/fpokrzywa/AppRivet/actions/runs/45055091739), commit 5807712: all four jobs passed; snapshot CLI roundtrips on Windows/Linux/macOS, Linux/macOS race checks, 10,000 native Linux fuzz inputs for each device/snapshot target, and existing Mac discovery/build/XCTest/regression checks |
| Extraction/admission local checks | Real Windows process-level create/inspect/extract/hash/no-overwrite/admission/denial smoke passed; native snapshot and admission JSON fuzz each completed 10,000 inputs |
| Extraction/admission hosted CI | [Run 45057268258](https://github.com/fpokrzywa/AppRivet/actions/runs/45057268258), commit 54e796c: all four jobs passed; Windows/Linux/macOS tests, vet and real CLI smoke, Linux/macOS race checks, 10,000 native Linux inputs for each device/snapshot/admission fuzz target, and Mac build/XCTest from extracted original source |
| Reservation local checks | Full Windows suite/vet, 10,000 journal fuzz inputs, real reservation/recovery CLI smoke, eight Linux test executables and six client cross-builds passed |
| Reservation hosted CI | [Run 45059896975](https://github.com/fpokrzywa/AppRivet/actions/runs/45059896975), commit d2bb5a9: all four jobs passed; Windows/Linux/macOS tests/vet/CLI smoke, Linux/macOS race checks, all four Linux fuzz targets (at least 10,000 inputs each), Mac discovery, extracted-source build/XCTest and framework-name collision build |
| Mac discovery smoke | devicectl 506.7 / JSON v3 returned zero devices; simctl returned 132 shutdown simulators across iOS/tvOS/watchOS/xrOS |
| Original SwiftUI app build | Passed with Xcode 16.4 (16F6), iOS Simulator SDK 18.5, hosted macOS arm64 |
| Simulator XCTest / module-collision build | One XCTest passed on iPhone 16 Pro / iOS 18.5; generated SwiftUI-named app built successfully |
| Physical USB/Wi-Fi devices, pairing states and modern physical-device schema | Unverified; requires live hardware |
| Signing, install, launch, Apple account APIs, remote execution | Not implemented; no validation claim |

Toolchain: Go 1.27.4, installed portably at %LOCALAPPDATA%\AppRivet\toolchains\go1.27.4\go; global PATH unchanged. Windows executable: bin/apprivet.exe. Reproduction commands: [VERIFICATION](VERIFICATION.md). Device format, privacy rules and full live matrix: [DEVICES](DEVICES.md).

## Review and decisions

Independent review found two M2 parser defects, fixed with failing-then-passing tests: duplicate JSON keys could overwrite inventories, and explicit modern nulls could revive stale legacy states. Duplicate case aliases, including Unicode simple-fold aliases, are rejected too. The suite also exposed a test-only coverage-runtime stderr warning; the assertion now checks separation of stdout payload rather than assuming empty stderr. No deferred minor review findings.

Independent snapshot review found one cancellation-at-final-EOF defect, reproduced with a failing test and corrected before push. All public success paths now check cancellation before accepting a result. Review found no other material archive, path, quota, CLI or output-handling defects. Hostile same-user filesystem races, content-level secret detection, buildability and live host validation were explicitly outside that review; these remain documented boundaries, not inferred guarantees.

Independent extraction/admission review found no P1/P2 issues. Its P3 enabled-field presence discrepancy was resolved by explicitly documenting and testing omission as disabled (only explicit true permits eligibility). The reviewer reproduced inherited broad read permissions on Windows: numeric modes do not establish private DACLs. Access-controlled destination ancestry and temporary storage are prerequisites; ACL hardening and hostile same-user races are not guaranteed. At that reviewed revision, authentication, replay prevention, reservations, revocation at use and execution authorization were absent. The subsequent local reservation implementation is described in WORKER_RESERVATIONS.md; authentication and execution remain absent. No unresolved minor review findings.

Independent reservation review found no P1/P2 issues. Its P3 request-conflict classification finding was reproduced with a failing regression and fixed by checking request IDs before nonce collisions. Full Windows tests and vet passed after correction. No unresolved review findings. The local store still assumes trusted identity input, filesystem ownership and clock; it cannot defeat state rollback or enforce execution isolation.

The reviewer could not establish physical-device compatibility, Mac file/cancellation behavior or physical state mappings from fixtures. Hosted discovery smoke now covers basic Mac tool invocation; the detailed physical-device matrix remains open. Local Linux testing supplies Unix behavior evidence, not Apple hardware evidence.

Engineering decisions: continue in the clean dedicated repository; add simulator discovery and offline inspection as useful credential-free work; reject ambiguous reports rather than return partial or misleading inventory. Trade-off: explicit null known fields and future schema versions require an adapter update, and install/launch remain unavailable until their own authorization/isolation validation.

CI reliability observation: [run 45052339395](https://github.com/fpokrzywa/AppRivet/actions/runs/45052339395), attempt 1, passed all Go jobs but timed out during simctl discovery at 60 seconds. An unchanged Mac retry passed discovery, then was superseded by the final CI/documentation update while XCTest was running. This is an observed intermittent timing failure, not a parser rejection or a proven Apple-service root cause. No discovery retry or timeout increase was added. Preserve this evidence and investigate if it recurs.

## Next work

1. Keep all CI checks green for subsequent changes. Implementation/workflow commit d2bb5a9 passed all four jobs in run 45059896975; the following handoff commit changes documentation only. Preserve the observed simulator timing caveat; no billing workaround is required.
2. Validate USB/Wi-Fi/disconnected/unpaired/locked/Developer Mode-disabled and older devices against Apple's actual reports. Add carefully sanitized captures with provenance; current committed fixtures are synthetic.
3. Design and test the credential-free worker/signing boundary and trusted human approvals before enabling signing or device mutations. Discovery cannot authorize installation. Keep Apple passwords and private keys out of the CLI.
4. Continue M2 installation/launch only with the required hardware/signing evidence. Remote builds, MCP and release distribution retain their separate acceptance gates.
5. Integrate the local extractor and offline admission core into authenticated worker transport only with protected policy deployment, execution-time revocation, durable recovery, fenced execution ownership and OS isolation designs. Local replay/capacity reservations and ownership/recovery checks now exist; external fencing, live supervisor evidence and verified teardown do not. See [WORKER_RESERVATIONS](WORKER_RESERVATIONS.md) and [WORKER_RECOVERY](WORKER_RECOVERY.md). Snapshot integrity and hypothetical principal flags are not execution permission. See [SNAPSHOTS](SNAPSHOTS.md) and [WORKER_ADMISSION](WORKER_ADMISSION.md) for exact limits and unvalidated gates.
