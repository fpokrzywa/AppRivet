package admission

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func appendHashed(prefix []byte, e event) []byte {
	e.Hash = ""
	b, _ := json.Marshal(e)
	e.Hash = sum(b)
	b, _ = json.Marshal(e)
	result := append([]byte(nil), prefix...)
	result = append(result, b...)
	return append(result, '\n')
}
func TestOwnershipJournalRejectsForgedTransitions(t *testing.T) {
	s, _, p, a, _, j := ownershipFixture(t)
	ctx := context.Background()
	path := filepath.Join(s.Dir, "journal.jsonl")
	before, _ := os.ReadFile(path)
	got, err := s.Claim(ctx, "developer", j.ID, "worker", p, a)
	if err != nil {
		t.Fatal(err)
	}
	claimed, _ := os.ReadFile(path)
	var claim event
	lines := bytes.Split(bytes.TrimSuffix(claimed, []byte{'\n'}), []byte{'\n'})
	if err = json.Unmarshal(lines[len(lines)-1], &claim); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*event){func(e *event) { e.Owner = nil }, func(e *event) { e.Owner.Fence-- }, func(e *event) { e.Owner.ClaimID = "bad" }, func(e *event) { e.Owner.ReservationID = strings.Repeat("f", 32) }, func(e *event) { e.At = stamp(testNow.Add(time.Minute)) }, func(e *event) { e.Stopped = true }, func(e *event) { e.Config = &Config{SchemaVersion: 1} }} {
		copyEvent := claim
		copyOwner := *claim.Owner
		copyEvent.Owner = &copyOwner
		mutate(&copyEvent)
		if _, err = replay(appendHashed(before, copyEvent)); err == nil {
			t.Fatal("invalid claim transition accepted")
		}
	}
	owner := ownerOf(got.Reservations[0])
	if _, err = s.Recover(ctx, "developer", owner, true); err != nil {
		t.Fatal(err)
	}
	recovered, _ := os.ReadFile(path)
	lines = bytes.Split(bytes.TrimSuffix(recovered, []byte{'\n'}), []byte{'\n'})
	var recovery event
	json.Unmarshal(lines[len(lines)-1], &recovery)
	for _, mutate := range []func(*event){func(e *event) { e.Stopped = false }, func(e *event) { e.Owner.Fence++ }, func(e *event) { e.Owner.WorkerID = "other" }, func(e *event) { e.Kind = "release"; e.Owner = nil; e.Stopped = false }, func(e *event) { e.Kind = "claim"; e.Stopped = false; e.Owner.Fence = e.Sequence }} {
		copyEvent := recovery
		copyOwner := *recovery.Owner
		copyEvent.Owner = &copyOwner
		mutate(&copyEvent)
		if _, err = replay(appendHashed(claimed, copyEvent)); err == nil {
			t.Fatal("unsafe recovery/release/reclaim accepted")
		}
	}
}

func TestOwnershipUncertainClaimAndRecovery(t *testing.T) {
	for _, kind := range []string{"claim", "recover"} {
		for _, mode := range []string{"partial", "sync", "cancel_after_sync"} {
			t.Run(kind+"/"+mode, func(t *testing.T) {
				s, _, p, a, _, j := ownershipFixture(t)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				owner := Ownership{ReservationID: j.ID, WorkerID: "worker", ClaimID: strings.Repeat("a", 32)}
				if kind == "recover" {
					got, err := s.Claim(ctx, "developer", j.ID, "worker", p, a)
					if err != nil {
						t.Fatal(err)
					}
					owner = ownerOf(got.Reservations[0])
				}
				tx, err := s.begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if kind == "claim" {
					owner.Fence = tx.state.sequence + 1
				}
				wrapped := failingJournal{journalFile: tx.file, partial: mode == "partial", syncError: mode == "sync"}
				if mode == "cancel_after_sync" {
					wrapped.cancel = cancel
				}
				tx.file = wrapped
				err = tx.append(ctx, event{Kind: kind, At: stamp(testNow), ID: j.ID, Owner: &owner, Stopped: kind == "recover"})
				tx.close()
				if mode == "cancel_after_sync" {
					if err != nil || ctx.Err() == nil {
						t.Fatalf("durable transition lost %v", err)
					}
				} else {
					code(t, err, "commit_uncertain")
				}
				report, err := s.RecoveryStatus(context.Background(), "developer")
				if mode == "partial" {
					code(t, err, "state_corrupt")
					return
				}
				expected := "claimed"
				if kind == "recover" {
					expected = "recovered"
				}
				if err != nil || report.Reservations[0].Status != expected || !owner.matches(report.Reservations[0]) {
					t.Fatalf("lost uncertain transition %+v %v", report, err)
				}
			})
		}
	}
}

func TestLegacyJournalCanonicalCompatibility(t *testing.T) {
	// Literal schema/order from the pre-ownership journal. New optional fields must
	// not change its hash or canonical bytes merely by decoding/re-encoding it.
	raw := `{"sequence":1,"previous":"","at":"2026-11-22T12:00:00Z","kind":"init","config":{"schema_version":1,"max_active":2,"max_per_principal":1,"max_reserved_bytes":1048576,"max_lease_seconds":120},"hash":""}`
	hash := sum([]byte(raw))
	line := strings.TrimSuffix(raw, `""}`) + `"` + hash + `"}` + "\n"
	state, err := replay([]byte(line))
	if err != nil || state.sequence != 1 {
		t.Fatalf("legacy init %v", err)
	}
	raw = `{"sequence":2,"previous":"` + hash + `","at":"2026-11-22T12:00:00Z","kind":"reserve","job":{"id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","request_id":"legacy","principal":"developer","project_id":"hello","grant_id":"grant-1","operation":"extract","snapshot_sha256":"` + strings.Repeat("b", 64) + `","reserved_bytes":4096,"created_at":"2026-11-22T12:00:00Z","expires_at":"2026-11-22T12:01:00Z","status":"reserved","fingerprint":"` + strings.Repeat("c", 64) + `","nonce_hash":"` + strings.Repeat("d", 64) + `"},"hash":""}`
	line += strings.TrimSuffix(raw, `""}`) + `"` + sum([]byte(raw)) + `"}` + "\n"
	state, err = replay([]byte(line))
	if err != nil || len(state.jobs) != 1 || state.jobs[0].ClaimID != "" {
		t.Fatalf("legacy reserve %v", err)
	}
}
