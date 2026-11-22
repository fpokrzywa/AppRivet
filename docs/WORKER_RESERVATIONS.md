# Local replay protection and worker reservations

These commands implement persistent local accounting. [Recovery and ownership commands](WORKER_RECOVERY.md) extend this model without adding execution. They do not run, extract, enqueue, authenticate, sign or install anything. `--principal` remains an operator-supplied assertion, never a network authentication mechanism. Every result reports `mode: "local_reservation"`, `authentication_verified: false` and `execution_authorized: false`.

## Initialize and reserve

Create an operator-owned configuration file:

```json
{"schema_version":1,"max_active":4,"max_per_principal":2,"max_reserved_bytes":536870912,"max_lease_seconds":900}
```

All limits are positive. Maximums: 64 active reservations, per-principal count no larger than the global count, 16 GiB reserved bytes and 3,600 seconds per lease. Configuration is immutable within a store. Documents use the same strict 64 KiB JSON rules as admission policy. State must be on a trusted local filesystem with access-controlled ancestry. POSIX creation requests directory mode 0700 and file mode 0600. Windows inherits ACLs; private DACL enforcement is not implemented.

```sh
apprivet worker init --state worker-state --config worker-limits.json --json
```

Initialization requires a new directory. Existing files/directories are never reset. An interrupted or failed initialization can leave incomplete state for inspection; normal commands refuse it rather than initialize it automatically. Keep the store at a stable location. There is no reset command. Explicit [in-place migration and lossless compaction](JOURNAL_MAINTENANCE.md) preserve every event and replay record. [Manifest repair](MANIFEST_REPAIR.md) handles only publications with a durable pending intent; it cannot repair initialization or damaged events.

Add these fields to an [admission request](WORKER_ADMISSION.md):

```json
{"request_id":"request-1","nonce":"0123456789abcdef0123456789abcdef","lease_seconds":60}
```

This is a fragment, not a complete request. Generate each production nonce using a cryptographically secure generator; the example is not suitable for reuse. Nonces must be exactly 32 lowercase hexadecimal characters. Request IDs use the existing 64-character portable identifier syntax. All three fields are required together for `reserve`; `check` accepts either the original request or the extended request. A nonce's syntax cannot establish randomness or approval provenance.

```sh
apprivet worker reserve --state worker-state --policy policy.json --request request.json --principal developer --archive source.tar --json
apprivet worker status --state worker-state --principal developer --request-id request-1 --json
apprivet worker release --state worker-state --principal developer --reservation RESERVATION_ID --json
```

Omit `--request-id` to list that principal's retained reservations. Status never lists another principal's records, but a local operator able to supply arbitrary principals is not an authenticated client. Reservation IDs are random 128-bit identifiers, not bearer capabilities.

## State and replay rules

- A successful reservation consumes both `(principal, request_id)` and `(principal, nonce)` across grants and projects. The journal stores the nonce hash and a fingerprint of the complete parsed request, not the raw nonce. IDs are case-sensitive.
- Exact retries return `replay_detected`. Changed input with the same request ID returns `request_conflict`; a consumed nonce with a fresh ID returns `replay_detected`. Use status by request ID after a lost response. No second job is created.
- Release is owner-bound and idempotent. Expiry and release retain replay history indefinitely within the store. Different principals have independent replay namespaces.
- A fresh request ID and nonce may reuse an active grant. This is not a once-per-grant, once-per-digest or human-approval system.
- Leases end at the earlier of the requested duration and grant expiry. At the exact expiry instant an unclaimed reservation becomes `expired` and accounting capacity becomes available. Claimed reservations instead become `recovery_required` and retain capacity until explicit exact-owner recovery. Released records remain `released`.
- Global active count, per-principal active count and global logical byte capacity are checked together under the store lock. Bytes equal verified archive bytes plus source bytes. These are accounting limits, not physical disk allocation, CPU/memory containment or per-principal byte quotas.
- Before committing, reserve reloads policy after archive verification, rechecks grant bindings/time and applies current snapshot limits. This does not provide atomic policy revocation across the final read/commit or authorize later execution. There is no execution consumer of these records.

## Persistence and failure behavior

Windows uses exclusive nonblocking `LockFileEx`; Linux/macOS use `flock`. A fixed lock file is retained, never deleted as a stale lock. Processes retry acquisition for at most five seconds with cancellation support. Kernel locks are released on process death. Normal operations require existing regular lock/journal files and verify their opened identities; no symlink target is accepted as either file.

