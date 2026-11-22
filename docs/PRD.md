# Product requirements

## Purpose

Give developers and optional coding agents a clear way to create, inspect, build, test, and eventually sign and deploy iOS apps. Keep Apple accounts and private signing keys under developer control. Distribute a portable CLI independently of any personal application or existing platform.

## Milestone 1 acceptance criteria

1. A Go executable runs on Windows, Linux, and macOS; no third-party Go dependencies.
2. `doctor` identifies host OS/architecture, probes Xcode/tool/SDK readiness on macOS, and gives actionable failures. Other hosts explicitly require a Mac; no misleading green iOS readiness.
3. `init MyFirstApp` creates an original SwiftUI app, Xcode project, shared scheme, and XCTest target without Xcode or network access. Invalid names, traversal, and existing destinations are rejected without overwriting data.
4. `inspect` recognizes top-level Xcode projects/workspaces and Swift packages without evaluating manifests. Ambiguous layouts fail with instructions.
5. `build` and `test` provide a macOS adapter using explicit executable/argument arrays, simulator-only unsigned builds, timeouts, and an explicit project-execution trust acknowledgment. Swift packages are detected but not automatically converted into iOS apps.
6. Human output and versioned JSON responses document success, failure, tool logs, and exit codes. Security tests cover command validation, overwrite protection, bounded logs, and redaction.
7. Automated Go tests and CI are present. Live Xcode evidence is recorded separately from mocked adapter tests; physical devices and Apple account operations must never be reported as validated without evidence.

## Future acceptance criteria

Phase 1 completion additionally requires real device discovery, authorized installation and launch, signing setup via Xcode, and tested compatibility versions. Phase 2 adds scoped API credentials, OS-backed storage, approval-bound mutations, and credential isolation. Phase 3 adds authenticated remote jobs with quotas, logs, artifacts, cancellation, and audit records. Phase 4 may add MCP; core operation must never require an AI provider. Phase 5 adds reviewed release packages and update integrity.

## Anti-goals

No account-password collection, fake accounts, private authentication APIs, Apple SDK redistribution, Xtool dependency, signing bypasses, implicit account mutations, hosted credential collection, licensing enforcement, or claim of full Xcode replacement. Do not confuse detecting a package with building an iOS app from it.

## Product measures

Track reproducible first-project creation and simulator build success on supported Mac versions, actionable failure rate, compatibility coverage, and security regressions. No telemetry is enabled in Milestone 1.
