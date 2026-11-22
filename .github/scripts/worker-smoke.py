"""Credential-free CLI integration; only trusted example source is used."""
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

print("Snapshot, admission, reservation/replay, exclusive ownership and explicit recovery checks passed.")
