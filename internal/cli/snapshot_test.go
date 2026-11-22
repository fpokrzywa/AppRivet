package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotCLIOnPortableHost(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.swift"), []byte("print(1)"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "files.txt"), []byte("main.swift\n"), 0600); err != nil {
		t.Fatal(err)
	}
	exit, result, raw := invoke(t, root, "snapshot", "create", "--root", ".", "--files-from", "files.txt", "--output", "app.tar", "--json")
	if exit != 0 || result["ok"] != true {
		t.Fatalf("create: %d %s", exit, raw)
	}
	data := result["data"].(map[string]any)
	digest := data["sha256"].(string)
	if data["format"] != "apprivet-source-v1" || len(data["files"].([]any)) != 1 {
		t.Fatal(raw)
	}
	exit, result, raw = invoke(t, root, "snapshot", "inspect", "--archive", "app.tar", "--expect-sha256", digest, "--json")
	if exit != 0 || result["data"].(map[string]any)["sha256"] != digest {
		t.Fatalf("inspect: %d %s", exit, raw)
	}
	exit, result, raw = invoke(t, root, "snapshot", "inspect", "--archive", "app.tar", "--expect-sha256", strings.Repeat("0", 64), "--json")
	if exit != 4 || result["data"] != nil || result["error"].(map[string]any)["code"] != "digest_mismatch" {
		t.Fatalf("mismatch: %d %s", exit, raw)
	}
	var out, errOut bytes.Buffer
	a := App{OS: "linux", Dir: root, Out: &out, Err: &errOut, Runner: forbidRunner{t}}
	if exit = a.Run(context.Background(), []string{"snapshot", "inspect", "--archive", "app.tar"}); exit != 0 || !strings.Contains(out.String(), digest) || !strings.Contains(out.String(), "not authorization") || !strings.Contains(out.String(), "main.swift") {
		t.Fatalf("human: %d %s %s", exit, out.String(), errOut.String())
	}
}

func TestSnapshotCLIRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{
		{"snapshot"}, {"snapshot", "upload"}, {"snapshot", "create"}, {"snapshot", "inspect"},
		{"snapshot", "create", "--files-from", "files", "--output", "out", "--root", ""},
		{"snapshot", "inspect", "--archive", "out", "--expect-sha256", ""},
		{"snapshot", "inspect", "--archive", "out", "--expect-sha256", "bad"},
		{"snapshot", "inspect", "--archive", "out", "extra"},
		{"snapshot", "create", "--archive", "out"},
	} {
		exit, r, _ := invoke(t, t.TempDir(), append(args, "--json")...)
		if exit != 2 || r["error"].(map[string]any)["code"] != "invalid_arguments" {
			t.Fatalf("%v: %d %+v", args, exit, r)
		}
	}
	for _, args := range [][]string{{"snapshot", "--help"}, {"snapshot", "create", "--help"}, {"snapshot", "inspect", "--help"}} {
		exit, _, raw := invoke(t, t.TempDir(), append(args, "--json")...)
		if exit != 0 || !strings.Contains(raw, "snapshot create") {
			t.Fatalf("help %d %s", exit, raw)
		}
	}
}
