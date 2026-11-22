package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestHelperProcess(t *testing.T) {
	var mode string
	for i, arg := range os.Args {
		if arg == "--apprivet-helper" && i+1 < len(os.Args) {
			mode = os.Args[i+1]
		}
	}
	if mode == "" {
		return
	}
	switch mode {
	case "output":
		fmt.Print("token=secret-value\nAuthorization: Bearer abc.def.ghi\n-----BEGIN PRIVATE KEY-----\nPRIVATEBYTES\n-----END PRIVATE KEY-----\nuseful error")
		os.Exit(7)
	case "flood":
		fmt.Print(strings.Repeat("x", MaxOutput+8192))
		os.Exit(0)
	case "sleep":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "env":
		_ = json.NewEncoder(os.Stdout).Encode(os.Environ())
		os.Exit(0)
	case "json":
		fmt.Fprint(os.Stdout, `{"name":"token=raw-value","devices":[]}`)
		fmt.Fprint(os.Stderr, "password=stderr-secret")
		os.Exit(0)
	}
	os.Exit(9)
}

func TestStructuredStdoutIsSeparateBoundedAndNeverSerialized(t *testing.T) {
	spec := helper(t, "json")
	spec.SeparateStdout = true
	result, err := (Exec{}).Run(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	if err := json.Unmarshal([]byte(result.Stdout), &report); err != nil || report["name"] != "token=raw-value" {
		t.Fatalf("structured stdout corrupted: %q %v", result.Stdout, err)
	}
	if strings.Contains(result.Output, "stderr-secret") || strings.Contains(result.Output, "raw-value") {
		t.Fatal("diagnostic output leaked raw data")
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "raw-value") {
		t.Fatal("raw stdout serialized into diagnostic result")
	}
	spec = helper(t, "flood")
	spec.SeparateStdout = true
	result, err = (Exec{}).Run(context.Background(), spec)
	if err != nil || !result.StdoutTruncated || len(result.Stdout) > MaxOutput || result.Output != "" {
		t.Fatalf("unbounded/separated capture failed: %v", err)
	}
}

func helper(t *testing.T, mode string) Spec {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return Spec{Path: exe, Args: []string{"-test.run=TestHelperProcess", "--", "--apprivet-helper", mode}, Timeout: 5 * time.Second}
}

func TestRunRedactsFailureOutput(t *testing.T) {
	result, err := (Exec{}).Run(context.Background(), helper(t, "output"))
	if err == nil || result.ExitCode != 7 {
		t.Fatalf("expected exit 7: %+v %v", result, err)
	}
	for _, secret := range []string{"secret-value", "abc.def.ghi", "PRIVATEBYTES"} {
		if strings.Contains(result.Output, secret) {
			t.Errorf("leaked %s", secret)
		}
	}
	if !strings.Contains(result.Output, "useful error") {
		t.Fatalf("lost diagnostics: %+v", result)
	}
}

func TestRunLimitsOutput(t *testing.T) {
	result, err := (Exec{}).Run(context.Background(), helper(t, "flood"))
	if err != nil || !result.Truncated || len(result.Output) > MaxOutput {
		t.Fatalf("output not bounded: length %d, truncated %v, err %v", len(result.Output), result.Truncated, err)
	}
}

func TestRunTimeout(t *testing.T) {
	spec := helper(t, "sleep")
	spec.Timeout = 100 * time.Millisecond
	start := time.Now()
	result, err := (Exec{}).Run(context.Background(), spec)
	if err == nil || !result.TimedOut || time.Since(start) > 5*time.Second {
		t.Fatalf("timeout failed: %+v %v", result, err)
	}
}

func TestRunCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := (Exec{}).Run(ctx, helper(t, "sleep"))
	if err == nil || !result.Cancelled {
		t.Fatalf("cancellation failed: %+v %v", result, err)
	}
}

func TestRunDoesNotInheritSecretsOrToolOverrides(t *testing.T) {
	t.Setenv("APP_STORE_CONNECT_PRIVATE_KEY", "secret")
	t.Setenv("GITHUB_TOKEN", "secret")
	t.Setenv("DYLD_INSERT_LIBRARIES", "evil")
	t.Setenv("DEVELOPER_DIR", "evil")
	result, err := (Exec{}).Run(context.Background(), helper(t, "env"))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"APP_STORE_CONNECT_PRIVATE_KEY", "GITHUB_TOKEN", "DYLD_INSERT_LIBRARIES", "DEVELOPER_DIR"} {
		if strings.Contains(result.Output, key) {
			t.Errorf("inherited %s", key)
		}
	}
}

func TestRunRejectsRelativeToolsAndMissingTimeout(t *testing.T) {
	for _, spec := range []Spec{{Path: "xcodebuild", Timeout: time.Second}, {Path: "/usr/bin/xcodebuild"}} {
		if _, err := (Exec{}).Run(context.Background(), spec); err == nil {
			t.Fatal("accepted unsafe spec")
		}
	}
}

func TestRedactIncompleteAndQuotedSecrets(t *testing.T) {
	for _, input := range []string{"api_key=supersecret", `"password": "supersecret"`, "-----BEGIN EC PRIVATE KEY-----\nsupersecret", "Authorization: Bearer supersecret", "access_token: supersecret", "secret='supersecret'"} {
		if output := Redact(input); strings.Contains(output, "supersecret") {
			t.Fatalf("leaked: %s", output)
		}
	}
}