The journal is canonical JSON Lines with sequential events and a SHA-256 chain. Each append is checked and `File.Sync` completes before success is acknowledged. Unknown/noncanonical records, invalid transitions, hash mismatch, partial final records and missing files fail closed. These hashes detect damage; they do not authenticate the operator or defeat deliberate rewriting. A valid prefix restored from backup cannot be distinguished from an earlier store. Deletion, duplication, replacement or rollback of state can defeat replay protection; protected storage and a future external rollback anchor remain required for stronger guarantees.

The observed UTC time watermark is persisted. A clock earlier than that watermark freezes operations with `clock_rollback`; expiry cannot be undone by a subsequently observed backward step. This is not a secure clock: an undetected rollback, a large forward jump or operator clock tampering remains outside the guarantee. Status and some rejected requests append time observations, so status is not read-only.

Legacy/active-segment bounds are 16 MiB and 16,384 events, with 4,096 bytes per event and 4,096 lifetime reservations. Explicit migration enables bounded compressed history with unchanged global event sequences; see JOURNAL_MAINTENANCE.md for cumulative limits. Time observations count as events. Full state fails closed, including maintenance/release/recovery operations that would append. The read-only `recovery-status` command can inspect a valid full journal. Records are never silently evicted. Explicit compaction preserves all replay and ownership history and reclaims only the append budget. Directory relocation and automatic damaged-state repair remain unsupported; deleting the directory and starting again is not a safe recovery procedure.

Cancellation before the append intent is published consumes no request. Once the event is synced and the intent is cleared, success wins over cancellation. A failure after intent publication may return `commit_uncertain`. If the intent remains, use [append repair](APPEND_REPAIR.md) to inspect and complete the exact recorded event. If the intent is absent, query the original request ID or retain damaged bytes for investigation. Never truncate a partial tail. Unix syncs the store directory around publication. Durability of the newly created store in its parent and Windows directory metadata remain deployment gaps.

## Errors

All store errors exit 4 except invalid arguments (2) and cancellation before commit (130). Standard admission errors still apply.

| Code | Meaning |
| --- | --- |
| invalid_config | Unsupported or invalid store limits |
| state_exists | Initialization destination exists or could not be created exclusively |
| state_io | Inaccessible, missing, replaced, nonregular state or unavailable locking |
| state_busy | Lock acquisition exceeded five seconds |
| state_corrupt | Invalid or incomplete journal; no reset performed |
| state_full | Journal or lifetime replay-history limit reached |
| append_pending | Interrupted event publication needs append repair inspection |
| clock_rollback | Clock precedes persisted watermark |
| replay_detected / request_conflict | Consumed nonce/request or conflicting request ID |
| capacity_exceeded | Aggregate count or byte budget exhausted |
| reservation_not_found | No reservation owned by the supplied principal |
| commit_uncertain | Write or sync failed; query retained state before retrying |

## Validation and remaining live gates

Automated security tests cover cross-process contention, process-death lock recovery, replay after restart/release/expiry, exact deadline boundaries, observed clock rollback, global/per-principal counts, byte budgets, policy revocation/quota changes during verification, cancellation, malformed state, partial writes, sync uncertainty, file replacement, private POSIX modes and strict parsing. Native journal fuzzing and CLI smoke run in CI. All four jobs in [run 45059896975](https://github.com/fpokrzywa/AppRivet/actions/runs/45059896975) passed at code/workflow commit d2bb5a9, including Windows/Linux/macOS reservation tests and CLI smoke, Linux/macOS race checks, Linux journal fuzzing and the existing Mac simulator integration. See [VERIFICATION](VERIFICATION.md) for complete evidence.

Still required before a live worker:

1. Authenticated transport, principal derivation, protected policy deployment and a trusted approval UI. No listener or credential integration exists.
2. Enforced external fencing, live process/VM restart reconciliation, verified teardown and execution-time revocation. Local claim identities, current-owner/policy checks and explicit accounting recovery now exist; see WORKER_RECOVERY.md. Releasing accounting does not prove a running process stopped; no process runs here.
3. Protected durable storage, rollback-resistant recovery/rotation, rate limits, actual power-cut/disk-full tests, Windows ACL hardening and deployment-specific local-filesystem locking behavior. Network/shared filesystems and hostile same-user races are unsupported boundaries.
4. Mac worker isolation against malicious source, signer separation, secure artifact publication and resource enforcement. Hosted trusted-source simulator tests do not establish these properties.
5. Physical USB/Wi-Fi pairing and device-state compatibility, signing, installation/launch and Apple account APIs. These require their own hardware/credential validation; none is inferred from local reservations.
