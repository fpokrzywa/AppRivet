package admission

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOwnershipPolicyChangesAndDeadlines(t *testing.T) {
	for _, operation := range []string{"claim", "check"} {
		for _, change := range []string{"revoke", "quota", "expire", "cancel", "archive"} {
			t.Run(operation+"/"+change, func(t *testing.T) {
				s, _, p, a, _, j := ownershipFixture(t)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				var owner Ownership
				if operation == "check" {
					got, err := s.Claim(ctx, "developer", j.ID, "worker", p, a)
					if err != nil {
						t.Fatal(err)
					}
					owner = ownerOf(got.Reservations[0])
				}
				if change == "archive" {
					if err := os.WriteFile(a, []byte("tampered"), 0600); err != nil {
						t.Fatal(err)
					}
				}
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
						return testNow.Add(time.Minute)
					}
					return testNow
				}
				var err error
				if operation == "claim" {
					_, err = s.Claim(ctx, "developer", j.ID, "worker", p, a)
				} else {
					_, err = s.CheckOwner(ctx, "developer", owner, p, a)
				}
				expected := map[string]string{"revoke": "admission_denied", "quota": "resource_limit", "expire": "ownership_expired", "cancel": "cancelled", "archive": "invalid_archive"}[change]
				code(t, err, expected)
				report, err := s.RecoveryStatus(context.Background(), "developer")
				if err != nil {
					t.Fatal(err)
				}
				if operation == "claim" && report.Reservations[0].ClaimID != "" {
					t.Fatal("failed claim allocated ownership")
				}
			})
		}
	}
}

func TestOwnershipRecoveryOfLiveClaimAndStaleTuple(t *testing.T) {
	s, _, p, a, _, j := ownershipFixture(t)
	ctx := context.Background()
	got, err := s.Claim(ctx, "developer", j.ID, "worker", p, a)
	if err != nil {
		t.Fatal(err)
	}
	owner := ownerOf(got.Reservations[0])
	// Recovery must remain possible after revocation, independently of archive availability.
	policy, _ := LoadPolicy(p)
	policy.Grants[0].Enabled = false
	writePolicy(t, p, policy)
	_, err = s.CheckOwner(ctx, "developer", owner, p, a)
	code(t, err, "admission_denied")
	if _, err = s.Recover(ctx, "developer", owner, true); err != nil {
		t.Fatal(err)
	}
	wrong := owner
	wrong.Fence++
	_, err = s.Recover(ctx, "developer", wrong, true)
	code(t, err, "ownership_conflict")
	_, err = s.CheckOwner(ctx, "developer", owner, p, "missing")
	code(t, err, "ownership_conflict")
}

func TestRecoveryInspectionFullJournalAndClockRollback(t *testing.T) {
	s, _, p, a, now, j := ownershipFixture(t)
	ctx := context.Background()
	got, err := s.Claim(ctx, "developer", j.ID, "worker", p, a)
	if err != nil {
		t.Fatal(err)
	}
	owner := ownerOf(got.Reservations[0])
	path := filepath.Join(s.Dir, "journal.jsonl")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	state, err := replay(b)
	if err != nil {
		t.Fatal(err)
	}
	// Construct a genuinely valid full journal rather than modifying counters only.
	for state.sequence < maxEvents {
		e := event{Sequence: state.sequence + 1, Previous: state.hash, At: stamp(testNow), Kind: "observe"}
		raw, _ := json.Marshal(e)
		e.Hash = sum(raw)
		raw, _ = json.Marshal(e)
		if err = state.apply(e); err != nil {
			t.Fatal(err)
		}
		b = append(b, raw...)
		b = append(b, '\n')
	}
	if err = os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Second)
	_, err = s.Status(ctx, "developer", "")
	code(t, err, "state_full")
	report, err := s.RecoveryStatus(ctx, "developer")
	if err != nil || report.JournalSequence != maxEvents || ownerOf(report.Reservations[0]) != owner {
		t.Fatalf("full recovery view %+v %v", report, err)
	}
	_, err = s.Recover(ctx, "developer", owner, true)
	code(t, err, "state_full")
	*now = testNow.Add(-time.Second)
	_, err = s.Recover(ctx, "developer", owner, true)
	code(t, err, "clock_rollback")
	after, _ := os.ReadFile(path)
	if !bytes.Equal(b, after) {
		t.Fatal("inspection or rejected mutation changed full journal")
	}
}

