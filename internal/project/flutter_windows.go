package project

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

var flutterArgument = regexp.MustCompile("^[A-Za-z0-9._=,-]+$")

func flutterCommand(ctx context.Context, executable string, args []string) (*exec.Cmd, error) {
	// Flutter ships a batch entry point on Windows. The working directory is
	// set separately, so user project paths never enter the command string.
	if strings.ContainsAny(executable, "%!\"\r\n") {
		return nil, fmt.Errorf("choose a Flutter SDK directory without percent signs or exclamation marks")
	}
	for _, arg := range args {
		if !flutterArgument.MatchString(arg) {
			return nil, fmt.Errorf("invalid Flutter tool argument")
		}
	}
	shell := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	if !filepath.IsAbs(shell) {
		return nil, fmt.Errorf("cannot locate Windows cmd.exe for the Flutter SDK")
	}
	cmd := exec.CommandContext(ctx, shell)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
		CmdLine:    "\"" + shell + "\" /d /v:off /s /c \"\"" + executable + "\" " + strings.Join(args, " ") + "\"",
	}
	return cmd, nil
}
