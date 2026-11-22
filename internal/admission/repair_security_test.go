package admission

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRepairProcessCrashAndConcurrentReplay(t *testing.T) {
	s, r, p, a, now, j := ownershipFixture(t)
	ctx := context.Background()
	if _, err := s.Claim(ctx, "developer", j.ID, "worker", p, a); err != nil {
		t.Fatal(err)
	}
	before, err := s.RecoveryStatus(ctx, "developer")
	if err != nil {
		t.Fatal(err)
	}
	tearPublication(t, s, "migrate", *now)
	cmd, input, _ := startReservationProcess(t, s, r, p, a, "crash-repair")
	if _, err = input.Write([]byte{'x'}); err != nil {
		t.Fatal(err)
	}
	if err = cmd.Wait(); err == nil || cmd.ProcessState.ExitCode() != 86 {
		t.Fatal("repair did not crash", err)
	}
	_, err = s.RecoveryStatus(ctx, "developer")
	code(t, err, "publication_pending")
	type child struct {
		cmd    *exec.Cmd
		input  io.WriteCloser
		output *bufio.Reader
		repair bool
	}
	var children []child
	for i := 0; i < 6; i++ {
		mode := "reserve"
		if i < 3 {
			mode = "repair"
		}
		cmd, in, out := startReservationProcess(t, s, r, p, a, mode)
		children = append(children, child{cmd, in, out, i < 3})
	}
	for _, c := range children {
		if _, err = c.input.Write([]byte{'x'}); err != nil {
			t.Fatal(err)
		}
	}
	wins := 0
	for _, c := range children {
		line, err := c.output.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		result := strings.TrimSpace(line)
		if c.repair {
			if result == "ok" {
				wins++
			} else if result != "repair_unavailable" {
				t.Fatal(result)
			}
		} else if result != "replay_detected" && result != "publication_pending" {
			t.Fatal("reservation escaped repair lock", result)
		}
		if err = c.cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	if wins != 1 {
		t.Fatalf("%d repairs won", wins)
	}
	after, err := s.RecoveryStatus(ctx, "developer")
	if err != nil || after.JournalSHA256 != before.JournalSHA256 || after.JournalSequence != before.JournalSequence || after.Reservations[0] != before.Reservations[0] {
		t.Fatal("crash/concurrency lost ownership", err)
	}
}

func pendingFixture(t *testing.T, compact bool) (Store, *time.Time, publicationIntent, journalManifest) {
	t.Helper()
	s, _, _, _, now := reservationFixture(t)
	ctx := context.Background()
	if compact {
		if _, err := s.Migrate(ctx); err != nil {
			t.Fatal(err)
		}
		*now = now.Add(time.Second)
		if _, err := s.Status(ctx, "developer", ""); err != nil {
			t.Fatal(err)
		}
	}
	action := "migrate"
	if compact {
		action = "compact"
	}
	tearPublication(t, s, action, *now)
	b, err := os.ReadFile(filepath.Join(s.Dir, pendingName))
	if err != nil {
		t.Fatal(err)
	}
	p, err := parseIntent(b)
	if err != nil {
		t.Fatal(err)
	}
	b, err = os.ReadFile(filepath.Join(s.Dir, p.PreparedManifest))
	if err != nil {
		t.Fatal(err)
	}
	m, err := parseManifest(b)
	if err != nil {
		t.Fatal(err)
	}
	return s, now, p, m
}
func writeTestEvidence(t *testing.T, s Store, name string, b []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(s.Dir, name), b, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRepairRefusesDamagedOrCompetingEvidence(t *testing.T) {
	for _, damage := range []string{"intent_partial", "intent_duplicate", "intent_unknown", "intent_path", "intent_hash", "intent_copy", "source", "source_active", "source_missing", "target", "target_path", "target_lineage", "archive", "active", "control_bitflip", "competitor_manifest", "competitor_archive", "competitor_active", "directory_limit", "before_image", "pending_link", "target_link"} {
		t.Run(damage, func(t *testing.T) {
			s, _, p, m := pendingFixture(t, true)
			ctx := context.Background()
			plan, err := s.InspectRepair(ctx)
			if err != nil {
				t.Fatal(err)
			}
			switch damage {
			case "intent_partial":
				writeTestEvidence(t, s, pendingName, []byte("{"))
			case "intent_duplicate":
				writeTestEvidence(t, s, pendingName, bytes.Replace(intentBytes(p), []byte(`"schema_version":1`), []byte(`"schema_version":1,"schema_version":1`), 1))
			case "intent_unknown":
				writeTestEvidence(t, s, pendingName, bytes.Replace(intentBytes(p), []byte(`"schema_version":1`), []byte(`"schema_version":1,"extra":1`), 1))
			case "intent_path":
				p.SourceFile = "../escape"
				writeTestEvidence(t, s, pendingName, intentBytes(p))
			case "intent_hash":
				p.Hash = strings.Repeat("0", 64)
				b := intentBytes(p)
				b = bytes.Replace(b, []byte(`"head_sha256":"`), []byte(`"head_sha256":"0`), 1)
				writeTestEvidence(t, s, pendingName, b)
			case "intent_copy":
				writeTestEvidence(t, s, "publication-"+p.OperationID+".intent.json", []byte("{}"))
			case "source":
				writeTestEvidence(t, s, p.SourceFile, []byte("{}"))
			case "source_active":
				writeTestEvidence(t, s, p.SourceActiveName, []byte("changed"))
			case "source_missing":
				if err = os.Remove(filepath.Join(s.Dir, p.SourceActiveName)); err != nil {
					t.Fatal(err)
				}
			case "target":
				writeTestEvidence(t, s, p.PreparedManifest, []byte("{}"))
			case "target_path":
				m.Active = "../escape"
				writeTestEvidence(t, s, p.PreparedManifest, manifestBytes(m))
			case "target_lineage":
				m.Previous = strings.Repeat("0", 64)
				body := manifestBytes(m)
				m, _ = parseManifest(body)
				p.ManifestSHA256 = m.Hash
				writeTestEvidence(t, s, p.PreparedManifest, body)
				writeTestEvidence(t, s, pendingName, intentBytes(p))
				writeTestEvidence(t, s, "publication-"+p.OperationID+".intent.json", intentBytes(p))
			case "archive":
				writeTestEvidence(t, s, m.Segments[0].Name, []byte("bad"))
			case "active":
				writeTestEvidence(t, s, m.Active, []byte("{"))
			case "control_bitflip":
				writeTestEvidence(t, s, "journal.jsonl", []byte("x"))
			case "competitor_manifest":
				writeTestEvidence(t, s, "manifest-"+strings.Repeat("0", 32)+".json", manifestBytes(m))
			case "competitor_archive":
				writeTestEvidence(t, s, "segment-"+strings.Repeat("0", 32)+".jsonl.gz", nil)
			case "competitor_active":
				writeTestEvidence(t, s, "active-"+strings.Repeat("0", 32)+".jsonl", nil)
			case "directory_limit":
				for i := 0; i < 257; i++ {
					writeTestEvidence(t, s, fmt.Sprintf("extra-%d", i), nil)
				}
			case "before_image":
				writeTestEvidence(t, s, "repair-control-"+plan.ControlSHA256+".bin", []byte("wrong"))
			case "pending_link", "target_link":
				name := pendingName
				if damage == "target_link" {
					name = p.PreparedManifest
				}
				path := filepath.Join(s.Dir, name)
				b, _ := os.ReadFile(path)
				outside := filepath.Join(t.TempDir(), "evidence")
				if err = os.WriteFile(outside, b, 0600); err != nil {
					t.Fatal(err)
				}
				if err = os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err = os.Symlink(outside, path); err != nil {
					t.Skip(err)
				}
			}
			before, _ := os.ReadFile(filepath.Join(s.Dir, "journal.jsonl"))
			if _, err = s.Repair(ctx, repairExpectation(plan)); err == nil {
				t.Fatal("unsafe repair accepted")
			}
			after, _ := os.ReadFile(filepath.Join(s.Dir, "journal.jsonl"))
			if !bytes.Equal(before, after) {
				t.Fatal("refused repair modified control")
			}
			if _, err = os.Lstat(filepath.Join(s.Dir, pendingName)); err != nil {
				t.Fatal("pending evidence removed")
			}
		})
	}
}

func TestRepairPrefixesAndRepeatedUncertainty(t *testing.T) {
	for _, length := range []int{0, 1, 17, -1, -2} {
		t.Run(fmt.Sprint(length), func(t *testing.T) {
			s, _, _, m := pendingFixture(t, false)
			body := manifestBytes(m)
			n := length
			if n == -1 {
				n = len(body) - 1
			}
			if n == -2 {
				n = len(body)
			}
			writeTestEvidence(t, s, "journal.jsonl", body[:n])
			plan, err := s.InspectRepair(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			result, err := s.Repair(context.Background(), repairExpectation(plan))
			if err != nil || result.ControlRewritten != (n < len(body)) {
				t.Fatalf("%+v %v", result, err)
			}
			_, err = s.Repair(context.Background(), repairExpectation(plan))
			code(t, err, "repair_unavailable")
		})
	}
	for _, mode := range []string{"truncate", "partial", "sync", "cancel_after_sync"} {
		t.Run(mode, func(t *testing.T) {
			s, now, _, _ := pendingFixture(t, false)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tx, err := s.openLocked(ctx)
			if err != nil {
				t.Fatal(err)
			}
			proof, err := tx.proveRepair(ctx, *now)
			if err != nil {
				t.Fatal(err)
			}
			f := publicationFailure{journalFile: tx.file, truncate: mode == "truncate", partial: mode == "partial", sync: mode == "sync"}
			if mode == "cancel_after_sync" {
				f.cancel = cancel
			}
			tx.file = f
			_, err = tx.repair(ctx, proof, repairExpectation(proof.report))
			tx.close()
			if mode == "cancel_after_sync" {
				if err != nil || ctx.Err() == nil {
					t.Fatal("committed repair lost", err)
				}
				return
			}
			code(t, err, "commit_uncertain")
			plan, err := s.InspectRepair(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Repair(context.Background(), repairExpectation(plan)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRepairClockCancellationAndPendingFreeze(t *testing.T) {
	s, now, _, _ := pendingFixture(t, false)
	ctx := context.Background()
	*now = now.Add(-time.Second)
	plan, err := s.InspectRepair(ctx)
	if err != nil || !plan.ClockRollback {
		t.Fatal("rollback not reported", err)
	}
	_, err = s.Repair(ctx, repairExpectation(plan))
	code(t, err, "clock_rollback")
	*now = now.Add(time.Second)
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = s.Repair(cancelledCtx, repairExpectation(plan))
	code(t, err, "cancelled")
	_, err = s.Migrate(ctx)
	code(t, err, "publication_pending")
	_, err = s.Compact(ctx)
	code(t, err, "publication_pending")
	_, err = s.Status(ctx, "developer", "")
	code(t, err, "publication_pending")
	// Even a valid old control is frozen while intent is pending.
	b, _ := os.ReadFile(filepath.Join(s.Dir, pendingName))
	p, _ := parseIntent(b)
	source, _ := os.ReadFile(filepath.Join(s.Dir, p.SourceFile))
	writeTestEvidence(t, s, "journal.jsonl", source)
	_, err = s.Status(ctx, "developer", "")
	code(t, err, "publication_pending")
}

func TestRepairPublishedTargetPreservesLaterEvents(t *testing.T) {
	s, now, _, m := pendingFixture(t, false)
	ctx := context.Background()
	writeTestEvidence(t, s, "journal.jsonl", manifestBytes(m))
	// Simulate an older binary that understands v2 but ignores the pending marker.
	root, err := os.OpenRoot(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	tx := &transaction{root: root}
	if err = tx.loadManifest(ctx, m); err != nil {
		t.Fatal(err)
	}
	tx.fileName = m.Active
	tx.lock, err = rootedFile(root, "lock")
	if err != nil {
		t.Fatal(err)
	}
	if err = lockFile(ctx, tx.lock); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Second)
	if err = tx.observe(ctx, *now); err != nil {
		t.Fatal(err)
	}
	head := tx.state.hash
	seq := tx.state.sequence
	tx.close()
	plan, err := s.InspectRepair(ctx)
	if err != nil || plan.JournalSHA256 != head || plan.JournalSequence != seq {
		t.Fatal("newer head lost", err)
	}
	result, err := s.Repair(ctx, repairExpectation(plan))
	if err != nil || result.ControlRewritten {
		t.Fatal("published target rewritten", err)
	}
	after, err := s.RecoveryStatus(ctx, "developer")
	if err != nil || after.JournalSHA256 != head {
		t.Fatal("history lost", err)
	}
}

func FuzzPublicationIntent(f *testing.F) {
	p := publicationIntent{SchemaVersion: 1, OperationID: strings.Repeat("a", 32), PreparedManifest: "manifest-" + strings.Repeat("a", 32) + ".json", ManifestSHA256: strings.Repeat("b", 64), SourceFile: "publication-source-" + strings.Repeat("a", 32) + ".bin", SourceSHA256: strings.Repeat("b", 64), SourceActiveName: "journal.jsonl", SourceActiveSHA256: strings.Repeat("b", 64), SourceActiveBytes: 1, Sequence: 1, Head: strings.Repeat("b", 64), RecordedAt: stamp(testNow)}
	f.Add(intentBytes(p))
	f.Add([]byte("{}"))
	f.Add([]byte("{"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > maxIntentBytes+1 {
			return
		}
		if p, err := parseIntent(b); err == nil {
			if !bytes.Equal(b, intentBytes(p)) || p.Sequence < 1 || p.Sequence > maxHistoryEvents || p.SourceActiveBytes > maxJournalBytes {
				t.Fatal("invalid intent accepted")
			}
		}
	})
}

func TestRepairResyncsExistingBeforeImage(t *testing.T) {
	s, _, _, _ := pendingFixture(t, false)
	ctx := context.Background()
	plan, err := s.InspectRepair(ctx)
	if err != nil {
		t.Fatal(err)
	}
	control, err := os.ReadFile(filepath.Join(s.Dir, "journal.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	name := "repair-control-" + plan.ControlSHA256 + ".bin"
	writeTestEvidence(t, s, name, control)
	f, err := os.OpenFile(filepath.Join(s.Dir, name), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Identical readable bytes may still only be in cache after a failed sync.
	err = syncMatchingEvidence(publicationFailure{journalFile: f, sync: true}, control)
	code(t, err, "state_io")
	f.Close()
	if _, err = s.Repair(ctx, repairExpectation(plan)); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(filepath.Join(s.Dir, name))
	if err != nil || !bytes.Equal(saved, control) {
		t.Fatal("before-image lost", err)
	}
}

func TestRepairResyncsPendingIntent(t *testing.T) {
	s, _, p, _ := pendingFixture(t, false)
	ctx := context.Background()
	// Model a complete marker whose writer exited before Sync. The source
	// control is still untouched at that point in publication.
	source, err := os.ReadFile(filepath.Join(s.Dir, p.SourceFile))
	if err != nil {
		t.Fatal(err)
	}
	writeTestEvidence(t, s, "journal.jsonl", source)
	f, err := os.OpenFile(filepath.Join(s.Dir, pendingName), os.O_RDWR|os.O_TRUNC, 0)
	if err != nil {
		t.Fatal(err)
	}
	body := intentBytes(p)
	if _, err = f.Write(body); err != nil {
		t.Fatal(err)
	}
	f.Close()
	plan, err := s.InspectRepair(ctx)
	if err != nil || plan.ControlState != "source" {
		t.Fatal(err)
	}
	f, err = os.OpenFile(filepath.Join(s.Dir, pendingName), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	err = syncMatchingEvidence(publicationFailure{journalFile: f, sync: true}, body)
	code(t, err, "state_io")
	f.Close()
	if _, err = s.Repair(ctx, repairExpectation(plan)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecoveryStatus(ctx, "developer"); err != nil {
		t.Fatal(err)
	}
}
