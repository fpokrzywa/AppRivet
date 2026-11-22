package admission

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writePolicy(t *testing.T, path string, p Policy) {
	t.Helper()
	b, _ := json.Marshal(p)
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestReservationRechecksPolicyAndGrantDeadline(t *testing.T) {
	for _, change := range []string{"revoke", "quota", "expire", "cancel"} {
		t.Run(change, func(t *testing.T) {
			s, r, p, a, _ := reservationFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			s.Clock = func() time.Time {
				calls++
				if calls == 2 {
					policy, _ := LoadPolicy(p)
					switch change {
					case "revoke":
						policy.Grants[0].Enabled = false
						writePolicy(t, p, policy)
					case "quota":
						policy.Grants[0].Limits.MaxSourceBytes = 1
						writePolicy(t, p, policy)
					case "cancel":
						cancel()
					}
				}
				if change == "expire" && calls >= 2 {
					return testNow.Add(time.Hour)
				}
				return testNow
			}
			_, err := s.Reserve(ctx, "developer", p, r, a)
			expected := "admission_denied"
			if change == "quota" {
				expected = "resource_limit"
			}
			if change == "cancel" {
				expected = "cancelled"
			}
			code(t, err, expected)
			s.Clock = func() time.Time { return testNow.Add(time.Hour) }
			status, err := s.Status(context.Background(), "developer", "")
			if err != nil || len(status.Reservations) != 0 {
				t.Fatalf("consumed failed request %+v %v", status, err)
			}
		})
	}
	s, r, p, a, _ := reservationFixture(t)
	policy, _ := LoadPolicy(p)
	policy.Grants[0].ExpiresAt = stamp(testNow.Add(10 * time.Second))
	writePolicy(t, p, policy)
	receipt, err := s.Reserve(context.Background(), "developer", p, r, a)
	if err != nil || receipt.Reservations[0].ExpiresAt != policy.Grants[0].ExpiresAt {
		t.Fatalf("lease exceeded grant %+v %v", receipt, err)
	}
}

func TestReservationGlobalAndByteBudgets(t *testing.T) {
	for _, limit := range []string{"global", "bytes"} {
		t.Run(limit, func(t *testing.T) {
			s, r, p, a, _ := reservationFixture(t)
			ctx := context.Background()
			c := Config{SchemaVersion: 1, MaxActive: 1, MaxPerPrincipal: 1, MaxReservedBytes: 1 << 20, MaxLeaseSeconds: 120}
			if limit == "bytes" {
				c.MaxActive = 2
				info, _ := os.Stat(a)
				c.MaxReservedBytes = info.Size() + int64(len("print(1)"))
			}
			s.Dir = filepath.Join(t.TempDir(), "limited")
			if err := s.Init(ctx, c); err != nil {
				t.Fatal(err)
			}
			policy, _ := LoadPolicy(p)
			other := policy.Grants[0]
			other.ID = "other-grant"
			other.Principal = "other"
			policy.Grants = append(policy.Grants, other)
			writePolicy(t, p, policy)
			result, err := s.Reserve(ctx, "developer", p, r, a)
			if err != nil {
				t.Fatal(err)
			}
			r.GrantID = other.ID
			_, err = s.Reserve(ctx, "other", p, r, a)
			code(t, err, "capacity_exceeded")
			if _, err = s.Release(ctx, "developer", result.Reservations[0].ID); err != nil {
				t.Fatal(err)
			}
			if _, err = s.Reserve(ctx, "other", p, r, a); err != nil {
				t.Fatal("rejected attempt consumed identity", err)
			}
			status, err := s.Status(ctx, "other", "")
			if err != nil || len(status.Reservations) != 1 || status.Reservations[0].Principal != "other" {
				t.Fatalf("leaked jobs %+v %v", status, err)
			}
		})
	}
}

func TestReservationRequestAndConfigBounds(t *testing.T) {
	s, r, p, a, _ := reservationFixture(t)
	for _, mutate := range []func(*Request){func(r *Request) { r.RequestID = "" }, func(r *Request) { r.RequestID = "../a" }, func(r *Request) { r.Nonce = "short" }, func(r *Request) { r.Nonce = strings.Repeat("A", 32) }, func(r *Request) { r.LeaseSeconds = 0 }, func(r *Request) { r.LeaseSeconds = 3601 }} {
		bad := r
		mutate(&bad)
		_, err := s.Reserve(context.Background(), "developer", p, bad, a)
		code(t, err, "invalid_request")
	}
	r.LeaseSeconds = 121
	_, err := s.Reserve(context.Background(), "developer", p, r, a)
	code(t, err, "resource_limit")
	configPath := filepath.Join(t.TempDir(), "config.json")
	for _, body := range []string{`null`, `{"schema_version":1,"max_active":1,"max_active":2}`, `{"schema_version":1,"max_active":1,"max_per_principal":2,"max_reserved_bytes":1,"max_lease_seconds":1}`, `{"schema_version":1,"max_active":1,"max_per_principal":1,"max_reserved_bytes":1,"max_lease_seconds":3601}`} {
		os.WriteFile(configPath, []byte(body), 0600)
		_, err = LoadConfig(configPath)
		code(t, err, "invalid_config")
	}
}

func TestReservationFailureDoesNotConsumeRequest(t *testing.T) {
	s, r, p, a, _ := reservationFixture(t)
	ctx := context.Background()
	original, _ := os.ReadFile(a)
	os.WriteFile(a, []byte("tampered"), 0600)
	_, err := s.Reserve(ctx, "developer", p, r, a)
	code(t, err, "invalid_archive")
	os.WriteFile(a, original, 0600)
	result, err := s.Reserve(ctx, "developer", p, r, a)
	if err != nil {
		t.Fatal(err)
	}
	id := result.Reservations[0].ID
	first, err := s.Release(ctx, "developer", id)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Release(ctx, "developer", id)
	if err != nil || first.Reservations[0] != again.Reservations[0] {
		t.Fatalf("release not idempotent %+v %v", again, err)
	}
}

// Test subprocesses exercise the kernel lock across actual process boundaries.
func TestReservationProcessHelper(t *testing.T) {
	mode := os.Getenv("APPRIVET_RESERVATION_HELPER")
	if mode == "" {
		return
	}
	s := Store{Dir: os.Getenv("APPRIVET_TEST_STATE"), Clock: func() time.Time { return testNow }}
	if mode == "hold" {
		tx, err := s.begin(context.Background())
		if err != nil {
			os.Exit(2)
		}
		defer tx.close()
		fmt.Println("ready")
		io.Copy(io.Discard, os.Stdin)
		return
	}
	var r Request
	if json.Unmarshal([]byte(os.Getenv("APPRIVET_TEST_REQUEST")), &r) != nil {
		os.Exit(2)
	}
	fmt.Println("ready")
	var start [1]byte
	if _, err := os.Stdin.Read(start[:]); err != nil {
		os.Exit(2)
	}
	var err error
	if strings.HasPrefix(mode, "claim:") {
		_, err = s.Claim(context.Background(), "developer", strings.TrimPrefix(mode, "claim:"), r.RequestID, os.Getenv("APPRIVET_TEST_POLICY"), os.Getenv("APPRIVET_TEST_ARCHIVE"))
	} else if mode == "crash-migrate" {
		var tx *transaction
		tx, err = s.begin(context.Background())
		if err == nil {
			tx.file = crashPublication{tx.file}
			_, err = tx.maintain(context.Background(), testNow, "migrate")
			tx.close()
		}
	} else if mode == "crash-repair" {
		var tx *transaction
		tx, err = s.openLocked(context.Background())
		if err == nil {
			var proof repairProof
			proof, err = tx.proveRepair(context.Background(), testNow)
			if err == nil {
				tx.file = crashPublication{tx.file}
				_, err = tx.repair(context.Background(), proof, repairExpectation(proof.report))
			}
			tx.close()
		}
	} else if mode == "repair" {
		var plan RepairReport
		plan, err = s.InspectRepair(context.Background())
		if err == nil {
			_, err = s.Repair(context.Background(), repairExpectation(plan))
		}
	} else if mode == "migrate" {
		_, err = s.Migrate(context.Background())
	} else if mode == "compact" {
		_, err = s.Compact(context.Background())
	} else {
		_, err = s.Reserve(context.Background(), "developer", os.Getenv("APPRIVET_TEST_POLICY"), r, os.Getenv("APPRIVET_TEST_ARCHIVE"))
	}
	result := "ok"
	if err != nil {
		var detail *Error
		if errors.As(err, &detail) {
			result = detail.Code
		} else {
			result = "unexpected"
		}
	}
	fmt.Println(result)
	os.Exit(0)
}

func startReservationProcess(t *testing.T, s Store, r Request, p, a, mode string) (*exec.Cmd, io.WriteCloser, *bufio.Reader) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	b, _ := json.Marshal(r)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestReservationProcessHelper$")
	cmd.Env = append(os.Environ(), "APPRIVET_RESERVATION_HELPER="+mode, "APPRIVET_TEST_STATE="+s.Dir, "APPRIVET_TEST_REQUEST="+string(b), "APPRIVET_TEST_POLICY="+p, "APPRIVET_TEST_ARCHIVE="+a)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stdin.Close()
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	})
	reader := bufio.NewReader(stdout)
	line, err := reader.ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "ready" {
		t.Fatalf("child handshake %q %v", line, err)
	}
	return cmd, stdin, reader
}

