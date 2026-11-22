# Development roadmap

| Milestone | Deliverable and exit evidence | State |
| --- | --- | --- |
| M1: foundation | Research, docs, portable doctor/init/inspect, simulator build/test adapter, automated tests, original example, CI | Code implemented; live Mac acceptance blocked |
| M2: local device workflow | Read-only devicectl discovery, Xcode-established signing, install/launch on explicitly authorized physical device; real Mac/device logs; trust/isolation policy resolved first | Planned |
| M3: signing broker | OS credential store, endpoint/role matrix, rotation/revocation, approval-bound mutations, worker isolation penetration tests | Planned |
| M4: remote Mac | mTLS pairing, per-user/project grants, queue, logs, cancellation, audit and artifacts; hostile upload/isolation tests | Planned |
| M5: agent integration | Stabilize JSON contract; optional MCP with explicit human approval channel | Planned; CLI JSON starts in M1 |
| M6: distribution | Supported-version matrix, private release rehearsal, signed/checksummed binaries, installation docs; approve licensing before public release | Planned |
| Later | Desktop GUI, broader project adapters, hosted providers | Unscheduled |

For each milestone: record outcome and acceptance criteria, implement the smallest usable slice, run tests, identify failures and unverified assumptions, update STATUS/CHANGELOG, then commit. Never conflate adapter tests, hosted simulator validation and physical device evidence.
