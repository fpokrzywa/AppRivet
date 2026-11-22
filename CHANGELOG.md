# Changelog

## Unreleased

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
