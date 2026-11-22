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
		{[]string{"devices"}, 3, "unsupported_platform"},
		{[]string{"devices", "--timeout", "6m"}, 2, "invalid_arguments"},
		{[]string{"devices", "--from-json", ""}, 2, "invalid_arguments"},
		{[]string{"devices", "--nonsense"}, 2, "invalid_arguments"},
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

func TestCLIOfflineDeviceReportsAreExplicit(t *testing.T) {
	for _, sim := range []bool{false, true} {
		root := t.TempDir()
		fixtureName := "devicectl-modern.json"
		if sim {
			fixtureName = "simctl.json"
		}
		body, err := os.ReadFile(filepath.Join("..", "devices", "testdata", fixtureName))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "report.json"), body, 0600); err != nil {
			t.Fatal(err)
		}
		args := []string{"devices", "--from-json", "report.json", "--json"}
		if sim {
			args = append(args, "--simulators")
		}
		exit, result, _ := invoke(t, root, args...)
		if exit != 0 {
			t.Fatalf("%d %+v", exit, result)
		}
		data := result["data"].(map[string]any)
		if data["live"] != false || data["source"] != "file" {
			t.Fatalf("offline report labeled live: %+v", data)
		}
	}
}

type deviceRunner func(context.Context, runner.Spec) (runner.Result, error)

func (f deviceRunner) Run(c context.Context, s runner.Spec) (runner.Result, error) { return f(c, s) }

func TestCLILiveDevicesJSONAndHumanEmptyList(t *testing.T) {
	for _, jsonMode := range []bool{false, true} {
		var out, errOut bytes.Buffer
		app := App{OS: "darwin", Dir: t.TempDir(), Out: &out, Err: &errOut, Runner: deviceRunner(func(_ context.Context, s runner.Spec) (runner.Result, error) {
			if err := os.WriteFile(s.Args[4], []byte(`{"info":{"commandType":"devicectl.list.devices","outcome":"success"},"result":{"devices":[]}}`), 0600); err != nil {
				t.Fatal(err)
			}
			return runner.Result{}, nil
		})}
		args := []string{"devices"}
		if jsonMode {
			args = append(args, "--json")
		}
		if exit := app.Run(context.Background(), args); exit != 0 || errOut.Len() != 0 {
			t.Fatalf("%d %s", exit, errOut.String())
		}
		if jsonMode {
			var env map[string]any
			if err := json.Unmarshal(out.Bytes(), &env); err != nil {
				t.Fatal(err)
			}
			data := env["data"].(map[string]any)
			if data["live"] != true || len(data["devices"].([]any)) != 0 {
				t.Fatal("bad live list")
			}
		} else {
			if !strings.Contains(out.String(), "No devices reported") || !strings.Contains(out.String(), "authorization") {
				t.Fatalf("misleading empty display: %s", out.String())
			}
		}
	}
}

func TestCLIDeviceFailureCodes(t *testing.T) {
	for _, tc := range []struct {
		result runner.Result
		exit   int
		code   string
	}{{runner.Result{TimedOut: true}, 124, "timeout"}, {runner.Result{Cancelled: true}, 130, "cancelled"}, {runner.Result{ExitCode: 1}, 5, "tool_failed"}, {runner.Result{}, 4, "invalid_report"}} {
		var out, errOut bytes.Buffer
		app := App{OS: "darwin", Dir: t.TempDir(), Out: &out, Err: &errOut, Runner: deviceRunner(func(context.Context, runner.Spec) (runner.Result, error) { return tc.result, nil })}
		if exit := app.Run(context.Background(), []string{"devices", "--json"}); exit != tc.exit {
			t.Fatalf("want %d got %d: %s", tc.exit, exit, out.String())
		}
		var env map[string]any
		if err := json.Unmarshal(out.Bytes(), &env); err != nil {
			t.Fatal(err)
		}
		if env["ok"] != false || env["error"].(map[string]any)["code"] != tc.code || env["data"] != nil || errOut.Len() != 0 {
			t.Fatalf("bad failure response: %s %s", out.String(), errOut.String())
		}
	}
}

func TestCLIHumanOfflineDeviceTable(t *testing.T) {
	var out, errOut bytes.Buffer
	app := App{OS: "windows", Dir: t.TempDir(), Out: &out, Err: &errOut, Runner: forbidRunner{t}}
	path, err := filepath.Abs(filepath.Join("..", "devices", "testdata", "devicectl-legacy.json"))
	if err != nil {
		t.Fatal(err)
	}
	if exit := app.Run(context.Background(), []string{"devices", "--from-json", path}); exit != 0 {
		t.Fatalf("%d %s", exit, errOut.String())
	}
	for _, want := range []string{"Offline report", "Lab iPhone", "physical", "disconnected", "unknown"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %s: %s", want, out.String())
		}
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
