# Worker reservation recovery and ownership implementation plan

> Implement inline using superpowers:executing-plans, with one fresh-context final review. Baseline: 95c06bc. Existing authorization covers continuing this repository and verified commits/pushes to main.

**Goal:** Recover local reservation accounting safely and reject stale execution-owner assertions, without launching processes or claiming authentication.

**Architecture:** Extend the existing locked, hash-chained journal with claim/recover events. A claim binds reservation, worker ID, random claim ID and journal sequence (fence). Claimed reservations retain capacity even after expiry; only an exact-owner recovery with an explicit stopped-worker assertion closes them. No automatic takeover or re-execution. A read-only recovery inspection supports valid but full/clock-regressed stores; corrupt state remains refused, never truncated.

**Tech stack:** Go standard library, existing CLI and CI; no Apple APIs, new dependencies or listener.

## Design decisions

- Claim and check-owner reverify the archive and current policy under the store lock, including a final policy/time recheck. Ownership checks are point-in-time local assertions, not permission to execute or protection of an external resource.
- Claim uses the reservation's original deadline; no renewal. Claimed expiry becomes recovery_required and retains global/per-principal counts and bytes. Release refuses claimed records.
- Recover requires principal plus the exact reservation/worker/claim/fence tuple and confirmed-stopped=true. It can close live or expired claims, retains all tombstones, is idempotent for the same tuple, and reports stop_assertion_verified=false. It never requeues or replaces ownership. Future authenticated teardown evidence is still necessary.
- Recovery-status verifies the complete journal without appending. It reports journal sequence/hash, recorded/observed times and rollback detection, and filters by principal. It is not a repair or authorization command. Full state can be inspected but mutations still fail closed.
- Optional ownership fields preserve legacy canonical records. Old binaries reject new event types; downgrade is unsupported after ownership events. Copy/rollback protection, secure clock and OS isolation remain outside this local model.

## Steps and ledger

- [x] Add failing admission tests for claim ownership, expiry capacity retention, stale/wrong tuples, revocation, explicit recovery, restart/lost acknowledgment and read-only inspection. Implement ownership.go and journal transitions; retain legacy compatibility.
- [x] Add real cross-process contention, invalid transition/fault/cancellation/fuzz cases and CLI tests. Implement claim/check-owner/recovery-status/recover commands and extend CI smoke.
- [x] Update contracts, architecture/threat model and live gaps. Run full tests/vet, native fuzz, cross-builds/Linux runtime; obtain independent review and fix findings with regressions.
- [x] Push implementation, observe all hosted CI jobs, record exact tested code/run in a docs-only handoff.

Progress: missing API/CLI tests failed, then passed after implementation. Full Windows tests/vet, real CLI smoke, 10,000 native journal fuzz inputs, eight Linux package executables and six cross-builds pass. Independent review found no actionable issues and independently passed admission/CLI tests and diff checks. Implementation/smoke commit 9448180 was pushed. All four jobs in run 45061672025 passed: Windows/Linux/macOS tests/vet/CLI smoke, Linux/macOS race checks, four Linux fuzz targets and Mac discovery/build/XCTest/regression. Final evidence is recorded in a separate docs-only handoff. All plan steps are complete. A new byte-budget test miscounted its own source fixture by one byte and was corrected.

Ruling: recovery inspection exposes verified journal metadata without persisting a new time watermark; it must never authorize execution. Corrupt state remains unusable. Full-state mutations remain blocked because dropping history or bypassing limits would weaken replay protection; secure compaction/migration is a separate future change.
