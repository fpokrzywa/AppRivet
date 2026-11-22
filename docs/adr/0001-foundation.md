# ADR 0001: portable Go CLI and official Mac executor

Status: accepted for Milestone 1 as an engineering decision within the requested scope. Date: 2026-11-22.

Use Go standard library, embedded original Xcode templates, and official Apple tool subprocesses. Cross-platform means portable control and project preparation; compiling iOS apps happens on a Mac. Do not redistribute SDKs or depend on Xtool. Keep native Windows/Linux iOS compilation as separate unsupported research.

Alternatives: Swift-only simplifies native API access but complicates client distribution. Rust adds upfront complexity without resolving Apple tool/licensing dependencies. Third-party project generators add supply-chain and installation dependencies. Hand-maintained PBX templates require Mac CI and compatibility upkeep; accept that cost for a small conventional project.
