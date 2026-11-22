package admission

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func repairPendingAppend(t *testing.T, s Store) AppendRepairReport {
	t.Helper()
	ctx := context.Background()
	plan, err := s.InspectAppendRepair(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.RepairAppend(ctx, AppendRepairExpect{plan.IntentSHA256, plan.ActiveSHA256})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func interruptRecoveryAppend(t *testing.T, f diskFaultFixture, mode string) string {
	t.Helper()
	ctx := context.Background()
	tx, err := f.store.begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.close()
	tx.file = diskFaultFile{tx.file, mode}
	err = tx.append(ctx, event{Kind: "recover", At: stamp(*f.clock), ID: f.owner.ReservationID, Owner: &f.owner, Stopped: true})
	code(t, err, "commit_uncertain")
	return filepath.Join(f.store.Dir, tx.fileName)
}

func TestAppendRepairCompletesExactOwnershipEvent(t *testing.T) {
	for _, format := range []string{"legacy", "segmented"} {
		for _, mode := range []string{"zero", "partial", "short", "sync"} {
			t.Run(format+"/"+mode, func(t *testing.T) {
				f := newDiskFaultFixture(t, format == "segmented")
				ctx := context.Background()
				path := interruptRecoveryAppend(t, f, mode)
				torn := readFaultFile(t, path)
				_, err := f.store.RecoveryStatus(ctx, "developer")
				code(t, err, "append_pending")
				_, err = f.store.Compact(ctx)
				code(t, err, "append_pending")
				_, err = f.store.Reserve(ctx, "developer", f.policy, f.request, f.archive)
				code(t, err, "append_pending")
				plan, err := f.store.InspectAppendRepair(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if plan.JournalSequence != f.before.JournalSequence+1 || plan.EventKind != "recover" || plan.AuthenticationVerified || plan.ExecutionAuthorized {
					t.Fatalf("bad proof: %+v", plan)
				}
				if (plan.PresentBytes == 0) != (mode == "zero") || (plan.PresentBytes == plan.EventBytes) != (mode == "sync") {
					t.Fatalf("unexpected persisted event bytes: %+v", plan)
				}
				if !bytes.Equal(torn, readFaultFile(t, path)) {
					t.Fatal("inspection wrote data")
				}
				expect := AppendRepairExpect{IntentSHA256: plan.IntentSHA256, ActiveSHA256: plan.ActiveSHA256}
				*f.clock = f.clock.Add(-time.Second)
				_, err = f.store.RepairAppend(ctx, expect)
				code(t, err, "clock_rollback")
				*f.clock = f.clock.Add(time.Second)
				result, err := f.store.RepairAppend(ctx, expect)
				if err != nil || !result.Changed || (result.BytesWritten == 0) != (mode == "sync") {
					t.Fatalf("bad repair: %+v %v", result, err)
				}
				after := readFaultFile(t, path)
				if !bytes.HasPrefix(after, torn) {
					t.Fatal("repair rewrote existing bytes")
				}
				view, err := f.store.RecoveryStatus(ctx, "developer")
				if err != nil || view.JournalSequence != plan.JournalSequence || view.JournalSHA256 != plan.JournalSHA256 || len(view.Reservations) != 1 || !f.owner.matches(view.Reservations[0]) || view.Reservations[0].Status != "recovered" {
					t.Fatalf("lost exact ownership: %+v %v", view, err)
				}
				_, err = f.store.Reserve(ctx, "developer", f.policy, f.request, f.archive)
				code(t, err, "replay_detected")
				_, err = f.store.CheckOwner(ctx, "developer", f.owner, f.policy, f.archive)
				code(t, err, "ownership_conflict")
				_, err = f.store.RepairAppend(ctx, expect)
				code(t, err, "append_repair_unavailable")
				if _, err = os.Lstat(filepath.Join(f.store.Dir, appendPendingName)); !os.IsNotExist(err) {
					t.Fatalf("pending intent not cleared: %v", err)
				}
			})
		}
	}
}

func TestAppendRepairPreservesTornReserveReplayKeys(t *testing.T) {
	s, req, p, a, now := reservationFixture(t)
	ctx := context.Background()
	job := storedReservation{Reservation: Reservation{ID: strings.Repeat("a", 32), RequestID: req.RequestID, Principal: "developer", ProjectID: req.ProjectID, GrantID: req.GrantID, Operation: req.Operation, SnapshotSHA256: req.SnapshotSHA256, ReservedBytes: 1024, CreatedAt: stamp(*now), ExpiresAt: stamp(now.Add(time.Minute)), Status: "reserved"}, Fingerprint: requestHash("developer", req), NonceHash: sum([]byte(req.Nonce))}
	tx, err := s.begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tx.file = diskFaultFile{tx.file, "partial"}
	err = tx.append(ctx, event{Kind: "reserve", At: stamp(*now), Job: &job})
	tx.close()
	code(t, err, "commit_uncertain")
	repairPendingAppend(t, s)
	view, err := s.RecoveryStatus(ctx, "developer")
	if err != nil || len(view.Reservations) != 1 || view.Reservations[0] != job.Reservation {
		t.Fatalf("lost exact reservation: %+v %v", view, err)
	}
	_, err = s.Reserve(ctx, "developer", p, req, a)
	code(t, err, "replay_detected")
	req.RequestID = "other-request"
	_, err = s.Reserve(ctx, "developer", p, req, a)
	code(t, err, "replay_detected")
}

func TestAppendRepairClaimRetainsCapacityAfterExpiry(t *testing.T) {
	s, req, p, a, now, job := ownershipFixture(t)
	ctx := context.Background()
	tx, err := s.begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	owner := Ownership{ReservationID: job.ID, WorkerID: "worker", ClaimID: strings.Repeat("b", 32), Fence: tx.state.sequence + 1}
	tx.file = diskFaultFile{tx.file, "partial"}
	err = tx.append(ctx, event{Kind: "claim", At: stamp(*now), ID: job.ID, Owner: &owner})
	tx.close()
	code(t, err, "commit_uncertain")
	*now = now.Add(time.Minute)
	repairPendingAppend(t, s)
	view, err := s.RecoveryStatus(ctx, "developer")
	if err != nil || len(view.Reservations) != 1 || view.Reservations[0].Status != "recovery_required" || !owner.matches(view.Reservations[0]) {
		t.Fatal(view, err)
	}
	_, err = s.Reserve(ctx, "developer", p, req, a)
	code(t, err, "replay_detected")
	fresh := req
	fresh.RequestID = "fresh"
	fresh.Nonce = strings.Repeat("e", 32)
	_, err = s.Reserve(ctx, "developer", p, fresh, a)
	code(t, err, "capacity_exceeded")
	if _, err = s.Recover(ctx, "developer", owner, true); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Reserve(ctx, "developer", p, fresh, a); err != nil {
		t.Fatal(err)
	}
}

func TestAppendRepairCompletesInitialization(t *testing.T) {
	ctx := context.Background()
	s := Store{Dir: t.TempDir(), Clock: func() time.Time { return testNow }}
	for _, name := range []string{"lock", "journal.jsonl"} {
		writeTestEvidence(t, s, name, nil)
	}
	tx, err := s.openLocked(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tx.file = diskFaultFile{tx.file, "partial"}
	c := Config{SchemaVersion: 1, MaxActive: 2, MaxPerPrincipal: 1, MaxReservedBytes: 1 << 20, MaxLeaseSeconds: 120}
	err = tx.append(ctx, event{Kind: "init", At: stamp(testNow), Config: &c})
	tx.close()
	code(t, err, "commit_uncertain")
	result := repairPendingAppend(t, s)
	if result.JournalSequence != 1 || result.EventKind != "init" {
		t.Fatal(result)
	}
	tx, err = s.begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.close()
	if tx.state.config != c {
		t.Fatal("initial limits changed")
	}
}

func TestAppendRepairAllowsOneConcurrentCompletion(t *testing.T) {
	f := newDiskFaultFixture(t, true)
	ctx := context.Background()
	interruptRecoveryAppend(t, f, "partial")
	plan, err := f.store.InspectAppendRepair(ctx)
	if err != nil {
		t.Fatal(err)
	}
	expect := AppendRepairExpect{plan.IntentSHA256, plan.ActiveSHA256}
	const attempts = 4
	start := make(chan struct{})
	results := make(chan error, attempts)
	for range attempts {
		go func() { <-start; _, err := f.store.RepairAppend(ctx, expect); results <- err }()
	}
	close(start)
	wins := 0
	for range attempts {
		if err = <-results; err == nil {
			wins++
		} else {
			code(t, err, "append_repair_unavailable")
		}
	}
	if wins != 1 {
		t.Fatalf("%d repairs completed", wins)
	}
	view, err := f.store.RecoveryStatus(ctx, "developer")
	if err != nil || view.JournalSequence != plan.JournalSequence || view.JournalSHA256 != plan.JournalSHA256 {
		t.Fatal(view, err)
	}
}

func TestAppendRepairRefusesChangedEvidence(t *testing.T) {
	for _, damage := range []string{"missing", "partial_intent", "oversized", "duplicate", "path", "bad_offset", "wrong_manifest", "wrong_fence", "changed_prefix", "different_tail", "extra_bytes", "manifest_pending", "symlink"} {
		t.Run(damage, func(t *testing.T) {
			f := newDiskFaultFixture(t, true)
			ctx := context.Background()
			path := interruptRecoveryAppend(t, f, "partial")
			plan, err := f.store.InspectAppendRepair(ctx)
			if err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(f.store.Dir, appendPendingName)
			raw := readFaultFile(t, marker)
			intent, e, err := parseAppendIntent(raw)
			if err != nil {
				t.Fatal(err)
			}
			active := readFaultFile(t, path)
			switch damage {
			case "missing":
				if err = os.Remove(marker); err != nil {
					t.Fatal(err)
				}
				_, err = f.store.RecoveryStatus(ctx, "developer")
				code(t, err, "state_corrupt")
			case "partial_intent":
				writeTestEvidence(t, f.store, appendPendingName, raw[:len(raw)/2])
			case "oversized":
				writeTestEvidence(t, f.store, appendPendingName, make([]byte, maxAppendIntentBytes+1))
			case "duplicate":
				writeTestEvidence(t, f.store, appendPendingName, bytes.Replace(raw, []byte(`"schema_version":1`), []byte(`"schema_version":1,"schema_version":1`), 1))
			case "path":
				intent.Active = "../outside"
			case "bad_offset":
				intent.Offset++
			case "wrong_manifest":
				intent.ManifestSHA256 = strings.Repeat("0", 64)
			case "wrong_fence":
				e.Owner.Fence++
				intent.Record = appendHashed(nil, e)
				active = active[:intent.Offset]
			case "changed_prefix":
				active[0] ^= 1
			case "different_tail":
				active[len(active)-1] ^= 1
			case "extra_bytes":
				active = append(active[:intent.Offset], append(append([]byte(nil), intent.Record...), '\n')...)
			case "manifest_pending":
				writeTestEvidence(t, f.store, pendingName, []byte("unresolved"))
			case "symlink":
				outside := filepath.Join(t.TempDir(), "evidence")
				if err = os.WriteFile(outside, raw, 0600); err != nil {
					t.Fatal(err)
				}
				if err = os.Remove(marker); err != nil {
					t.Fatal(err)
				}
				if err = os.Symlink(outside, marker); err != nil {
					t.Skip(err)
				}
			}
			switch damage {
			case "path", "bad_offset", "wrong_manifest", "wrong_fence":
				writeTestEvidence(t, f.store, appendPendingName, appendIntentBytes(intent))
			}
			if err = os.WriteFile(path, active, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = f.store.InspectAppendRepair(ctx); err == nil {
				t.Fatal("inspection accepted unsafe evidence")
			}
			if _, err = f.store.RepairAppend(ctx, AppendRepairExpect{plan.IntentSHA256, plan.ActiveSHA256}); err == nil {
				t.Fatal("repair accepted unsafe evidence")
			}
			if !bytes.Equal(active, readFaultFile(t, path)) {
				t.Fatal("refused repair changed journal")
			}
		})
	}
}

func TestAppendRepairProcessExitAndStaleInspection(t *testing.T) {
	f := newDiskFaultFixture(t, false)
	ctx := context.Background()
	crash := func(mode string) {
		t.Helper()
		cmd, in, _ := startReservationProcess(t, f.store, f.request, f.policy, f.archive, mode)
		if _, err := io.WriteString(in, "x"); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Wait(); err == nil || cmd.ProcessState.ExitCode() != 86 {
			t.Fatal("expected process exit during write", err)
		}
	}
	crash("crash-append")
	plan, err := f.store.InspectAppendRepair(ctx)
	if err != nil || plan.EventKind != "observe" || plan.PresentBytes == 0 {
		t.Fatal(plan, err)
	}
	crash("crash-append-repair")
	_, err = f.store.RepairAppend(ctx, AppendRepairExpect{plan.IntentSHA256, plan.ActiveSHA256})
	code(t, err, "repair_changed")
	result := repairPendingAppend(t, f.store)
	if result.JournalSHA256 != plan.JournalSHA256 || result.JournalSequence != plan.JournalSequence {
		t.Fatal("changed intended history")
	}
	f.before.JournalSequence = result.JournalSequence
	f.before.JournalSHA256 = result.JournalSHA256
	f.assertHistory(t)
}

func FuzzAppendIntent(f *testing.F) {
	b := appendHashed(nil, event{Sequence: 2, Previous: strings.Repeat("0", 64), At: stamp(testNow), Kind: "observe"})
	f.Add(appendIntentBytes(appendIntent{SchemaVersion: 1, Active: "journal.jsonl", Offset: 100, Record: b}))
	f.Add([]byte(`{"schema_version":1}`))
	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > maxAppendIntentBytes+1 {
			return
		}
		intent, e, err := parseAppendIntent(body)
		if err == nil {
			line, err := json.Marshal(e)
			if err != nil || !bytes.Equal(intent.Record, append(line, '\n')) || !bytes.Equal(body, appendIntentBytes(intent)) {
				t.Fatal("accepted noncanonical intent")
			}
		}
	})
}
