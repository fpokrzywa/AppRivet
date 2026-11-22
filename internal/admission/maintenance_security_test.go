package admission

import (
	"bufio"
	"bytes"
	"compress/gzip"
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

func TestMaintenanceSerializesWithCrossProcessReservations(t *testing.T) {
	for _, mode := range []string{"migrate", "compact"} {
		t.Run(mode, func(t *testing.T) {
			s, r, p, a, _ := reservationFixture(t)
			ctx := context.Background()
			if mode == "compact" {
				if _, err := s.Migrate(ctx); err != nil {
					t.Fatal(err)
				}
			}
			type child struct {
				cmd         *exec.Cmd
				input       io.WriteCloser
				output      *bufio.Reader
				maintenance bool
			}
			children := []child{}
			for i := 0; i < 6; i++ {
				action := "reserve"
				if i < 2 {
					action = mode
				}
				cmd, input, output := startReservationProcess(t, s, r, p, a, action)
				children = append(children, child{cmd, input, output, i < 2})
			}
			for _, c := range children {
				if _, err := c.input.Write([]byte{'x'}); err != nil {
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
				if c.maintenance {
					if result != "ok" {
						t.Fatal(result)
					}
				} else if result == "ok" {
					wins++
				} else if result != "replay_detected" {
					t.Fatal(result)
				}
				if err = c.cmd.Wait(); err != nil {
					t.Fatal(err)
				}
			}
			if wins != 1 {
				t.Fatalf("%d reservations won", wins)
			}
			report, err := s.RecoveryStatus(ctx, "developer")
			if err != nil || len(report.Reservations) != 1 {
				t.Fatalf("%+v %v", report, err)
			}
			_, err = s.Reserve(ctx, "developer", p, r, a)
			code(t, err, "replay_detected")
		})
	}
}

func fillSegment(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()
	tx, err := s.begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Build valid observe events in memory, then persist once for efficient tests.
	if _, err = tx.file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(s.Dir, tx.fileName))
	if err != nil {
		t.Fatal(err)
	}
	state := tx.state
	for tx.activeEvents < maxEvents {
		e := event{Sequence: state.sequence + 1, Previous: state.hash, At: stamp(state.at), Kind: "observe"}
		raw, _ := json.Marshal(e)
		e.Hash = sum(raw)
		raw, _ = json.Marshal(e)
		if err = state.apply(e); err != nil {
			t.Fatal(err)
		}
		b = append(b, raw...)
		b = append(b, '\n')
		tx.activeEvents++
	}
	if err = tx.file.Truncate(0); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.file.Write(b); err != nil {
		t.Fatal(err)
	}
	if err = tx.file.Sync(); err != nil {
		t.Fatal(err)
	}
	tx.close()
}

func TestFullJournalMigrationAndCompactionPreserveGlobalFences(t *testing.T) {
	s, r, p, a, now, j := ownershipFixture(t)
	ctx := context.Background()
	fillSegment(t, s)
	*now = now.Add(time.Second)
	_, err := s.Status(ctx, "developer", "")
	code(t, err, "state_full")
	if _, err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := s.Claim(ctx, "developer", j.ID, "worker", p, a)
	if err != nil {
		t.Fatal(err)
	}
	owner := ownerOf(got.Reservations[0])
	if owner.Fence <= maxEvents {
		t.Fatal("fence reset")
	}
	fillSegment(t, s)
	*now = now.Add(time.Second)
	_, err = s.Recover(ctx, "developer", owner, true)
	code(t, err, "state_full")
	if _, err = s.Compact(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CheckOwner(ctx, "developer", owner, p, a); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Recover(ctx, "developer", owner, true); err != nil {
		t.Fatal(err)
	}
	_, err = s.Reserve(ctx, "developer", p, r, a)
	code(t, err, "replay_detected")
	if _, err = s.Compact(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = s.CheckOwner(ctx, "developer", owner, p, a)
	code(t, err, "ownership_conflict")
}

func TestManifestAndArchiveCorruptionNeverFallsBack(t *testing.T) {
	for _, damage := range []string{"manifest_partial", "manifest_unknown", "manifest_duplicate", "path", "archive_missing", "archive_tamper", "archive_link", "active_missing", "active_partial", "head", "reorder"} {
		t.Run(damage, func(t *testing.T) {
			s, _, _, _, now := reservationFixture(t)
			ctx := context.Background()
			if _, err := s.Migrate(ctx); err != nil {
				t.Fatal(err)
			}
			*now = now.Add(time.Second)
			if _, err := s.Status(ctx, "developer", ""); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Compact(ctx); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(s.Dir, "journal.jsonl")
			original, _ := os.ReadFile(path)
			m, err := parseManifest(original)
			if err != nil {
				t.Fatal(err)
			}
			switch damage {
			case "manifest_partial":
				os.WriteFile(path, original[:len(original)/2], 0600)
			case "manifest_unknown":
				os.WriteFile(path, bytes.Replace(original, []byte(`"schema_version":2`), []byte(`"schema_version":2,"unknown":1`), 1), 0600)
			case "manifest_duplicate":
				os.WriteFile(path, bytes.Replace(original, []byte(`"schema_version":2`), []byte(`"schema_version":2,"schema_version":2`), 1), 0600)
			case "path":
				m.Active = "../escape"
				os.WriteFile(path, manifestBytes(m), 0600)
			case "head":
				m.Head = strings.Repeat("0", 64)
				os.WriteFile(path, manifestBytes(m), 0600)
			case "reorder":
				m.Segments[0], m.Segments[1] = m.Segments[1], m.Segments[0]
				os.WriteFile(path, manifestBytes(m), 0600)
			case "archive_missing":
				os.Remove(filepath.Join(s.Dir, m.Segments[0].Name))
			case "archive_tamper":
				os.WriteFile(filepath.Join(s.Dir, m.Segments[0].Name), []byte("bad"), 0600)
			case "archive_link":
				outside := filepath.Join(t.TempDir(), "outside")
				os.WriteFile(outside, []byte("sentinel"), 0600)
				name := filepath.Join(s.Dir, m.Segments[0].Name)
				os.Remove(name)
				if err = os.Symlink(outside, name); err != nil {
					t.Skip(err)
				}
			case "active_missing":
				os.Remove(filepath.Join(s.Dir, m.Active))
			case "active_partial":
				os.WriteFile(filepath.Join(s.Dir, m.Active), []byte("{"), 0600)
			}
			if _, err = s.RecoveryStatus(ctx, "developer"); err == nil {
				t.Fatal("bad state accepted or older manifest used")
			}
			if _, err = s.Compact(ctx); err == nil {
				t.Fatal("bad history compacted")
			}
		})
	}
}

func gzipBytes(b []byte) []byte {
	var out bytes.Buffer
	z := gzip.NewWriter(&out)
	z.Write(b)
	z.Close()
	return out.Bytes()
}
func TestSegmentEncodingAndExpansionBounds(t *testing.T) {
	data := []byte("bounded data\n")
	valid := gzipBytes(data)
	for _, packed := range [][]byte{append(append([]byte(nil), valid...), valid...), append(append([]byte(nil), valid...), 'x'), valid[:len(valid)-2], gzipBytes(bytes.Repeat([]byte{'a'}, 1<<20))} {
		d := historySegment{Bytes: int64(len(data)), SHA256: sum(data), CompressedBytes: int64(len(packed)), CompressedSHA256: sum(packed)}
		if _, err := unpackSegment(packed, d); err == nil {
			t.Fatal("concatenated/trailing/truncated/oversized gzip accepted")
		}
	}
	for _, size := range []int64{-1, 0, maxJournalBytes + 1} {
		d := historySegment{Bytes: size, CompressedBytes: int64(len(valid)), CompressedSHA256: sum(valid)}
		if _, err := unpackSegment(valid, d); err == nil {
			t.Fatal("invalid expansion bound accepted")
		}
	}
}

type publicationFailure struct {
	journalFile
	truncate bool
	partial  bool
	sync     bool
	cancel   context.CancelFunc
}

type crashPublication struct{ journalFile }

func (f crashPublication) Write(b []byte) (int, error) {
	_, _ = f.journalFile.Write(b[:len(b)/2])
	_ = f.journalFile.Sync()
	os.Exit(86)
	return 0, nil
}

func TestProcessCrashDuringPublicationFailsClosed(t *testing.T) {
	s, r, p, a, _ := reservationFixture(t)
	before, err := s.RecoveryStatus(context.Background(), "developer")
	if err != nil {
		t.Fatal(err)
	}
	cmd, input, _ := startReservationProcess(t, s, r, p, a, "crash-migrate")
	if _, err = input.Write([]byte{'x'}); err != nil {
		t.Fatal(err)
	}
	if err = cmd.Wait(); err == nil || cmd.ProcessState.ExitCode() != 86 {
		t.Fatalf("expected crash exit, got %v", err)
	}
	_, err = s.RecoveryStatus(context.Background(), "developer")
	code(t, err, "publication_pending")
	prepared, err := filepath.Glob(filepath.Join(s.Dir, "manifest-*.json"))
	if err != nil || len(prepared) != 1 {
		t.Fatalf("prepared evidence missing %v %v", prepared, err)
	}
	body, _ := os.ReadFile(prepared[0])
	m, err := parseManifest(body)
	if err != nil || m.Head != before.JournalSHA256 || m.SealedSequence != before.JournalSequence {
		t.Fatal("prepared history lost", err)
	}
	packed, _ := os.ReadFile(filepath.Join(s.Dir, m.Segments[0].Name))
	original, err := unpackSegment(packed, m.Segments[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err = replay(original); err != nil {
		t.Fatal(err)
	}
	plan, err := s.InspectRepair(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Repair(context.Background(), repairExpectation(plan)); err != nil {
		t.Fatal(err)
	}
	after, err := s.RecoveryStatus(context.Background(), "developer")
	if err != nil || after.JournalSHA256 != before.JournalSHA256 || after.JournalSequence != before.JournalSequence {
		t.Fatal("crash repair changed history", err)
	}
}

func (f publicationFailure) Truncate(n int64) error {
	if f.truncate {
		return errors.New("injected truncate failure")
	}
	return f.journalFile.Truncate(n)
}
func (f publicationFailure) Write(b []byte) (int, error) {
	if f.partial {
		n, _ := f.journalFile.Write(b[:len(b)/2])
		return n, errors.New("injected partial manifest")
	}
	return f.journalFile.Write(b)
}
func (f publicationFailure) Sync() error {
	if f.sync {
		return errors.New("injected manifest sync")
	}
	err := f.journalFile.Sync()
	if f.cancel != nil {
		f.cancel()
	}
	return err
}
func TestMaintenancePublicationFaultsAndCommitCancellation(t *testing.T) {
	for _, op := range []string{"migrate", "compact"} {
		for _, mode := range []string{"truncate", "partial", "sync", "cancel_after_sync"} {
			t.Run(op+"/"+mode, func(t *testing.T) {
				s, _, _, _, now := reservationFixture(t)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if op == "compact" {
					if _, err := s.Migrate(ctx); err != nil {
						t.Fatal(err)
					}
					*now = now.Add(time.Second)
					if _, err := s.Status(ctx, "developer", ""); err != nil {
						t.Fatal(err)
					}
				}
				before, err := s.RecoveryStatus(ctx, "developer")
				if err != nil {
					t.Fatal(err)
				}
				tx, err := s.begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				control := tx.file
				if tx.control != nil {
					control = tx.control
				}
				fault := publicationFailure{journalFile: control, truncate: mode == "truncate", partial: mode == "partial", sync: mode == "sync"}
				if mode == "cancel_after_sync" {
					fault.cancel = cancel
				}
				if tx.control == nil {
					tx.file = fault
				} else {
					tx.control = fault
				}
				_, err = tx.maintain(ctx, *now, op)
				tx.close()
				if mode == "cancel_after_sync" {
					if err != nil || ctx.Err() == nil {
						t.Fatalf("durable maintenance lost %v", err)
					}
				} else {
					code(t, err, "commit_uncertain")
				}
				after, err := s.RecoveryStatus(context.Background(), "developer")
				if mode != "cancel_after_sync" {
					code(t, err, "publication_pending")
					plan, inspectErr := s.InspectRepair(context.Background())
					if inspectErr != nil {
						t.Fatal(inspectErr)
					}
					if _, repairErr := s.Repair(context.Background(), repairExpectation(plan)); repairErr != nil {
						t.Fatal(repairErr)
					}
					after, err = s.RecoveryStatus(context.Background(), "developer")
					if err != nil || before.JournalSHA256 != after.JournalSHA256 || before.JournalSequence != after.JournalSequence {
						t.Fatal("repair lost history", err)
					}
				} else if err != nil || before.JournalSHA256 != after.JournalSHA256 || before.JournalSequence != after.JournalSequence {
					t.Fatalf("history lost %+v %v", after, err)
				}
				prepared, _ := filepath.Glob(filepath.Join(s.Dir, "manifest-*.json"))
				if len(prepared) == 0 {
					t.Fatal("uncertain publication discarded prepared evidence")
				}
			})
		}
	}
}

func TestMaintenanceBoundsAndClock(t *testing.T) {
	s, _, _, _, now := reservationFixture(t)
	ctx := context.Background()
	if _, err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	unchanged, err := s.Compact(ctx)
	if err != nil || unchanged.Changed {
		t.Fatalf("empty segment should not create generation %+v %v", unchanged, err)
	}
	*now = now.Add(-time.Second)
	_, err = s.Compact(ctx)
	code(t, err, "clock_rollback")
	*now = testNow
	for i := 1; i < maxSegments; i++ {
		*now = now.Add(time.Second)
		if _, err = s.Status(ctx, "developer", ""); err != nil {
			t.Fatal(err)
		}
		if _, err = s.Compact(ctx); err != nil {
			t.Fatal(err)
		}
	}
	*now = now.Add(time.Second)
	if _, err = s.Status(ctx, "developer", ""); err != nil {
		t.Fatal(err)
	}
	_, err = s.Compact(ctx)
	code(t, err, "state_full")
	report, err := s.RecoveryStatus(ctx, "developer")
	if err != nil || report.JournalSequence != maxSegments+1 {
		t.Fatalf("history evicted %+v %v", report, err)
	}
}

func FuzzJournalStorage(f *testing.F) {
	s, _, _, _, _ := reservationFixture(f)
	if _, err := s.Migrate(context.Background()); err != nil {
		f.Fatal(err)
	}
	manifest, _ := os.ReadFile(filepath.Join(s.Dir, "journal.jsonl"))
	m, _ := parseManifest(manifest)
	packed, _ := os.ReadFile(filepath.Join(s.Dir, m.Segments[0].Name))
	f.Add(manifest, packed, m.Segments[0].Bytes)
	f.Add([]byte("{}"), []byte("bad"), int64(-1))
	f.Fuzz(func(t *testing.T, b, compressed []byte, n int64) {
		if len(b) > maxManifestBytes+1 || len(compressed) > 1<<20 {
			return
		}
		if m, err := parseManifest(b); err == nil && (m.SchemaVersion != 2 || m.Generation != len(m.Segments)) {
			t.Fatal("invalid manifest accepted")
		}
		// Keep fuzz allocation bounded independently of hostile declared lengths.
		if n > 65536 {
			return
		}
		d := historySegment{Bytes: n, SHA256: strings.Repeat("0", 64), CompressedBytes: int64(len(compressed)), CompressedSHA256: sum(compressed)}
		_, _ = unpackSegment(compressed, d)
	})
}

func TestMaintenanceNoHistoryLimitReset(t *testing.T) {
	s, _, _, _, _ := reservationFixture(t)
	ctx := context.Background()
	if _, err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	tx, err := s.begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.close()
	tx.state.sequence = maxHistoryEvents
	code(t, tx.observe(ctx, testNow.Add(time.Second)), "state_full")
	tx.state.sequence = 1
	tx.historyBytes = maxHistoryBytes
	code(t, tx.observe(ctx, testNow.Add(time.Second)), "state_full")
	// Manifest integer bounds are validated before reading any named file.
	raw, _ := os.ReadFile(filepath.Join(s.Dir, "journal.jsonl"))
	m, _ := parseManifest(raw)
	for i, mutate := range []func(*journalManifest){func(m *journalManifest) { m.Generation = maxSegments + 1 }, func(m *journalManifest) { m.SealedSequence = maxHistoryEvents + 1 }, func(m *journalManifest) { m.Segments[0].Bytes = maxJournalBytes + 1 }, func(m *journalManifest) { m.Segments[0].CompressedBytes = maxCompressedBytes + 1 }} {
		candidate := m
		candidate.Segments = append([]historySegment(nil), m.Segments...)
		mutate(&candidate)
		if _, err := parseManifest(manifestBytes(candidate)); err == nil {
			t.Fatal(fmt.Sprint("invalid manifest limit ", i))
		}
	}
}
