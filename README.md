# AppRivet

AppRivet creates Flutter apps for selected mobile, web and desktop platforms from a shared Dart codebase. It also supports native SwiftUI projects, iOS Simulator builds and tests, and read-only Apple device discovery. Flutter projects build with the official SDK and the tools for each selected platform. iOS builds and live discovery use Xcode on a Mac.

**[Install AppRivet on your MacBook](docs/DESKTOP.md#macos)** to download the app, generate a project from a brief, and run it in the iOS Simulator.

**[Install AppRivet on Windows](docs/DESKTOP.md#install-on-windows)** to open the local browser workspace and build Flutter projects when the SDK is installed.

Open a project's **Publish** view to prepare a signed iOS or Android release and review its local build history. [Store build and connection guide](docs/DESKTOP.md#build-for-store)

Status: Milestone 2 device discovery is implemented with synthetic-fixture and offline tests. The repository owner made AppRivet public on 2026-11-22, removing the prior CI billing admission blocker. See [project status](docs/STATUS.md) for current live Mac evidence and remaining physical-device validation. This is an early engineering preview, not a production signing service.

## Principles

- Original implementation; no Xtool code, packages, or binaries.
- No Apple account passwords or undocumented authentication APIs.
- No Apple SDK redistribution or native Windows/Linux iOS compilation claims.
- No credential management, account mutations, or distribution in Milestone 1.
- Optional AI integration through the same documented CLI used by people.

## Development

The AppRivet frontends let you write or import an app brief, select platforms, review the exact prompt and generate Dart or SwiftUI source with OpenAI or Anthropic. Offline starters work without a model account. [Download and setup guide](docs/DESKTOP.md)

Install Go from [go.dev](https://go.dev/dl/). The tested version and commands are recorded in [verification](docs/VERIFICATION.md).

```sh
go test ./...
go vet ./...
go build -o bin/apprivet ./cmd/apprivet
```

On Windows use `go build -o bin/apprivet.exe ./cmd/apprivet`.

```sh
bin/apprivet doctor
bin/apprivet init MyFirstApp --bundle-id org.example.myfirstapp
bin/apprivet inspect --project MyFirstApp --json
# On a Mac with full Xcode, for trusted source:
bin/apprivet build --project MyFirstApp --trust-project
bin/apprivet devices --json
bin/apprivet devices --simulators --json
# Offline inspection works on Windows/Linux too:
bin/apprivet devices --from-json internal/devices/testdata/devicectl-modern.json --json
```

Implemented commands: `doctor`, `init`, `inspect`, `build`, `test`, `devices`, `version`, and `help`. Build/test target iOS Simulator. Device listing does not establish attachment, pairing permission or installation authorization. `install` and `launch` explicitly return `not_implemented`. Signing, remote builds and Apple credential management remain planned.

Portable `snapshot create`, `snapshot inspect` and `snapshot extract` package, verify and materialize explicitly selected source files without execution or credentials. Extraction requires a matching expected digest and a new destination. See [source snapshots](docs/SNAPSHOTS.md) for format limits and filesystem prerequisites. A valid snapshot is not proof of a complete or safe build.

`worker check` evaluates local principal/project/digest/time/quota bindings against a policy and snapshot. It is an [offline admission check](docs/WORKER_ADMISSION.md). `worker init/reserve/status/release` add [persistent replay protection and capacity accounting](docs/WORKER_RESERVATIONS.md), without authentication, execution permission or a running worker service. [Claim/current-owner/recovery commands](docs/WORKER_RECOVERY.md) add durable ownership identities and explicit accounting reconciliation; they do not execute or terminate jobs. [In-place journal migration and compaction](docs/JOURNAL_MAINTENANCE.md) preserve replay and ownership history while reclaiming the append segment. [Intent-backed manifest repair](docs/MANIFEST_REPAIR.md) restores only a proven pending publication and retains damaged bytes; older unanchored tears remain blocked.

See the [CLI reference](docs/CLI.md) for flags, JSON, exit codes and security boundaries, and [verification](docs/VERIFICATION.md) for reproducible Mac tests. The [roadmap](docs/ROADMAP.md) separates Milestone 1 from future signing, devices, remote builds, and distribution.

## Documentation

- [Product requirements](docs/PRD.md)
- [Research and feature parity](docs/RESEARCH.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Threat model](docs/THREAT_MODEL.md)
- [Architecture decisions](docs/adr/0001-foundation.md)
- [Device discovery and live-validation matrix](docs/DEVICES.md)
- [Portable source snapshots](docs/SNAPSHOTS.md)
- [Offline worker admission checks](docs/WORKER_ADMISSION.md)
- [Contribution guide](CONTRIBUTING.md) and [security policy](SECURITY.md)

No open-source license has been selected. Repository access does not grant a public redistribution license. No payments, activation, or license enforcement are implemented. AppRivet is not affiliated with or endorsed by Apple or Xtool.
