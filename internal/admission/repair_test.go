package admission

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func repairExpectation(r RepairReport) RepairExpect {
	return RepairExpect{IntentSHA256: r.IntentSHA256, ControlSHA256: r.ControlSHA256, HistorySHA256: r.JournalSHA256, Sequence: r.JournalSequence}
}
func tearPublication(t *testing.T, s Store, action string, now time.Time) {
	t.Helper()
	tx, err := s.begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tx.control == nil {
		tx.file = publicationFailure{journalFile: tx.file, partial: true}
	} else {
		tx.control = publicationFailure{journalFile: tx.control, partial: true}
	}
	_, err = tx.maintain(context.Background(), now, action)
	tx.close()
	code(t, err, "commit_uncertain")
}
func TestRepairPreservesHistoryReplayAndOwnership(t *testing.T) {
	for _, action := range []string{"migrate", "compact"} {
		t.Run(action, func(t *testing.T) {
			s, r, p, a, now, j := ownershipFixture(t)
			ctx := context.Background()
			claimed, err := s.Claim(ctx, "developer", j.ID, "worker", p, a)
			if err != nil {
				t.Fatal(err)
			}
			owner := ownerOf(claimed.Reservations[0])
			if action == "compact" {
				if _, err = s.Migrate(ctx); err != nil {
					t.Fatal(err)
				}
				*now = now.Add(time.Second)
				if _, err = s.Status(ctx, "developer", ""); err != nil {
					t.Fatal(err)
				}
			}
			before, err := s.RecoveryStatus(ctx, "developer")
			if err != nil {
				t.Fatal(err)
			}
			tearPublication(t, s, action, *now)
			torn, _ := os.ReadFile(filepath.Join(s.Dir, "journal.jsonl"))
			_, err = s.RecoveryStatus(ctx, "developer")
			code(t, err, "publication_pending")
			plan, err := s.InspectRepair(ctx)
			if err != nil || plan.ControlState != "torn" || plan.AuthenticationVerified || plan.ExecutionAuthorized {
				t.Fatalf("%+v %v", plan, err)
			}
			unchanged, _ := os.ReadFile(filepath.Join(s.Dir, "journal.jsonl"))
			if !bytes.Equal(torn, unchanged) {
				t.Fatal("inspection changed control")
			}
			result, err := s.Repair(ctx, repairExpectation(plan))
			if err != nil || !result.Changed || !result.ControlRewritten {
				t.Fatalf("%+v %v", result, err)
			}
			saved, err := os.ReadFile(filepath.Join(s.Dir, result.BeforeImage))
			if err != nil || !bytes.Equal(saved, torn) {
				t.Fatal("damaged evidence lost", err)
			}
			after, err := s.RecoveryStatus(ctx, "developer")
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("history changed %+v %v", after, err)
			}
			_, err = s.Reserve(ctx, "developer", p, r, a)
			code(t, err, "replay_detected")
			if _, err = s.CheckOwner(ctx, "developer", owner, p, a); err != nil {
				t.Fatal(err)
			}
			*now = now.Add(time.Minute)
			next := r
			next.RequestID = "next"
			next.Nonce = strings.Repeat("b", 32)
			_, err = s.Reserve(ctx, "developer", p, next, a)
			code(t, err, "capacity_exceeded")
			if _, err = s.Recover(ctx, "developer", owner, true); err != nil {
				t.Fatal(err)
			}
			if _, err = s.Reserve(ctx, "developer", p, next, a); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRepairRequiresIntentAndPinnedEvidence(t *testing.T) {
	s, _, _, _, now := reservationFixture(t)
	ctx := context.Background()
	_, err := s.InspectRepair(ctx)
	code(t, err, "repair_unavailable")
	tearPublication(t, s, "migrate", *now)
	plan, err := s.InspectRepair(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*RepairExpect){func(e *RepairExpect) { e.IntentSHA256 = strings.Repeat("0", 64) }, func(e *RepairExpect) { e.ControlSHA256 = strings.Repeat("0", 64) }, func(e *RepairExpect) { e.HistorySHA256 = strings.Repeat("0", 64) }, func(e *RepairExpect) { e.Sequence++ }} {
		expect := repairExpectation(plan)
		mutate(&expect)
		_, err = s.Repair(ctx, expect)
		code(t, err, "repair_changed")
	}
	// Historical unanchored tears must not use prepared-file discovery as authority.
	if err = os.Remove(filepath.Join(s.Dir, "publication.pending")); err != nil {
		t.Fatal(err)
	}
	_, err = s.InspectRepair(ctx)
	code(t, err, "repair_unavailable")
}
