package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fpokrzywa/AppRivet/internal/admission"
)

func TestWorkerJournalMaintenanceCLI(t *testing.T) {
	dir, _ := workerInputs(t)
	c := admission.Config{SchemaVersion: 1, MaxActive: 2, MaxPerPrincipal: 1, MaxReservedBytes: 1 << 20, MaxLeaseSeconds: 120}
	b, _ := json.Marshal(c)
	os.WriteFile(filepath.Join(dir, "limits.json"), b, 0600)
	run := func(args ...string) map[string]any {
		t.Helper()
		exit, r, raw := invoke(t, dir, append(args, "--json")...)
		if exit != 0 {
			t.Fatalf("%d %s", exit, raw)
		}
		return r["data"].(map[string]any)
	}
	run("worker", "init", "--state", "state", "--config", "limits.json")
	exit, r, raw := invoke(t, dir, "worker", "compact", "--state", "state", "--json")
	if exit != 4 || r["error"].(map[string]any)["code"] != "migration_required" {
		t.Fatal(raw)
	}
	migrated := run("worker", "migrate", "--state", "state")
	if migrated["changed"] != true || migrated["format"] != float64(2) || migrated["authentication_verified"] != false || migrated["execution_authorized"] != false {
		t.Fatal(migrated)
	}
	if again := run("worker", "migrate", "--state", "state"); again["changed"] != false {
		t.Fatal(again)
	}
	run("worker", "status", "--state", "state", "--principal", "developer")
	if compacted := run("worker", "compact", "--state", "state"); compacted["changed"] != true || compacted["generation"] != float64(2) {
		t.Fatal(compacted)
	}
	for _, command := range []string{"migrate", "compact"} {
		exit, _, raw = invoke(t, dir, "worker", command, "--json")
		if exit != 2 {
			t.Fatal(raw)
		}
		exit, _, raw = invoke(t, dir, "worker", command, "--help", "--json")
		if exit != 0 || !strings.Contains(raw, "worker compact") {
			t.Fatal(raw)
		}
		exit, _, raw = invoke(t, dir, "worker", command, "--state", "state", "--destination", "other", "--json")
		if exit != 2 {
			t.Fatal("relocation unexpectedly available", raw)
		}
	}
	var out, errOut bytes.Buffer
	app := App{Dir: dir, Out: &out, Err: &errOut, Runner: forbidRunner{t}}
	if exit = app.Run(context.Background(), []string{"worker", "compact", "--state", "state"}); exit != 0 || !strings.Contains(out.String(), "not authenticated") || !strings.Contains(out.String(), "history retained") {
		t.Fatalf("%d %s %s", exit, out.String(), errOut.String())
	}
}
