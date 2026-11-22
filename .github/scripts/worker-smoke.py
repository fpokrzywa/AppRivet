"""Credential-free CLI integration; only trusted example source is used."""
import base64
import datetime
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

cli = str(Path(sys.argv[1] if len(sys.argv) > 1 else ("apprivet.exe" if os.name == "nt" else "apprivet")).resolve())
base = Path(os.environ.get("RUNNER_TEMP", ".scratch")).resolve()
base.mkdir(parents=True, exist_ok=True)


def run(*args, exit_code=0):
    result = subprocess.run([cli, *map(str, args), "--json"], capture_output=True, text=True, timeout=60)
    if result.returncode != exit_code:
        raise RuntimeError(f"CLI exit {result.returncode}: {result.stdout[:4000]} {result.stderr[:1000]}")
    report = json.loads(result.stdout)
    assert report["ok"] == (exit_code == 0)
    if exit_code:
        assert "data" not in report
    return report


with tempfile.TemporaryDirectory(prefix="apprivet-smoke-", dir=base) as temporary:
    root = Path(temporary).resolve()
    assert root.is_relative_to(base) and root != base
    archive, destination = root / "source.tar", root / "extracted"
    created = run("snapshot", "create", "--root", "examples/HelloAppRivet",
                  "--files-from", "examples/HelloAppRivet.snapshot-files", "--output", archive)
    assert created == run("snapshot", "inspect", "--archive", archive)
    digest = created["data"]["sha256"]
    assert hashlib.sha256(archive.read_bytes()).hexdigest() == digest
    extracted = run("snapshot", "extract", "--archive", archive, "--destination", destination,
                    "--expect-sha256", digest)
    assert extracted["data"]["snapshot"] == created["data"]
    for entry in created["data"]["files"]:
        assert hashlib.sha256((destination / entry["path"]).read_bytes()).hexdigest() == entry["sha256"]
    assert run("snapshot", "extract", "--archive", archive, "--destination", destination,
               "--expect-sha256", digest, exit_code=4)["error"]["code"] == "destination_exists"

    now = datetime.datetime.now(datetime.timezone.utc)
    grant = dict(id="smoke-grant", principal="ci-developer", project_id="hello", operation="extract",
                 snapshot_sha256=digest, enabled=True,
                 not_before=(now - datetime.timedelta(minutes=1)).isoformat(),
                 expires_at=(now + datetime.timedelta(minutes=10)).isoformat(),
                 limits=dict(max_files=16, max_file_bytes=1048576,
                             max_source_bytes=1048576, max_archive_bytes=2097152))
    policy_path, request_path = root / "policy.json", root / "request.json"
    policy_path.write_text(json.dumps(dict(schema_version=1, grants=[grant])), encoding="utf-8")
    request_path.write_text(json.dumps(dict(schema_version=1, grant_id=grant["id"], project_id="hello",
                                            operation="extract", snapshot_sha256=digest)), encoding="utf-8")
    before = set(root.iterdir())
    decision = run("worker", "check", "--policy", policy_path, "--request", request_path,
                   "--principal", "ci-developer", "--archive", archive)["data"]
    assert decision["eligible"] and decision["mode"] == "offline_policy_check"
    assert not decision["authentication_verified"] and not decision["execution_authorized"]
    assert set(root.iterdir()) == before
    denied = run("worker", "check", "--policy", policy_path, "--request", request_path,
                 "--principal", "ungranted-actor", "--archive", root / "missing.tar", exit_code=4)
    assert denied["error"]["code"] == "admission_denied"

    config_path, state = root / "config.json", root / "state"
    config_path.write_text(json.dumps(dict(schema_version=1, max_active=2, max_per_principal=1,
                                          max_reserved_bytes=8388608, max_lease_seconds=120)), encoding="utf-8")
    run("worker", "init", "--state", state, "--config", config_path)
    request = json.loads(request_path.read_text(encoding="utf-8"))
    request.update(request_id="smoke-1", nonce="a" * 32, lease_seconds=60)
    request_path.write_text(json.dumps(request), encoding="utf-8")
    reserve_args = ("worker", "reserve", "--state", state, "--policy", policy_path,
                    "--request", request_path, "--principal", "ci-developer", "--archive", archive)
    reserved = run(*reserve_args)["data"]
    assert reserved["mode"] == "local_reservation"
    assert not reserved["authentication_verified"] and not reserved["execution_authorized"]
    job = reserved["reservations"][0]
    assert job["status"] == "reserved"
    assert job["reserved_bytes"] == created["data"]["archive_bytes"] + created["data"]["total_bytes"]
    assert run(*reserve_args, exit_code=4)["error"]["code"] == "replay_detected"
    recovered = run("worker", "status", "--state", state, "--principal", "ci-developer",
                    "--request-id", "smoke-1")["data"]["reservations"]
    assert recovered == [job]
    released = run("worker", "release", "--state", state, "--principal", "ci-developer",
                   "--reservation", job["id"])["data"]["reservations"][0]
    assert released["status"] == "released"
    request["request_id"] = "smoke-2"
    request_path.write_text(json.dumps(request), encoding="utf-8")
    assert run(*reserve_args, exit_code=4)["error"]["code"] == "replay_detected"
    request["nonce"] = "b" * 32
    request_path.write_text(json.dumps(request), encoding="utf-8")
    next_job = run(*reserve_args)["data"]["reservations"][0]
    assert next_job["status"] == "reserved"
    claimed = run("worker", "claim", "--state", state, "--principal", "ci-developer",
                  "--reservation", next_job["id"], "--worker", "smoke-worker",
                  "--policy", policy_path, "--archive", archive)["data"]["reservations"][0]
    assert claimed["status"] == "claimed" and claimed["fence"] > 0
    owner_args = ("--state", state, "--principal", "ci-developer", "--reservation", claimed["id"],
                  "--worker", claimed["worker_id"], "--claim", claimed["claim_id"], "--fence", claimed["fence"])
    checked = run("worker", "check-owner", *owner_args, "--policy", policy_path, "--archive", archive)["data"]
    assert checked["ownership_valid"] and not checked["authentication_verified"] and not checked["execution_authorized"]
    # Build a disposable fixture for CLI dispatch. Go tests interrupt the
    # actual publisher and repair writer across process boundaries.
    active = state / "journal.jsonl"
    prefix = active.read_bytes()
    previous = json.loads(prefix.splitlines()[-1])

    def canonical(obj):
        return json.dumps(obj, separators=(",", ":")).encode("ascii")

    event = dict(sequence=previous["sequence"] + 1, previous=previous["hash"],
                 at=previous["at"], kind="observe", hash="")
    event["hash"] = hashlib.sha256(canonical(event)).hexdigest()
    record = canonical(event) + b"\n"
    intent = dict(schema_version=1, active="journal.jsonl", manifest_sha256="",
                  offset=len(prefix), record=base64.b64encode(record).decode("ascii"), hash="")
    intent["hash"] = hashlib.sha256(canonical(intent)).hexdigest()
    (state / "append.pending").write_bytes(canonical(intent) + b"\n")
    active.write_bytes(prefix + record[:17])
    assert run("worker", "recovery-status", "--state", state, "--principal", "ci-developer",
               exit_code=4)["error"]["code"] == "append_pending"
    append_plan = run("worker", "append-repair-status", "--state", state)["data"]
    assert append_plan["present_bytes"] == 17 and not append_plan["changed"]
    assert not append_plan["authentication_verified"] and not append_plan["execution_authorized"]
    appended = run("worker", "append-repair", "--state", state,
                   "--expect-intent-sha256", append_plan["intent_sha256"],
                   "--expect-active-sha256", append_plan["active_sha256"])["data"]
    assert appended["changed"] and appended["bytes_written"] == len(record) - 17
    assert appended["bytes_reasserted"] == 17
    assert active.read_bytes() == prefix + record
    assert run(*reserve_args, exit_code=4)["error"]["code"] == "replay_detected"
    pre_migration = run("worker", "recovery-status", "--state", state, "--principal", "ci-developer")["data"]
    migrated = run("worker", "migrate", "--state", state)["data"]
    assert migrated["changed"] and migrated["format"] == 2 and migrated["generation"] == 1
    assert not migrated["authentication_verified"] and not migrated["execution_authorized"]
    post_migration = run("worker", "recovery-status", "--state", state, "--principal", "ci-developer")["data"]
    for field in ("reservations", "journal_sequence", "journal_sha256", "recorded_at"):
        assert post_migration[field] == pre_migration[field]
    # Synthetic torn-publication fixture using the publisher's actual retained
    # intent. Process-exit crashes and write/sync faults are covered in Go tests.
    operation_id = migrated["prepared_manifest"][len("manifest-"):-len(".json")]
    (state / "publication.pending").write_bytes((state / f"publication-{operation_id}.intent.json").read_bytes())
    torn = (state / "journal.jsonl").read_bytes()[:17]
    (state / "journal.jsonl").write_bytes(torn)
    assert run("worker", "recovery-status", "--state", state, "--principal", "ci-developer",
               exit_code=4)["error"]["code"] == "publication_pending"
    plan = run("worker", "repair-status", "--state", state)["data"]
    assert not plan["changed"] and plan["control_state"] == "torn"
    assert not plan["authentication_verified"] and not plan["execution_authorized"]
    assert (state / "journal.jsonl").read_bytes() == torn
    repaired = run("worker", "repair", "--state", state,
                   "--expect-intent-sha256", plan["intent_sha256"],
                   "--expect-control-sha256", plan["control_sha256"],
                   "--expect-history-sha256", plan["journal_sha256"],
                   "--expect-sequence", plan["journal_sequence"])["data"]
    assert repaired["changed"] and repaired["control_rewritten"]
    assert (state / repaired["before_image"]).read_bytes() == torn
    post_repair = run("worker", "recovery-status", "--state", state, "--principal", "ci-developer")["data"]
    for field in ("reservations", "journal_sequence", "journal_sha256", "recorded_at"):
        assert post_repair[field] == post_migration[field]
    assert run(*reserve_args, exit_code=4)["error"]["code"] == "replay_detected"
    assert not run("worker", "migrate", "--state", state)["data"]["changed"]
    assert run("worker", "check-owner", *owner_args, "--policy", policy_path, "--archive", archive)["data"]["ownership_valid"]
    compacted = run("worker", "compact", "--state", state)["data"]
    assert compacted["changed"] and compacted["generation"] == 2
    before = (state / "journal.jsonl").read_bytes()
    inspection = run("worker", "recovery-status", "--state", state, "--principal", "ci-developer")["data"]
    assert inspection["mode"] == "local_recovery_inspection" and not inspection["execution_authorized"]
    assert inspection["reservations"][-1] == claimed
    assert before == (state / "journal.jsonl").read_bytes()
    assert run("worker", "release", "--state", state, "--principal", "ci-developer",
               "--reservation", claimed["id"], exit_code=4)["error"]["code"] == "ownership_required"
    assert run("worker", "recover", *owner_args, exit_code=4)["error"]["code"] == "stop_confirmation_required"
    closed = run("worker", "recover", *owner_args, "--confirmed-stopped")["data"]
    assert not closed["stop_assertion_verified"] and closed["reservations"][0]["status"] == "recovered"
    assert run("worker", "check-owner", *owner_args, "--policy", policy_path, "--archive", archive,
               exit_code=4)["error"]["code"] == "ownership_conflict"
    assert run(*reserve_args, exit_code=4)["error"]["code"] == "replay_detected"
    assert run("worker", "compact", "--state", state)["data"]["changed"]
    assert run(*reserve_args, exit_code=4)["error"]["code"] == "replay_detected"

print("Snapshot, admission, replay, ownership, maintenance, manifest repair and append repair checks passed.")
