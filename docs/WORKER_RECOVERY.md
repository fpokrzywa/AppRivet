# Reservation recovery and execution ownership checks

The local ledger now supports exclusive claims, current-owner checks and explicit recovery. No command launches, resumes, kills or isolates a process. Principal and worker flags are operator assertions. All responses retain `authentication_verified: false` and `execution_authorized: false`; a matching claim is not an execution ticket.

## Claim and check ownership

After [reserving a snapshot](WORKER_RESERVATIONS.md), claim its reservation:

```sh
apprivet worker claim --state worker-state --principal developer --reservation RESERVATION_ID --worker worker-1 --policy policy.json --archive source.tar --json
```

Claim accepts only an unexpired, unclaimed reservation belonging to the supplied principal. Under the existing cross-process lock it verifies actual archive bytes against the reservation digest, checks current policy and quotas, reloads policy after verification, then rechecks the clock and reservation deadline before appending. A claim does not extend the original reservation lease. Competing claims, including repeated claims from the same worker ID, fail with `ownership_conflict`; no second owner is allocated.

The returned reservation gains `worker_id`, a random 128-bit lowercase hexadecimal `claim_id`, a positive integer `fence` equal to the claim's journal sequence, and `claimed_at`. These values survive process restart. The tuple is a local compare-and-match identity; it is not secret or an authenticated bearer capability. Sequence numbers increase within one retained journal, not across independent/copied stores.

```sh
apprivet worker check-owner --state worker-state --principal developer --reservation RESERVATION_ID --worker worker-1 --claim CLAIM_ID --fence FENCE --policy policy.json --archive source.tar --json
```

Every field must match the durable claim. Closed or missing ownership is refused before accessing the archive. The claim must still be live, and actual snapshot/policy verification runs again. Successful output uses `mode: "local_ownership_check"` and `ownership_valid: true`, with `checked_at`, `valid_until` and the reservation. `valid_until` is the earlier of the reservation deadline and the currently loaded grant expiry. It is an upper bound, not a promise that later policy changes cannot revoke eligibility. Failed checks return no partial decision.

A successful check is point-in-time evidence only. The lock is released before the caller acts. An external executor or artifact store must atomically enforce the ownership tuple at each protected side effect; a check followed by a separate action is not fencing. That executor does not exist yet.

## Capacity and lifecycle

| Status | Holds count/byte capacity? | Next supported action |
| --- | --- | --- |
| reserved | Until reservation expiry | Claim or ordinary release |
| expired | No, if never claimed | Inspection; request/nonce remain consumed |
| claimed | Yes | Current-owner check or explicit recovery |
| recovery_required | Yes; claim's deadline has passed | Explicit recovery |
| released / recovered | No | Inspection; replay history remains consumed |

Claimed reservations retain global/per-principal count and global byte capacity after expiry. Policy revocation or a failed owner check also does not free claimed capacity. Ordinary `release` returns `ownership_required` for live or expired claims. This avoids treating a missing heartbeat or elapsed lease as proof that work stopped.

There is no automatic takeover, lease renewal, requeue, job restart or completion/artifact-publication command. Recovery closes the original reservation permanently; a new reservation requires a fresh request ID and nonce, and any later claim receives a fresh claim ID and higher sequence. All old ownership tuples fail against the new reservation.

## Inspect and recover after interruption

```sh
apprivet worker recovery-status --state worker-state --principal developer --json
```

This command verifies the entire canonical journal under its kernel lock without appending, repairing or resetting anything. It filters reservations to the supplied principal and includes journal sequence/head hash, recorded time, observed time and `clock_rollback`. The mode is `local_recovery_inspection`. Unlike ordinary status, it works when a valid journal is full or the clock precedes the persisted watermark. Missing, partial, damaged or noncanonical journals still fail closed.

Use this report to recover the exact tuple after a lost claim/recovery acknowledgment. It makes no liveness claim: a `claimed` record does not establish a running process. Derived expiry uses the later of the recorded and observed times, but inspection does not persist a new watermark. This preview must not authorize execution or substitute for a mutating owner check. Global journal sequence/hash are operator diagnostics; they are not an authenticated or rollback-resistant audit anchor.

After independently establishing that the worker stopped, explicitly close its accounting record:

```sh
apprivet worker recover --state worker-state --principal developer --reservation RESERVATION_ID --worker worker-1 --claim CLAIM_ID --fence FENCE --confirmed-stopped --json
```

