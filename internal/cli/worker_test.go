package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fpokrzywa/AppRivet/internal/admission"
	"github.com/fpokrzywa/AppRivet/internal/snapshot"
)

func workerInputs(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.swift"), []byte("print(1)"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := snapshot.Create(context.Background(), dir, []string{"a.swift"}, filepath.Join(dir, "source.tar"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	grant := admission.Grant{ID: "grant-1", Principal: "developer", ProjectID: "hello", Operation: "extract", SnapshotSHA256: r.SHA256, Enabled: true, NotBefore: now.Add(-time.Minute).Format(time.RFC3339), ExpiresAt: now.Add(time.Hour).Format(time.RFC3339), Limits: admission.Limits{MaxFiles: 5, MaxFileBytes: 1024, MaxSourceBytes: 1024, MaxArchiveBytes: 16384}}
	policy := admission.Policy{SchemaVersion: 1, Grants: []admission.Grant{grant}}
	request := admission.Request{SchemaVersion: 1, GrantID: grant.ID, ProjectID: grant.ProjectID, Operation: grant.Operation, SnapshotSHA256: r.SHA256}
	for name, value := range map[string]any{"policy.json": policy, "request.json": request} {
		b, _ := json.Marshal(value)
		if err := os.WriteFile(filepath.Join(dir, name), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir, r.SHA256
}

func TestSnapshotExtractionCLI(t *testing.T) {
	dir, digest := workerInputs(t)
	exit, r, raw := invoke(t, dir, "snapshot", "extract", "--archive", "source.tar", "--destination", "extracted", "--expect-sha256", digest, "--json")
	if exit != 0 || r["data"].(map[string]any)["snapshot"].(map[string]any)["sha256"] != digest {
		t.Fatalf("%d %s", exit, raw)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "extracted/a.swift")); err != nil || string(b) != "print(1)" {
		t.Fatalf("extracted %s %v", b, err)
	}
	exit, r, _ = invoke(t, dir, "snapshot", "extract", "--archive", "source.tar", "--destination", "extracted", "--expect-sha256", digest, "--json")
	if exit != 4 || r["error"].(map[string]any)["code"] != "destination_exists" {
		t.Fatalf("%d %+v", exit, r)
	}
	for _, args := range [][]string{{"snapshot", "extract", "--archive", "source.tar", "--destination", "new"}, {"snapshot", "extract", "--archive", "source.tar", "--expect-sha256", digest}, {"snapshot", "extract", "--destination", "new", "--expect-sha256", digest}} {
		exit, _, _ := invoke(t, dir, append(args, "--json")...)
		if exit != 2 {
			t.Fatalf("invalid flags accepted %v %d", args, exit)
		}
	}
}

func TestWorkerCheckCLIHonestOfflineDecision(t *testing.T) {
	dir, _ := workerInputs(t)
	args := []string{"worker", "check", "--policy", "policy.json", "--request", "request.json", "--principal", "developer", "--archive", "source.tar"}
	exit, r, raw := invoke(t, dir, append(args, "--json")...)
	if exit != 0 {
		t.Fatalf("%d %s", exit, raw)
	}
	d := r["data"].(map[string]any)
	if d["mode"] != "offline_policy_check" || d["eligible"] != true || d["authentication_verified"] != false || d["execution_authorized"] != false {
		t.Fatal(raw)
	}
	var out, errOut bytes.Buffer
	a := App{Dir: dir, OS: "windows", Out: &out, Err: &errOut, Runner: forbidRunner{t}}
	if exit = a.Run(context.Background(), args); exit != 0 || !strings.Contains(out.String(), "not authenticated") || !strings.Contains(out.String(), "not authorized") {
		t.Fatalf("%d %s %s", exit, out.String(), errOut.String())
	}
	args[7] = "attacker"
	exit, r, _ = invoke(t, dir, append(args, "--json")...)
	if exit != 4 || r["data"] != nil || r["error"].(map[string]any)["code"] != "admission_denied" {
		t.Fatalf("%d %+v", exit, r)
	}
}

func TestWorkerCLIRejectsExecutionAndMissingArguments(t *testing.T) {
	for _, args := range [][]string{{"worker"}, {"worker", "run"}, {"worker", "check"}, {"worker", "check", "--execute"}, {"worker", "check", "--policy", "p", "--request", "r", "--principal", "a", "--archive", "x", "extra"}} {
		exit, _, _ := invoke(t, t.TempDir(), append(args, "--json")...)
		if exit != 2 {
			t.Fatalf("%v %d", args, exit)
		}
	}
	for _, args := range [][]string{{"worker", "--help"}, {"worker", "check", "--help"}, {"snapshot", "extract", "--help"}} {
		exit, _, raw := invoke(t, t.TempDir(), append(args, "--json")...)
		if exit != 0 || !strings.Contains(raw, "worker check") {
			t.Fatalf("help %d %s", exit, raw)
		}
	}
}
