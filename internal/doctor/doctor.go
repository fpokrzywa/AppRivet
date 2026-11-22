package doctor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fpokrzywa/AppRivet/internal/runner"
)

type CheckResult struct {
	Name     string `json:"name"`
	OK       bool   `json:"ok"`
	Required bool   `json:"required"`
	Detail   string `json:"detail"`
	Remedy   string `json:"remedy,omitempty"`
}
type Report struct {
	HostOS          string        `json:"host_os"`
	HostArch        string        `json:"host_arch"`
	Ready           bool          `json:"ready_for_ios_simulator"`
	RemoteAvailable bool          `json:"remote_available"`
	Checks          []CheckResult `json:"checks"`
}

func Check(ctx context.Context, host, arch string, r runner.Runner) Report {
	report := Report{HostOS: host, HostArch: arch, Checks: []CheckResult{}}
	if host != "darwin" {
		report.Checks = append(report.Checks, CheckResult{Name: "macOS", Required: true, Detail: "iOS compilation requires a Mac with full Xcode", Remedy: "Use init/inspect here, then move the project to a Mac. Remote execution is not implemented."})
		return report
	}
	report.Ready = true
	probes := []struct {
		name, path string
		args       []string
		required   bool
		remedy     string
	}{
		{"developer-directory", "/usr/bin/xcode-select", []string{"-p"}, true, "Install full Xcode, open it to complete setup, then select it in Xcode Settings > Locations."},
		{"xcodebuild", "/usr/bin/xcodebuild", []string{"-version"}, true, "Install full Xcode and select its developer directory. Command Line Tools alone are insufficient."},
		{"swift", "/usr/bin/xcrun", []string{"--find", "swift"}, true, "Complete Xcode installation and command-line tool selection."},
		{"simctl", "/usr/bin/xcrun", []string{"--find", "simctl"}, true, "Complete Xcode first-launch setup and install iOS Simulator support in Xcode."},
		{"devicectl", "/usr/bin/xcrun", []string{"--find", "devicectl"}, false, "Use Xcode 15 or later for future physical-device support."},
		{"ios-simulator-sdk", "/usr/bin/xcrun", []string{"--sdk", "iphonesimulator", "--show-sdk-path"}, true, "Install iOS platform support using Xcode Settings > Components/Platforms."},
		{"ios-device-sdk", "/usr/bin/xcrun", []string{"--sdk", "iphoneos", "--show-sdk-path"}, true, "Install iOS platform support using Xcode Settings > Components/Platforms."},
	}
	for _, probe := range probes {
		result, err := r.Run(ctx, runner.Spec{Path: probe.path, Args: probe.args, Timeout: 15 * time.Second})
		detail := strings.TrimSpace(result.Output)
		ok := err == nil && result.ExitCode == 0 && detail != ""
		if probe.name == "developer-directory" && !strings.Contains(detail, ".app/Contents/Developer") {
			ok = false
		}
		if probe.name == "xcodebuild" && !strings.HasPrefix(detail, "Xcode ") {
			ok = false
		}
		if len(detail) > 2000 {
			detail = detail[:2000]
		}
		if !ok && detail == "" {
			detail = "Tool probe failed"
			if err != nil {
				detail = fmt.Sprint(err)
			}
		}
		check := CheckResult{Name: probe.name, OK: ok, Required: probe.required, Detail: detail}
		if !ok {
			check.Remedy = probe.remedy
			if probe.required {
				report.Ready = false
			}
		}
		report.Checks = append(report.Checks, check)
		if ctx.Err() != nil {
			report.Ready = false
			break
		}
	}
	return report
}