`--confirmed-stopped` is required, recorded in the recovery event, and **not verified by AppRivet**. The response includes `stop_assertion_verified: false`. It performs no OS process lookup, termination or VM teardown. Worker IDs are not PIDs; do not infer process identity or death from a reused PID. A future service must replace this operator assertion with authenticated supervisor evidence and enforced fencing before enabling execution.

Recovery verifies principal and every ownership field under the lock, then appends and syncs the transition to `recovered`. It can close a live or expired claim; current policy/archives are not required to close accounting after revocation or storage removal. Repeating recovery with the identical tuple is idempotent, while stale or conflicting tuples fail. The recovered claim can never be claimed again. Request IDs, nonce hashes, claim identity and fences remain in the journal.

Cancellation before the append changes no ownership. A fully synced append wins over cancellation. Write/sync failure is `commit_uncertain`: retain the store and inspect it before retrying. A complete but uncertain event can be inspected; an incomplete event blocks all operations. No partial-tail truncation or replay-history deletion is implemented.

Full or clock-regressed stores can be inspected but cannot bypass the normal mutation checks to claim or recover. There is no capacity reserved for recovery events or automatic repair. [Explicit in-place migration and compaction](JOURNAL_MAINTENANCE.md) can reclaim a full append segment while preserving every event and fence. Cumulative history/segment limits still fail closed; deleting state to free capacity defeats replay protection. This bounded local prototype is not a production scheduler.

## Compatibility and errors

Existing journals containing only init/reserve/observe/release events retain their canonical encoding and are readable without rewriting. New claim/recover events use optional ownership fields. Older binaries reject these new journals; downgrading after claims is unsupported. Journal limits and trusted-local-filesystem/clock prerequisites remain as documented in [WORKER_RESERVATIONS](WORKER_RESERVATIONS.md).

| Code | Exit | Meaning |
| --- | --- | --- |
| invalid_arguments | 2 | Malformed or missing principal/worker/reservation/claim/fence |
| reservation_not_found | 4 | Reservation does not belong to the supplied principal |
| ownership_conflict | 4 | Absent, closed, already claimed or mismatched ownership |
| ownership_expired | 4 | Claim/reservation deadline elapsed during the checked operation |
| ownership_required | 4 | Ordinary release cannot close a claimed reservation |
| stop_confirmation_required | 4 | Explicit stopped-worker assertion omitted |
| cancelled | 130 | Operation cancelled before its commit boundary |

Existing policy, digest, quota, journal, clock and commit-uncertainty errors still apply. An already-expired unclaimed reservation fails claim as `ownership_conflict`; expiry encountered during verification is `ownership_expired`.

## Evidence and remaining live validation

Tests cover six-process claim races, durable ownership recovery after the winning process exits, principal/worker/claim/fence mismatches, expired claims retaining all budgets, current policy revocation/quota changes, changed archive bytes, cancellation, repeated recovery, old-owner refusal, replay retention, legacy canonical records, invalid correctly rehashed transitions, full journal inspection and injected partial-write/sync errors. Journal fuzz seeds now include claimed and recovered states. Actual CLI smoke exercises the workflow on each CI host. All four jobs in [run 45061672025](https://github.com/fpokrzywa/AppRivet/actions/runs/45061672025) passed at code/smoke commit 9448180, including Windows/Linux/macOS tests and CLI smoke, Linux/macOS race checks, Linux journal fuzzing and the existing Mac simulator integration. See [VERIFICATION](VERIFICATION.md) for complete evidence.

Still unimplemented or unvalidated:

1. Authenticated principal/worker identities, protected policy deployment and trusted approval/teardown evidence. Local flags are not this boundary.
2. Real execution supervision, atomic side-effect fencing, artifact publication, restart reconciliation against live process/VM identity, and verified termination. No claimed record proves a process exists or stopped.
3. Rollback/clone-resistant storage and fencing, power-cut durability, authenticated relocation, administrative repair, disk-full recovery, rate limits, private Windows ACL enforcement and deployment-specific filesystem behavior. A copied or rolled-back store can preserve stale ownership; hashes do not prevent this.
4. Isolated Mac workers and signer separation under malicious project code; physical-device pairing/state compatibility, signing, installation/launch and Apple account workflows. Hosted trusted-source simulator tests do not validate these gates.
