package doctor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fpokrzywa/AppRivet/internal/runner"
)

type probeRunner struct {
	fail  string
	calls []runner.Spec
}

func (r *probeRunner) Run(_ context.Context, s runner.Spec) (runner.Result, error) {
	r.calls = append(r.calls, s)
	key := s.Path + " " + strings.Join(s.Args, " ")
	if strings.Contains(key, r.fail) && r.fail != "" {
		return runner.Result{ExitCode: 1}, errors.New("missing tool")
	}
	out := "/Applications/Xcode.app/Contents/Developer/usr/bin/tool"
	if s.Path == "/usr/bin/xcode-select" {
		out = "/Applications/Xcode.app/Contents/Developer"
	}
	if strings.Contains(key, "-version") {
		out = "Xcode 16.4\nBuild version 16F6"
	}
	return runner.Result{Output: out}, nil
}

func TestNonMacDoctorNeverExecutesAppleTools(t *testing.T) {
	for _, host := range []string{"windows", "linux"} {
		r := &probeRunner{}
		report := Check(context.Background(), host, "amd64", r)
		if report.Ready || len(r.calls) != 0 || report.HostOS != host {
			t.Fatalf("incorrect host report: %+v", report)
		}
	}
}

func TestMacDoctorReadinessRequiresBuildToolsAndSDK(t *testing.T) {
	for _, failing := range []string{"", "xcode-select", "-version", "--find swift", "--find simctl", "--sdk iphonesimulator", "--sdk iphoneos"} {
		r := &probeRunner{fail: failing}
		report := Check(context.Background(), "darwin", "arm64", r)
		if report.Ready != (failing == "") {
			t.Fatalf("failure %q: %+v", failing, report)
		}
		for _, call := range r.calls {
			if call.Timeout <= 0 || !strings.HasPrefix(call.Path, "/usr/bin/") {
				t.Fatalf("unsafe probe %+v", call)
			}
		}
	}
}

func TestDeviceToolIsOptionalForSimulatorMilestone(t *testing.T) {
	r := &probeRunner{fail: "--find devicectl"}
	report := Check(context.Background(), "darwin", "arm64", r)
	if !report.Ready {
		t.Fatal("missing device tool incorrectly blocks simulator builds")
	}
	found := false
	for _, check := range report.Checks {
		if check.Name == "devicectl" {
			found = true
			if check.OK || check.Required {
				t.Fatal("wrong device status")
			}
		}
	}
	if !found {
		t.Fatal("device check missing")
	}
}
