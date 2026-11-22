# Distribution preparation

No versioned binary release, installer, license, update channel, subscription, payment or activation system is implemented. The owner made the source repository public on 2026-11-22. Source-build instructions are the current delivery mechanism; license selection and release publishing remain separate decisions.

Candidate artifacts: macOS arm64/amd64, Windows amd64/arm64 `.exe`, Linux amd64/arm64 CLI binaries. Cross-compiling a Go client is not iOS app compilation. Windows/Linux users initially get project preparation and diagnostics; remote builds will be advertised only after that service exists and is tested.

Before a release: approve distribution/license terms; set compatibility matrix; pin toolchain and action revisions; test each architecture; generate version via `-ldflags '-X main.version=VERSION'`; build with `-trimpath`; produce SHA256 manifest, provenance and SBOM; vulnerability-scan dependencies/toolchain; review artifact contents for keys and Apple SDK files; protect release tags and restrict workflow permissions. Rehearse privately. Public publishing requires explicit approval.

Then evaluate a notarized/signed macOS package or Homebrew formula, Windows signed executable/package, and Linux archives/packages. Verify clean-machine installation and uninstall. A checksum hosted next to a compromised binary is not sufficient integrity protection; use signed provenance or an independently trusted signature. No automatic updater until metadata signing, rollback and key-rotation behavior are designed.

Private repository fallback setup, if needed in another environment:

```sh
gh auth login --web
gh repo create AppRivet --private --source . --remote origin
gh repo view --json nameWithOwner,isPrivate,url
git push -u origin main
```

Use the official browser/device authorization flow; never give AppRivet or an assistant an account password. Do not recreate a repository that already exists.
