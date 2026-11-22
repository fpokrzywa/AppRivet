# Portable source snapshots

Source snapshots are an implemented, credential-free foundation for a future remote worker. Creation and inspection work on Windows, Linux and macOS. They do not extract, upload, execute, build, sign or authorize code. Remote transport and worker isolation remain unimplemented.

## Use

Select files explicitly in a newline-delimited UTF-8 text file. Paths are relative to the source root and use forward slashes. Blank lines, CRLF and an initial UTF-8 BOM are accepted. There are no glob patterns, directory expansion or comment lines. Selection order does not affect archive bytes.

```sh
apprivet snapshot create --root examples/HelloAppRivet \
  --files-from examples/HelloAppRivet.snapshot-files \
  --output hello-source.tar --json

apprivet snapshot inspect --archive hello-source.tar --json

# Supply a digest obtained separately through a trusted channel:
apprivet snapshot inspect --archive hello-source.tar --expect-sha256 REPLACE_WITH_64_LOWERCASE_HEX_DIGITS --json
```

PowerShell users can put each command on a single line. A ready-to-use list is provided for the original HelloAppRivet example. `--root` defaults to the current working directory. The list, output and archive paths are relative to the CLI working directory, not the source root. Existing output is never overwritten.

The schema-1 CLI envelope's `data` contains `format: "apprivet-source-v1"`, `sha256`, `archive_bytes`, `total_bytes`, and sorted `files` with `path`, `size` and `sha256`. Absolute source paths, usernames, timestamps and file contents are not emitted in the report. Human output includes the file inventory and an integrity-only notice. A failed operation returns no partial report.

Archive contents include the selected source verbatim. File selection and the credential-filename guard are not secret scanning. Review files before sharing; credentials embedded in ordinary source/configuration files will still be packaged. Generated TAR files are Git-ignored by default. There is no automatic Git enumeration, implicit exclusion list that silently drops required files, account access or application service call. Filesystem access follows the selected local or mounted filesystem.

## Format and verification

Version 1 is an uncompressed canonical USTAR archive: lexicographically sorted `source/<relative-path>` regular files, then `manifest.json`, then exactly the canonical TAR ending. The manifest is canonical UTF-8 JSON plus one newline, with schema version 1 and ordered file entries. Source bytes are unchanged. All records use mode 0644, epoch modification time, zero user/group IDs and no owner names or additional metadata. The outer archive is created with mode 0600 where POSIX permissions apply; Windows uses existing filesystem ACLs.

Verification streams data, checks file hashes and sizes against the exact expected manifest, and compares the input's SHA-256 with an independently reconstructed canonical archive. This rejects changed metadata, reordered/duplicate records, extra JSON fields or duplicate keys, extra padding, appended archives, hidden PAX/GNU metadata and trailing data. Arbitrary TAR/ZIP archives are not supported, even if another tool considers them valid. No filesystem extraction occurs.

`--expect-sha256` optionally compares the whole archive to a separately obtained digest. Without it, inspection establishes internal consistency only. An attacker can replace source and manifest and create a different internally valid snapshot. Even a matching expected digest establishes neither sender identity nor permission to execute. Future transport must authenticate identities and bind the digest to a project grant and authorized operation.

Determinism applies to identical selected paths and file bytes under the version-1 encoding. Line-ending changes affect the digest. Filesystem timestamps, owner names, source root and selection order do not. Unicode names, executable bits, extended attributes, ACLs, symlinks, directory entries and empty-directory preservation are unsupported. A bundle may be incomplete or unsuitable for a build; successful inspection is not a build-readiness check.

## Limits and filesystem boundary

| Limit | Value |
| --- | --- |
| Selected files | 1-4096 |
| Individual source file | 16 MiB |
| Total source bytes | 64 MiB |
| Manifest / selection file | 1 MiB each |
| Complete archive | 70 MiB |
| Relative path / segment | 180 / 80 ASCII characters |

Reject absolute/traversal paths, backslashes, empty segments, control characters, Windows device names and forbidden filename characters, leading/trailing segment spaces, trailing dots, duplicate paths and case conflicts in files or directory prefixes. Common credential/signing filenames (`.env` and `.env.*`, private-key/certificate containers, provisioning profiles, SSH key names) and VCS/credential directories (`.git`, `.svn`, `.hg`, `.ssh`, `.aws`, `.azure`) fail visibly. This intentionally rejects some harmless files such as `.env.example` or public `.pem` certificates. The guard never certifies content as secret-free.

Creation uses Go's [os.Root](https://pkg.go.dev/os#Root), checks each component, rejects symlinks and nonregular files, compares opened-file identity, bounds copied bytes and checks size/modification-time consistency. The selected root and its ancestors, output directory and OS are trusted; source should be quiescent. These checks do not promise a transactional snapshot of a concurrently modified tree or defeat a hostile process running as the same OS user. Local regular files with multiple hardlinks are read as bytes; no hardlink records are written or accepted. Filesystem aliases/mounts are not an isolation boundary.

Output uses exclusive creation and may be visible while writing; callers must wait for success and verify before consuming it. Normal errors/cancellation remove the operation's partial output; abrupt process death or a cleanup I/O failure can leave a partial file. Inspect/remove that file deliberately or choose a new destination before retrying. Context cancellation is checked between reads; it cannot interrupt an OS read stuck on an unresponsive filesystem. There is no general filesystem I/O deadline.

The format uses standard [archive/tar](https://pkg.go.dev/archive/tar), with a narrower canonical policy. No external archiving programs or dependencies are invoked.

## Errors

| Code | Exit | Meaning |
| --- | --- | --- |
| invalid_arguments | 2 | Invalid flags, empty selection, invalid expected digest or output selected as input |
| unsafe_path / sensitive_path | 4 | Selection/archive violates path or credential-filename policy |
| unsafe_source | 4 | Symlink, directory, special file or changed opened-file identity |
| archive_exists | 4 | Existing output preserved |
| limit_exceeded | 4 | File, byte, count, manifest or archive limit exceeded |
| io_error / source_changed | 4 | Access or consistency failure; no complete result accepted |
| invalid_snapshot / digest_mismatch | 4 | Malformed/noncanonical archive or expected digest mismatch |
| cancelled | 130 | Operation cancelled; no result accepted |

## Evidence and remaining validation

Automated tests cover deterministic roundtrips, independent hashes, corruption/truncation/trailing bytes, metadata/manifest ambiguity, archive links, path/case/prefix conflicts, sensitive filenames, quotas, source symlinks, cancellation, partial-output cleanup, no overwrite, and portable CLI behavior with a runner that fails on any subprocess invocation. A valid archive seeds the bounded fuzz target alongside malformed inputs. CI runs a real CLI roundtrip of the original example on Windows/Linux/macOS and native Linux fuzzing. Exact completed runs are recorded in [STATUS](STATUS.md) and [VERIFICATION](VERIFICATION.md); a configured job alone is not evidence.

Still open: authenticated remote transfer, extraction into a hardened worker, credential isolation, hostile build-script penetration tests, server-side quotas/grants, approval binding/replay protection and artifact transfer to a separate signer. Those require their own implementations and validation; a snapshot digest does not satisfy them. No extracted-snapshot Xcode build has been tested because extraction is not implemented. Physical-device states, signing, installation and launch retain the [device validation gaps](DEVICES.md#live-validation-still-required). No Apple credentials were needed for this feature.
