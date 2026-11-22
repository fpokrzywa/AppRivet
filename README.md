# AppRivet

An independent iOS development CLI with environment diagnostics, SwiftUI project creation, simulator builds/tests, and read-only device discovery. Windows and Linux can prepare projects and inspect saved device reports; iOS builds and live discovery use official Xcode tools on a Mac. Remote Mac execution is planned, not implemented.

Status: Milestone 2 device discovery is implemented with synthetic-fixture and offline tests. The repository owner made AppRivet public on 2026-11-22, removing the prior CI billing admission blocker. See [project status](docs/STATUS.md) for current live Mac evidence and remaining physical-device validation. This is an early engineering preview, not a production signing service.

## Principles

- Original implementation; no Xtool code, packages, or binaries.
- No Apple account passwords or undocumented authentication APIs.
- No Apple SDK redistribution or native Windows/Linux iOS compilation claims.
- No credential management, account mutations, or distribution in Milestone 1.
- Optional AI integration through the same documented CLI used by people.

## Development

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

Portable `snapshot create` and `snapshot inspect` also package explicitly selected source files and verify their hashes without extraction, execution or credentials. See [source snapshots](docs/SNAPSHOTS.md) for the example list, strict format/limits and integrity-versus-authorization boundary. A valid snapshot is not proof of a complete or safe build.

See the [CLI reference](docs/CLI.md) for flags, JSON, exit codes and security boundaries, and [verification](docs/VERIFICATION.md) for reproducible Mac tests. The [roadmap](docs/ROADMAP.md) separates Milestone 1 from future signing, devices, remote builds, and distribution.

## Documentation

- [Product requirements](docs/PRD.md)
- [Research and feature parity](docs/RESEARCH.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Threat model](docs/THREAT_MODEL.md)
- [Architecture decisions](docs/adr/0001-foundation.md)
- [Device discovery and live-validation matrix](docs/DEVICES.md)
- [Portable source snapshots](docs/SNAPSHOTS.md)
- [Contribution guide](CONTRIBUTING.md) and [security policy](SECURITY.md)

No open-source license has been selected. Repository access does not grant a public redistribution license. No payments, activation, or license enforcement are implemented. AppRivet is not affiliated with or endorsed by Apple or Xtool.
