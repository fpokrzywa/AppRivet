# Threat model

Assets: source code, Apple accounts/API tokens, signing identities, developer devices, Mac host, artifacts, and remote-client identities. Adversaries: malicious repositories/dependencies, compromised AI sessions, local unprivileged users, remote attackers, and compromised release infrastructure.

Trust boundaries: user -> CLI; CLI -> project/filesystem; orchestration -> tool process; future client -> agent; future worker -> signer; release pipeline -> end user. The OS account and installed Apple/Go toolchains are part of the trusted computing base.

| Threat | M1 control | Required later / residual risk |
| --- | --- | --- |
| Password phishing or logging | No Apple login/password API or fields | Keep Xcode as account workflow |
| API/private-key exfiltration | No credential feature; key file ignore rules; minimal subprocess environment | Separate signer, OS storage, rotation/revocation, no secret exports; ignore rules alone cannot secure files |
| Malicious build scripts / Swift manifests / AI code | Passive discovery; explicit trust gate; unsigned simulator only | Local execution is NOT sandboxed. Use credential-free isolated Mac worker before hostile builds |
| Shell/argument injection | Fixed executable paths, separate arguments, validated scheme/destination and no arbitrary flags | Tool-level build scripts remain executable |
| Traversal/overwrite during init | Strict portable names; exclusive destination creation; fixed template paths | Local hostile concurrent filesystem mutation is outside M1 trust model |
| Diagnostic secret disclosure | Bounded logs; redact token assignments, bearer tokens and PEM blocks | Pattern redaction cannot detect arbitrary/encoded secrets; review before sharing; no telemetry |
| Log resource exhaustion | Capture limit and deadline | Remote quotas and streaming backpressure |
| Cancellation leaves child jobs | Process group termination on macOS for build execution | Test descendant cleanup; Windows is CLI-only for Apple jobs |
| Remote impersonation/replay | No listening server in M1 | mTLS, expiring pairing, grants, nonces, audited approval |
| Unsafe file upload/artifact access | No remote file API | Strict extraction, quotas, symlink rejection and tenant authorization |
| Unauthorized signing/provisioning | No signing or provisioning; no allowProvisioningUpdates | One-time human approvals bound to exact mutation/artifact, separate broker |
| Unauthorized device installation | No device mutation commands in M1 | Explicit target selection, paired device, signed app verification, approval policy |
| Dependency compromise | Standard library only; pinned Go patch and CI action commit SHAs | Review upgrades, vulnerability checks, SBOM before release |
| Compromised release/update | No auto-update or public release in M1 | Protected tags, reproducible build inputs, hashes, signed provenance, macOS notarization / Windows signing where appropriate |

Security-focused tests cover invalid/path-like names, reserved Windows names, existing destinations, malformed bundle IDs, passive/ambiguous discovery, non-Mac execution refusal, trust refusal before subprocess invocation, unsupported package builds, fixed command arguments, timeout/cancellation, output limits, secret redaction, and JSON errors.

Security acceptance for remote work: demonstrate a malicious build cannot read a seeded signer key, contact the signer, escape source/artifact roots, impersonate another user, retain a job after cancellation, or overwrite logs. Until that evidence exists, remote/untrusted execution stays disabled.
