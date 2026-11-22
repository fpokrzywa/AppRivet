# Security policy

AppRivet is an early development preview. No release is yet supported for untrusted remote builds or credential management. Review [the threat model](docs/THREAT_MODEL.md) before use.

Report suspected vulnerabilities privately to the repository owner using an available private GitHub security advisory channel. If private reporting is unavailable, contact the owner privately to establish a channel; do not place secrets or exploit details in public issues. No monitored security mailbox or response SLA has been established yet.

Never send Apple account passwords, private keys, provisioning files or unsanitized build logs. If a credential is exposed, revoke it through Apple's official account tools and rotate dependent access; deleting a log or Git commit does not revoke a credential.

Build/test executes project-controlled code. The trust flag is not a sandbox. M1 does not collect credentials, sign apps, provision devices or publish builds.
