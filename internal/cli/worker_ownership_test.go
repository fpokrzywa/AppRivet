package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fpokrzywa/AppRivet/internal/admission"
)

func TestWorkerOwnershipAndRecoveryCLI(t *testing.T) {
	dir, _ := workerInputs(t)
	write := func(name string, value any) {
		b, _ := json.Marshal(value)
		if err := os.WriteFile(filepath.Join(dir, name), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("limits.json", admission.Config{SchemaVersion: 1, MaxActive: 2, MaxPerPrincipal: 1, MaxReservedBytes: 1 << 20, MaxLeaseSeconds: 120})
	r, err := admission.LoadRequest(filepath.Join(dir, "request.json"))
	if err != nil {
		t.Fatal(err)
	}
	r.RequestID = "request"
	r.Nonce = strings.Repeat("a", 32)
	r.LeaseSeconds = 60
	write("request.json", r)
	run := func(args ...string) map[string]any {
		t.Helper()
		exit, result, raw := invoke(t, dir, append(args, "--json")...)
		if exit != 0 {
			t.Fatalf("%d %s", exit, raw)
		}
		return result["data"].(map[string]any)
	}
	run("worker", "init", "--state", "state", "--config", "limits.json")
	reserved := run("worker", "reserve", "--state", "state", "--principal", "developer", "--policy", "policy.json", "--request", "request.json", "--archive", "source.tar")
	id := reserved["reservations"].([]any)[0].(map[string]any)["id"].(string)
	claimed := run("worker", "claim", "--state", "state", "--principal", "developer", "--reservation", id, "--worker", "worker-1", "--policy", "policy.json", "--archive", "source.tar")
	job := claimed["reservations"].([]any)[0].(map[string]any)
	tuple := []string{"--state", "state", "--principal", "developer", "--reservation", id, "--worker", "worker-1", "--claim", job["claim_id"].(string), "--fence", fmt.Sprint(job["fence"])}
	check := append(append([]string{"worker", "check-owner"}, tuple...), "--policy", "policy.json", "--archive", "source.tar")
	checked := run(check...)
	if checked["ownership_valid"] != true || checked["execution_authorized"] != false || checked["authentication_verified"] != false {
		t.Fatal(checked)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "state/journal.jsonl"))
	report := run("worker", "recovery-status", "--state", "state", "--principal", "developer")
	if report["mode"] != "local_recovery_inspection" || report["execution_authorized"] != false || len(report["reservations"].([]any)) != 1 {
		t.Fatal(report)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "state/journal.jsonl"))
	if !bytes.Equal(before, after) {
		t.Fatal("recovery inspection mutated state")
	}
	recovery := append([]string{"worker", "recover"}, tuple...)
	exit, result, raw := invoke(t, dir, append(recovery, "--json")...)
	if exit != 4 || result["error"].(map[string]any)["code"] != "stop_confirmation_required" {
		t.Fatal(raw)
	}
	recovered := run(append(recovery, "--confirmed-stopped")...)
	if recovered["stop_assertion_verified"] != false || recovered["reservations"].([]any)[0].(map[string]any)["status"] != "recovered" {
		t.Fatal(recovered)
	}
	exit, result, raw = invoke(t, dir, append(check, "--json")...)
	if exit != 4 || result["data"] != nil || result["error"].(map[string]any)["code"] != "ownership_conflict" {
		t.Fatal(raw)
	}
	var out, errOut bytes.Buffer
	app := App{Dir: dir, Out: &out, Err: &errOut, Runner: forbidRunner{t}}
	if exit = app.Run(context.Background(), []string{"worker", "recovery-status", "--state", "state", "--principal", "developer"}); exit != 0 || !strings.Contains(out.String(), "not authenticated") || !strings.Contains(out.String(), "recovered") || !strings.Contains(out.String(), job["claim_id"].(string)) {
		t.Fatalf("%d %s %s", exit, out.String(), errOut.String())
	}
}

func TestWorkerOwnershipCLIArguments(t *testing.T) {
	for _, command := range []string{"claim", "check-owner", "recover", "recovery-status"} {
		exit, _, raw := invoke(t, t.TempDir(), "worker", command, "--json")
		if exit != 2 {
			t.Fatal(raw)
		}
		exit, _, raw = invoke(t, t.TempDir(), "worker", command, "--help", "--json")
		if exit != 0 || !strings.Contains(raw, "check-owner") {
			t.Fatal(raw)
		}
	}
	for _, args := range [][]string{{"recovery-status", "--state", "s", "--principal", "p", "--confirmed-stopped"}, {"claim", "--state", "s", "--principal", "p", "--reservation", "r", "--worker", "w"}, {"recover", "--state", "s", "--principal", "p", "--reservation", "r", "--worker", "w", "--claim", "c", "--fence", "not-an-int"}} {
		exit, _, raw := invoke(t, t.TempDir(), append(append([]string{"worker"}, args...), "--json")...)
		if exit != 2 {
			t.Fatal(raw)
		}
	}
}
