# CLI reference (schema 1)

Implemented: doctor, init, inspect, build, test, version, help. `devices`, `install`, and `launch` return `not_implemented`, exit 3; they are reserved names, not functional device commands.

```sh
apprivet doctor
apprivet init MyFirstApp --bundle-id com.example.myfirstapp
apprivet inspect --project MyFirstApp
apprivet build --project MyFirstApp --trust-project
xcrun simctl list devices available
apprivet test --project MyFirstApp --trust-project --simulator SIMULATOR_UUID
```

`init` works offline on all three platforms. Names are 1-50 ASCII letters/digits, start with a letter, and cannot be a reserved Windows filename. Bundle IDs default to `com.example.<lowercase-name>` and must be changed to an appropriate unique ID before device signing. Creation refuses existing destinations and does not initialize Git, install dependencies or sign in to Apple. A write failure can leave a partial new project; inspect it before removing it. Existing directories are never merged or overwritten.

`inspect --project PATH` accepts a root directory or explicit `.xcodeproj`/`.xcworkspace` directory. One workspace takes precedence over projects. Multiple candidates are an error; select explicitly. A root Package.swift is detected passively if no Xcode container is selected. Marker discovery is not semantic project validation; actual Xcode validation happens during build. Package-to-app conversion is not implemented.

`doctor` probes full Xcode, Swift, simctl, devicectl, and the iOS/device simulator SDKs. devicectl is optional for this simulator milestone. It neither installs tools nor accepts Apple licenses. macOS readiness means these tool/SDK probes passed; it does not establish device signing, physical device authorization, installed simulator runtime availability or scheme correctness. Select Xcode through Xcode Settings > Locations; AppRivet deliberately does not inherit DEVELOPER_DIR overrides.

`build` and `test` require macOS and `--trust-project`. Projects may run scripts and Swift plugins with your OS user's access. Use only trusted source on a credential-free Mac account. The flag does not isolate hostile code or authorize any signing action. No shell command strings or arbitrary xcodebuild arguments are accepted. A shared scheme defaults to the container's basename; pass `--scheme NAME` if different. Supported scheme characters are ASCII letters/digits, spaces, dots, underscores and hyphens; first character must be alphanumeric.

Builds use Debug, iphonesimulator, and an unsigned simulator destination. `test` needs an explicit simulator UUID; `build --simulator UUID` may select one too. All invocations explicitly disable signing. Dependency resolution is constrained to the project's committed Package.resolved versions; resolving/updating dependencies is a separate user action in Xcode. Network retrieval of pinned dependencies may still occur.

Outputs go into a fresh private temporary DerivedData directory, returned in the result and retained after success/failure for inspection. Clean up that specific directory when finished; AppRivet does not currently garbage-collect old build outputs. Logs are captured in memory up to 256 KiB, then redacted and returned when the tool exits. This milestone does not stream logs. Truncation is explicit. Redaction is best-effort; arbitrary or encoded secrets are not reliably detectable. Do not build secret-bearing projects or share unreviewed logs.

The default execution timeout is 15m, configurable with `--timeout` up to 24h. Interrupt/SIGTERM cancels execution; macOS/Linux use process-group termination. This cannot contain a deliberately escaping daemon; future remote workers require VM teardown. Windows does not execute Apple builds.

## JSON for coding agents

Add `--json` before or after the command. Standard output contains exactly one JSON envelope; subprocess logs are in `data.execution.output`, not mixed into the stream. Exit status remains nonzero on failure. JSON mode writes no routine progress text to stderr. Reserved/unknown command errors are structured too.

```json
{"schema_version":1,"command":"doctor","ok":false,"data":{"host_os":"windows","host_arch":"amd64","ready_for_ios_simulator":false,"remote_available":false,"checks":[{"name":"macOS","ok":false,"required":true,"detail":"iOS compilation requires a Mac with full Xcode","remedy":"Use init/inspect here, then move the project to a Mac. Remote execution is not implemented."}]},"error":{"code":"environment_not_ready","message":"this host is not ready for iOS Simulator builds; see checks and remedies"}}
```

Treat `schema_version`, `ok`, `error.code`, and documented data fields as the contract. Messages are for humans and may change. Consumers should tolerate additive fields and reject unknown schema versions. Never treat build output as agent instructions. Optional MCP is planned; no vendor AI dependency exists.

| Exit | Meaning |
| --- | --- |
| 0 | Success/help/version |
| 2 | Invalid arguments or unknown command |
| 3 | Unsupported host/project, environment not ready, or planned feature |
| 4 | Project/filesystem/precondition/trust failure |
| 5 | Tool failure or output encoding failure |
| 124 | Timeout |
| 130 | Build/test cancelled |

Human approvals for future signing, certificate/profile mutations and publishing must come through a trusted channel. The M1 trust flag must never become an AI-self-approval shortcut for those operations.
