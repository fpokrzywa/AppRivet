package devices

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fpokrzywa/AppRivet/internal/runner"
)

type toolFunc func(context.Context, runner.Spec) (runner.Result, error)

func (f toolFunc) Run(c context.Context, s runner.Spec) (runner.Result, error) { return f(c, s) }
func request() Request                                                         { return Request{Timeout: 30 * time.Second} }
func forbid(t *testing.T) toolFunc {
	return func(context.Context, runner.Spec) (runner.Result, error) {
		t.Fatal("unexpected Apple process")
		return runner.Result{}, nil
	}
}
func errorCode(t *testing.T, err error, want string) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Code != want {
		t.Fatalf("want %s, got %v", want, err)
	}
}

func TestLiveCoreDeviceDiscoveryUsesPrivateFileAndCleansUp(t *testing.T) {
	var reportPath string
	r := toolFunc(func(_ context.Context, s runner.Spec) (runner.Result, error) {
		if s.Path != "/usr/bin/xcrun" || s.Timeout != 30*time.Second || s.Dir != "" || s.SeparateStdout {
			t.Fatalf("unsafe spec: %+v", s)
		}
		if len(s.Args) != 5 || !reflect.DeepEqual(s.Args[:4], []string{"devicectl", "list", "devices", "--json-output"}) {
			t.Fatalf("unexpected command %q", s.Args)
		}
		reportPath = s.Args[4]
		if !filepath.IsAbs(reportPath) || !strings.HasPrefix(filepath.Base(filepath.Dir(reportPath)), "apprivet-devices-") {
			t.Fatal("report not in private temp directory")
		}
		info, err := os.Lstat(reportPath)
		if err != nil || !info.Mode().IsRegular() {
			t.Fatalf("report not precreated: %v", err)
		}
		if err := os.WriteFile(reportPath, fixture(t, "devicectl-legacy.json"), 0600); err != nil {
			t.Fatal(err)
		}
		return runner.Result{Output: "UNTRUSTED RAW LOG"}, nil
	})
	inv, err := List(context.Background(), "darwin", r, request())
	if err != nil {
		t.Fatal(err)
	}
	if !inv.Live || inv.Source != "local" || len(inv.Devices) != 3 {
		t.Fatalf("bad live metadata: %+v", inv)
	}
	if _, err := os.Lstat(filepath.Dir(reportPath)); !os.IsNotExist(err) {
		t.Fatal("temporary metadata retained")
	}
}

func TestLiveSimctlUsesSeparateStdout(t *testing.T) {
	req := request()
	req.Simulators = true
	r := toolFunc(func(_ context.Context, s runner.Spec) (runner.Result, error) {
		if s.Path != "/usr/bin/xcrun" || !s.SeparateStdout || !reflect.DeepEqual(s.Args, []string{"simctl", "list", "devices", "--json"}) {
			t.Fatalf("wrong invocation: %+v", s)
		}
		return runner.Result{Stdout: string(fixture(t, "simctl.json")), Output: "stderr diagnostic not JSON"}, nil
	})
	inv, err := List(context.Background(), "darwin", r, req)
	if err != nil || !inv.Live || len(inv.Devices) != 3 {
		t.Fatalf("%+v %v", inv, err)
	}
}

func TestOfflineReportsNeverRunToolsOrClaimLiveDiscovery(t *testing.T) {
	for _, host := range []string{"windows", "linux", "darwin"} {
		path := filepath.Join(t.TempDir(), "devices.json")
		if err := os.WriteFile(path, fixture(t, "devicectl-modern.json"), 0600); err != nil {
			t.Fatal(err)
		}
		req := request()
		req.FromJSON = path
		inv, err := List(context.Background(), host, forbid(t), req)
		if err != nil || inv.Live || inv.Source != "file" {
			t.Fatalf("%+v %v", inv, err)
		}
	}
}

func TestDiscoveryRefusesNonMacAndInvalidTimeout(t *testing.T) {
	for _, host := range []string{"windows", "linux"} {
		_, err := List(context.Background(), host, forbid(t), request())
		errorCode(t, err, "unsupported_platform")
	}
	for _, duration := range []time.Duration{0, -time.Second, 6 * time.Minute} {
		req := request()
		req.Timeout = duration
		_, err := List(context.Background(), "darwin", forbid(t), req)
		errorCode(t, err, "invalid_arguments")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := List(ctx, "darwin", forbid(t), request())
	errorCode(t, err, "cancelled")
}

func TestToolFailuresNeverBecomeEmptySuccessAndAlwaysCleanFiles(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result runner.Result
		err    error
		code   string
	}{
		{"failed", runner.Result{ExitCode: 1}, errors.New("tool failed"), "tool_failed"},
		{"exit status", runner.Result{ExitCode: 1}, nil, "tool_failed"},
		{"timeout", runner.Result{TimedOut: true}, context.DeadlineExceeded, "timeout"},
		{"cancelled", runner.Result{Cancelled: true}, context.Canceled, "cancelled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var path string
			r := toolFunc(func(_ context.Context, s runner.Spec) (runner.Result, error) {
				path = s.Args[4]
				return tc.result, tc.err
			})
			inv, err := List(context.Background(), "darwin", r, request())
			errorCode(t, err, tc.code)
			if len(inv.Devices) != 0 {
				t.Fatal("partial failure exposed devices")
			}
			if _, err := os.Lstat(filepath.Dir(path)); !os.IsNotExist(err) {
				t.Fatal("failure left temp files")
			}
		})
	}
}

func TestBadToolReportsAreRejectedAndCleaned(t *testing.T) {
	for _, mode := range []string{"missing", "empty", "invalid", "oversize", "directory", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			var path string
			r := toolFunc(func(_ context.Context, s runner.Spec) (runner.Result, error) {
				path = s.Args[4]
				switch mode {
				case "missing":
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				case "empty":
				case "invalid":
					if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
						t.Fatal(err)
					}
				case "oversize":
					if err := os.WriteFile(path, []byte(strings.Repeat("x", MaxReportBytes+1)), 0600); err != nil {
						t.Fatal(err)
					}
				case "directory":
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(path, 0700); err != nil {
						t.Fatal(err)
					}
				case "symlink":
					target := filepath.Join(t.TempDir(), "external.json")
					if err := os.WriteFile(target, fixture(t, "devicectl-legacy.json"), 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(target, path); err != nil {
						t.Skipf("symlinks unavailable: %v", err)
					}
				}
				return runner.Result{}, nil
			})
			if _, err := List(context.Background(), "darwin", r, request()); err == nil {
				t.Fatal("bad output accepted")
			}
			if _, err := os.Lstat(filepath.Dir(path)); !os.IsNotExist(err) {
				t.Fatal("bad output retained")
			}
		})
	}
	req := request()
	req.Simulators = true
	r := toolFunc(func(context.Context, runner.Spec) (runner.Result, error) {
		return runner.Result{Stdout: `{"devices":{}}`, StdoutTruncated: true}, nil
	})
	_, err := List(context.Background(), "darwin", r, req)
	errorCode(t, err, "report_too_large")
}

func TestOfflineReadFailures(t *testing.T) {
	for _, path := range []string{filepath.Join(t.TempDir(), "missing.json"), t.TempDir()} {
		req := request()
		req.FromJSON = path
		_, err := List(context.Background(), "windows", forbid(t), req)
		errorCode(t, err, "report_read_failed")
	}
}
