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
		return failure("invalid_arguments", "use worker check --policy FILE --request FILE --principal ID --archive FILE; no execution command is available", 2)
	}
	helpResult := func() int { response.Data = map[string]string{"help": help}; return a.emit(response, jsonMode, 0) }
	if len(args) == 0 {
		return badArgs()
	}
	if args[0] == "--help" || args[0] == "-h" {
		return helpResult()
	}
	if args[0] != "check" {
		return badArgs()
	}
	fs := flag.NewFlagSet("worker check", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var policyPath, requestPath, principal, archive string
	fs.StringVar(&policyPath, "policy", "", "trusted local policy file")
	fs.StringVar(&requestPath, "request", "", "untrusted request file")
	fs.StringVar(&principal, "principal", "", "hypothetical principal for offline evaluation; not authentication")
	fs.StringVar(&archive, "archive", "", "snapshot archive")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return helpResult()
		}
		return badArgs()
	}
	if fs.NArg() != 0 || policyPath == "" || requestPath == "" || principal == "" || archive == "" {
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
	p, err := admission.LoadPolicy(resolve(policyPath))
	var r admission.Request
	if err == nil {
		r, err = admission.LoadRequest(resolve(requestPath))
	}
	if err == nil {
		response.Data, err = admission.Check(ctx, time.Now(), principal, p, r, resolve(archive))
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
