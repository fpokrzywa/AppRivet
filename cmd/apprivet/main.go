package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/fpokrzywa/AppRivet/internal/cli"
	"github.com/fpokrzywa/AppRivet/internal/runner"
)

var version = "0.1.0-dev"

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot read current working directory")
		os.Exit(4)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	app := cli.App{OS: runtime.GOOS, Arch: runtime.GOARCH, Dir: cwd, Version: version, Runner: runner.Exec{}, In: os.Stdin, Out: os.Stdout, Err: os.Stderr}
	code := app.Run(ctx, os.Args[1:])
	stop()
	os.Exit(code)
}