func TestOwnershipMalformedInputAndCancellation(t *testing.T) {
	s, _, p, a, _, j := ownershipFixture(t)
	ctx := context.Background()
	for _, worker := range []string{"", "../worker", strings.Repeat("a", 65)} {
		_, err := s.Claim(ctx, "developer", j.ID, worker, p, a)
		code(t, err, "invalid_arguments")
	}
	got, err := s.Claim(ctx, "developer", j.ID, "worker", p, a)
	if err != nil {
		t.Fatal(err)
	}
	owner := ownerOf(got.Reservations[0])
	for _, mutate := range []func(*Ownership){func(o *Ownership) { o.Fence = 0 }, func(o *Ownership) { o.Fence = maxEvents + 1 }, func(o *Ownership) { o.ClaimID = "bad" }, func(o *Ownership) { o.ReservationID = "bad" }, func(o *Ownership) { o.WorkerID = "" }} {
		bad := owner
		mutate(&bad)
		_, err = s.CheckOwner(ctx, "developer", bad, p, a)
		code(t, err, "invalid_arguments")
		_, err = s.Recover(ctx, "developer", bad, true)
		code(t, err, "invalid_arguments")
	}
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = s.Recover(cancelledCtx, "developer", owner, true)
	code(t, err, "cancelled")
	_, err = s.RecoveryStatus(cancelledCtx, "developer")
	code(t, err, "cancelled")
	_, err = s.Claim(cancelledCtx, "developer", j.ID, "worker", p, a)
	code(t, err, "cancelled")
	report, err := s.RecoveryStatus(ctx, "developer")
	if err != nil || report.Reservations[0].Status != "claimed" {
		t.Fatalf("cancellation lost claim %+v %v", report, err)
	}
}

func TestOwnershipCrossProcessSingleClaim(t *testing.T) {
	s, r, p, a, _, j := ownershipFixture(t)
	type child struct {
		cmd   *exec.Cmd
		input io.WriteCloser
		read  func() (string, error)
	}
	children := []child{}
	for i := 0; i < 6; i++ {
		r.RequestID = fmt.Sprintf("worker-%d", i)
		cmd, input, output := startReservationProcess(t, s, r, p, a, "claim:"+j.ID)
		children = append(children, child{cmd, input, func() (string, error) { return output.ReadString('\n') }})
	}
	for _, c := range children {
		if _, err := c.input.Write([]byte{'x'}); err != nil {
			t.Fatal(err)
		}
	}
	successes := 0
	for _, c := range children {
		line, err := c.read()
		if err != nil {
			t.Fatal(err)
		}
		result := strings.TrimSpace(line)
		if result == "ok" {
			successes++
		} else if result != "ownership_conflict" {
			t.Fatal(result)
		}
		if err = c.cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatalf("%d claimants won", successes)
	}
	report, err := s.RecoveryStatus(context.Background(), "developer")
	if err != nil || len(report.Reservations) != 1 || report.Reservations[0].Status != "claimed" {
		t.Fatalf("%+v %v", report, err)
	}
	// A fresh process/Store can recover the durable owner after the winning process exits.
	restarted := Store{Dir: s.Dir, Clock: s.Clock}
	if _, err = restarted.Recover(context.Background(), "developer", ownerOf(report.Reservations[0]), true); err != nil {
		t.Fatal(err)
	}
}

