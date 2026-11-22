# Intent-backed manifest repair

`worker repair-status` inspects an interrupted journal publication without changing files. `worker repair` completes only the successor named by a durable pending intent, after rechecking exact inspection hashes under the store's existing kernel lock. It retains every replay key, reservation, ownership tuple, fence and clock watermark. Neither command authenticates a caller, grants execution permission, starts a job or verifies a stopped worker.

```sh
apprivet worker repair-status --state worker-state --json
apprivet worker repair --state worker-state \
  --expect-intent-sha256 INTENT_SHA256 \
  --expect-control-sha256 CONTROL_SHA256 \
  --expect-history-sha256 JOURNAL_SHA256 \
  --expect-sequence JOURNAL_SEQUENCE --json
```

Use the four values from the inspection response. Paths to alternative manifests, force flags, reset, history truncation and automatic candidate selection are deliberately unavailable. Filesystem access is the operator boundary. Reports contain `authentication_verified: false` and `execution_authorized: false`; inspection mode is `local_manifest_repair_inspection`, repair mode is `local_manifest_repair`.

## Publication evidence

New migration/compaction publications retain a source-control backup, immutable canonical intent, compressed history, empty successor active file and prepared manifest. Every file is created exclusively and synced. The publisher then exclusively creates and syncs `publication.pending`, followed by directory sync on Unix, before modifying `journal.jsonl`. An incomplete pending marker still blocks normal operations; it does not authorize repair.

The intent binds the operation ID, source control SHA-256, source active name/size/SHA-256, target manifest name/hash, global sequence/head and clock watermark. Its fixed and immutable copies must match byte for byte. After successful control sync, the publisher clears and syncs the marker before removing the superseded raw active file. All other evidence remains. Once pending-marker creation starts, uncertain failures retain evidence rather than cleaning it away.

Normal reservation, claim, owner check, recovery, status and maintenance commands return `publication_pending` while any pending marker exists. The lock file is never replaced or removed. Inspection and repair use that same lock.

## What repair proves

1. All named evidence is bounded, canonical and rooted in the existing store, with regular-file identity checks. Intent is capped at 4 KiB; source backup at 16 MiB; existing manifest, archive, event and cumulative limits still apply.
2. The candidate is the exact recorded successor. Legacy migration replays the complete source backup; format-2 compaction validates the prior manifest, exact archived prefix, prior active bytes and successor lineage.
3. Every archived and active event is replayed and validated. Source history must reach the recorded seal. No history, ownership or consumed replay key is reconstructed from a summary.
4. Current control is either the exact recorded source, an exact prefix of the intended target (including empty), or the complete target. Arbitrary corruption is refused. A candidate active file must be empty unless current control already equals the complete target; in that case all valid later events are preserved and pinned too.
5. A bounded inventory (256 entries) rejects competing manifest/segment/active names. Any retained superseded active file must equal its sealed successor. This scan can veto repair; it never chooses an authority. Orphan staging files can therefore require separate investigation rather than permissive recovery.
6. Repair reacquires the lock and repeats the proof, compares all four expected values and rejects clock rollback. It saves the exact control bytes in `repair-control-<control-sha256>.bin` before writing. Existing before-images must match; they are never overwritten.

Before touching control, repair revalidates and syncs the pending marker and any reused before-image, then syncs the directory. Readable matching bytes left by a failed sync are not treated as already durable. Repair rewrites and syncs the existing control file, then clears the exact pending marker and syncs the directory where supported. An already-complete target is synced without rewriting it. It never deletes archives, source backups, old raw active files, prepared manifests, immutable intents or before-images. `changed: true` means the pending publication was finalized; `control_rewritten` distinguishes a rewrite from complete-target finalization. The returned control hash/state describe the inspected before-image; `before_image` names its saved file.

## Failure and retry

| Error | Meaning |
| --- | --- |
| `publication_pending` | Normal operations frozen; use repair-status |
| `repair_unavailable` | No pending intent; no candidate may be inferred from prepared files |
| `repair_refused` | Conflicting evidence, unsupported control damage, competing generation, nonmatching before-image or inventory bound |
| `repair_changed` | Inspection pins no longer match; inspect again |
| `state_corrupt` / `state_io` | Invalid, missing, inaccessible or nonregular required evidence |
| `clock_rollback` | Inspection works, mutation waits for a trustworthy clock at/after the persisted watermark |
| `commit_uncertain` | Write/sync/marker cleanup acknowledgement failed; keep all evidence and inspect again |

These errors use exit 4; invalid arguments use 2 and cancellation before publication uses 130. A synced, completed publication wins over late cancellation. If repair crashes or returns uncertainty while the marker remains, a new inspection supplies current pins for a retry. If the marker was already cleared, use ordinary `recovery-status` to resolve the lost acknowledgement. A partial before-image write, damaged intent or missing evidence is not automatically repaired. Never delete/reset a store to make a retry succeed.

## Compatibility and remaining live-validation gaps

- **Older tears without intent remain unsupported.** A prepared manifest alone is not proof it was selected. There is no safe automatic upgrade of an already-torn legacy publication.
- Format 2 remains readable by the immediately preceding implementation, which does **not** honor pending markers. Mixed-version writers are unsupported. Repair detects retained competing generations and changed old active files, but deleted evidence, hostile same-user modification, whole-store rollback or cloning still require external authority/rollback anchors. Hashes do not authenticate an administrator.
- Process-exit crashes, injected write/truncate/sync errors, concurrent processes and bounded parser fuzzing are automated tests. They do **not** establish power-cut, controller-cache, disk-full, network/shared-filesystem or Windows directory-entry durability. Windows private ACL enforcement and metadata durability still need deployment work and live validation.
- Authenticated workers/principals, trusted policy deployment, external side-effect fencing, live process/VM reconciliation and verified teardown remain unimplemented. Repair preserves ownership accounting; it cannot establish that a worker is stopped.
- Isolated Mac execution under malicious source, signer separation, physical-device USB/Wi-Fi/pairing states, signing, install/launch and Apple account workflows still need live validation. Hosted trusted-source simulator CI covers only the existing generated app/build/test path.

All four jobs in [run 45067510656](https://github.com/fpokrzywa/AppRivet/actions/runs/45067510656) passed at code/workflow commit `cebca8c`: Windows/Linux/macOS tests and repair CLI smoke, Unix race checks, six Linux fuzz targets (10,000 inputs each), and the existing trusted-source simulator integration. Independent review identified two retry durability gaps; both were fixed and re-reviewed. The subsequent handoff is documentation-only.

See [verification evidence](VERIFICATION.md), [journal maintenance](JOURNAL_MAINTENANCE.md) and [ownership/recovery](WORKER_RECOVERY.md).
