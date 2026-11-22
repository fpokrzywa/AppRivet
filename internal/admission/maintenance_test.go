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

func TestMigrationAndCompactionPreserveReplayOwnershipAndHistory(t *testing.T) {
	s, r, p, a, now, j := ownershipFixture(t)
	ctx := context.Background()
	got, err := s.Claim(ctx, "developer", j.ID, "worker", p, a)
	if err != nil {
		t.Fatal(err)
	}
	owner := ownerOf(got.Reservations[0])
	legacy, err := os.ReadFile(filepath.Join(s.Dir, "journal.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.RecoveryStatus(ctx, "developer")
	if err != nil {
		t.Fatal(err)
	}
	report, err := s.Migrate(ctx)
	if err != nil || !report.Changed || report.Format != 2 || report.AuthenticationVerified || report.ExecutionAuthorized {
		t.Fatalf("%+v %v", report, err)
	}
	control, _ := os.ReadFile(filepath.Join(s.Dir, "journal.jsonl"))
	if _, err = replay(control); err == nil {
		t.Fatal("legacy decoder accepted new control")
	}
	m, err := parseManifest(control)
	if err != nil {
		t.Fatal(err)
	}
	packed, _ := os.ReadFile(filepath.Join(s.Dir, m.Segments[0].Name))
	unpacked, err := unpackSegment(packed, m.Segments[0])
	if err != nil || !bytes.Equal(unpacked, legacy) {
		t.Fatal("history changed", err)
	}
	after, err := s.RecoveryStatus(ctx, "developer")
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("state changed %+v %v", after, err)
	}
	again, err := s.Migrate(ctx)
	if err != nil || again.Changed {
		t.Fatalf("migration not idempotent %+v %v", again, err)
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
	oldActive := m.Active
	before, err = s.RecoveryStatus(ctx, "developer")
	if err != nil {
		t.Fatal(err)
	}
	compacted, err := s.Compact(ctx)
	if err != nil || !compacted.Changed || compacted.Generation != 2 {
		t.Fatalf("%+v %v", compacted, err)
	}
	if _, err = os.Stat(filepath.Join(s.Dir, oldActive)); !os.IsNotExist(err) {
		t.Fatal("superseded raw segment retained", err)
	}
	after, err = s.RecoveryStatus(ctx, "developer")
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("compaction changed state", err)
	}
	if _, err = s.Recover(ctx, "developer", owner, true); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Compact(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = s.CheckOwner(ctx, "developer", owner, p, a)
	code(t, err, "ownership_conflict")
	_, err = s.Reserve(ctx, "developer", p, r, a)
	code(t, err, "replay_detected")
	if _, err = s.Reserve(ctx, "developer", p, next, a); err != nil {
		t.Fatal(err)
	}
}

func TestMaintenanceUninitializedCorruptAndCancelled(t *testing.T) {
	s, _, _, _, _ := reservationFixture(t)
	ctx := context.Background()
	_, err := s.Compact(ctx)
	code(t, err, "migration_required")
	before, _ := os.ReadFile(filepath.Join(s.Dir, "journal.jsonl"))
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = s.Migrate(cancelledCtx)
	code(t, err, "cancelled")
	after, _ := os.ReadFile(filepath.Join(s.Dir, "journal.jsonl"))
	if !bytes.Equal(before, after) {
		t.Fatal("cancelled migration mutated journal")
	}
	if err = os.WriteFile(filepath.Join(s.Dir, "journal.jsonl"), append(before, '{'), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = s.Migrate(ctx)
	code(t, err, "state_corrupt")
	missing := Store{Dir: filepath.Join(t.TempDir(), "missing")}
	_, err = missing.Migrate(ctx)
	code(t, err, "state_io")
}
