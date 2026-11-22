# Safe reservation journal migration and compaction

Migration upgrades the journal **in place** to a segmented format. Compaction compresses the current append segment and starts an empty one while retaining every original event. Neither command creates a second store, drops replay history, changes configuration, closes ownership, renumbers fences or authorizes execution. Moving/cloning a store to another directory or host is not implemented.

```sh
apprivet worker migrate --state worker-state --json
apprivet worker compact --state worker-state --json
```

These are operator maintenance commands for the whole store; they do not accept a principal, destination, new policy or new limits. Filesystem ownership remains the access boundary. All responses report `mode: "local_journal_maintenance"`, `authentication_verified: false`, and `execution_authorized: false`.

## What remains intact

The archive contains the exact original JSONL bytes, including every init, observe, reserve, release, claim and recover event. Reads decompress and revalidate every event's canonical encoding, sequence, previous hash and semantic transition. Compressed/uncompressed sizes and SHA-256 values are checked separately. A single gzip member is allowed; trailing members/bytes, truncation and expansion beyond the declared bounded size are rejected.

Preserved data includes the original configuration, principal/project/grant/digest bindings, request fingerprints and consumed nonce hashes, all reservation states, worker and claim IDs, claim times, fencing sequence numbers, recovery stopped assertions and clock watermark. Released, recovered and expired records are never evicted. A claimed record retains capacity after expiry exactly as before. Compaction is lossless storage maintenance, not a summarized checkpoint or an audit-retention policy.

The current append segment's byte/event budget is reset after successful sealing, but the global event sequence and lifetime reservation/history limits are not reset. Claims created after the first segment may have fences above 16,384; original ownership tuples remain valid only under the existing [ownership checks](WORKER_RECOVERY.md). No stale tuple is made current by maintenance.

## Format 2 and bounds

`worker init` still creates the legacy format. `migrate` accepts a completely valid legacy journal, including a full one. After migration, `journal.jsonl` holds a canonical, self-hashed schema-2 manifest referencing immutable compressed segments and one append file. The control filename and kernel lock remain fixed. Repeating migration on a valid format-2 store is a verified no-op; compacting an empty append segment is also a no-op. Compaction of a legacy store returns `migration_required`.

| Component | Limit |
| --- | --- |
| Each append or uncompressed archived segment | 16 MiB and 16,384 events |
| Each original event | 4,096 bytes excluding newline |
| All retained history, archived plus active | 64 MiB and 65,536 events |
| Immutable segments / generation number | 16 |
| Canonical manifest | 64 KiB |
| Each compressed segment | 16 MiB plus 64 KiB overhead allowance |
| Lifetime reservations | 4,096, unchanged |

All bounds are enforced on reads as well as writes. Each generation retains a prepared manifest for inspection. At the segment-count or cumulative-history limit, no history is pruned; further operations that would exceed the limit fail closed. Frequent compaction of tiny segments can consume the generation budget, so compaction is explicit rather than automatic. This is bounded local accounting, not unlimited archival storage.

Segment and active filenames must match strict generated lowercase-hex names under the existing `os.Root`. Files must be regular with stable opened identities; path traversal, symlinks and duplicate segment names are refused. A missing segment or active file never means empty state. No directory scan selects an alternate generation.

## Publication and interruption

Maintenance holds the same kernel lock used by normal reservations throughout these steps:

1. Validate all current history and reject a clock below the recorded watermark. Maintenance preserves that watermark; it does not append a new clock observation.
2. Exclusively create a random-named compressed segment, empty append file and prepared manifest. Flush each with `File.Sync`.
3. Reopen the staged layout, decompress and replay its entire history, and compare the complete derived state with the original. On Unix, sync directory entries before publication.
4. Retain a synced source-control backup and immutable publication intent. Recheck identities/cancellation, then create and sync the fixed pending intent before truncating/writing/syncing the existing control file under the lock.
5. Clear and sync the pending marker before removing the superseded raw append file through an identity-checked rooted removal. Archives, the fixed lock and the control file are never deleted. Keep the prepared manifest, intent and source backup.

