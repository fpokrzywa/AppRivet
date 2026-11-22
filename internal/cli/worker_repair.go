package cli

import (
	"context"
	"errors"
	"flag"
	"io"
	"path/filepath"

	"github.com/fpokrzywa/AppRivet/internal/admission"
)

func (a App) runWorkerRepair(ctx context.Context, args []string, jsonMode bool) int {
	response := Envelope{SchemaVersion: 1, Command: "worker", OK: true}
	fail := func(code, message string, exit int) int {
		response.OK = false
		response.Data = nil
		response.Error = &Error{code, message}
		return a.emit(response, jsonMode, exit)
	}
	badArgs := func() int {
		return fail("invalid_arguments", "use worker --help for manifest and append repair arguments", 2)
	}
	command := args[0]
	fs := flag.NewFlagSet("worker "+command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var state string
	var expect admission.RepairExpect
	var appendExpect admission.AppendRepairExpect
	fs.StringVar(&state, "state", "", "initialized reservation store")
	if command == "repair" {
		fs.StringVar(&expect.IntentSHA256, "expect-intent-sha256", "", "publication intent hash from repair-status")
		fs.StringVar(&expect.ControlSHA256, "expect-control-sha256", "", "control byte hash from repair-status")
		fs.StringVar(&expect.HistorySHA256, "expect-history-sha256", "", "complete history head from repair-status")
		fs.IntVar(&expect.Sequence, "expect-sequence", 0, "complete history sequence from repair-status")
	}
	if command == "append-repair" {
		fs.StringVar(&appendExpect.IntentSHA256, "expect-intent-sha256", "", "append intent hash from append-repair-status")
		fs.StringVar(&appendExpect.ActiveSHA256, "expect-active-sha256", "", "active file hash from append-repair-status")
	}
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			response.Data = map[string]string{"help": help}
			return a.emit(response, jsonMode, 0)
		}
		return badArgs()
	}
	if fs.NArg() != 0 || state == "" {
		return badArgs()
	}
	if !filepath.IsAbs(state) {
		state = filepath.Join(a.Dir, state)
	}
	s := admission.Store{Dir: state}
	var err error
	switch command {
	case "repair-status":
		response.Data, err = s.InspectRepair(ctx)
	case "repair":
		response.Data, err = s.Repair(ctx, expect)
	case "append-repair-status":
		response.Data, err = s.InspectAppendRepair(ctx)
	case "append-repair":
		response.Data, err = s.RepairAppend(ctx, appendExpect)
	}
	if err != nil {
		var detail *admission.Error
		if errors.As(err, &detail) {
			exit := 4
			if detail.Code == "invalid_arguments" {
				exit = 2
			}
			if detail.Code == "cancelled" {
				exit = 130
			}
			return fail(detail.Code, detail.Message, exit)
		}
		return fail("admission_failed", "journal repair failed; retain all evidence", 4)
	}
	return a.emit(response, jsonMode, 0)
}
