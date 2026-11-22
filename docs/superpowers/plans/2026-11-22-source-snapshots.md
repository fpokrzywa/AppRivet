# Portable Source Snapshots Implementation Plan

Goal: create and verify explicitly selected source bundles on Windows/Linux/macOS without executing source, extracting archives, contacting services or accessing credentials.

Architecture: a standard-library-only internal/snapshot package owns a deliberately narrow, versioned, uncompressed canonical TAR format. Sorted regular source files precede a canonical JSON manifest containing path, size and SHA-256. Inspection hashes the actual bytes and independently reconstructed canonical archive, checks every file against the manifest, and optionally checks a separately supplied archive digest. CLI adds snapshot create/inspect subcommands.

Decisions under the user's authorization to continue independently: use an explicit newline file list rather than automatically walking a workspace; use streaming TAR rather than ZIP to avoid decompression and central-directory allocation risks; do not add remote endpoints, extraction, signing or approval flags. Continue in the dedicated repository with small commits, following the existing authorized push workflow. Native implementation, then one independent whole-change review.

Contract: version 1 supports portable ASCII relative paths, regular files normalized to mode 0644, no executable metadata, no links or directories in the archive. Reject traversal, case collisions including directory prefixes, reserved Windows names, common credential/VCS paths, duplicate entries, hidden metadata and trailing bytes. Limits: 4096 files, 16 MiB per file, 64 MiB total source, 1 MiB manifest/list, 70 MiB archive. Output is exclusively created with mode 0600, never overwritten; failed/cancelled creation removes owned partial output. Inspection never writes files. File-list selection is not a secret scanner; hashes are integrity checks, not identity, permission, buildability or approval.

Interfaces: Create(ctx, root, files, output) (Report,error); Inspect(ctx, archive, expectedSHA256) (Report,error); Verify(ctx, reader, expectedSHA256) (Report,error); ReadSelection(path) ([]string,error). Report supplies archive SHA256, archive bytes, total source bytes and sorted file entries, without absolute source paths. Errors omit OS paths/content. Source root and ancestors are trusted and quiescent; os.Root confines opened source paths, with component/type and opened-file identity checks. Local hardlinked files are read as bytes, never archived as hardlinks; hostile same-user concurrent filesystem mutation is outside this boundary.

## Tasks

- [ ] Write failing format/security tests: deterministic roundtrip, independent digest, tampering, traversal/case aliases, file-prefix conflicts, symlinks/hardlinks in archives, metadata, malformed/truncated/trailing data, limits, cancellation, source symlinks, sensitive paths and no-overwrite cleanup. Implement streaming creation and verification. Expected: tests first fail for missing API, then pass.
- [ ] Write failing CLI tests for create/inspect, JSON errors, human output, relative paths, help, digest mismatch and no subprocess. Add isolated CLI handler and help. Expected: unknown-command failures first, then portable success and stable errors.
- [ ] Add archive fuzz target, native CI fuzz budget, and a CLI roundtrip smoke against the original example on every Go matrix host. Run tests, vet, Windows native fuzz, Linux runtime tests and six cross-builds. Independent review; reproduce and fix material findings.
- [ ] Update user docs, threat model, architecture, roadmap, CHANGELOG and status with exact guarantees, rejected formats and remaining Mac/device/credential/isolation gates. Commit, push, follow CI to completion and record actual results.

Review focus: canonical archive ambiguity and extra bytes; bounded streaming on malicious sizes; output cleanup/no overwrite; local filesystem confinement versus unsupported hostile races; integrity versus authorization and source-content privacy. No release or listener is enabled.

Ledger: baseline 4099444. Plan interfaces align: CLI consumes the package's typed report/errors; CI exercises the same public commands. Approval/permission flows remain future design, not a fake CLI switch. Native execution authorized by ongoing user request and earlier opt-out from routine design checkpoints.
