//go:build !windows

package project

import (
	"context"
	"os/exec"
)

func flutterCommand(ctx context.Context, executable string, args []string) (*exec.Cmd, error) {
	return exec.CommandContext(ctx, executable, args...), nil
}
