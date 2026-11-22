# Milestone 2 device discovery implementation plan

Goal: provide read-only, typed device inventory without Apple credentials and make discovery parsing testable on non-Mac hosts. This is the discovery portion of M2; signing/install/launch remain gated on real hardware and isolation work.

Architecture: internal/devices owns normalization and adapters. CLI presents a common inventory for devicectl or simctl, never merges their identifiers, and marks offline reports explicitly. Runner remains responsible for deadlines and process cleanup. No additional dependencies. Spec: PRD Phase 1 discovery, ARCHITECTURE device boundary, and this plan's acceptance criteria.

Decisions: continue in the existing clean dedicated repository under the user's authorization to progress routinely. Use native devicectl JSON files for compatibility with older Xcode; do not scrape display tables. Add simctl discovery as useful credential-free work. Offline `--from-json` reads a report, never executes a project or treats that report as device authorization.

## Acceptance criteria / review focus

- `devices` on macOS invokes only fixed `xcrun devicectl list devices` arguments and reads a private temporary JSON report. `devices --simulators` invokes `xcrun simctl list devices --json`.
- `devices --from-json FILE [--simulators]` works on all CLI hosts, records live=false/source=file, and never calls a process. Its records are observations, not authority to install or pair.
- Parse legacy and modern `properties` devicectl layouts; prefer modern values; never infer physical hardware from an identifier's shape. Preserve unknown fields as unknown normalized states, not false connectivity/authorization claims.
- Empty inventories succeed. Tool failure, failed outcome, missing/malformed structures, duplicate identifiers and oversized reports fail visibly; no partial list masquerades as complete. Unknown additive fields are ignored; unsupported schema versions fail clearly.
- Bound report sizes, reject nonregular/symlink reports, remove private temporary reports after every path, omit raw serial numbers/network addresses/private fields, sanitize display strings, and provide stable error codes and exit statuses.
- Tests use original synthetic fixtures labeled as such. Test both pure parsing and real filesystem adapter boundaries with fake Apple subprocesses; also CLI formatting/errors and fuzz invariants.
- Add Mac CI discovery smoke checks plus a manual device validation matrix; never claim live hardware evidence from fixture or hosted-empty-list tests.

## Tasks

- [ ] Research current devicectl schema changes and record source/provenance limitations.
- [ ] Write failing parser tests and synthetic fixtures; implement normalized devicectl/simctl parsers.
- [ ] Write failing adapter tests for bounded safe reads, cleanup, timeout/cancellation and platform refusal; implement List(ctx, host, runner, Request).
- [ ] Write failing CLI tests; add devices flags, human output and JSON contract.
- [ ] Add offline fuzz/smoke verification and credential-free Mac CI discovery commands.
- [ ] Run Windows/Linux tests, vet, cross-builds and independent review; fix concrete findings with regression tests.
- [ ] Update README, architecture/threat model, roadmap, CHANGELOG and STATUS; commit and push completed work; record remaining live validation gates and CI outcome.
