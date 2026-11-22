package cli

import (
	"context"
	"errors"
	"flag"
	"io"
	"path/filepath"
	"time"

	"github.com/fpokrzywa/AppRivet/internal/admission"
)

func (a App) runWorker(ctx context.Context, args []string, jsonMode bool) int {
	response := Envelope{SchemaVersion: 1, Command: "worker", OK: true}
	failure := func(code, message string, exit int) int {
		response.OK = false
		response.Data = nil
		response.Error = &Error{code, message}
		return a.emit(response, jsonMode, exit)
	}
	badArgs := func() int {
		return failure("invalid_arguments", "use worker init/check/reserve/status/release --help; no execution command is available", 2)
	}
	helpResult := func() int { response.Data = map[string]string{"help": help}; return a.emit(response, jsonMode, 0) }
	if len(args) == 0 {
		return badArgs()
	}
	if args[0] == "--help" || args[0] == "-h" {
		return helpResult()
	}
	command := args[0]
	if command == "claim" || command == "check-owner" || command == "recover" || command == "recovery-status" {
		return a.runWorkerOwnership(ctx, args, jsonMode)
	}
	fs := flag.NewFlagSet("worker "+command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var policyPath, requestPath, principal, archive, state, configPath, requestID, reservationID string
	switch command {
	case "check", "reserve":
		fs.StringVar(&policyPath, "policy", "", "trusted local policy file")
		fs.StringVar(&requestPath, "request", "", "untrusted request file")
		fs.StringVar(&principal, "principal", "", "hypothetical principal for offline evaluation; not authentication")
		fs.StringVar(&archive, "archive", "", "snapshot archive")
	case "init":
		fs.StringVar(&configPath, "config", "", "trusted local limits file")
	case "status", "release":
		fs.StringVar(&principal, "principal", "", "operator-supplied principal; not authentication")
		if command == "status" {
			fs.StringVar(&requestID, "request-id", "", "optional request ID for recovery")
		} else {
			fs.StringVar(&reservationID, "reservation", "", "reservation ID to release")
		}
	default:
		return badArgs()
	}
	if command != "check" {
		fs.StringVar(&state, "state", "", "initialized local reservation store")
	}
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return helpResult()
		}
		return badArgs()
	}
	if fs.NArg() != 0 || (command != "check" && state == "") || ((command == "check" || command == "reserve") && (policyPath == "" || requestPath == "" || principal == "" || archive == "")) || (command == "init" && configPath == "") || ((command == "status" || command == "release") && principal == "") || (command == "release" && reservationID == "") {
		return badArgs()
	}
	if ctx.Err() != nil {
		return failure("cancelled", "admission check cancelled", 130)
	}
	resolve := func(p string) string {
		if filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(a.Dir, p)
	}
	var err error
	store := admission.Store{Dir: resolve(state)}
	switch command {
	case "init":
		var config admission.Config
		config, err = admission.LoadConfig(resolve(configPath))
		if err == nil {
			err = store.Init(ctx, config)
		}
		response.Data = admission.Receipt{Mode: "local_reservation", Reservations: []admission.Reservation{}}
	case "status":
		response.Data, err = store.Status(ctx, principal, requestID)
	case "release":
		response.Data, err = store.Release(ctx, principal, reservationID)
	case "check", "reserve":
		var r admission.Request
		r, err = admission.LoadRequest(resolve(requestPath))
		if err == nil {
			if command == "reserve" {
				response.Data, err = store.Reserve(ctx, principal, resolve(policyPath), r, resolve(archive))
			} else {
				var p admission.Policy
				p, err = admission.LoadPolicy(resolve(policyPath))
				if err == nil {
					response.Data, err = admission.Check(ctx, time.Now(), principal, p, r, resolve(archive))
				}
			}
		}
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
			return failure(detail.Code, detail.Message, exit)
		}
		return failure("admission_failed", "admission check failed; no decision accepted", 4)
	}
	return a.emit(response, jsonMode, 0)
}
