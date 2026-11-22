# AppRivet

An independent iOS development CLI, starting with environment diagnostics and SwiftUI project creation. Windows and Linux can create and inspect projects; iOS builds use official Xcode tools on a Mac. Remote Mac execution is planned, not implemented.

Status: Milestone 1 code is implemented; live Mac acceptance is still open. Windows and Linux tests pass. GitHub Actions could not start because of an account billing/spending-limit restriction. See [project status](docs/STATUS.md) for evidence and limitations. This is an early engineering preview, not a production signing service.

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
```

Implemented commands: `doctor`, `init`, `inspect`, `build`, `test`, `version`, and `help`. Build/test are macOS simulator adapters awaiting live Xcode acceptance. `devices`, `install`, and `launch` explicitly return `not_implemented`. No signing, remote-build, or Apple credential capability is advertised as working.

See the [CLI reference](docs/CLI.md) for flags, JSON, exit codes and security boundaries, and [verification](docs/VERIFICATION.md) for reproducible Mac tests. The [roadmap](docs/ROADMAP.md) separates Milestone 1 from future signing, devices, remote builds, and distribution.

## Documentation

- [Product requirements](docs/PRD.md)
- [Research and feature parity](docs/RESEARCH.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Threat model](docs/THREAT_MODEL.md)
- [Architecture decisions](docs/adr/0001-foundation.md)
- [Contribution guide](CONTRIBUTING.md) and [security policy](SECURITY.md)

No open-source license has been selected. Repository access does not grant a public redistribution license. No payments, activation, or license enforcement are implemented. AppRivet is not affiliated with or endorsed by Apple or Xtool.
