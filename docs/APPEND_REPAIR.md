# Recovery for interrupted journal appends

An interrupted append with a valid `append.pending` intent can be completed in a legacy or segmented reservation store. Repair writes only the missing suffix of the recorded event. It preserves earlier events and any already written bytes, including replay keys, ownership tuples, fences and the recorded clock watermark. An older tear without intent, an unrelated tail, or damaged evidence blocks repair.

```sh
apprivet worker append-repair-status --state worker-state --json
apprivet worker append-repair --state worker-state \
  --expect-intent-sha256 INTENT_SHA256 \
  --expect-active-sha256 ACTIVE_SHA256 --json
apprivet worker recovery-status --state worker-state --principal PRINCIPAL --json
```

The first command verifies the store without modifying it. Copy its `intent_sha256` and `active_sha256` into the second command. `present_bytes` and `event_bytes` describe the event at inspection. `journal_sequence` and `journal_sha256` describe the history after completion. The receipt reports `bytes_written: 0` if the complete event was already present and needed only syncing and finalization. The commands report `authentication_verified: false` and `execution_authorized: false`.

## Write and repair behavior

1. Normal writes validate the proposed event against current history and bounds under the existing kernel lock. They write and sync a temporary intent, then give it the `append.pending` name with a same-directory hard link that cannot replace an existing file. Unix directory sync precedes the journal write. The intent contains the exact canonical event, active filename, append offset, and current manifest hash for segmented stores. The event binds the preceding journal head and next sequence.
2. After the event is synced, the writer removes the intent and syncs the directory where supported. A failure before the intent gets its final name leaves the journal unchanged. Once it is visible, an interruption may leave zero, some or all event bytes. Ordinary journal operations report `append_pending` in all three cases.
3. Inspection and repair select the active file from the current control file, replay sealed segments and all preceding active events, and validate the intended event with the normal transition rules. They reject extra bytes, an unrelated tail, a different manifest, invalid history, or a simultaneous manifest publication. Repair rechecks both inspection hashes under the lock, refuses clock rollback, syncs the intent, and only then writes the missing bytes. It never truncates the journal or skips an event.

Repair finishes the accounting transition prepared at its recorded time. A repaired reservation consumes its original request and nonce even if the lease has since expired. A repaired claim still holds capacity until the exact owner is reconciled. A repaired recovery retains the original stopped-worker assertion. The assertion is still unverified, and using a reservation or claim still requires the normal current policy and ownership checks.

## Retry and boundaries

- `append_pending` points to `append-repair-status`. `repair_changed` means the bytes changed since inspection. Inspect again and use the new hashes.
- After `commit_uncertain`, retain the state and inspect again before retrying. If the intent was cleared, use `recovery-status` or query the original request ID. A full disk may block writing the suffix until space is freed outside the evidence files.
- `append_repair_unavailable` means there is no intent. Arbitrary corruption, a damaged intent and nonmatching bytes need separate investigation. Retain the journal and evidence. A torn manifest with a publication intent uses [manifest repair](MANIFEST_REPAIR.md).
- New writes require same-directory hard links. Existing healthy legacy and segmented records remain readable. Run one supported binary version per store. Older binaries do not honor `append.pending`. Filesystem ownership is the trust boundary. The hashes detect damaged or inconsistent evidence. External anchoring is still required to detect whole-store rollback or cloning.
- Tests cover process exit, repeated repair, short writes and actual Linux `ENOSPC` on a bounded tmpfs. Persistent quotas and delayed allocation, disk-full sync failures on deployment filesystems, sector or controller faults, physical power loss, and Windows directory metadata durability and private ACLs still require live validation. Authenticated worker execution, external fencing and verified teardown, isolated Mac workers, physical device operation and Apple signing remain separate work.

See [verification evidence](VERIFICATION.md).
