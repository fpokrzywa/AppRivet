# Changelog

## Unreleased: native Linux journal writeback testing

- Exercise real journal `fsync` `EIO` on a disposable ext4 device-mapper image. Verify recovery of a durable pending event and refusal of divergent persisted tails after device restoration.

## Unreleased: persistent Linux journal fault testing

- Exercise ext4 user quota after remount and native journal sync on a full loopback image. Test exact append recovery after kernel `EDQUOT` and a separately injected sync error.

## Unreleased: recovery for interrupted journal appends

- Record each validated append before writing its event. Add read-only inspection and explicit, hash-pinned completion for the exact missing bytes, preserving replay keys and ownership history.
- Cover process exits during append and repair, stale and incompatible evidence, concurrent repair, expired claimed capacity and kernel `ENOSPC`. Older tears without intent still fail closed.

## Unreleased: journal disk-full fault tests

- Add injected write, sync and process interruption tests and an isolated Linux tmpfs test that triggers kernel `ENOSPC`.
- Publish pending markers and repair before-images from synced staging files using no-replace hard links. Failed staging leaves the final name available for retry and preserves journal history.

## Unreleased: intent-backed manifest repair

- Record a durable pending publication intent and source backup before replacing the journal control bytes; freeze normal operations until publication is finalized.
- Add read-only `worker repair-status` and hash/sequence-pinned `worker repair`, retaining damaged bytes and every replay/ownership event. Reject ambiguous generations, changed source history and older unanchored tears.
- Add crash/retry/concurrency, stale-evidence, corruption/path/bounds, clock/cancellation security tests, publication-intent fuzzing and real CLI repair smoke. Document unsupported power-cut, rollback, mixed-version, hardware and Apple-credential validation.

## Unreleased

- Add explicit in-place journal migration and lossless compaction using bounded immutable compressed history, strict manifests and the existing kernel lock. Retain all replay/ownership events and global fences; no relocation or stale-history fallback.
- Add full-journal, process-concurrency/crash, publication-fault, gzip/path/corruption security tests and storage fuzz/CLI CI coverage. Document cumulative limits, torn-manifest recovery and live durability gaps.

- Add durable exclusive claims, worker/claim/fence ownership checks with current policy and snapshot verification, and exact-owner recovery with an explicitly unverified stopped-worker assertion. Claimed capacity remains held after expiry.
- Add read-only recovery inspection for valid full/clock-regressed journals, legacy compatibility, process-contention/failure/security tests and CLI smoke. Document external fencing, supervisor/teardown, storage and Apple validation gaps.

- Add local worker init/reserve/status/release with persistent principal-scoped replay history, bounded leases, aggregate capacity checks and policy rechecking before commit. No execution or authentication is implied.
- Add canonical bounded reservation journal, cross-process kernel locks, clock watermark, fail-closed corruption/uncertainty handling, security tests and native fuzz/CLI CI coverage. Document recovery and live deployment gaps.

- Add digest-required snapshot extraction using a stable verified copy, exclusive destination ownership, bounded writes, no link/metadata restoration and checked cleanup after failures or cancellation.
- Add read-only offline worker admission checks binding grants to principal, project, extract operation, digest, validity window and resource caps; explicitly no authentication or execution authorization.
- Add extraction/admission security tests and policy JSON fuzzing, real CLI smoke on each CI host, and hosted Mac build/XCTest from extracted original source. Document Windows ACL prerequisites and remaining authentication/isolation/replay gates.

- Add portable source snapshot create/inspect commands with explicit selection, deterministic canonical archives, per-file/whole-archive hashes and optional expected digest verification.
- Reject archive ambiguity, unsafe/colliding paths, links, common credential filenames and oversized inputs; preserve existing output and clean up failed creation.
- Add snapshot security/CLI tests, native fuzzing and example roundtrips in every CI platform job. Remote execution, extraction, signing and authorization remain unimplemented.

- Add read-only `devices` through devicectl and `devices --simulators` through simctl, with bounded capture, cleanup and cancellation.
- Add cross-platform `devices --from-json` inspection with explicit offline provenance, privacy filtering and stable error codes.
- Add original synthetic legacy/modern/simulator fixtures, parser/adapter/CLI tests and fuzz target; reject duplicate JSON keys and ambiguous null fallbacks.
- Add credential-free Mac discovery smoke checks, bounded native Linux parser fuzzing and a physical-device validation matrix.
- Record the owner's change to public repository visibility; license remains undecided. Hosted Windows/Linux/macOS tests, discovery, SwiftUI builds and simulator XCTest now pass.

- Document product requirements, independent implementation policy, researched feature parity, architecture and security boundaries.
- Add portable doctor/init/inspect/help/version commands and schema-1 JSON responses.
- Add original SwiftUI project generator, shared Xcode scheme, XCTest target and checked-in example.
- Add guarded macOS simulator build/test adapter, bounded/redacted logs, environment filtering and process-group cleanup.
- Test Windows behavior and Linux runtime behavior; cross-compile six client targets.
- Add pinned GitHub CI for Windows/Linux/macOS and Xcode simulator acceptance; the initial account billing/spending admission failure was subsequently resolved by the owner's visibility change.
- Fix generated Swift module-name collisions and surviving Unix descendant processes with regression tests.
