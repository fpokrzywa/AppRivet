package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestWorkerRepairCLI(t *testing.T) {
	dir, _ := workerInputs(t)
	run := func(args ...string) map[string]any {
		t.Helper()
		exit, r, raw := invoke(t, dir, append(args, "--json")...)
		if exit != 0 {
			t.Fatalf("%d %s", exit, raw)
		}
		return r["data"].(map[string]any)
	}
	if err := os.WriteFile(filepath.Join(dir, "limits.json"), []byte(`{"schema_version":1,"max_active":2,"max_per_principal":1,"max_reserved_bytes":1048576,"max_lease_seconds":120}`), 0600); err != nil {
		t.Fatal(err)
	}
	run("worker", "init", "--state", "state", "--config", "limits.json")
	exit, r, raw := invoke(t, dir, "worker", "repair-status", "--state", "state", "--json")
	if exit != 4 || r["error"].(map[string]any)["code"] != "repair_unavailable" {
		t.Fatal(raw)
	}
	migrated := run("worker", "migrate", "--state", "state")
	id := strings.TrimSuffix(strings.TrimPrefix(migrated["prepared_manifest"].(string), "manifest-"), ".json")
	state := filepath.Join(dir, "state")
	intent, err := os.ReadFile(filepath.Join(state, "publication-"+id+".intent.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(state, "publication.pending"), intent, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(state, "journal.jsonl"), []byte(`{"schema_version":`), 0600); err != nil {
		t.Fatal(err)
	}
	plan := run("worker", "repair-status", "--state", "state")
	if plan["changed"] != false || plan["authentication_verified"] != false || plan["execution_authorized"] != false || plan["control_state"] != "torn" {
		t.Fatal(plan)
	}
	args := []string{"worker", "repair", "--state", "state", "--expect-intent-sha256", plan["intent_sha256"].(string), "--expect-control-sha256", plan["control_sha256"].(string), "--expect-history-sha256", plan["journal_sha256"].(string), "--expect-sequence", strconv.Itoa(int(plan["journal_sequence"].(float64)))}
	repaired := run(args...)
	if repaired["changed"] != true || repaired["control_rewritten"] != true || repaired["before_image"] == "" {
		t.Fatal(repaired)
	}
	for _, command := range []string{"repair-status", "repair"} {
		exit, _, raw = invoke(t, dir, "worker", command, "--help", "--json")
		if exit != 0 || !strings.Contains(raw, "worker repair") {
			t.Fatal(raw)
		}
		exit, _, raw = invoke(t, dir, "worker", command, "--json")
		if exit != 2 {
			t.Fatal(raw)
		}
		exit, _, raw = invoke(t, dir, "worker", command, "--state", "state", "--force", "--json")
		if exit != 2 {
			t.Fatal(raw)
		}
	}
	exit, _, raw = invoke(t, dir, "worker", "repair", "--state", "state", "--json")
	if exit != 2 {
		t.Fatal(raw)
	}
}