func TestExpiredClaimsRetainGlobalCountAndByteBudgets(t *testing.T) {
	for _, limit := range []string{"global", "bytes"} {
		t.Run(limit, func(t *testing.T) {
			s, r, p, a, now := reservationFixture(t)
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
			other.ID = "other"
			other.Principal = "other"
			policy.Grants = append(policy.Grants, other)
			writePolicy(t, p, policy)
			reserved, err := s.Reserve(ctx, "developer", p, r, a)
			if err != nil {
				t.Fatal(err)
			}
			claimed, err := s.Claim(ctx, "developer", reserved.Reservations[0].ID, "worker", p, a)
			if err != nil {
				t.Fatal(err)
			}
			*now = now.Add(time.Minute)
			r.GrantID = "other"
			_, err = s.Reserve(ctx, "other", p, r, a)
			code(t, err, "capacity_exceeded")
			if _, err = s.Recover(ctx, "developer", ownerOf(claimed.Reservations[0]), true); err != nil {
				t.Fatal(err)
			}
			if _, err = s.Reserve(ctx, "other", p, r, a); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func ownershipFixture(t testing.TB) (Store, Request, string, string, *time.Time, Reservation) {
	t.Helper()
	s, r, p, a, now := reservationFixture(t)
	reserved, err := s.Reserve(context.Background(), "developer", p, r, a)
	if err != nil {
		t.Fatal(err)
	}
	return s, r, p, a, now, reserved.Reservations[0]
}
func ownerOf(r Reservation) Ownership {
	return Ownership{ReservationID: r.ID, WorkerID: r.WorkerID, ClaimID: r.ClaimID, Fence: r.Fence}
}

func TestOwnershipClaimAndRecoveryRetainReplay(t *testing.T) {
	s, r, p, a, now, j := ownershipFixture(t)
	ctx := context.Background()
	claimed, err := s.Claim(ctx, "developer", j.ID, "worker-1", p, a)
	if err != nil {
		t.Fatal(err)
	}
	job := claimed.Reservations[0]
	owner := ownerOf(job)
	if job.Status != "claimed" || owner.ClaimID == "" || owner.Fence < 1 || job.ExpiresAt != j.ExpiresAt || claimed.ExecutionAuthorized || claimed.AuthenticationVerified {
		t.Fatalf("invalid claim %+v", claimed)
	}
	_, err = s.Claim(ctx, "developer", j.ID, "worker-1", p, a)
	code(t, err, "ownership_conflict")
	checked, err := s.CheckOwner(ctx, "developer", owner, p, a)
	if err != nil || !checked.OwnershipValid || checked.ExecutionAuthorized || checked.AuthenticationVerified {
		t.Fatalf("%+v %v", checked, err)
	}
	_, err = s.Release(ctx, "developer", j.ID)
	code(t, err, "ownership_required")
	*now = now.Add(time.Minute)
	_, err = s.CheckOwner(ctx, "developer", owner, p, a)
	code(t, err, "ownership_expired")
	next := r
	next.RequestID = "next"
	next.Nonce = strings.Repeat("b", 32)
	_, err = s.Reserve(ctx, "developer", p, next, a)
	code(t, err, "capacity_exceeded")
	restarted := Store{Dir: s.Dir, Clock: s.Clock}
	report, err := restarted.RecoveryStatus(ctx, "developer")
	if err != nil || len(report.Reservations) != 1 || report.Reservations[0].Status != "recovery_required" || ownerOf(report.Reservations[0]) != owner {
		t.Fatalf("lost claim %+v %v", report, err)
	}
	_, err = restarted.Recover(ctx, "developer", owner, false)
	code(t, err, "stop_confirmation_required")
	recovered, err := restarted.Recover(ctx, "developer", owner, true)
	if err != nil || recovered.StopAssertionVerified || recovered.Reservations[0].Status != "recovered" {
		t.Fatalf("%+v %v", recovered, err)
	}
	if _, err = restarted.Recover(ctx, "developer", owner, true); err != nil {
		t.Fatal(err)
	}
	_, err = s.CheckOwner(ctx, "developer", owner, p, a)
	code(t, err, "ownership_conflict")
	_, err = s.Claim(ctx, "developer", j.ID, "worker-2", p, a)
	code(t, err, "ownership_conflict")
	_, err = s.Reserve(ctx, "developer", p, r, a)
	code(t, err, "replay_detected")
	fresh, err := s.Reserve(ctx, "developer", p, next, a)
	if err != nil {
		t.Fatal(err)
	}
	newClaim, err := s.Claim(ctx, "developer", fresh.Reservations[0].ID, "worker-1", p, a)
	if err != nil {
		t.Fatal(err)
	}
	if newClaim.Reservations[0].Fence <= owner.Fence || newClaim.Reservations[0].ClaimID == owner.ClaimID {
		t.Fatal("ownership identity reused")
	}
}

func TestOwnershipTupleAndPrincipalBindings(t *testing.T) {
	s, _, p, a, _, j := ownershipFixture(t)
	ctx := context.Background()
	_, err := s.Claim(ctx, "attacker", j.ID, "worker-1", p, "missing")
	code(t, err, "reservation_not_found")
	got, err := s.Claim(ctx, "developer", j.ID, "worker-1", p, a)
	if err != nil {
		t.Fatal(err)
	}
	owner := ownerOf(got.Reservations[0])
	for _, mutate := range []func(*Ownership){func(o *Ownership) { o.WorkerID = "worker-2" }, func(o *Ownership) { o.ClaimID = strings.Repeat("0", 32) }, func(o *Ownership) { o.Fence++ }} {
		wrong := owner
		mutate(&wrong)
		_, err = s.CheckOwner(ctx, "developer", wrong, p, "missing")
		code(t, err, "ownership_conflict")
		_, err = s.Recover(ctx, "developer", wrong, true)
		code(t, err, "ownership_conflict")
	}
	_, err = s.Recover(ctx, "attacker", owner, true)
	code(t, err, "reservation_not_found")
	_, err = s.CheckOwner(ctx, "attacker", owner, p, "missing")
	code(t, err, "reservation_not_found")
}

func TestRecoveryInspectionDoesNotMutateOrAuthorize(t *testing.T) {
	s, _, p, a, now, j := ownershipFixture(t)
	ctx := context.Background()
	if _, err := s.Claim(ctx, "developer", j.ID, "worker-1", p, a); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.Dir, "journal.jsonl")
	before, _ := os.ReadFile(path)
	*now = now.Add(-time.Second)
	report, err := s.RecoveryStatus(ctx, "developer")
	if err != nil || !report.ClockRollback || report.ExecutionAuthorized || report.AuthenticationVerified || report.JournalSequence < 1 || report.JournalSHA256 == "" {
		t.Fatalf("%+v %v", report, err)
	}
	hidden, err := s.RecoveryStatus(ctx, "other")
	if err != nil || len(hidden.Reservations) != 0 {
		t.Fatalf("leaked principal %+v %v", hidden, err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("inspection changed ledger")
	}
	if err = os.WriteFile(path, append(before, '{'), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = s.RecoveryStatus(ctx, "developer")
	code(t, err, "state_corrupt")
}