This is **not a claim of cross-platform atomic replacement**. A process crash during control-file rewrite can leave a partial manifest. A pending intent blocks normal operations with `publication_pending`; use [intent-backed repair inspection and repair](MANIFEST_REPAIR.md). Older tears without intent remain unsupported. There is no automatic fallback, rollback or “pick the newest filename” recovery. An older manifest may describe stale ownership and must not be restored blindly.

Before pending-marker creation starts, errors or cancellation preserve the original authority and attempt to remove only the operation's exclusively owned staged files. After that boundary, publication failure returns `commit_uncertain` and retains evidence. Do not retry by deleting/reinitializing the store. While the marker remains, `repair-status` verifies both complete and partial uncertain publication; after marker clearance, ordinary `recovery-status` resolves a lost acknowledgement. A completed publication wins over cancellation.

Successful output includes action, format, generation, global sequence/head hash, retained reservation count, manifest hash and prepared-manifest filename. `changed: false` identifies a no-op. `cleanup_pending: true` means redundant raw-file removal or the final directory sync did not complete; the published layout remains the selected history. Retain the store for inspection rather than deleting files by pattern.

On Windows, files are synced but no portable directory-entry flush or private DACL enforcement is claimed. POSIX files request 0600, and Unix directory sync runs where supported. Actual power-loss, disk-controller/filesystem guarantees, Windows metadata durability and backup rollback remain deployment validation gaps. For the library contracts underlying the implementation, see Go's [File.Sync](https://pkg.go.dev/os#File.Sync) and [gzip single-stream handling](https://pkg.go.dev/compress/gzip#Reader.Multistream).

## Compatibility and authority

The format upgrade modifies the existing control inode while holding the unchanged lock. Legacy-only binaries reject the new manifest instead of appending legacy events; supported writers acquire that lock before opening the current journal. The preceding format-2 implementation does not honor pending intents, so mixed-version operation is unsupported; see [repair compatibility checks](MANIFEST_REPAIR.md). There is one selected append file. Prepared manifests and superseded files are evidence, not independently writable stores.

Legacy canonical records remain readable without migration. Downgrading a migrated store is unsupported. Copying/restoring the whole directory can still clone or roll back authority; local hashes and the store ID are not external rollback anchors. This work deliberately provides no relocation/export/import command that would imply safe handoff to another live store.

Errors retain existing exit codes: `migration_required`, `state_full`, `state_corrupt`, `state_io`, `state_busy`, `clock_rollback` and `commit_uncertain` use exit 4; invalid CLI arguments use 2; cancellation before publication uses 130. No error returns a partial success report.

## Evidence and remaining live validation

Security tests cover byte-identical history roundtrips, owner/replay/capacity preservation, full-journal migration and compaction, fences spanning segments, concurrent maintenance/reservation processes, actual child-process exit during partial publication, injected truncate/write/sync failure and cancellation after sync, strict manifests, corrupt/missing/linked files, reordered history, gzip expansion/trailing-member limits and lifetime budgets. The CLI smoke exercises migration/compaction with a claimed job and checks its stale/replayed identities after recovery. A dedicated fuzz target covers manifests and compressed segments. All four jobs in [run 45064383805](https://github.com/fpokrzywa/AppRivet/actions/runs/45064383805) passed at implementation/workflow commit efe2716: Windows/Linux/macOS tests and CLI smoke, Linux/macOS race checks, five Linux fuzz targets and existing Mac simulator integration. Complete evidence is recorded in [VERIFICATION](VERIFICATION.md).

Remaining gates:

1. External rollback/clone anchoring, authenticated relocation/handoff, repair of older unanchored tears or damaged intent, retention beyond cumulative limits, and deployment-specific power-cut/disk-full fault campaigns. Explicit intent-backed repair is implemented; automatic fallback remains unavailable.
2. Windows private ACL enforcement and metadata durability; hostile same-user/filesystem mutation and network/shared-filesystem locking are unsupported boundaries. Only trusted local storage is assumed.
3. Authenticated principals/workers, trusted policy/approval deployment, atomic external side-effect fencing, live process/VM reconciliation and verified teardown. Maintenance preserves ownership records but cannot establish whether a process exists or stopped.
4. Isolated Mac workers and signer separation under malicious source, physical-device pairing/state compatibility, signing, installation/launch and Apple account workflows. Hosted trusted-source simulator checks do not establish those properties.
