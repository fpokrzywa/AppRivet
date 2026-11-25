# Verification record

## Graphical frontend preview

All five jobs passed at `6627dd4f8e52b3539614b078e334de547f33115d`. The packaged macOS app created and reopened the SwiftUI sample, selected an iPhone 16 simulator, built the app, booted the simulator, installed the build and launched `org.example.apprivetdesktop`. CI checked both Mach-O architecture slices. Runtime launch was tested on hosted Apple silicon. Windows and Linux ran the packaged launchers through project creation, inventory and clean shutdown. A local Chrome test also exercised the form, narrow layout, inventory update and quit. Initial Mac runs exposed a `lipo` argument order error and a Simulator command timeout. The final workflow corrected the verifier and uses an independent Mac job with phase diagnostics and bounded startup, install and launch deadlines. [Passing CI run and downloads](https://github.com/fpokrzywa/AppRivet/actions/runs/45553338536#artifacts)

## Native Linux journal writeback errors

The Linux device-mapper harness fails I/O to one initialized data block on a disposable ext4 image. It requires native `fsync` `EIO` after a buffered journal append and during repair. One case remounts first and verifies that an exact prefix can be completed and divergent bytes stay blocked. Another retries on the restored mount and verifies a successful acknowledgement survives remount. Failed repair of a fully written event must retain its marker. Subsequent repair preserves the intended head, rejects replay and stale ownership, and retains the original worker, claim and fence after another remount. The initial run found that an event could appear complete from cache after `EIO` but diverge on disk. A regression failed before the repair writer reasserted the exact event ahead of sync. See [Linux native writeback tests and remaining gaps](LINUX_JOURNAL_FAULTS.md).

All four jobs passed at `20695a98717f3222d6a34bcc8f3a1c3742348a9b`. The Linux test observed native `EIO` in all three cases, refused the divergent tail after remount, and verified that a same-mount retry of the exact 416 bytes survived remount. Local Windows Go tests, vet and CLI smoke and Linux admission tests passed. Hosted CI also passed all seven Linux fuzzers with 10,000 inputs each, Unix race checks and simulator XCTest. Two earlier hosted attempts timed out at the 60-second simulator discovery step. The final workflow permits the supported five-minute simulator discovery deadline and still requires discovery success. [CI run 45077886251](https://github.com/fpokrzywa/AppRivet/actions/runs/45077886251)

## Persistent Linux quota and disk-full sync

The ext4 harness uses a bounded 32 MiB image and an unprivileged journal process. It checks persistent user quota after a clean unmount and remount, actual `EDQUOT` at the journal write with global capacity available, and repair retry after freeing quota. It also fills the volume between an accepted journal write and its native sync, records the native result, and separately injects `ENOSPC` at the application sync boundary. The assertions check exact completion, ownership and fence preservation, stale owner refusal, nonce and request replay rejection, and no duplicate event on retry. See [Linux test instructions and live gaps](LINUX_JOURNAL_FAULTS.md).

All four hosted jobs passed at `3b38951e41de2130aef76d2ab1ff45a61df8730e`. Linux recorded `EDQUOT` on the active write with 6,414 available blocks, then `ENOSPC` on single-block allocation and successful native `fsync` with zero available blocks. The injected sync error left a complete pending event that repaired with zero bytes written. Windows, Linux and macOS Go tests and CLI smoke, Unix race checks, all seven Linux fuzz targets, and hosted simulator XCTest and builds passed. The handoff commit changes documentation only. [CI run 45074823532](https://github.com/fpokrzywa/AppRivet/actions/runs/45074823532)

## Recovery for interrupted journal appends

The portable tests cover zero-byte, partial, short and complete-but-unsynced append states in both journal formats. Actual child processes exit during an append and during repair. The tests verify stale inspection refusal, single completion under contention, preserved request and nonce replay rejection, exact ownership and fences, expired claimed capacity, and read-only inspection. Malformed, oversized, duplicate, linked, wrong-path, wrong-manifest, wrong-fence and divergent-tail evidence is refused. A torn initial event can be completed only when the recorded initialization intent survives.

The Linux 1 MiB tmpfs test obtains kernel `ENOSPC` while writing the repair suffix after intent publication, frees space and completes the exact event. Real CLI smoke exercises both repair commands using a synthetic torn append. CI also fuzzes the bounded canonical intent parser. Persistent filesystem quotas, delayed allocation, disk-full sync errors, power cuts and Windows metadata durability remain live gaps. See [append repair contract and boundaries](APPEND_REPAIR.md).

Local Windows tests, vet, CLI smoke and 10,000 append-intent fuzz inputs passed. Local Linux admission tests and the real tmpfs exhaustion checks passed in a restricted container. All four hosted jobs passed at `11e46c3ce75f84951e95feca8e0f927ac993b593`, including the Windows/Linux/macOS Go suites and CLI smoke, Unix race checks, kernel `ENOSPC`, seven Linux fuzz targets with 10,000 inputs each, simulator XCTest and the framework-name collision build. The handoff commit changes documentation only. [CI run 45072074260](https://github.com/fpokrzywa/AppRivet/actions/runs/45072074260)

## Disk-full and interrupted journal writes

Portable tests cover file creation, zero-byte writes, partial writes with `ENOSPC`, short writes and failed file syncs. Real subprocess exits interrupt staging of both the pending marker and the before-image. Migration and compaction tests verify that faults before the marker preserve the original journal, and that control-write faults after the marker leave enough evidence for explicit repair. Ownership append tests check both formats. A torn event blocks replay and ownership checks without modifying the stored bytes. A complete recovery event followed by a sync error can be observed on reopen, and retry does not append a second event.

The Linux CI step fills an isolated 1 MiB tmpfs using actual writes. It requires kernel `ENOSPC`, then tests migration, before-image creation during repair, and recovery append against that full mount. After releasing the filler, the test checks successful retry, unchanged history where expected, exact ownership, and replay rejection. Tmpfs exercises the kernel allocation failure path. Persistent filesystem quotas, delayed allocation, disk-full sync behavior, physical power loss and Windows metadata durability remain open. See [manifest repair and deployment limits](MANIFEST_REPAIR.md).

Local Windows tests, vet and CLI smoke passed. Local Linux admission tests and all three tmpfs scenarios passed in a restricted Docker container. All four jobs passed at implementation commit `e1938a3ccd23df04cc0a0858de01e47ebf3f56c2`, including hosted tmpfs exhaustion, Windows/Linux/macOS Go tests and CLI smoke, Unix race checks, six Linux fuzzers with 10,000 inputs each, and the hosted simulator build and XCTest. The handoff commit changes documentation only. [CI run 45069431194](https://github.com/fpokrzywa/AppRivet/actions/runs/45069431194)

## Intent-backed manifest repair (2026-11-22)

Local Windows tests and vet passed across all eight tested packages; admission coverage is 85.6%, CLI 88.5%. Native Windows publication-intent and journal-storage fuzz targets each completed 10,000 inputs. All eight Linux package executables passed in the restricted container, and all six macOS/Linux/Windows amd64/arm64 client cross-builds passed. Real CLI smoke exercised a synthetic torn control using the actual publisher's retained intent, pinned repair, saved damaged bytes, preserved claimed ownership and replay rejection. All four jobs in [run 45067510656](https://github.com/fpokrzywa/AppRivet/actions/runs/45067510656) passed at implementation/workflow commit `cebca8c58ca9f31bb1aefe0927cb5e5bc4a25979`: Windows/Linux/macOS tests, vet and CLI smoke; Linux/macOS race checks; all six native Linux fuzz targets with 10,000 inputs each; and Mac discovery, extracted-source build, one passing XCTest and framework-name collision build. The following handoff commit changes documentation only.

Security tests cover recorded migration/compaction tears, exact source/empty/prefix/complete-target control states, valid later events on a complete target, retained replay keys and expired claimed capacity, stale inspection pins, malformed/duplicate/unknown/path-bearing intent, changed source/active/archive/target/lineage, competing generations, symlinks, bounded inventory, before-image collisions, clock rollback and cancellation. Injected truncate/write/sync failures cover repeated repair and late cancellation. Actual child processes exit during publication and repair; concurrent repairs yield one winner while reservations remain blocked or reject replay. Missing API and CLI tests failed before implementation and passed afterward.

Independent review identified two retry durability gaps: matching existing before-images and a complete pending marker were not re-synced before control publication. Both are fixed with validation plus file sync before directory sync/control mutation. Regression tests cover complete leftover evidence and injected sync rejection. The reviewer verified both fixes and independently passed all repair tests, finding no residual actionable issues.

The [repair contract and remaining live gates](MANIFEST_REPAIR.md) explicitly distinguish local process-crash/fault tests from power-cut and deployment durability. Older tears without intent, damaged evidence, external rollback/cloning, mixed-version writers, private Windows ACLs, authenticated execution/fencing/teardown, isolated Mac workers and physical-device/Apple credential workflows remain outside this validation.

```sh
go test ./internal/admission -run='^$' -fuzz=FuzzPublicationIntent -fuzztime=10000x -parallel=2 -timeout=60s
```

## Safe journal migration and compaction (2026-11-22)

Local Windows full tests and vet passed across all eight tested packages (admission coverage 86.3%, CLI 88.8%). Native Windows storage fuzzing completed 10,000 inputs. Real CLI smoke passed migration and compaction with a claimed job, preserved its owner tuple and event head, rejected stale ownership after recovery, and retained replay refusal. All eight compiled Linux test executables and six client cross-builds passed.

Security tests verify byte-identical archived history, all-event replay, capacity retention, full-segment maintenance and global fences beyond 16,384, concurrent maintenance/reservation processes, strict manifest/path/size checks, missing/corrupt/linked/reordered segments, single-member gzip and expansion limits, publication truncate/write/sync faults, cancellation after commit, and an actual child-process exit during a partial control-file publication. The crashed store refuses normal operations while the prepared manifest and verified history remain intact. Missing API/CLI tests failed before implementation and passed afterward.

Independent review found no actionable issues and independently passed the admission/CLI suites. [Run 45064383805](https://github.com/fpokrzywa/AppRivet/actions/runs/45064383805) at commit efe2716 passed all four jobs, including Windows/Linux/macOS tests/vet/CLI smoke, Linux/macOS race checks and five Linux fuzz targets with at least 10,000 inputs each. Mac discovery, extracted-source build/XCTest and framework-name collision build passed. The following handoff commit changes documentation only. [JOURNAL_MAINTENANCE](JOURNAL_MAINTENANCE.md#evidence-and-remaining-live-validation) records the remaining limits: no automatic torn-manifest repair/fallback, no relocation or external rollback/clone anchor, bounded lifetime history, Windows metadata/ACL and power-cut validation, authenticated ownership and live execution teardown, or Apple hardware/signing guarantees. Migration is an in-place format upgrade; compaction is lossless segment sealing.

```sh
go test ./internal/admission -run='^$' -fuzz=FuzzJournalStorage -fuzztime=10000x -parallel=2 -timeout=60s
```

## Reservation recovery and ownership (2026-11-22)

Local Windows tests and vet passed across all eight tested packages; admission coverage is 86.6%, CLI 88.9%. Native Windows journal fuzzing completed 10,000 inputs with reserved/claimed/recovered state seeds. Real CLI smoke passed exclusive claim, current-owner checking, read-only recovery inspection, ordinary-release refusal, stopped-assertion requirement, explicit recovery, old-owner refusal and retained request replay protection. All eight compiled Linux package test executables and six client cross-builds passed.

Security tests cover six-process claim contention with one winner, durable ownership inspection after process exit, exact tuple/principal bindings, retained global/per-principal/byte capacity after claimed expiry, changed/revoked policy and archive bytes, final deadline checks, cancelled operations, complete/partial uncertain writes and cancellation after sync, fully populated journal inspection, clock rollback, legacy canonical records and invalid correctly rehashed transitions. API/CLI tests failed before implementation. One new byte-budget test initially undercounted its eight-byte source fixture; correcting the test restored the expected capacity boundary.

Independent review found no actionable issues and independently passed the admission/CLI suites and diff checks. [Run 45061672025](https://github.com/fpokrzywa/AppRivet/actions/runs/45061672025) at commit 9448180 passed all four jobs, including Windows/Linux/macOS tests/vet/CLI smoke, Linux/macOS race checks and all four Linux fuzz targets with at least 10,000 inputs each. Mac discovery, extracted-source build/XCTest and framework-name collision build passed. The following handoff commit changes documentation only. [WORKER_RECOVERY](WORKER_RECOVERY.md#evidence-and-remaining-live-validation) records the limits: no authenticated worker, external side-effect fencing, live process/VM reconciliation, verified teardown, protected state migration/rollback anchoring, private Windows ACL enforcement, isolated Mac execution or physical-device/signing validation. Recovery is local accounting and never proof that a process stopped.

## Replay protection and local reservations (2026-11-22)

Local Windows tests and vet passed across all eight tested packages; admission coverage is 86.2%, CLI 89.0%. Native Windows journal fuzzing completed 10,000 inputs. The real CLI smoke passed extraction/admission plus init, reserve, exact replay refusal, status recovery, release, retained nonce refusal and a fresh reservation. All eight compiled Linux package test executables passed in the restricted container; all six macOS/Linux/Windows amd64/arm64 client cross-builds passed.

Tests cover cross-process duplicate/capacity races (six claimants, one winner), lock release after process death, restart/release/expiry replay history, request conflicts, principal separation, actual-byte budgets, grant expiry clamping, post-verification policy revocation/quota changes, cancellation, observed clock rollback, journal bounds/canonical encoding/invalid transitions, missing/replaced/symlinked state, injected partial writes/sync failures and cancellation after sync. Missing APIs/CLI commands failed first, then passed after implementation. The Windows suite may skip symlink or open-file replacement cases when the host denies those operations; Unix CI exercises its platform behavior.

Independent review found no P1/P2 issues. One P3 request-conflict/error-precedence inconsistency was reproduced with a failing two-record regression, then fixed; the full Windows suite and vet passed again. [Run 45059896975](https://github.com/fpokrzywa/AppRivet/actions/runs/45059896975) at commit d2bb5a9 passed all four jobs: Windows/Linux/macOS tests, vet and CLI smoke, Linux/macOS race checks, and four native Linux fuzz targets with at least 10,000 inputs each. The Mac integration job passed discovery, extracted-source build/XCTest and the framework-name collision build. The following handoff commit changes documentation only. Existing Mac checks remain enabled; no Mac/Apple execution property is inferred from the local tests. [WORKER_RESERVATIONS](WORKER_RESERVATIONS.md#validation-and-remaining-live-gates) records the deployment gaps, including authenticated identity, execution fencing/teardown, rollback-resistant storage, power-loss recovery, Windows ACL enforcement and physical devices/signing.

```sh
go test ./internal/admission -run='^$' -fuzz=FuzzReservationJournal -fuzztime=10000x -parallel=2 -timeout=60s
```

## Safe extraction and offline admission (2026-11-22)

Final [run 45057268258](https://github.com/fpokrzywa/AppRivet/actions/runs/45057268258) passed all four jobs at commit 54e796c. Windows/Linux/macOS tests, vet and real extraction/admission CLI smoke passed; Linux/macOS race checks passed; each native Linux device/snapshot/admission fuzz target completed 10,000 inputs. The hosted Mac successfully built the extracted original SwiftUI project, ran its single XCTest, and built the framework-name collision regression. The following handoff commit changes documentation only. This is local extraction plus ordinary trusted-source Mac build evidence, not authenticated or isolated worker execution.

Windows tests and vet passed across eight packages: snapshot coverage 81.0%, admission 89.0%, CLI 88.1%. All eight Linux test executables passed in the restricted container; six client cross-builds passed. Native Windows snapshot verification and admission JSON fuzz each completed 10,000 inputs. The real Windows CLI smoke passed create/inspect/extract, independently calculated file/archive hashes, existing-destination refusal, honest offline decisions and policy denial before nonexistent-archive access.

Extraction and CLI tests failed for absent APIs/commands before implementation. Security cases cover tampering, traversal/links/metadata, mandatory digest, changed original archive after verification, concurrent destination ownership, cancellation/partial cleanup, refusal to recursively delete a replaced destination, preplanted files/links, and POSIX modes. Admission tests cover strict JSON, bindings, grant validity/expiry, disabled and omitted-enabled behavior, actual resource counts, wrong archive bytes, cancellation, symlinks and no subprocess execution. Fuzzing is bounded evidence, not exhaustive validation.

Independent review found no P1/P2 issue. Its minor enabled-presence discrepancy was settled as documented default-disabled behavior with a regression test. The reviewer independently reproduced inherited Windows ACL permissions despite numeric mode requests; access-controlled temp/destination storage is required. These tests do not establish authenticated transport, policy deployment security, one-time approval, replay protection, reservations, worker isolation, credential separation or physical-device behavior.

CI performs real extraction/admission smoke on Windows/Linux/macOS, native Linux fuzzing for all three parsers, and build/XCTest from an extracted original SwiftUI project. The completed run above supplies that evidence. Reproduce the new parser check with:

```sh
go test ./internal/admission -run='^$' -fuzz=FuzzAdmissionJSON -fuzztime=10000x -parallel=2 -timeout=60s
```

## Historical portable source snapshot baseline (2026-11-22)

Final [run 45055091739](https://github.com/fpokrzywa/AppRivet/actions/runs/45055091739) passed all four jobs at commit 5807712. Windows/Linux/macOS tests, vet and real snapshot CLI roundtrips passed; Linux/macOS race checks passed; native Linux device and snapshot fuzz targets each completed 10,000 inputs. Existing Mac discovery, original SwiftUI app build, simulator XCTest and framework-name collision regression passed. The following handoff commit changes documentation only. Local verification also passed all seven Linux package executables in the restricted container, all six client cross-builds, and a real Windows CLI example roundtrip whose archive digest was independently checked with PowerShell Get-FileHash. The final Windows native snapshot fuzz run completed 10,000 inputs after the cancellation fix.

Windows tests and vet passed across seven packages; snapshot coverage is 83.5%. Tests cover canonical deterministic archives, independent per-file/archive hashes, tampering/truncation/metadata/manifest/trailing bytes, archive links, path/case/prefix/device-name conflicts, credential filenames, byte/count quotas, source symlink refusal, cancellation, no overwrite and partial-output cleanup. CLI tests run with a subprocess-forbidding runner. Tests failed before the package/CLI implementations existed. Additional console-name and final-EOF cancellation regressions failed before their fixes.

The snapshot reviewer found the final-EOF cancellation edge and no other material defect. That finding is fixed with a deterministic reader regression. Same-user filesystem races, content-level secret detection and source buildability remain outside the contract. Live cross-platform CI evidence is recorded in STATUS after it completes; source packaging neither requires nor validates Apple credentials, physical devices, extraction, signing or isolated workers.

Native snapshot fuzzing uses a valid archive seed plus malformed inputs, with 10,000 executions and a 60-second outer deadline:

```sh
go test ./internal/snapshot -run='^$' -fuzz=FuzzVerify -fuzztime=10000x -parallel=2 -timeout=60s
```

At that revision CI created and inspected an archive from `examples/HelloAppRivet.snapshot-files` on each Go matrix host, comparing complete JSON reports. Extraction and extracted-source builds were not yet implemented. See the newer section above and [SNAPSHOTS](SNAPSHOTS.md) for current evidence.

## Milestone 2 update (2026-11-22)

The owner changed the repository to public and hosted CI now starts. Final [run 45053200343](https://github.com/fpokrzywa/AppRivet/actions/runs/45053200343) passed all four jobs at commit 7c3d011: Windows/Linux/macOS Go tests and vet, Linux/macOS race checks, 10,000 native Linux coverage-guided fuzz inputs, read-only Mac device discovery, generated SwiftUI app build, simulator XCTest and framework-name collision build. Xcode 16.4 (16F6), iOS Simulator SDK 18.5, macOS arm64; one XCTest passed on iPhone 16 Pro / iOS 18.5. devicectl 506.7 / JSON v3 returned zero devices; simctl listed 132 shutdown simulators. The following handoff commit changes documentation only. The earlier billing failures below are historical. The first successful integration baseline was [run 45051791524](https://github.com/fpokrzywa/AppRivet/actions/runs/45051791524) at 4f1ab4b.

Current local Windows `go test -count=1 -cover ./...` and `go vet ./...` passed across six packages. Device-package coverage was 92.4%. Real CLI offline inventory inspection passed; live Windows discovery returned structured unsupported_platform/exit 3 as intended. Current Linux test binaries for all six packages passed in the restricted local Docker setup below. All six CLI cross-builds passed.

Both review findings were reproduced before fixing: duplicate object keys silently overwrote inventories, and explicit null modern states fell back to stale connected/paired values. Regression tests now pass, including ASCII and Unicode simple-fold case-alias duplicates. A coverage-enabled child process also exposed an overly strict test expectation about empty stderr; the corrected test verifies payload separation while allowing Go's own coverage warning.

Parser fuzz smoke: a 10-second Linux run executed 274,120 inputs with no failure. The cross-compiled test binary lacked coverage-guided instrumentation, so this is a bounded smoke check rather than comprehensive fuzzing. Native time-limited runs on Windows and Linux passed but exercised only six mutations and are weak evidence. A subsequent native Windows coverage-guided run completed 10,000 inputs in 1.6 seconds without failure. CI now uses an execution count with an outer deadline so a nominally passing short timed run is not mistaken for substantial exploration. This remains bounded fuzz evidence, not exhaustive validation:

```sh
go test ./internal/devices -run='^$' -fuzz=FuzzParse -fuzztime=10000x -parallel=2 -timeout=60s
```

Synthetic fixtures and mocked Apple processes prove normalization, file cleanup, limits and command construction; they do not prove physical-device operation. See [the live validation matrix](DEVICES.md#live-validation-still-required). Signing, device installation/launch and Apple account integration remain unimplemented.

CI timing caveat: attempt 1 of [run 45052339395](https://github.com/fpokrzywa/AppRivet/actions/runs/45052339395) passed all Go jobs but simctl discovery exceeded its 60-second deadline. An unchanged Mac retry passed discovery before being superseded by the final CI/documentation update during XCTest. The timeout was surfaced as exit 124 with no inventory accepted. This is a known intermittent observation; a successful rerun does not establish its root cause or eliminate the reliability risk.

## Milestone 1 historical record

Date: 2026-11-22. Host: Windows amd64. Toolchain: Go 1.27.4, downloaded from the official Go distribution and SHA256 verified before extraction. Windows archive hash: `178ab98950ac69ab25069307dc6ade79f3816e414cce95b4da166a115cde1e7e`.

## Local evidence

- `go test ./...`: passed for project, runner, doctor, build and CLI packages.
- `go vet ./...`: passed.
- `go build -o bin/apprivet.exe ./cmd/apprivet`: passed.
- Real CLI `init`, `inspect`, `version --json`: passed on Windows. Checked-in example generated by the actual CLI.
- Real CLI `doctor --json`: returned structured Windows report and exit 3 as expected; iOS readiness false.
- Tests initially failed because the relevant implementations did not exist, then passed after implementation.
- Linux runtime: all five package test executables passed in an existing local Python Debian container, running as UID/GID 65534, network disabled, capabilities dropped, root filesystem and mounted tests read-only, with a writable temporary directory.
- Review regressions: generated module-name collision cases failed before the fix and passed after it. A real Unix descendant heartbeat continued after parent exit before cleanup was fixed; open/closed-output exit, active cancellation and timeout cases all passed after the fix.
- Six client cross-builds passed: darwin/linux/windows, each amd64/arm64. This checks Go client portability only, not Apple SDK compilation or runtime behavior on all architectures.

The tool runner tests execute real child processes for output capture, exit status, environment filtering, timeout and cancellation. Apple tooling is mocked for local doctor/build tests. These do not prove Xcode compilation, signing, device installation or account API access.

## Reproduce on any supported CLI host

```sh
go test -count=1 -cover ./...
go vet ./...
go build -o bin/apprivet ./cmd/apprivet
bin/apprivet version --json
bin/apprivet init MyFirstApp --bundle-id org.example.myfirstapp --json
bin/apprivet inspect --project MyFirstApp --json
```

Use `bin/apprivet.exe` on Windows. `go test -race ./...` is also run by Linux/macOS CI; local Windows race testing requires a compatible C compiler.

## Mac integration acceptance

Use a Mac with full Xcode, accepted agreements, installed iOS SDK and simulator runtime. Run with trusted generated source under a credential-free account:

```sh
bin/apprivet doctor --json
plutil -lint MyFirstApp/MyFirstApp.xcodeproj/project.pbxproj
bin/apprivet build --project MyFirstApp --trust-project --timeout 15m
xcrun simctl list devices available
bin/apprivet test --project MyFirstApp --trust-project --simulator REPLACE_WITH_UUID --timeout 20m
```

The CI workflow performs these operations on macos-15 and runs Go tests on Windows, Linux and macOS. A workflow file is not evidence of a successful run; record the run URL and conclusion in STATUS after it completes. Hosted runner image labels can change Xcode versions; capture doctor output for the version tested rather than declaring every Xcode version compatible.

No Apple credentials are required for the simulator acceptance test. Physical device discovery/installation/launch, signing and App Store Connect operations are outside this milestone and have no live validation evidence.

## Hosted CI result

[Run 45049105646](https://github.com/fpokrzywa/AppRivet/actions/runs/45049105646) did not execute any job steps. GitHub annotations reported account payment/spending-limit restrictions. This historical admission failure was resolved when the owner made the repository public. No billing settings were changed by AppRivet. See the M2 section and STATUS for later actual hosted results; a billing admission failure is not a code test failure or a successful validation.

## Linux reproduction from Windows with Docker

The local test used the already-installed `python:3.12-slim-bookworm` image. These static Go test binaries need no Go runtime in the container. From PowerShell with Go on PATH:

```powershell
$env:GOOS = 'linux'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'
foreach ($package in @('project','runner','doctor','build','cli','devices','snapshot','admission')) {
    go test -c -o "bin/linux-tests/$package.test" "./internal/$package"
    if ($LASTEXITCODE -ne 0) { throw "Compilation failed: $package" }
}
$testRoot = (Resolve-Path 'bin/linux-tests').Path
$sourceRoot = (Resolve-Path 'internal').Path
docker run --rm --network none --read-only --user 65534:65534 --cap-drop ALL --security-opt no-new-privileges --tmpfs /tmp:rw,size=512m --mount "type=bind,source=$testRoot,target=/tests,readonly" --mount "type=bind,source=$sourceRoot,target=/src,readonly" python:3.12-slim-bookworm sh -c 'for package in project runner doctor build cli devices snapshot admission; do cd /src/"$package" || exit 1; /tests/"$package".test || exit 1; done'
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED
```

Linux verifies the shared Unix runner implementation, but does not replace macOS/Xcode validation. The local container run did not use the race detector; hosted Linux/macOS CI subsequently passed race checks after repository visibility changed.
