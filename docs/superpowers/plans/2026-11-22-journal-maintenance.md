# Safe journal compaction and migration implementation plan

> Implement inline with superpowers:executing-plans and one fresh-context final review. Baseline d3ce538. Existing authorization covers continued verified work and pushing main.

**Goal:** Reclaim the bounded append segment without forgetting replay records, ownership history, configuration, clock watermark or fencing sequences.

**Architecture:** Explicit in-place migration from the legacy JSONL file to a schema-2 manifest in the same journal.jsonl inode. The manifest references bounded immutable gzip history segments plus one append segment. Compaction seals the current append segment losslessly and creates a new empty one. Replay always verifies every original event, hash link and sequence; no checkpoint summarization, tombstone deletion or renumbering. The unchanged kernel lock serializes all generations, including legacy writers, which reject the new manifest.

**Tech stack:** Go standard library; existing cross-platform locks and CLI. No dependencies, Apple APIs, authentication or executor.

## Decisions and failure contract

- Migration means format upgrade in place, not relocation/duplication. No second writable store is created. Directory relocation remains unsupported.
- Stage exclusive random-named archive, active file and prepared manifest; sync and replay-verify them before changing the authoritative control file. Keep the prepared manifest for inspection. Publication rewrites the control file while holding the lock, with checked truncate/write/sync. It is intentionally fail-closed rather than a claimed cross-platform atomic rename: torn publication blocks the store, never falls back to an earlier manifest. No automatic repair.
- Preserve every event, including observe/release/recover and stopped assertions. Original request fingerprints, nonce hashes, claim IDs, fence numbers and times remain byte-identical inside compressed segments. Legacy record canonical encoding remains unchanged.
- Keep 16 MiB/16,384 events per append segment; bound cumulative history to 64 MiB/65,536 events, 16 immutable segments, 64 KiB manifest and 4,096 lifetime reservations. Compaction does not reset lifetime limits. Ownership fences accept global retained sequence values beyond one segment.
- Stage new directory entries before publication; sync directories on Unix. Windows retains file Sync and trusted ACL prerequisites; actual power-loss/metadata durability remains a live validation gap. Delete only the superseded raw append file after acknowledged publication and identity checks; failure leaves redundant data and is reported. Never delete archives or lock/control files.
- Corrupt/full lifetime history, clock rollback and unsupported schemas fail closed. Full valid legacy/active segments can migrate/compact without appending an observe event. Maintenance preserves rather than advances the clock watermark.

## Steps

- [x] Write failing API tests for legacy migration, exact history preservation, compaction, replay/ownership/capacity preservation, full segment recovery and fences beyond 16,384. Implement bounded format parsing, replay and locked maintenance.
- [x] Add corruption/path/gzip bounds, publication fault/cancellation, concurrency and compatibility security tests; CLI migrate/compact and real CI smoke; manifest/segment fuzz target.
- [x] Update contracts and live gaps. Run full tests/vet/fuzz, Linux runtime and cross-builds; independent review and regression fixes.
- [x] Push implementation and observe all hosted CI jobs. Record exact successful code/run in a documentation-only handoff.

Progress: missing API/CLI tests failed before implementation and now pass. Windows full suite/vet (admission 86.3%, CLI 88.8%), 10,000 native storage fuzz inputs, real CLI smoke, eight Linux test executables and six client cross-builds passed. Independent review found no actionable issues and independently passed admission/CLI tests. Implementation/workflow commit efe2716 was pushed; all four jobs in run 45064383805 passed, including Windows/Linux/macOS tests/vet/CLI smoke, Unix race checks, five Linux fuzz targets and Mac discovery/build/XCTest/regression. Final evidence is recorded in a separate documentation-only handoff. All plan steps are complete.

Ruling: maintenance retains prepared manifests and compressed evidence after any publication uncertainty; no automatic repair is added, because selecting an older manifest can restore stale ownership. Cumulative history/segment/reservation limits remain fixed and are not reset by compaction.
