package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fpokrzywa/AppRivet/internal/runner"
)

type forbidRunner struct{ t *testing.T }

func (f forbidRunner) Run(context.Context, runner.Spec) (runner.Result, error) {
	f.t.Fatal("unexpected subprocess")
	return runner.Result{}, nil
}

func invoke(t *testing.T, dir string, args ...string) (int, map[string]any, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	app := App{OS: "windows", Arch: "amd64", Dir: dir, Runner: forbidRunner{t}, Out: &out, Err: &errOut, Version: "test-version"}
	exit := app.Run(context.Background(), args)
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON %q: %v", out.String(), err)
	}
	if result["schema_version"] != float64(1) {
		t.Fatal("missing schema version")
	}
	if errOut.Len() != 0 {
		t.Fatalf("JSON command wrote stderr: %q", errOut.String())
	}
	return exit, result, out.String()
}

func TestCLIInitAndInspectJSON(t *testing.T) {
	root := t.TempDir()
	exit, result, _ := invoke(t, root, "init", "Hello", "--bundle-id", "org.example.hello", "--json")
	if exit != 0 || result["ok"] != true {
		t.Fatalf("%d %+v", exit, result)
	}
	if _, err := os.Stat(filepath.Join(root, "Hello/Hello.xcodeproj/project.pbxproj")); err != nil {
		t.Fatal(err)
	}
	exit, result, _ = invoke(t, root, "--json", "inspect", "--project", "Hello")
	if exit != 0 || result["data"].(map[string]any)["kind"] != "xcode-project" {
		t.Fatalf("%d %+v", exit, result)
	}
}

func TestCLIJSONErrors(t *testing.T) {
	for _, tc := range []struct {
		args []string
		exit int
		code string
	}{
		{[]string{"doctor"}, 3, "environment_not_ready"},
		{[]string{"build", "--trust-project"}, 3, "unsupported_platform"},
		{[]string{"init", "../Escape"}, 4, "project_creation_failed"},
		{[]string{"init"}, 2, "invalid_arguments"},
		{[]string{"init", "A", "B"}, 2, "invalid_arguments"},
		{[]string{"inspect", "--nonsense"}, 2, "invalid_arguments"},
		{[]string{"devices"}, 3, "not_implemented"},
		{[]string{"install", "--device", "abc"}, 3, "not_implemented"},
		{[]string{"launch", "--device", "abc"}, 3, "not_implemented"},
		{[]string{"typo"}, 2, "unknown_command"},
		{[]string{"build", "--timeout", "0"}, 2, "invalid_arguments"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			exit, result, _ := invoke(t, t.TempDir(), append(tc.args, "--json")...)
			if exit != tc.exit || result["ok"] != false || result["error"].(map[string]any)["code"] != tc.code {
				t.Fatalf("%d %+v", exit, result)
			}
		})
	}
}

func TestCLIHelpAndVersion(t *testing.T) {
	for _, args := range [][]string{{"--json"}, {"help", "--json"}, {"build", "--help", "--json"}, {"version", "--json"}} {
		exit, result, _ := invoke(t, t.TempDir(), args...)
		if exit != 0 || result["ok"] != true {
			t.Fatalf("%d %+v", exit, result)
		}
	}
}

func TestCLIHumanDoctorActionable(t *testing.T) {
	var out, errOut bytes.Buffer
	app := App{OS: "linux", Arch: "amd64", Dir: t.TempDir(), Runner: forbidRunner{t}, Out: &out, Err: &errOut}
	if exit := app.Run(context.Background(), []string{"doctor"}); exit != 3 {
		t.Fatalf("exit %d", exit)
	}
	if !strings.Contains(out.String(), "Mac") || !strings.Contains(out.String(), "linux/amd64") {
		t.Fatalf("not actionable: %s", out.String())
	}
}
