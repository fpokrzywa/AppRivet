//go:build darwin || linux

package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestDescendantHelper(t *testing.T) {
	index := -1
	for i, arg := range os.Args {
		if arg == "--descendant-helper" {
			index = i
			break
		}
	}
	if index < 0 {
		return
	}
	mode, path := os.Args[index+1], os.Args[index+2]
	if mode == "child" {
		if err := os.WriteFile(path+".pid", []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
			os.Exit(20)
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			os.Exit(21)
		}
		for {
			_, _ = f.WriteString("tick\n")
			time.Sleep(10 * time.Millisecond)
		}
	}
	exe, err := os.Executable()
	if err != nil {
		os.Exit(22)
	}
	child := exec.Command(exe, "-test.run=TestDescendantHelper", "--", "--descendant-helper", "child", path)
	if mode != "exit-closed" {
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
	}
	if err := child.Start(); err != nil {
		os.Exit(23)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			break
		}
		if time.Now().After(deadline) {
			os.Exit(24)
		}
		time.Sleep(10 * time.Millisecond)
	}
	fmt.Print("child started")
	if strings.HasPrefix(mode, "exit") {
		os.Exit(0)
	}
	time.Sleep(30 * time.Second)
	os.Exit(0)
}

func TestOwnedDescendantsStopWhenParentExitsOrJobCancels(t *testing.T) {
	for _, mode := range []string{"exit-open", "exit-closed", "cancel", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			heartbeat := filepath.Join(t.TempDir(), "heartbeat")
			t.Cleanup(func() {
				if data, err := os.ReadFile(heartbeat + ".pid"); err == nil {
					if pid, err := strconv.Atoi(string(data)); err == nil {
						_ = syscall.Kill(pid, syscall.SIGKILL)
					}
				}
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			spec := Spec{Path: exe, Args: []string{"-test.run=TestDescendantHelper", "--", "--descendant-helper", mode, heartbeat}, Timeout: 8 * time.Second}
			if mode == "timeout" {
				spec.Timeout = time.Second
			}
			if mode == "cancel" {
				go func() {
					for ctx.Err() == nil {
						if _, err := os.Stat(heartbeat); err == nil {
							cancel()
							return
						}
						time.Sleep(10 * time.Millisecond)
					}
				}()
			}
			result, err := (Exec{}).Run(ctx, spec)
			if mode == "cancel" && (err == nil || !result.Cancelled) {
				t.Fatalf("not cancelled: %+v %v", result, err)
			}
			if mode == "timeout" && (err == nil || !result.TimedOut) {
				t.Fatalf("not timed out: %+v %v", result, err)
			}
			if _, err := os.Stat(heartbeat); err != nil {
				t.Fatalf("child did not start: %v; result %+v", err, result)
			}
			// A live descendant continues appending. Allow delivery of SIGKILL first;
			// then assert that no work continues after the completed job.
			time.Sleep(100 * time.Millisecond)
			before, err := os.ReadFile(heartbeat)
			if err != nil {
				t.Fatal(err)
			}
			time.Sleep(150 * time.Millisecond)
			after, err := os.ReadFile(heartbeat)
			if err != nil {
				t.Fatal(err)
			}
			if len(after) != len(before) {
				t.Fatal("descendant kept running after runner returned")
			}
		})
	}
}
