# Contributing

This repository is private and its distribution license is undecided. Do not publish it, copy third-party implementations, or assume an open-source license.

Use the pinned Go version from go.mod. Run `go test ./...`, `go vet ./...`, and `gofmt` on changes. Add tests for meaningful behavior and security boundaries. Review dependencies before adding them; the initial CLI uses only the standard library. Keep user-controlled strings out of shell commands and do not add arbitrary command passthrough.

Every milestone needs acceptance criteria, verification evidence, explicit unverified assumptions, an updated status file and small descriptive commits. Tests using simulated Apple tools are not evidence of actual Xcode builds, account access or device installation. Keep real credentials and signing material outside this repository, including examples and fixtures.

Do not execute untrusted build scripts on a machine with signing access. Never disable validation to get a demonstration to pass. Public releases, signing/provisioning changes and new credential use require explicit authorization.
