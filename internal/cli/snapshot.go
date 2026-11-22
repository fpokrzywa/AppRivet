package cli

import (
	"context"
	"errors"
	"flag"
	"io"
	"path/filepath"

	"github.com/fpokrzywa/AppRivet/internal/snapshot"
)

func (a App) runSnapshot(ctx context.Context, args []string, jsonMode bool) int {
	response := Envelope{SchemaVersion: 1, Command: "snapshot", OK: true}
	failure := func(code, message string, exit int) int {
		response.OK = false
		response.Data = nil
		response.Error = &Error{code, message}
		return a.emit(response, jsonMode, exit)
	}
	badArgs := func() int {
		return failure("invalid_arguments", "use snapshot create, inspect or extract with their documented required flags; run snapshot --help", 2)
	}
	showHelp := func() int { response.Data = map[string]string{"help": help}; return a.emit(response, jsonMode, 0) }
	if len(args) == 0 {
		return badArgs()
	}
	if args[0] == "--help" || args[0] == "-h" {
		return showHelp()
	}
	action := args[0]
	if action != "create" && action != "inspect" && action != "extract" {
		return badArgs()
	}
	fs := flag.NewFlagSet("snapshot "+action, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var root, list, output, archive, expected, destination string
	if action == "create" {
		fs.StringVar(&root, "root", ".", "source root")
		fs.StringVar(&list, "files-from", "", "newline list of relative files")
		fs.StringVar(&output, "output", "", "new archive path; never overwritten")
	} else {
		fs.StringVar(&archive, "archive", "", "archive to inspect without extraction")
		fs.StringVar(&expected, "expect-sha256", "", "separately obtained archive digest")
		if action == "extract" {
			fs.StringVar(&destination, "destination", "", "new extraction directory; never merged or overwritten")
		}
	}
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return showHelp()
		}
		return badArgs()
	}
	emptyDigest := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "expect-sha256" && expected == "" {
			emptyDigest = true
		}
	})
	if fs.NArg() != 0 || emptyDigest {
		return badArgs()
	}
	resolve := func(p string) string {
		if filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(a.Dir, p)
	}
	var data any
	var err error
	if action == "create" {
		if root == "" || list == "" || output == "" {
			return badArgs()
		}
		var files []string
		files, err = snapshot.ReadSelection(resolve(list))
		if err == nil {
			data, err = snapshot.Create(ctx, resolve(root), files, resolve(output))
		}
	} else {
		if archive == "" {
			return badArgs()
		}
		if action == "extract" {
			if destination == "" || expected == "" {
				return badArgs()
			}
			data, err = snapshot.Extract(ctx, resolve(archive), resolve(destination), expected)
		} else {
			data, err = snapshot.Inspect(ctx, resolve(archive), expected)
		}
	}
	if err != nil {
		var detail *snapshot.Error
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
		return failure("snapshot_failed", "snapshot operation failed; no result accepted", 4)
	}
	response.Data = data
	return a.emit(response, jsonMode, 0)
}
