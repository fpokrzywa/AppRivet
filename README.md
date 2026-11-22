# AppRivet

An independent iOS development CLI, starting with environment diagnostics and SwiftUI project creation. Windows and Linux can create and inspect projects; iOS builds use official Xcode tools on a Mac. Remote Mac execution is planned, not implemented.

Status: Milestone 1 in development. See [project status](docs/STATUS.md) for actual verification evidence and limitations. This is an early engineering preview, not a production signing service.

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

Usage, supported commands, security boundaries, and Mac validation steps will be recorded in [CLI reference](docs/CLI.md). The [roadmap](docs/ROADMAP.md) separates Milestone 1 from future signing, devices, remote builds, and distribution.

## Documentation

- [Product requirements](docs/PRD.md)
- [Research and feature parity](docs/RESEARCH.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Threat model](docs/THREAT_MODEL.md)
- [Architecture decisions](docs/adr/0001-foundation.md)
- [Contribution guide](CONTRIBUTING.md) and [security policy](SECURITY.md)

No open-source license has been selected. Repository access does not grant a public redistribution license. No payments, activation, or license enforcement are implemented. AppRivet is not affiliated with or endorsed by Apple or Xtool.