func TestReservationCrossProcessContention(t *testing.T) {
	for _, duplicate := range []bool{true, false} {
		t.Run(fmt.Sprint(duplicate), func(t *testing.T) {
			s, r, p, a, _ := reservationFixture(t)
			type child struct {
				cmd    *exec.Cmd
				input  io.WriteCloser
				output *bufio.Reader
			}
			children := []child{}
			for i := 0; i < 6; i++ {
				if !duplicate {
					r.RequestID = fmt.Sprintf("request-%d", i)
					r.Nonce = fmt.Sprintf("%032x", i)
				}
				cmd, input, output := startReservationProcess(t, s, r, p, a, "reserve")
				children = append(children, child{cmd, input, output})
			}
			for _, c := range children {
				if _, err := c.input.Write([]byte{'x'}); err != nil {
					t.Fatal(err)
				}
			}
			successes := 0
			for _, c := range children {
				line, err := c.output.ReadString('\n')
				if err != nil {
					t.Fatal(err)
				}
				result := strings.TrimSpace(line)
				if result == "ok" {
					successes++
				} else if (duplicate && result != "replay_detected") || (!duplicate && result != "capacity_exceeded") {
					t.Fatal(result)
				}
				if err = c.cmd.Wait(); err != nil {
					t.Fatal(err)
				}
			}
			if successes != 1 {
				t.Fatalf("%d reservations won", successes)
			}
			status, err := s.Status(context.Background(), "developer", "")
			if err != nil || len(status.Reservations) != 1 {
				t.Fatalf("%+v %v", status, err)
			}
		})
	}
}

