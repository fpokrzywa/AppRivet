package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/fpokrzywa/AppRivet/internal/build"
	"github.com/fpokrzywa/AppRivet/internal/doctor"
	"github.com/fpokrzywa/AppRivet/internal/project"
	"github.com/fpokrzywa/AppRivet/internal/runner"
)

type App struct {
	OS, Arch, Dir, Version string
	Runner                 runner.Runner
	Out, Err               io.Writer
}
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type Envelope struct {
	SchemaVersion int    `json:"schema_version"`
	Command       string `json:"command"`
	OK            bool   `json:"ok"`
	Data          any    `json:"data,omitempty"`
	Error         *Error `json:"error,omitempty"`
}

const help = `AppRivet - independent iOS development toolkit (Milestone 1)

Usage:
  apprivet doctor [--json]
  apprivet init NAME [--bundle-id com.example.app] [--json]
  apprivet inspect [--project PATH] [--json]
  apprivet build [--project PATH] [--scheme NAME] --trust-project [--json]
  apprivet test [--project PATH] [--scheme NAME] --simulator UUID --trust-project [--json]
  apprivet version [--json]

Build/test options:
  --timeout DURATION   Maximum execution time (default 15m, maximum 24h)
  --simulator UUID     Select a simulator; required for test
  --trust-project      Acknowledge project scripts execute as your OS user

Use --help with a command. --json is supported before or after the command.
Windows/Linux: project creation and inspection only. iOS builds require a Mac
with full Xcode. Remote execution, signing, devices, install, launch, and
publishing are not implemented. Build/test is simulator-only and not sandboxed.
`

func (a App) Run(ctx context.Context, args []string) int {
	jsonMode := false
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--json" {
			jsonMode = true
		} else {
			filtered = append(filtered, arg)
		}
	}
	command := "help"
	if len(filtered) > 0 {
		command = filtered[0]
		filtered = filtered[1:]
	}
	response := Envelope{SchemaVersion: 1, Command: command, OK: true}
	fail := func(code, message string, exit int) int {
		response.OK = false
		response.Error = &Error{code, runner.Redact(message)}
		return a.emit(response, jsonMode, exit)
	}
	if command == "help" || command == "--help" || command == "-h" {
		response.Data = map[string]string{"help": help}
		return a.emit(response, jsonMode, 0)
	}
	if command == "devices" || command == "install" || command == "launch" {
		return fail("not_implemented", command+" is planned after Milestone 1; use Xcode's official device workflow", 3)
	}
	if command != "doctor" && command != "init" && command != "inspect" && command != "build" && command != "test" && command != "version" {
		return fail("unknown_command", "unknown command; run apprivet help", 2)
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	projectPath, scheme, bundle, simulator := ".", "", "", ""
	trust := false
	timeout := 15 * time.Minute
	switch command {
	case "init":
		fs.StringVar(&bundle, "bundle-id", "", "reverse-DNS bundle identifier")
		// The documented init NAME --options form also works with Go's flag parser.
		if len(filtered) > 0 && !strings.HasPrefix(filtered[0], "-") {
			filtered = append(append([]string{}, filtered[1:]...), filtered[0])
		}
	case "inspect":
		fs.StringVar(&projectPath, "project", ".", "project root or Xcode container")
	case "build", "test":
		fs.StringVar(&projectPath, "project", ".", "project root or Xcode container")
		fs.StringVar(&scheme, "scheme", "", "shared Xcode scheme")
		fs.StringVar(&simulator, "simulator", "", "simulator UUID")
		fs.BoolVar(&trust, "trust-project", false, "acknowledge execution of trusted project code")
		fs.DurationVar(&timeout, "timeout", 15*time.Minute, "execution timeout")
	}
	if err := fs.Parse(filtered); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			response.Data = map[string]string{"help": help}
			return a.emit(response, jsonMode, 0)
		}
		return fail("invalid_arguments", "invalid or unknown option; run apprivet "+command+" --help", 2)
	}
	if (command == "init" && fs.NArg() != 1) || (command != "init" && fs.NArg() != 0) {
		return fail("invalid_arguments", "unexpected or missing positional argument; run apprivet "+command+" --help", 2)
	}
	if timeout <= 0 || timeout > 24*time.Hour {
		return fail("invalid_arguments", "timeout must be greater than zero and at most 24h", 2)
	}
	if !filepath.IsAbs(projectPath) {
		projectPath = filepath.Join(a.Dir, projectPath)
	}
	switch command {
	case "version":
		response.Data = map[string]string{"version": a.Version}
	case "doctor":
		report := doctor.Check(ctx, a.OS, a.Arch, a.Runner)
		response.Data = report
		if !report.Ready {
			return fail("environment_not_ready", "this host is not ready for iOS Simulator builds; see checks and remedies", 3)
		}
	case "init":
		name := fs.Arg(0)
		if bundle == "" {
			bundle = "com.example." + strings.ToLower(name)
		}
		info, err := project.Create(a.Dir, name, bundle)
		if err != nil {
			return fail("project_creation_failed", err.Error(), 4)
		}
		response.Data = info
	case "inspect":
		info, err := project.Detect(projectPath)
		if err != nil {
			return fail("invalid_project", err.Error(), 4)
		}
		response.Data = info
	case "build", "test":
		result, err := build.Execute(ctx, a.OS, a.Runner, build.Request{Project: projectPath, Scheme: scheme, Action: command, Simulator: simulator, TrustProject: trust, Timeout: timeout})
		response.Data = result
		if err != nil {
			var detail *build.Error
			if errors.As(err, &detail) {
				exit := 4
				switch detail.Code {
				case "unsupported_platform", "unsupported_project":
					exit = 3
				case "invalid_arguments":
					exit = 2
				case "tool_failed":
					exit = 5
				case "timeout":
					exit = 124
				case "cancelled":
					exit = 130
				}
				return fail(detail.Code, detail.Message, exit)
			}
			return fail("operation_failed", "operation failed", 5)
		}
	}
	return a.emit(response, jsonMode, 0)
}

