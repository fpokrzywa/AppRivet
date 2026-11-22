package devices

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/fpokrzywa/AppRivet/internal/runner"
)

const DefaultTimeout = 30 * time.Second
const MaxTimeout = 5 * time.Minute

type Request struct {
	Simulators bool
	FromJSON   string
	Timeout    time.Duration
}

// List runs a single read-only Apple tool or parses an explicitly selected
// offline file. No source code, account API, pairing or device mutation runs.
func List(ctx context.Context, host string, r runner.Runner, req Request) (Inventory, error) {
	if req.Timeout <= 0 || req.Timeout > MaxTimeout {
		return Inventory{}, failure("invalid_arguments", "discovery timeout must be greater than zero and at most 5m")
	}
	if ctx.Err() != nil {
		return Inventory{}, contextFailure(ctx.Err())
	}
	if req.FromJSON != "" {
		body, err := readReport(req.FromJSON)
		if err != nil {
			return Inventory{}, err
		}
		inv, err := Parse(body, req.Simulators)
		if err != nil {
			return Inventory{}, err
		}
		inv.Source = "file"
		return inv, nil
	}
	if host != "darwin" {
		return Inventory{}, failure("unsupported_platform", "live device discovery requires a Mac with full Xcode; use --from-json FILE to inspect a saved report offline. Remote discovery is not implemented.")
	}
	var reportPath string
	args := []string{"simctl", "list", "devices", "--json"}
	if !req.Simulators {
		dir, err := os.MkdirTemp("", "apprivet-devices-")
		if err != nil {
			return Inventory{}, failure("report_io_error", "could not create private discovery workspace")
		}
		defer os.RemoveAll(dir)
		reportPath = filepath.Join(dir, "devices.json")
		f, err := os.OpenFile(reportPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return Inventory{}, failure("report_io_error", "could not create private discovery report")
		}
		if err := f.Close(); err != nil {
			return Inventory{}, failure("report_io_error", "could not close private discovery report")
		}
		args = []string{"devicectl", "list", "devices", "--json-output", reportPath}
	}
	result, err := r.Run(ctx, runner.Spec{Path: "/usr/bin/xcrun", Args: args, Timeout: req.Timeout, SeparateStdout: req.Simulators})
	if result.TimedOut || errors.Is(err, context.DeadlineExceeded) {
		return Inventory{}, contextFailure(context.DeadlineExceeded)
	}
	if result.Cancelled || errors.Is(err, context.Canceled) || ctx.Err() != nil {
		if ctx.Err() != nil {
			return Inventory{}, contextFailure(ctx.Err())
		}
		return Inventory{}, contextFailure(context.Canceled)
	}
	if err != nil || result.ExitCode != 0 {
		return Inventory{}, failure("tool_failed", fmt.Sprintf("xcrun %s list devices failed (exit %d). Install/select full Xcode, complete its first-launch setup and run apprivet doctor. No device inventory was accepted.", args[0], result.ExitCode))
	}
	var body []byte
	if req.Simulators {
		if result.StdoutTruncated {
			return Inventory{}, failure("report_too_large", "simctl JSON exceeds the 256 KiB live capture limit; no partial inventory was accepted")
		}
		body = []byte(result.Stdout)
	} else {
		body, err = readReport(reportPath)
		if err != nil {
			return Inventory{}, err
		}
	}
	inv, err := Parse(body, req.Simulators)
	if err != nil {
		return Inventory{}, err
	}
	inv.Source = "local"
	inv.Live = true
	return inv, nil
}

func contextFailure(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return failure("timeout", "device discovery exceeded its deadline; no inventory was accepted")
	}
	return failure("cancelled", "device discovery cancelled; no inventory was accepted")
}

func readReport(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, failure("report_read_failed", "device report must be an existing regular file, not a symlink or directory")
	}
	if info.Size() > MaxReportBytes {
		return nil, failure("report_too_large", "device report exceeds the 4 MiB limit")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, failure("report_read_failed", "device report could not be opened")
	}
	defer f.Close()
	current, err := f.Stat()
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(info, current) {
		return nil, failure("report_read_failed", "device report changed while opening it")
	}
	body, err := io.ReadAll(io.LimitReader(f, MaxReportBytes+1))
	if err != nil {
		return nil, failure("report_read_failed", "device report could not be read")
	}
	if len(body) > MaxReportBytes {
		return nil, failure("report_too_large", "device report exceeds the 4 MiB limit")
	}
	return body, nil
}
