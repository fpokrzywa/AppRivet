# Verification record

## Replay protection and local reservations (2026-11-22)

Local Windows tests and vet passed across all eight tested packages; admission coverage is 86.2%, CLI 89.0%. Native Windows journal fuzzing completed 10,000 inputs. The real CLI smoke passed extraction/admission plus init, reserve, exact replay refusal, status recovery, release, retained nonce refusal and a fresh reservation. All eight compiled Linux package test executables passed in the restricted container; all six macOS/Linux/Windows amd64/arm64 client cross-builds passed.

Tests cover cross-process duplicate/capacity races (six claimants, one winner), lock release after process death, restart/release/expiry replay history, request conflicts, principal separation, actual-byte budgets, grant expiry clamping, post-verification policy revocation/quota changes, cancellation, observed clock rollback, journal bounds/canonical encoding/invalid transitions, missing/replaced/symlinked state, injected partial writes/sync failures and cancellation after sync. Missing APIs/CLI commands failed first, then passed after implementation. The Windows suite may skip symlink or open-file replacement cases when the host denies those operations; Unix CI exercises its platform behavior.

Independent review found no P1/P2 issues. One P3 request-conflict/error-precedence inconsistency was reproduced with a failing two-record regression, then fixed; the full Windows suite and vet passed again. Hosted CI for this change is pending. Existing Mac checks remain enabled; no Mac/Apple execution property is inferred from the local tests. [WORKER_RESERVATIONS](WORKER_RESERVATIONS.md#validation-and-remaining-live-gates) records the deployment gaps, including authenticated identity, execution fencing/teardown, rollback-resistant storage, power-loss recovery, Windows ACL enforcement and physical devices/signing.

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
