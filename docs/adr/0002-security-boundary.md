# ADR 0002: simulator-only initial builds; signing is a separate boundary

Status: accepted. Date: 2026-11-22.

M1 builds trusted projects for iOS Simulator with signing explicitly disabled. Build/test requires a visible trust acknowledgment. No private credentials, provisioning flags, device mutations, or account APIs are implemented. A flag is not isolation or approval of signing.

Untrusted build scripts can access everything available to their OS user, including Keychain access allowed to tools. Putting credentials in Keychain while running arbitrary project code as that user does not meet the product's security promise. Require a separate worker and signing broker with a trusted human approval channel before introducing automated credential access.

Consequence: no signed physical device demo in M1. This is a deliberate milestone boundary, not a claim that device support is implemented.
