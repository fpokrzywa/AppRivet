package build

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fpokrzywa/AppRivet/internal/project"
	"github.com/fpokrzywa/AppRivet/internal/runner"
)

type captureRunner struct {
	calls []runner.Spec
	fail  bool
}

func (r *captureRunner) Run(_ context.Context, s runner.Spec) (runner.Result, error) {
	r.calls = append(r.calls, s)
	if r.fail {
		return runner.Result{Output: "compiler diagnostic", ExitCode: 65}, errors.New("tool failed")
	}
	return runner.Result{Output: "build complete"}, nil
}
func request(t *testing.T) Request {
	t.Helper()
	p, err := project.Create(t.TempDir(), "Demo", "com.example.demo")
	if err != nil {
		t.Fatal(err)
	}
	return Request{Project: p.Root, Action: "build", TrustProject: true, Timeout: time.Minute}
}

func TestBuildRefusesUntrustedAndNonMacBeforeExecuting(t *testing.T) {
	for _, host := range []string{"windows", "linux", "darwin"} {
		r := &captureRunner{}
		req := request(t)
		if host == "darwin" {
			req.TrustProject = false
		}
		_, err := Execute(context.Background(), host, r, req)
		if err == nil || len(r.calls) != 0 {
			t.Fatalf("executed refused build on %s", host)
		}
	}
}

func TestBuildUsesOnlySimulatorAndNoProvisioning(t *testing.T) {
	r := &captureRunner{}
	req := request(t)
	result, err := Execute(context.Background(), "darwin", r, req)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 1 {
		t.Fatalf("expected build call, got %d", len(r.calls))
	}
	call := r.calls[0]
	if call.Path != "/usr/bin/xcodebuild" || call.Dir != req.Project || call.Timeout != time.Minute {
		t.Fatalf("bad execution %+v", call)
	}
	joined := strings.Join(call.Args, "|")
	for _, part := range []string{"-project|", "-scheme|Demo", "-sdk|iphonesimulator", "-destination|generic/platform=iOS Simulator", "CODE_SIGNING_ALLOWED=NO", "CODE_SIGNING_REQUIRED=NO", "-disableAutomaticPackageResolution"} {
		if !strings.Contains(joined, part) {
			t.Errorf("missing %s in %s", part, joined)
		}
	}
	if strings.Contains(joined, "allowProvisioning") || result.Execution.ExitCode != 0 {
		t.Fatal("unsafe build plan")
	}
	if !strings.HasPrefix(result.DerivedData, os.TempDir()) {
		t.Fatalf("build outputs should be in a fresh temp directory: %s", result.DerivedData)
	}
	t.Cleanup(func() { _ = os.RemoveAll(result.DerivedData) })
}

func TestTestRequiresExplicitSimulator(t *testing.T) {
	r := &captureRunner{}
	req := request(t)
	req.Action = "test"
	if _, err := Execute(context.Background(), "darwin", r, req); err == nil || len(r.calls) != 0 {
		t.Fatal("test accepted missing destination")
	}
	req.Simulator = "11111111-2222-3333-4444-555555555555"
	result, err := Execute(context.Background(), "darwin", r, req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(result.DerivedData) })
	if !strings.Contains(strings.Join(r.calls[0].Args, "|"), "platform=iOS Simulator,id=11111111-2222-3333-4444-555555555555") {
		t.Fatal("wrong test destination")
	}
}

func TestRejectsUnsafeOptions(t *testing.T) {
	cases := []func(*Request){func(r *Request) { r.Scheme = "-help" }, func(r *Request) { r.Scheme = "x\nCODE_SIGNING_ALLOWED=YES" }, func(r *Request) { r.Simulator = "id,platform=iOS" }, func(r *Request) { r.Action = "archive" }, func(r *Request) { r.Timeout = 0 }, func(r *Request) { r.Timeout = 25 * time.Hour }}
	for _, change := range cases {
		req := request(t)
		change(&req)
		r := &captureRunner{}
		if _, err := Execute(context.Background(), "darwin", r, req); err == nil || len(r.calls) != 0 {
			t.Fatal("unsafe options accepted")
		}
	}
}

func TestSwiftPackageIsNotAnIOSApplication(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Package.swift"), []byte("invalid executable Swift"), 0600); err != nil {
		t.Fatal(err)
	}
	r := &captureRunner{}
	req := Request{Project: root, Action: "build", TrustProject: true, Timeout: time.Minute}
	if _, err := Execute(context.Background(), "darwin", r, req); err == nil || len(r.calls) != 0 {
		t.Fatal("attempted package app conversion")
	}
}

func TestBuildFailurePreservesDiagnostics(t *testing.T) {
	r := &captureRunner{fail: true}
	result, err := Execute(context.Background(), "darwin", r, request(t))
	if result.DerivedData != "" {
		t.Cleanup(func() { _ = os.RemoveAll(result.DerivedData) })
	}
	if err == nil || result.Execution.ExitCode != 65 || result.Execution.Output != "compiler diagnostic" {
		t.Fatalf("lost failure: %+v %v", result, err)
	}
}
