# Architecture

## Foundation

Go CLI -> application command -> project / doctor / build packages -> bounded process runner -> official Apple tools on macOS.

The CLI owns argument validation, exit codes and a versioned JSON envelope. Project generation uses embedded, original text templates; discovery examines filesystem markers without executing code. Doctor is read-only and probes absolute system tool paths. The build package accepts a typed request, checks trust/platform/project kind, and constructs fixed xcodebuild arguments. The runner supplies deadlines, limited output, a minimal child environment, and redaction. No shell interprets user input.

M1 supports simulator builds only, explicitly disables signing, and never passes automatic provisioning flags. The current runner is not a security sandbox: project scripts and plugins are arbitrary code. Only trusted local source may be built. Do not run builds as a credential-bearing signing user. An acknowledgment flag is a usability barrier, not a way to make hostile source safe.

## Modules and boundaries

| Module | Responsibility | Must not do |
| --- | --- | --- |
| cmd/apprivet, internal/cli | Portable entry point and machine contract | Read passwords or silently grant approvals |
| internal/project | Validate, render, discover projects | Evaluate Package.swift or overwrite projects |
| internal/doctor | Check host, full Xcode, SDK and tools | Install tools or accept Apple agreements |
| internal/devices | Read-only devicectl/simctl inventory and offline report normalization | Pair, install, infer authorization, or expose raw metadata |
| internal/build | Simulator build/test orchestration | Sign, provision or publish |
| internal/runner | Fixed executable invocation, cancellation, bounded/redacted output | Invoke a shell or claim isolation |
| Future signing broker | Keychain references, policy, approved signing | Execute project scripts |
| Future Mac agent | Pairing, authorized job lifecycle and artifacts | General shell/remote desktop endpoint |
| Optional AI adapter | Expose same constrained operations | Hold keys or approve sensitive mutations |

Use small concrete packages today. Add transport/signing interfaces when implementing them; do not ship empty services masquerading as features.

M2 discovery uses separate source adapters with a common inventory. CoreDevice IDs and hardware UDIDs remain distinct; simulators are classified from explicit evidence, not ID shape. The runner can capture bounded raw structured stdout separately from redacted stderr; raw stdout is excluded from diagnostic JSON. The devices parser validates ambiguity, selects known fields and sanitizes display text before output. devicectl uses a private temporary report file; simctl uses separate stdout. Offline files carry explicit provenance and never grant authority for later device mutations. See DEVICES.md for schema policy and unresolved compatibility checks.

## Future signing design

Official Xcode account setup remains interactive on the Mac. API credentials are imported locally into OS-protected storage; configuration contains opaque references and nonsecret metadata only. Windows Credential Manager / Linux Secret Service would protect remote client identities; Apple signing/API keys stay on the Mac. Never accept Apple account passwords, place keys in arguments/environment, or export private material.

Maintain a capability table keyed by credential kind, role, membership, and endpoint. Individual ASC keys cannot provision. Team keys are not per-app scoped; enforce project/resource allowlists in the broker as additional policy. Default to read-only APIs. Prepare a concrete mutation preview with team, bundle ID, certificate/profile changes and distribution target. A trusted human UI authorizes the digest, expiry, and one-time nonce; an AI-controlled CLI flag is insufficient. Rotation imports a replacement, verifies minimal access, switches the reference, and revokes the old key through the official workflow. Revocation immediately invalidates broker sessions and cached tokens; never promise revocation without API evidence.

Build in an ephemeral Mac VM or dedicated clean worker identity with no signing/API key access and restricted egress. Transfer only a validated artifact manifest to a distinct signer identity/host. Validate paths, hashes, executable types, entitlements, bundle IDs and dependencies; do not execute build hooks in the signer. Avoid broad codesign Keychain ACLs. A separate process under the same user is not adequate isolation. Prove this boundary with hostile build-script tests before enabling untrusted or AI-generated builds.

## Future remote protocol

Developer-controlled agent, disabled until paired. TLS 1.3 plus mutually authenticated client certificates; short-lived pairing code verified through a trusted local Mac UI, fingerprint comparison, attempt limits, expiry and revocation. Bind identities to users and project grants server-side. No unauthenticated LAN discovery or insecure fallback.

Typed endpoints: submit immutable source snapshot, get status, stream bounded logs with cursor, fetch hash-addressed artifact, cancel. Jobs transition queued -> preparing -> running -> succeeded/failed/cancelled/timed_out. Enforce admission limits, per-user concurrency, CPU/disk/log quotas and bounded retention. Cancellation kills the worker process tree/VM and closes artifact publication before reporting final state. Reconcile jobs after restart; never run a job twice without an idempotency decision.

Reject archive traversal, absolute paths, symlinks/hardlinks, devices, duplicate/case-colliding paths, decompression bombs, and files outside the declared root. Content IDs are not authorization. Authenticate artifact downloads, verify content hashes, and prevent clients from providing host paths or shell commands. Audit actor, project, commit/snapshot digest, approval ID, action and result; omit secrets and source contents. Remote logging and artifact retrieval are planned, not implemented.

## Alternatives and trade-offs

Go was retained for simple portable binaries and standard-library coverage. Swift offers native Apple APIs but adds client toolchain complexity; Rust offers strong types but adds implementation overhead for this initial scope. A direct original Xcode template avoids requiring XcodeGen/Tuist or depending on Xtool, at the cost of maintaining PBX project compatibility. A generic plugin engine or public MCP server now would enlarge the security surface before the core is validated.