func TestReservationKernelLockReleasedOnProcessDeath(t *testing.T) {
	s, r, p, a, _ := reservationFixture(t)
	cmd, _, _ := startReservationProcess(t, s, r, p, a, "hold")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := s.Reserve(ctx, "developer", p, r, a)
	code(t, err, "cancelled")
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	cmd.Wait()
	if _, err = s.Reserve(context.Background(), "developer", p, r, a); err != nil {
		t.Fatal(err)
	}
}

func reservationFixture(t testing.TB) (Store, Request, string, string, *time.Time) {
	t.Helper()
	p, r, a := fixture(t)
	r.RequestID = "request-1"
	r.Nonce = strings.Repeat("a", 32)
	r.LeaseSeconds = 60
	path := filepath.Join(t.TempDir(), "policy.json")
	b, _ := json.Marshal(p)
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	now := testNow
	s := Store{Dir: filepath.Join(t.TempDir(), "state"), Clock: func() time.Time { return now }}
	if err := s.Init(context.Background(), Config{SchemaVersion: 1, MaxActive: 2, MaxPerPrincipal: 1, MaxReservedBytes: 1 << 20, MaxLeaseSeconds: 120}); err != nil {
		t.Fatal(err)
	}
	return s, r, path, a, &now
}

func TestReservationReplaySurvivesReleaseExpiryAndRestart(t *testing.T) {
	for _, terminal := range []string{"released", "expired"} {
		t.Run(terminal, func(t *testing.T) {
			s, r, p, a, now := reservationFixture(t)
			ctx := context.Background()
			receipt, err := s.Reserve(ctx, "developer", p, r, a)
			if err != nil {
				t.Fatal(err)
			}
			if receipt.Mode != "local_reservation" || receipt.AuthenticationVerified || receipt.ExecutionAuthorized || receipt.Reservations[0].Status != "reserved" {
				t.Fatalf("false authority: %+v", receipt)
			}
			id := receipt.Reservations[0].ID
			_, err = s.Release(ctx, "attacker", id)
			code(t, err, "reservation_not_found")
			if terminal == "released" {
				if _, err = s.Release(ctx, "developer", id); err != nil {
					t.Fatal(err)
				}
			} else {
				*now = now.Add(time.Minute)
			}
			restarted := Store{Dir: s.Dir, Clock: s.Clock}
			status, err := restarted.Status(ctx, "developer", r.RequestID)
			if err != nil || len(status.Reservations) != 1 || status.Reservations[0].Status != terminal {
				t.Fatalf("status %+v %v", status, err)
			}
			_, err = restarted.Reserve(ctx, "developer", p, r, a)
			code(t, err, "replay_detected")
			changed := r
			changed.LeaseSeconds++
			_, err = restarted.Reserve(ctx, "developer", p, changed, a)
			code(t, err, "request_conflict")
			changed = r
			changed.RequestID = "request-2"
			_, err = restarted.Reserve(ctx, "developer", p, changed, a)
			code(t, err, "replay_detected")
			changed.Nonce = strings.Repeat("b", 32)
			if _, err = restarted.Reserve(ctx, "developer", p, changed, a); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRequestConflictPrecedesEarlierNonceCollision(t *testing.T) {
	s, r, p, a, _ := reservationFixture(t)
	ctx := context.Background()
	first, err := s.Reserve(ctx, "developer", p, r, a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Release(ctx, "developer", first.Reservations[0].ID); err != nil {
		t.Fatal(err)
	}
	second := r
	second.RequestID = "request-2"
	second.Nonce = strings.Repeat("b", 32)
	if _, err = s.Reserve(ctx, "developer", p, second, a); err != nil {
		t.Fatal(err)
	}
	second.Nonce = r.Nonce
	_, err = s.Reserve(ctx, "developer", p, second, a)
	code(t, err, "request_conflict")
}

func TestReservationCapacityAndClock(t *testing.T) {
	s, r, p, a, now := reservationFixture(t)
	ctx := context.Background()
	if _, err := s.Reserve(ctx, "developer", p, r, a); err != nil {
		t.Fatal(err)
	}
	r.RequestID = "second"
	r.Nonce = strings.Repeat("b", 32)
	_, err := s.Reserve(ctx, "developer", p, r, a)
	code(t, err, "capacity_exceeded")
	*now = now.Add(time.Minute)
	if _, err = s.Reserve(ctx, "developer", p, r, a); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(-time.Second)
	_, err = s.Status(ctx, "developer", "")
	code(t, err, "clock_rollback")
}

func TestReservationRefusesBrokenStateAndCancelledWork(t *testing.T) {
	for _, damage := range []string{"missing", "partial", "modified", "link"} {
		t.Run(damage, func(t *testing.T) {
			s, r, p, a, _ := reservationFixture(t)
			path := filepath.Join(s.Dir, "journal.jsonl")
			switch damage {
			case "missing":
				os.Remove(path)
			case "partial":
				f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
				f.WriteString("{")
				f.Close()
			case "modified":
				b, _ := os.ReadFile(path)
				b[0] = '['
				os.WriteFile(path, b, 0600)
			case "link":
				outside := filepath.Join(t.TempDir(), "other")
				os.WriteFile(outside, []byte("sentinel"), 0600)
				os.Remove(path)
				if err := os.Symlink(outside, path); err != nil {
					t.Skip(err)
				}
			}
			if _, err := s.Reserve(context.Background(), "developer", p, r, a); err == nil {
				t.Fatal("damaged state accepted")
			}
		})
	}
	s, r, p, a, _ := reservationFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.Reserve(ctx, "developer", p, r, a)
	code(t, err, "cancelled")
	if _, err = s.Reserve(context.Background(), "developer", p, r, a); err != nil {
		t.Fatal(err)
	}
	if err = s.Init(context.Background(), Config{}); err == nil {
		t.Fatal("state reset")
	}
}