func (a App) emit(response Envelope, jsonMode bool, exit int) int {
	if jsonMode {
		if err := json.NewEncoder(a.Out).Encode(response); err != nil {
			return 5
		}
		return exit
	}
	switch data := response.Data.(type) {
	case map[string]string:
		if text, ok := data["help"]; ok {
			fmt.Fprint(a.Out, text)
		} else {
			fmt.Fprintln(a.Out, "AppRivet", data["version"])
		}
	case project.Info:
		fmt.Fprintf(a.Out, "Project: %s\nType: %s\n", data.Path, data.Kind)
		if data.Scheme != "" {
			fmt.Fprintln(a.Out, "Suggested scheme:", data.Scheme)
		}
		if response.Command == "init" {
			fmt.Fprintln(a.Out, "Next: on a Mac, run apprivet doctor, then apprivet build --project", data.Root, "--trust-project")
		}
	case doctor.Report:
		fmt.Fprintf(a.Out, "Host: %s/%s\n", data.HostOS, data.HostArch)
		for _, check := range data.Checks {
			status := "OK"
			if !check.OK {
				status = "MISSING"
			}
			fmt.Fprintf(a.Out, "%s %s: %s\n", status, check.Name, check.Detail)
			if check.Remedy != "" {
				fmt.Fprintln(a.Out, "  Next:", check.Remedy)
			}
		}
		fmt.Fprintf(a.Out, "Ready for iOS Simulator builds: %t\n", data.Ready)
	case build.Result:
		if data.Execution.Output != "" {
			fmt.Fprintln(a.Err, data.Execution.Output)
		}
		if data.Execution.Truncated {
			fmt.Fprintln(a.Err, "Tool output truncated at 256 KiB; inspect retained DerivedData for additional diagnostics.")
		}
		if data.DerivedData != "" {
			fmt.Fprintln(a.Out, "DerivedData (retained; remove when no longer needed):", data.DerivedData)
		}
		if response.OK {
			fmt.Fprintln(a.Out, data.Action+" completed for iOS Simulator")
		}
	}
	if response.Error != nil {
		fmt.Fprintf(a.Err, "%s: %s\n", response.Error.Code, response.Error.Message)
	}
	return exit
}
