package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/fpokrzywa/AppRivet/internal/credentials"
	"github.com/fpokrzywa/AppRivet/internal/generation"
	"github.com/fpokrzywa/AppRivet/internal/studio"
)

var version string // Set from the desktop release version when packaging.

func main() {
	if err := run(); err != nil {
		reportError(err.Error())
		os.Exit(1)
	}
}

func run() error {
	openBrowser := true
	if len(os.Args) == 2 && os.Args[1] == "--serve-only" {
		openBrowser = false // Integration tests exercise the same HTTP service.
	} else if len(os.Args) != 1 {
		return errors.New("Open AppRivet without command-line arguments.")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("Find your home folder: %w", err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("Open the local AppRivet window: %w", err)
	}
	defer listener.Close()
	host, session := listener.Addr().String(), rand.Text()
	address := "http://" + host + "/#" + session
	quit := make(chan struct{})
	var once sync.Once
	server := &http.Server{
		Handler: studio.Handler(studio.Config{
			Host: host, Session: session, Workspace: filepath.Join(home, "AppRivet Projects"), Version: version,
			OpenFolder: func(path string) error { return open(path, false) },
			Quit:       func() { once.Do(func() { close(quit) }) },
			Generate:   generation.Generate, GenerateBudgeted: generation.GenerateBudgeted,
			Vault: credentials.NewVault(),
		}),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 195 * time.Second, IdleTimeout: 30 * time.Second,
	}
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(listener) }()
	defer server.Close()
	if openBrowser {
		if err := open(address, true); err != nil {
			return fmt.Errorf("Open your default browser: %w", err)
		}
	} else {
		fmt.Println(address)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	select {
	case <-quit:
	case <-ctx.Done():
	case err := <-finished:
		return err
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return server.Shutdown(shutdown)
}

func open(target string, browser bool) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		if browser {
			command = exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "rundll32.exe"), "url.dll,FileProtocolHandler", target)
		} else {
			command = exec.Command(filepath.Join(os.Getenv("SystemRoot"), "explorer.exe"), target)
		}
	case "darwin":
		command = exec.Command("/usr/bin/open", target)
	case "linux":
		command = exec.Command("xdg-open", target)
	default:
		return errors.New("This operating system does not have a graphical launcher.")
	}
	if err := command.Start(); err != nil {
		return err
	}
	// Reap desktop opener processes without blocking the HTTP request.
	go func() { _ = command.Wait() }()
	return nil
}
