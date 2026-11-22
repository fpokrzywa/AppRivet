# Research and feature parity

Reviewed 2026-11-22. This is engineering research, not a legal opinion. The agreements accepted by each developer and current Apple documentation govern actual use. Validate commercial hosting terms with qualified counsel before offering a shared Mac service.

## Sources and findings

1. [Xtool README](https://github.com/xtool-org/xtool): documents a Swift CLI, SwiftPM app workflow, SDK management, developer-service integration, signing, and device commands. Its public library surfaces include XKit and XToolSupport. Its README labels Xcode project support and hot reload as 2.0 beta features. Only publicly documented behavior was used as a reference; no implementation was copied or imported.
2. [Xcode and Apple SDKs Agreement, section 2.2](https://www.apple.com/legal/sla/docs/xcode.pdf): places installation and internal use of Apple software on owned/controlled Apple-branded computers and limits SDK use. AppRivet therefore chooses a developer-controlled Mac executor; remote access is not native Windows/Linux compilation. No SDKs ship with AppRivet.
3. [Xcode command-line tool reference](https://developer.apple.com/documentation/xcode/xcode-command-line-tool-reference): xcodebuild builds projects/workspaces; devicectl and simctl are shipped with full Xcode. Command Line Tools alone are insufficient. Runtime help from the selected Xcode version remains the authority for individual command flags.
4. [Running on simulated or physical devices](https://developer.apple.com/documentation/xcode/running-your-app-on-simulated-or-physical-devices): Xcode handles personal/developer account sign-in, team selection, bundle IDs, and automatic provisioning. AppRivet must not duplicate account authentication. Physical device use requires pairing, appropriate provisioning and device settings.
5. [Creating App Store Connect API keys](https://developer.apple.com/documentation/AppStoreConnectAPI/creating-api-keys-for-app-store-connect-api): individual keys inherit the user's access and cannot use provisioning endpoints, Sales/Finance, or notaryTool. Team keys carry assigned roles and access across apps, not per-app isolation. Treat a least-role team key as still broadly scoped.
6. [Generating API tokens](https://developer.apple.com/documentation/AppStoreConnectAPI/generating-tokens-for-api-requests) and [revoking API keys](https://developer.apple.com/documentation/AppStoreConnectAPI/revoking-api-keys): JWT-based access and revocation are documented. Validate endpoint, role, key type, issuer format, membership, and current token rules individually before implementation.
7. [Signing identities](https://developer.apple.com/documentation/xcode/sharing-your-teams-signing-certificates): certificates and private keys are distinct; the private identity must remain protected. A Keychain-backed key is not automatically inaccessible to build scripts running under the same user.
8. [Swift Package Manager](https://www.swift.org/documentation/package-manager/): useful for dependencies and package tests; a Package.swift file is executable Swift and is not a passive project configuration file. Discovery must not evaluate it.

## Parity matrix

| Capability documented by Xtool | AppRivet approach | Feasibility / validation | Release boundary |
| --- | --- | --- | --- |
| Portable CLI | Go executable | Feasible; cross-build and runtime CI | M1 |
| Project creation | Original Xcode SwiftUI template | Feasible; template tests plus Mac compilation | M1 |
| SwiftPM iOS apps | Recognize packages; Xcode app host uses SwiftPM dependencies | Package-to-app conversion needs experiments | Detection M1; conversion deferred |
| Xcode project builds (Xtool beta) | Official xcodebuild on Mac | Feasible; validate selected Xcode versions | Simulator adapter M1 |
| SDK management | Diagnose locally installed Apple SDKs | Feasible; no copying or distribution | M1 diagnostics |
| Native Linux/Windows iOS builds | Not offered | Licensing/SDK/linker compatibility unresolved | Outside initial release |
| Developer-service authentication | Xcode user workflow; official ASC APIs only | Key roles and endpoint limitations apply | Phase 2 |
| Signing/provisioning | Xcode-created configuration; separate trusted signer later | Requires real account/device tests and isolation | Beyond M1 |
| Device discovery/install/launch | Official devicectl adapter | Requires compatible Mac/device and signing | Next Phase 1 milestone |
| App uninstall | Explicit future device mutation | Requires confirmation policy | Deferred |
| Hot reload/macOS apps (Xtool beta) | No M1 implementation | Separate compatibility experiments | Deferred |
| Embeddable Swift libraries | Internal Go interfaces, optional future SDK | No Xtool library use | No public library API in M1 |
| Tests | xcodebuild test for existing shared test schemes | Real simulator required | M1 adapter |
| Publishing | Official distribution APIs/tools | Separate upload/submission/approval design | Later |

## Alternative approaches

| Approach | Technical / compatibility considerations | Licensing / security considerations | Decision |
| --- | --- | --- | --- |
| Developer-owned local/remote Mac | Highest official tool compatibility; network costs for remote use | Apple-branded hardware; developer controls access; remote isolation still essential | Selected |
| Dedicated hosted Mac | Feasible official builds; provider and device-access constraints | Review service-provider and account terms; dedicated tenants; no credential resale | Research later |
| Swift cross-compilation with extracted Apple SDK | Linker, asset compilation, frameworks, deployment and version gaps | SDK use/redistribution concerns; original tool source license does not confer SDK rights | Unsupported |
| Fully independent SDK/framework stack | Enormous ABI/API compatibility effort | Original implementation and legal review still needed | Outside scope |
| Xcode Cloud | Official managed CI; existing project/repository prerequisites | Different authorization and service lifecycle; not a general device-control backend | Optional future adapter |

Unverified: exact devicectl JSON stability across Xcode versions; all team-key provisioning role combinations; commercial remote-host tenancy terms; availability of hosted Mac hardware for device access. None is assumed solved by this milestone.
