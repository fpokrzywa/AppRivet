package build

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/fpokrzywa/AppRivet/internal/project"
	"github.com/fpokrzywa/AppRivet/internal/runner"
)

type Request struct {
	Project      string
	Scheme       string
	Action       string
	Simulator    string
	TrustProject bool
	Timeout      time.Duration
}
type Result struct {
	Project     project.Info  `json:"project"`
	Action      string        `json:"action"`
	Scheme      string        `json:"scheme"`
	DerivedData string        `json:"derived_data"`
	Execution   runner.Result `json:"execution"`
}
type Error struct{ Code, Message string }

func (e *Error) Error() string           { return e.Message }
func failure(code, message string) error { return &Error{Code: code, Message: message} }

var schemePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,127}$`)
var simulatorPattern = regexp.MustCompile(`^[A-Fa-f0-9]{8}-[A-Fa-f0-9]{4}-[A-Fa-f0-9]{4}-[A-Fa-f0-9]{4}-[A-Fa-f0-9]{12}$`)

func Execute(ctx context.Context, host string, r runner.Runner, req Request) (Result, error) {
	result := Result{Action: req.Action}
	if host != "darwin" {
		return result, failure("unsupported_platform", "iOS builds require macOS with full Xcode; remote Mac execution is not implemented")
	}
	if !req.TrustProject {
		return result, failure("project_trust_required", "build/test runs project scripts and plugins as your OS user. Review the source and use a credential-free Mac account, then pass --trust-project. This flag is not a sandbox.")
	}
	if req.Action != "build" && req.Action != "test" {
		return result, failure("invalid_arguments", "only simulator build and test actions are supported")
	}
	if req.Timeout <= 0 || req.Timeout > 24*time.Hour {
		return result, failure("invalid_arguments", "timeout must be greater than zero and at most 24h")
	}
	if req.Simulator != "" && !simulatorPattern.MatchString(req.Simulator) {
		return result, failure("invalid_arguments", "simulator must be a UUID from xcrun simctl list devices available")
	}
	if req.Action == "test" && req.Simulator == "" {
		return result, failure("invalid_arguments", "test requires --simulator UUID; list available simulators with xcrun simctl list devices available")
	}
	info, err := project.Detect(req.Project)
	if err != nil {
		return result, failure("invalid_project", err.Error())
	}
	result.Project = info
	if info.Kind == "swift-package" {
		return result, failure("unsupported_project", "Swift package detected; package-to-iOS-app conversion is not implemented. Use an Xcode app project or workspace.")
	}
	scheme := req.Scheme
	if scheme == "" {
		scheme = info.Scheme
	}
	if !schemePattern.MatchString(scheme) {
		return result, failure("invalid_arguments", "scheme must start with a letter/digit and contain only letters, digits, spaces, dots, underscores or hyphens (up to 128 characters)")
	}
	result.Scheme = scheme
	// A fresh system-owned temporary directory avoids following a repository's
	// preplanted DerivedData symlink and separates each invocation's artifacts.
	derived, err := os.MkdirTemp("", "apprivet-build-")
	if err != nil {
		return result, failure("io_error", "cannot create private build output directory")
	}
	result.DerivedData = derived
	selector := "-project"
	if info.Kind == "xcode-workspace" {
		selector = "-workspace"
	}
	destination := "generic/platform=iOS Simulator"
	if req.Simulator != "" {
		destination = "platform=iOS Simulator,id=" + req.Simulator
	}
	args := []string{selector, info.Path, "-scheme", scheme, "-configuration", "Debug", "-sdk", "iphonesimulator", "-destination", destination, "-destination-timeout", "60", "-derivedDataPath", derived, "-clonedSourcePackagesDirPath", filepath.Join(derived, "SourcePackages"), "-disableAutomaticPackageResolution", "-onlyUsePackageVersionsFromResolvedFile", "CODE_SIGNING_ALLOWED=NO", "CODE_SIGNING_REQUIRED=NO", "CODE_SIGN_IDENTITY=", req.Action}
	if req.Action == "test" {
		args = append(args, "-parallel-testing-enabled", "NO")
	}
	result.Execution, err = r.Run(ctx, runner.Spec{Path: "/usr/bin/xcodebuild", Args: args, Dir: info.Root, Timeout: req.Timeout})
	if err != nil {
		if result.Execution.TimedOut {
			return result, failure("timeout", "Xcode exceeded the timeout; build process group cancellation requested")
		}
		if result.Execution.Cancelled {
			return result, failure("cancelled", "Xcode operation cancelled")
		}
		return result, failure("tool_failed", fmt.Sprintf("xcodebuild failed (exit %d). Inspect output, verify full Xcode/platform setup, and confirm the shared scheme and resolved dependencies.", result.Execution.ExitCode))
	}
	return result, nil
}
