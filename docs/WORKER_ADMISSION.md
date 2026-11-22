# Offline worker admission checks

`worker check` evaluates a local policy against a request, a separately supplied principal and the actual snapshot bytes. It is read-only and portable. It does not authenticate the principal, create an extraction directory, reserve a worker, submit a job or authorize execution. This is the policy-checking core for a future worker service, not that service.

```sh
apprivet worker check --policy policy.json --request request.json \
  --principal developer --archive source.tar --json
```

Use one line in PowerShell. All filesystem paths are relative to the CLI working directory. Policy and request must be regular files; final-component symlinks are refused. Their parents and the filesystem are trusted. Keep policy under operator control; never load worker policy from an uploaded source bundle or accept the request's assertion of its own identity.

## Schema 1

Policy example below is deliberately expired. Replace the validity window and digest for a deliberate local check; creating this JSON is not a trusted human-approval workflow. IDs/principals are case-sensitive ASCII identifiers: a letter/digit followed by letters/digits, dots, underscores or hyphens, at most 64 characters. No account credentials are involved.

```json
{
  "schema_version": 1,
  "grants": [{
    "id": "grant-1",
    "principal": "developer",
    "project_id": "hello",
    "operation": "extract",
    "snapshot_sha256": "0000000000000000000000000000000000000000000000000000000000000000",
    "enabled": true,
    "not_before": "2026-01-01T00:00:00Z",
    "expires_at": "2026-01-01T01:00:00Z",
    "limits": {
      "max_files": 16,
      "max_file_bytes": 1048576,
      "max_source_bytes": 1048576,
      "max_archive_bytes": 2097152
    }
  }]
}
```

Corresponding untrusted request:

```json
{
  "schema_version": 1,
  "grant_id": "grant-1",
  "project_id": "hello",
  "operation": "extract",
  "snapshot_sha256": "0000000000000000000000000000000000000000000000000000000000000000"
}
```

Only `extract` is supported. Requests contain no principal assertion, host path, command, environment, signing option or executable arguments. Unknown fields, nulls, duplicate/case-alias keys, non-ASCII object keys, malformed field types, extra JSON documents and nesting beyond 16 levels are rejected. Each document is capped at 64 KiB. Policies contain 1-64 grants with unique IDs. Disabled grants are valid policy but cannot match an eligible request; omitted `enabled` defaults to disabled.

Both timestamps are required RFC3339 instants. A grant's window must be positive and no longer than 24 hours. Evaluation requires `not_before <= checked_at < expires_at`. Each resource cap must be positive and no larger than the [snapshot hard limits](SNAPSHOTS.md#limits-and-filesystem-boundary). Unknown schema versions fail; there is no permissive future-version fallback.

## Evaluation and result

1. Validate the principal, request and entire policy.
2. Select the exact grant ID; match enabled state, principal, project, operation, digest and validity window. Denials happen before opening the archive.
3. Check archive type/identity and size, then bound the read to the grant's archive-byte cap plus one overflow-detection byte. Verify canonical encoding and the bound expected digest.
4. Check actual file count, per-file bytes and total source bytes against the grant. Cancellation produces no accepted decision.

Successful schema-1 CLI output includes `mode: "offline_policy_check"`, `eligible: true`, **`authentication_verified: false`**, **`execution_authorized: false`**, the matched IDs/operation, `checked_at`, `valid_until`, and the verified snapshot report. Human output carries the same limitation. Errors return no partial decision. `checked_at` is the trusted caller's clock instant (the system clock for the CLI), not a promise that the grant will still be valid when a later consumer acts.

The CLI's `--principal` is a hypothetical identity supplied by its operator. Future network services must obtain identity from authenticated transport, load operator-owned policy independently, recheck validity/revocation at use, bind the exact digest to the authorized operation, and isolate execution. No reusable bearer token, signature, one-time approval, replay ledger, concurrency reservation or durable job is issued. Repeating a check is allowed and consumes no grant. There is no automatic chain from this result to extraction or execution.

## Errors

| Code | Exit | Meaning |
| --- | --- | --- |
| invalid_arguments | 2 | Missing flags, invalid principal or unset caller clock |
| invalid_policy / invalid_request | 4 | Unsupported schema, ambiguous JSON or invalid fields |
| admission_denied | 4 | No active grant matches every binding |
| resource_limit | 4 | Document, archive or declared source quota exceeded |
| io_error | 4 | Inaccessible/nonregular file or changed opened-file identity |
| invalid_archive | 4 | Canonical integrity or expected digest verification failed |
| cancelled | 130 | No decision accepted |

## Validation boundary

Security tests cover binding mismatches before archive access, unsupported build/sign operations, disabled/not-yet-valid/expired and overly long grants, duplicate IDs, malformed/ambiguous JSON, quotas, wrong archive bytes, cancellation, symlink refusal and explicit non-authentication. CI exercises the real CLI on Windows/Linux/macOS and fuzzes policy/request parsing. Completed run evidence belongs in [VERIFICATION](VERIFICATION.md).

Still unimplemented/unvalidated: mTLS or another authenticated transport, protected policy deployment/rotation, trustworthy human approvals, revocation at use, replay prevention, concurrent admission reservations, per-user aggregate disk/CPU/memory quotas, an isolated Mac build worker, hostile build-script containment, signer isolation and artifact transfer. Local checks and extraction are not substitutes for those controls. Apple credentials and physical-device workflows remain separate live-validation gates.
