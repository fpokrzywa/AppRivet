# Intent-backed torn-manifest repair implementation plan

> Inline execution with superpowers:executing-plans; one fresh-context final security review. Baseline c8b0498. Existing authorization covers verified work and pushing main.

**Goal:** Restore a manifest torn during a recorded publication without dropping replay/ownership history or selecting an older authority.

**Architecture:** Before publication, persist an immutable source-control backup and canonical publication intent, plus a fixed publication.pending marker. Normal operations block while that marker exists. Repair inspection locks the same store, verifies the exact intent/candidate/source lineage and all event history, refuses competing storage artifacts or changed superseded active files, and accepts only the recorded source control, an exact prefix of the target, or the already-complete target. Repair requires explicit compare-and-match hashes/sequence from inspection, saves damaged bytes, syncs the target and clears the marker. No newest-file selection or generic corruption repair.

**Tech stack:** Existing Go standard-library storage, kernel locks, CLI and CI. No Apple credentials, new dependencies or executor.

## Decisions

- New evidence is required. Older unanchored tears, torn/missing intent, arbitrary bit corruption, competing publications or changed source history fail closed. A valid prepared manifest alone does not establish authority.
- Pending markers freeze new-version readers/writers, even when control is still valid. Older binaries lack this marker check, so repair also rejects competing manifest/segment names and validates any retained old active file against its sealed archive. Mixed-version operation is unsupported, and malicious deletion/rollback of evidence remains outside the trusted-storage boundary.
- Candidate active data must be empty unless the authoritative control already equals the candidate. Complete-target finalization preserves all valid appended events. Repair does not change any event, fence, clock watermark, policy or ownership state.
- Publication retains its source raw active file until the pending marker is durably cleared. Marker-clear failures are commit_uncertain. Repeated repair after uncertain publication is safe; after completed cleanup, normal recovery-status is the lost-ack path.
- Repair uses a bounded directory inventory (256 entries), strict rooted regular files and canonical bounded metadata. It never repairs archives/active event records, creates missing store files or prunes evidence.
- Save a hash-named before-image before control changes. Interrupted repair leaves pending intent and before-images; no automatic fallback. Windows directory durability, external rollback anchors and real power-cut testing remain live gaps.

## Steps

- [x] Add failing tests for recorded migration/compaction tears, current-target finalization, replay/ownership/capacity preservation and stale evidence refusal. Implement publication intent, frozen-state opening and repair verification/execution.
- [x] Add competing/old-writer, path/corruption/bounds, cancellation, crash/retry and concurrency security tests. Add repair-status/repair CLI commands and real smoke/fuzz coverage.
- [x] Update contracts/live gaps, run full tests/vet/fuzz/Linux runtime/cross-builds, obtain independent review and resolve findings.
- [ ] Push implementation; observe all hosted CI jobs. Record exact successful commit/run in a documentation-only handoff.

Progress: Windows full tests/vet/CLI smoke, 10,000-input intent/storage fuzz, eight Linux runtime suites and six cross-builds passed. Independent review found two retry durability gaps; both fixed by validating and syncing existing before-image/pending evidence before touching control, with regression tests and reviewer confirmation. Final post-fix Windows tests/vet/CLI smoke, Linux admission runtime and six cross-builds passed. Hosted CI pending. Initial API tests failed with missing methods; CLI test failed with invalid_arguments before dispatch implementation. Actual process-exit tests cover interrupted migration and repair; contention permits one repair and no replay admission.
