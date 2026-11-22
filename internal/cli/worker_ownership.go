package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"github.com/fpokrzywa/AppRivet/internal/admission"
)

func (a App) runWorkerOwnership(ctx context.Context, args []string, jsonMode bool) int {
	response := Envelope{SchemaVersion: 1, Command: "worker", OK: true}
	fail := func(code, message string, exit int) int {
		response.OK = false
		response.Data = nil
		response.Error = &Error{code, message}
		return a.emit(response, jsonMode, exit)
	}
	badArgs := func() int {
		return fail("invalid_arguments", "use worker claim/check-owner/recover/recovery-status --help; no execution command is available", 2)
	}
	command := args[0]
	fs := flag.NewFlagSet("worker "+command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var state, principal, policyPath, archive string
	var owner admission.Ownership
	var stopped bool
	fs.StringVar(&state, "state", "", "initialized reservation store")
	fs.StringVar(&principal, "principal", "", "operator-supplied principal; not authentication")
	if command != "recovery-status" {
		fs.StringVar(&owner.ReservationID, "reservation", "", "reservation ID")
		fs.StringVar(&owner.WorkerID, "worker", "", "operator-supplied worker ID; not authentication")
	}
	if command == "claim" || command == "check-owner" {
		fs.StringVar(&policyPath, "policy", "", "trusted local policy file")
		fs.StringVar(&archive, "archive", "", "snapshot archive")
	}
	if command == "check-owner" || command == "recover" {
		fs.StringVar(&owner.ClaimID, "claim", "", "exact claim ID")
		fs.IntVar(&owner.Fence, "fence", 0, "exact claim sequence")
	}
	if command == "recover" {
		fs.BoolVar(&stopped, "confirmed-stopped", false, "operator asserts the worker is stopped; no termination is verified")
	}
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			response.Data = map[string]string{"help": help}
			return a.emit(response, jsonMode, 0)
		}
		return badArgs()
	}
	if fs.NArg() != 0 || state == "" || principal == "" || (command != "recovery-status" && (owner.ReservationID == "" || owner.WorkerID == "")) || ((command == "claim" || command == "check-owner") && (policyPath == "" || archive == "")) || ((command == "check-owner" || command == "recover") && (owner.ClaimID == "" || owner.Fence <= 0)) {
		return badArgs()
	}
	resolve := func(p string) string {
		if filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(a.Dir, p)
	}
	store := admission.Store{Dir: resolve(state)}
	var err error
	switch command {
	case "claim":
		response.Data, err = store.Claim(ctx, principal, owner.ReservationID, owner.WorkerID, resolve(policyPath), resolve(archive))
	case "check-owner":
		response.Data, err = store.CheckOwner(ctx, principal, owner, resolve(policyPath), resolve(archive))
	case "recover":
		response.Data, err = store.Recover(ctx, principal, owner, stopped)
	case "recovery-status":
		response.Data, err = store.RecoveryStatus(ctx, principal)
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
		return fail("admission_failed", "ownership/recovery check failed; no result accepted", 4)
	}
	return a.emit(response, jsonMode, 0)
}

func writeReservations(out io.Writer, jobs []admission.Reservation) {
	for _, r := range jobs {
		fmt.Fprintf(out, "%s: %s; request: %s; expires: %s; bytes: %d\n", r.ID, r.Status, r.RequestID, r.ExpiresAt, r.ReservedBytes)
		if r.ClaimID != "" {
			fmt.Fprintf(out, "Worker: %s; claim: %s; fence: %d\n", r.WorkerID, r.ClaimID, r.Fence)
		}
	}
}
