# Replay protection and local worker reservations

Implementer: inline. Baseline: 0018f72. Existing authorization covers continued work and pushing small verified changes to main.

## Contract

Add explicit worker init/reserve/status/release commands, without execution or authentication. Require request ID, random 128-bit nonce and bounded lease for reservations. Persist principal-scoped request fingerprints and nonce hashes across release, expiry and process restart. Reject replay/conflicting reuse; recover lost acknowledgments through status by request ID. Enforce global/per-principal active counts and verified archive-plus-source byte budgets. Recheck policy and time at commit.

Use a bounded canonical, hash-chained append-only journal with file Sync under a kernel cross-process lock. Missing, malformed, truncated, full or clock-regressed state fails closed; never silently reset or evict replay history. Explicit initialization creates a new private directory. Trusted local filesystem, owner, clock, policy deployment and Windows ACLs remain prerequisites. Hashes do not prevent authorized filesystem rollback. Lease expiry reserves accounting only; no process exists to terminate.

## Steps and evidence ledger

1. Write failing tests for replay, conflicts, ownership, budgets, expiry, clock rollback, policy recheck, malformed state, cancellation and process contention. Implement store and request extensions. Run admission tests.
2. Add CLI commands, honest envelopes and process-level smoke; add journal fuzz coverage. Run full tests, vet, cross-build and real CLI smoke.
3. Update worker contract, threat model, architecture, roadmap and validation gaps. One independent fresh-context review as required by executing-plans/requesting-code-review. Resolve findings with regression tests.
4. Commit/push implementation, watch all CI jobs, fix actual failures. Record exact tested commit/run and remaining live gates in a documentation-only handoff.

Ruling: keep the store in internal/admission to reuse strict bounded JSON and policy validation; avoid a new database dependency.
Ruling: persist an observed-time watermark; derive expiry from it instead of appending individual expiry events. This prevents resurrection after observed clock rollback while retaining tombstones.
Ruling: reject exact replay rather than returning a new success receipt. Status is the explicit recovery path. Request IDs and nonce hashes are scoped to a principal across grants/projects.
Ruling: use kernel locks, not deletable stale lockfiles. Lock and journal inodes must remain stable, with no automatic creation on normal operations.

Progress: API/CLI missing tests failed before implementation, then passed. Windows full suite/vet, 10,000 native journal fuzz inputs, real CLI smoke, eight Linux package executables and six client cross-builds pass. Independent review found no P1/P2 issues. Its P3 request-conflict precedence finding failed a dedicated regression before correction; the full Windows suite and vet pass after the fix (admission coverage 86.2%). Hosted CI follows review.

Ruling: failed initialization retains its exclusive directory for inspection, rather than deleting possible evidence. File Sync is the acknowledged append boundary; directory-entry power-loss durability and external rollback anchoring remain explicit live gaps.
