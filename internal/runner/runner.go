// Package runner executes tools directly, with bounded diagnostics and deadlines.
// It is not a sandbox for project-controlled build scripts.
package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const MaxOutput = 256 * 1024

type Spec struct {
	Path    string
	Args    []string
	Dir     string
	Timeout time.Duration
}

type Result struct {
	Output    string `json:"output"`
	ExitCode  int    `json:"exit_code"`
	Truncated bool   `json:"truncated"`
	TimedOut  bool   `json:"timed_out"`
	Cancelled bool   `json:"cancelled"`
}

type Runner interface {
	Run(context.Context, Spec) (Result, error)
}
type Exec struct{}

func (Exec) Run(parent context.Context, spec Spec) (Result, error) {
	if !filepath.IsAbs(spec.Path) || spec.Timeout <= 0 {
		return Result{ExitCode: -1}, fmt.Errorf("tool requires an absolute executable path and positive timeout")
	}
	ctx, cancel := context.WithTimeout(parent, spec.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, spec.Path, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = childEnvironment()
	cmd.WaitDelay = 2 * time.Second
	cleanup := configureProcess(cmd)
	defer cleanup()
	output := &limitedBuffer{}
	cmd.Stdout = output
	cmd.Stderr = output
	err := cmd.Run()
	result := Result{Output: Redact(output.String()), Truncated: output.truncated, ExitCode: 0}
	if ctx.Err() != nil {
		result.ExitCode = -1
		result.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
		result.Cancelled = errors.Is(ctx.Err(), context.Canceled)
		return result, ctx.Err()
	}
	if err != nil {
		result.ExitCode = -1
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			result.ExitCode = exit.ExitCode()
		}
		return result, fmt.Errorf("tool failed (exit %d); verify installation and inspect redacted output", result.ExitCode)
	}
	return result, nil
}

func childEnvironment() []string {
	// Do not inherit tokens, injected libraries, arbitrary PATH entries, toolchain
	// overrides, or shell startup configuration from an AI/editor environment.
	env := []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "LANG=en_US.UTF-8"}
	for _, key := range []string{"HOME", "USER", "LOGNAME", "TMPDIR", "TMP", "TEMP", "SystemRoot", "WINDIR", "USERPROFILE"} {
		if v, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+v)
		}
	}
	return env
}

type limitedBuffer struct {
	mu        sync.Mutex
	body      []byte
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	remaining := MaxOutput - len(b.body)
	if len(p) > remaining {
		b.truncated = true
		p = p[:remaining]
	}
	b.body = append(b.body, p...)
	return n, nil
}
func (b *limitedBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return string(b.body) }

var pemPattern = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?(?:-----END [A-Z0-9 ]*PRIVATE KEY-----|$)`)
var bearerPattern = regexp.MustCompile(`(?i)(Bearer\s+)[A-Za-z0-9._~+/=-]+`)
var assignmentPattern = regexp.MustCompile(`(?i)((?:[A-Za-z0-9_]*token|[A-Za-z0-9_]*password|[A-Za-z0-9_]*secret|api[_-]?key|private[_-]?key)["']?\s*[:=]\s*)('[^'\r\n]*'?|"[^"\r\n]*"?|[^\s,;]+)`)

// Redact is defense in depth, not a guarantee that arbitrary build output is safe
// to publish. No credentials should be provided to the build worker at all.
func Redact(s string) string {
	s = pemPattern.ReplaceAllString(s, "[REDACTED PRIVATE KEY]")
	s = bearerPattern.ReplaceAllString(s, "${1}[REDACTED]")
	s = assignmentPattern.ReplaceAllString(s, "${1}[REDACTED]")
	// Strip terminal control characters, retaining ordinary line/tab formatting.
	return strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\t' && r != '\r' {
			return -1
		}
		if r == 127 {
			return -1
		}
		return r
	}, s)
}
