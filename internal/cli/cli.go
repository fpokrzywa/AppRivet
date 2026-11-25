package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/fpokrzywa/AppRivet/internal/admission"
	"github.com/fpokrzywa/AppRivet/internal/build"
	"github.com/fpokrzywa/AppRivet/internal/devices"
	"github.com/fpokrzywa/AppRivet/internal/doctor"
	"github.com/fpokrzywa/AppRivet/internal/generation"
	"github.com/fpokrzywa/AppRivet/internal/project"
	"github.com/fpokrzywa/AppRivet/internal/runner"
	"github.com/fpokrzywa/AppRivet/internal/snapshot"
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

const help = `AppRivet - independent iOS development toolkit

Usage:
  apprivet doctor [--json]
  apprivet init NAME [--bundle-id com.example.app] [--brief TEXT] [--starter TYPE] [--review] [--json]
  apprivet init NAME --brief TEXT --provider PROVIDER --model MODEL (--review | --approve HASH) [--json]
  apprivet inspect [--project PATH] [--json]
  apprivet build [--project PATH] [--scheme NAME] --trust-project [--json]
  apprivet test [--project PATH] [--scheme NAME] --simulator UUID --trust-project [--json]
  apprivet devices [--simulators] [--timeout 30s] [--json]
  apprivet devices --from-json FILE [--simulators] [--json]
  apprivet snapshot create --root DIR --files-from LIST --output FILE [--json]
  apprivet snapshot inspect --archive FILE [--expect-sha256 HEX] [--json]
  apprivet snapshot extract --archive FILE --destination DIR --expect-sha256 HEX [--json]
  apprivet worker check --policy FILE --request FILE --principal ID --archive FILE [--json]
  apprivet worker init --state DIR --config FILE [--json]
  apprivet worker reserve --state DIR --policy FILE --request FILE --principal ID --archive FILE [--json]
  apprivet worker status --state DIR --principal ID [--request-id ID] [--json]
  apprivet worker release --state DIR --principal ID --reservation ID [--json]
  apprivet worker claim --state DIR --principal ID --reservation ID --worker ID --policy FILE --archive FILE [--json]
  apprivet worker check-owner --state DIR --principal ID --reservation ID --worker ID --claim ID --fence N --policy FILE --archive FILE [--json]
  apprivet worker recovery-status --state DIR --principal ID [--json]
  apprivet worker recover --state DIR --principal ID --reservation ID --worker ID --claim ID --fence N --confirmed-stopped [--json]
  apprivet worker migrate --state DIR [--json]
  apprivet worker compact --state DIR [--json]
  apprivet worker repair-status --state DIR [--json]
  apprivet worker append-repair-status --state DIR [--json]
  apprivet worker append-repair --state DIR --expect-intent-sha256 HASH --expect-active-sha256 HASH [--json]
  apprivet worker repair --state DIR --expect-intent-sha256 HASH --expect-control-sha256 HASH --expect-history-sha256 HASH --expect-sequence N [--json]
  apprivet version [--json]

Build/test options:
  --timeout DURATION   Maximum execution time (default 15m, maximum 24h)
  --simulator UUID     Select a simulator; required for test
  --trust-project      Acknowledge project scripts execute as your OS user

Device discovery is read-only. --simulators selects simctl; otherwise devicectl.
--from-json inspects an offline report on any host, never a live device query.
Discovery timeout defaults to 30s (maximum 5m). Listing is not authorization.
Snapshots package explicitly listed files and verify integrity on any host.
Extraction requires a matching digest and a new destination; it executes no code.
Worker checks evaluate local policy offline; they do not authenticate or authorize execution.
Reservations retain replay history and account for capacity; they launch no jobs.
Expired claims retain capacity until exact-owner recovery. Recovery verifies no process termination.
Migration upgrades the journal in place; compaction preserves all replay and ownership history.
Repair requires a durable pending intent and exact inspection hashes; older unanchored tears remain blocked.

Use --help with a command. --json is supported before or after the command.
Windows/Linux: project preparation and offline report inspection. iOS builds and
live discovery require a Mac with full Xcode. Remote execution, signing, install, launch, and
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
	if command == "snapshot" {
		return a.runSnapshot(ctx, filtered, jsonMode)
	}
	if command == "worker" {
		return a.runWorker(ctx, filtered, jsonMode)
	}
	if command == "install" || command == "launch" {
		return fail("not_implemented", command+" is planned after Milestone 1; use Xcode's official device workflow", 3)
	}
	if command != "doctor" && command != "init" && command != "inspect" && command != "build" && command != "test" && command != "version" && command != "devices" {
		return fail("unknown_command", "unknown command; run apprivet help", 2)
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	projectPath, scheme, bundle, simulator := ".", "", "", ""
	var starterOptions project.StarterOptions
	var provider, model, approval string
	review := false
	trust := false
	simulators := false
	fromJSON := ""
	timeout := 15 * time.Minute
	switch command {
	case "devices":
		fs.BoolVar(&simulators, "simulators", false, "list simulator inventory using simctl")
		fs.StringVar(&fromJSON, "from-json", "", "inspect an offline tool JSON report")
		fs.DurationVar(&timeout, "timeout", devices.DefaultTimeout, "discovery deadline, maximum 5m")
	case "init":
		fs.StringVar(&bundle, "bundle-id", "", "reverse-DNS bundle identifier")
		fs.StringVar(&starterOptions.Brief, "brief", "", "app idea to save with the project")
		fs.StringVar(&starterOptions.Starter, "starter", "", "auto, blank, checklist, notes or counter")
		fs.BoolVar(&review, "review", false, "validate inputs and preview the prompt without creating a project")
		fs.StringVar(&provider, "provider", "", "OpenAI or Anthropic API provider ID")
		fs.StringVar(&model, "model", "", "provider API model ID")
		fs.StringVar(&approval, "approve", "", "approval hash from the current AI prompt review")
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
	emptyReportPath := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "from-json" && fromJSON == "" {
			emptyReportPath = true
		}
	})
	if emptyReportPath {
		return fail("invalid_arguments", "--from-json requires a nonempty report path", 2)
	}
	if timeout <= 0 || timeout > 24*time.Hour {
		return fail("invalid_arguments", "timeout must be greater than zero and at most 24h", 2)
	}
	if !filepath.IsAbs(projectPath) {
		projectPath = filepath.Join(a.Dir, projectPath)
	}
	switch command {
	case "devices":
		if fromJSON != "" && !filepath.IsAbs(fromJSON) {
			fromJSON = filepath.Join(a.Dir, fromJSON)
		}
		inventory, err := devices.List(ctx, a.OS, a.Runner, devices.Request{Simulators: simulators, FromJSON: fromJSON, Timeout: timeout})
		if err != nil {
			var detail *devices.Error
			if errors.As(err, &detail) {
				exit := 4
				switch detail.Code {
				case "invalid_arguments":
					exit = 2
				case "unsupported_platform", "unsupported_schema":
					exit = 3
				case "tool_failed", "discovery_failed":
					exit = 5
				case "timeout":
					exit = 124
				case "cancelled":
					exit = 130
				}
				return fail(detail.Code, detail.Message, exit)
			}
			return fail("discovery_failed", "device discovery failed; no inventory accepted", 5)
		}
		response.Data = inventory
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
		if provider != "" {
			draft, err := generation.Review(name, bundle, starterOptions.Brief, provider, model)
			if err != nil {
				return fail("invalid_arguments", err.Error(), 2)
			}
			if review {
				response.Data = draft
				break
			}
			info, err := generation.Create(ctx, a.Dir, draft, approval, os.Getenv("APPRIVET_API_KEY"), generation.Generate)
			if err != nil {
				return fail("generation_failed", err.Error(), 4)
			}
			response.Data = info
			break
		}
		if model != "" || approval != "" {
			return fail("invalid_arguments", "select a provider when using an AI model or approval", 2)
		}
		if review {
			draft, err := project.Review(name, bundle, starterOptions)
			if err != nil {
				return fail("invalid_arguments", err.Error(), 2)
			}
			response.Data = draft
			break
		}
		info, err := project.CreateWithOptions(a.Dir, name, bundle, starterOptions)
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
	case admission.AppendRepairReport:
		fmt.Fprintf(a.Out, "Append repair: %s\nEvent: %s\nSequence: %d\nEvent bytes present: %d/%d\nBytes added: %d\nIdentical bytes rewritten: %d\n", data.Mode, data.EventKind, data.JournalSequence, data.PresentBytes, data.EventBytes, data.BytesWritten, data.BytesReasserted)
		fmt.Fprintf(a.Out, "Intent SHA-256: %s\nActive SHA-256: %s\nHistory SHA-256: %s\nClock rollback: %t\n", data.IntentSHA256, data.ActiveSHA256, data.JournalSHA256, data.ClockRollback)
		fmt.Fprintln(a.Out, "Caller not authenticated. Execution not authorized.")
	case admission.RepairReport:
		fmt.Fprintf(a.Out, "Manifest repair: %s; changed=%t; control=%s; sequence=%d; retained reservations=%d\n", data.Mode, data.Changed, data.ControlState, data.JournalSequence, data.RetainedReservations)
		fmt.Fprintf(a.Out, "Intent SHA-256: %s\nControl SHA-256: %s\nHistory SHA-256: %s\nClock rollback: %t\n", data.IntentSHA256, data.ControlSHA256, data.JournalSHA256, data.ClockRollback)
		fmt.Fprintln(a.Out, "Caller not authenticated; execution not authorized. All replay and ownership history retained.")
		if data.BeforeImage != "" {
			fmt.Fprintf(a.Out, "Saved control before-image: %s\n", data.BeforeImage)
		}
	case admission.MaintenanceReport:
		fmt.Fprintf(a.Out, "Journal %s: changed=%t; format=%d; generation=%d; sequence=%d; history retained for %d reservations.\n", data.Action, data.Changed, data.Format, data.Generation, data.JournalSequence, data.RetainedReservations)
		fmt.Fprintln(a.Out, "Caller not authenticated; execution not authorized. No history discarded or second store created.")
		if data.CleanupPending {
			fmt.Fprintln(a.Out, "Redundant-file cleanup or directory sync is pending; retain the store for inspection.")
		}
	case admission.Receipt:
		fmt.Fprintln(a.Out, "Local reservation accounting; caller not authenticated, execution not authorized.")
		writeReservations(a.Out, data.Reservations)
	case admission.RecoveryReceipt:
		fmt.Fprintln(a.Out, "Local recovery accounting; caller not authenticated, execution not authorized. Stopped-worker assertion not verified; no process terminated.")
		writeReservations(a.Out, data.Reservations)
	case admission.RecoveryReport:
		fmt.Fprintln(a.Out, "Read-only recovery inspection; caller not authenticated, execution not authorized.")
		fmt.Fprintf(a.Out, "Journal sequence: %d; recorded: %s; observed: %s; clock rollback: %t\n", data.JournalSequence, data.RecordedAt, data.ObservedAt, data.ClockRollback)
		writeReservations(a.Out, data.Reservations)
	case admission.OwnershipDecision:
		fmt.Fprintf(a.Out, "Local ownership matches at %s; valid until %s. Caller not authenticated, execution not authorized.\n", data.CheckedAt, data.ValidUntil)
		writeReservations(a.Out, []admission.Reservation{data.Reservation})
	case snapshot.Extraction:
		fmt.Fprintf(a.Out, "Extracted source: %s\nSHA-256: %s\nFiles: %d\n", data.Destination, data.Snapshot.SHA256, len(data.Snapshot.Files))
		fmt.Fprintln(a.Out, "No code executed. Integrity is not authorization or proof of safe/buildable source.")
	case admission.Decision:
		fmt.Fprintf(a.Out, "Offline policy check: eligible for %s\nPrincipal: %s; project: %s; grant: %s\nChecked at: %s; grant expires: %s\nSHA-256: %s\n", data.Operation, data.Principal, data.ProjectID, data.GrantID, data.CheckedAt, data.ValidUntil, data.Snapshot.SHA256)
		fmt.Fprintln(a.Out, "Caller not authenticated; execution not authorized. No extraction or job submission performed.")
	case snapshot.Report:
		fmt.Fprintf(a.Out, "Source snapshot: %s\nSHA-256: %s\nFiles: %d; source bytes: %d; archive bytes: %d\n", data.Format, data.SHA256, len(data.Files), data.TotalBytes, data.ArchiveBytes)
		fmt.Fprintln(a.Out, "Integrity checked; not authorization or proof of safe/buildable source. No extraction or execution performed.")
		table := tabwriter.NewWriter(a.Out, 0, 4, 2, ' ', 0)
		fmt.Fprintln(table, "PATH\tBYTES\tSHA-256")
		for _, entry := range data.Files {
			fmt.Fprintf(table, "%s\t%d\t%s\n", entry.Path, entry.Size, entry.SHA256)
		}
		_ = table.Flush()
	case devices.Inventory:
		if data.Live {
			fmt.Fprintf(a.Out, "Local %s inventory\n", data.Tool)
		} else {
			fmt.Fprintf(a.Out, "Offline report (%s); not a live device query\n", data.Tool)
		}
		fmt.Fprintln(a.Out, "Listing does not establish physical attachment, pairing permission or installation authorization.")
		if len(data.Devices) == 0 {
			fmt.Fprintln(a.Out, "No devices reported.")
			break
		}
		table := tabwriter.NewWriter(a.Out, 0, 4, 2, ' ', 0)
		fmt.Fprintln(table, "KIND\tID\tNAME\tCONNECTION\tBOOT\tPAIRING\tAVAILABLE")
		for _, d := range data.Devices {
			available := "unknown"
			if d.Available != nil {
				available = fmt.Sprint(*d.Available)
			}
			fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", d.Kind, d.ID, d.Name, d.ConnectionState, d.BootState, d.PairingState, available)
		}
		_ = table.Flush()
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
	case project.Draft:
		fmt.Fprintf(a.Out, "Local starter: %s\n\n%s\n", data.Starter, data.Prompt)
	case generation.Draft:
		fmt.Fprintf(a.Out, "Destination: %s (%s)\n\n%s\n\nApproval: %s\n", data.Provider, data.Model, data.Prompt, data.Approval)
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
