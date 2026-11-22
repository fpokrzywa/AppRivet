package admission

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

type failingJournal struct {
	journalFile
	partial   bool
	syncError bool
	cancel    context.CancelFunc
}

func (f failingJournal) Write(b []byte) (int, error) {
	if f.partial {
		n, _ := f.journalFile.Write(b[:len(b)/2])
		return n, errors.New("injected write failure")
	}
	return f.journalFile.Write(b)
}
func (f failingJournal) Sync() error {
	if f.syncError {
		return errors.New("injected sync failure")
	}
	err := f.journalFile.Sync()
	if f.cancel != nil {
		f.cancel()
	}
	return err
}

func TestJournalCommitUncertaintyAndCancellationBoundary(t *testing.T) {
	for _, mode := range []string{"partial", "sync", "cancel_after_sync"} {
		t.Run(mode, func(t *testing.T) {
			s, _, _, _, _ := reservationFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tx, err := s.begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			wrapper := failingJournal{journalFile: tx.file, partial: mode == "partial", syncError: mode == "sync"}
			if mode == "cancel_after_sync" {
				wrapper.cancel = cancel
			}
			tx.file = wrapper
			err = tx.observe(ctx, testNow.Add(time.Second))
			tx.close()
			if mode == "cancel_after_sync" {
				if err != nil || ctx.Err() == nil {
					t.Fatalf("committed write lost to cancellation: %v", err)
				}
			} else {
				code(t, err, "commit_uncertain")
			}
			s.Clock = func() time.Time { return testNow.Add(time.Second) }
			if mode != "cancel_after_sync" {
				_, err = s.Status(context.Background(), "developer", "")
				code(t, err, "append_pending")
				repairPendingAppend(t, s)
			}
			if _, err = s.Status(context.Background(), "developer", ""); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestJournalCorruptionCanonicalAndBounded(t *testing.T) {
	s, r, p, a, _ := reservationFixture(t)
	if _, err := s.Reserve(context.Background(), "developer", p, r, a); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(s.Dir, "journal.jsonl"))
	for name, bad := range map[string][]byte{"truncated": b[:len(b)-1], "trailing": append(append([]byte(nil), b...), '\n'), "unknown": bytes.Replace(b, []byte(`"kind":"reserve"`), []byte(`"kind":"reserve","unknown":1`), 1), "duplicate": bytes.Replace(b, []byte(`"kind":"reserve"`), []byte(`"kind":"reserve","kind":"reserve"`), 1), "hash": bytes.Replace(b, []byte(`"principal":"developer"`), []byte(`"principal":"attacker"`), 1), "large": make([]byte, maxJournalBytes+1)} {
		t.Run(name, func(t *testing.T) {
			if _, err := replay(bad); err == nil {
				t.Fatal("corrupt journal accepted")
			}
		})
	}
	// Correctly rehashed but semantically invalid transitions must also fail.
	lines := bytes.Split(bytes.TrimSuffix(b, []byte{'\n'}), []byte{'\n'})
	for _, mutation := range []func(*event){func(e *event) { e.Job.ReservedBytes = -1 }, func(e *event) { e.Job.ExpiresAt = e.At }, func(e *event) { e.Job.Status = "released" }, func(e *event) { e.At = stamp(testNow.Add(-time.Hour)) }, func(e *event) { e.Sequence++ }} {
		var e event
		json.Unmarshal(lines[len(lines)-1], &e)
		mutation(&e)
		e.Hash = ""
		raw, _ := json.Marshal(e)
		e.Hash = sum(raw)
		raw, _ = json.Marshal(e)
		bad := append(bytes.Join(lines[:len(lines)-1], []byte{'\n'}), '\n')
		bad = append(bad, raw...)
		bad = append(bad, '\n')
		if _, err := replay(bad); err == nil {
			t.Fatal("invalid transition accepted")
		}
	}
}

func TestJournalFullAndReplacementFailClosed(t *testing.T) {
	s, _, _, _, _ := reservationFixture(t)
	ctx := context.Background()
	tx, err := s.begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.close()
	original, _ := os.ReadFile(filepath.Join(s.Dir, "journal.jsonl"))
	tx.size = maxJournalBytes
	code(t, tx.observe(ctx, testNow.Add(time.Second)), "state_full")
	tx.size = int64(len(original))
	tx.activeEvents = maxEvents
	code(t, tx.observe(ctx, testNow.Add(time.Second)), "state_full")
	got, _ := os.ReadFile(filepath.Join(s.Dir, "journal.jsonl"))
	if !bytes.Equal(original, got) {
		t.Fatal("full state mutated")
	}
	tx.activeEvents = 1
	// Replacement is tested where renaming an open file is supported.
	path := filepath.Join(s.Dir, "journal.jsonl")
	if err = os.Rename(path, path+".old"); err != nil {
		t.Skip(err)
	}
	os.WriteFile(path, original, 0600)
	code(t, tx.observe(ctx, testNow.Add(time.Second)), "state_io")
}

func TestJournalPOSIXModesAndMissingLock(t *testing.T) {
	s, _, _, _, _ := reservationFixture(t)
	if runtime.GOOS != "windows" {
		for _, name := range []string{".", "lock", "journal.jsonl"} {
			info, err := os.Stat(filepath.Join(s.Dir, name))
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm()&0077 != 0 {
				t.Fatalf("broad permissions %s %o", name, info.Mode().Perm())
			}
		}
	}
	if err := os.Remove(filepath.Join(s.Dir, "lock")); err != nil {
		t.Fatal(err)
	}
	_, err := s.Status(context.Background(), "developer", "")
	code(t, err, "state_io")
	if _, err = os.Stat(filepath.Join(s.Dir, "lock")); !os.IsNotExist(err) {
		t.Fatal("missing lock recreated")
	}
}

func FuzzReservationJournal(f *testing.F) {
	s, r, p, a, _ := reservationFixture(f)
	reserved, err := s.Reserve(context.Background(), "developer", p, r, a)
	if err != nil {
		f.Fatal(err)
	}
	seed, _ := os.ReadFile(filepath.Join(s.Dir, "journal.jsonl"))
	f.Add(seed)
	claimed, err := s.Claim(context.Background(), "developer", reserved.Reservations[0].ID, "fuzz-worker", p, a)
	if err != nil {
		f.Fatal(err)
	}
	seed, _ = os.ReadFile(filepath.Join(s.Dir, "journal.jsonl"))
	f.Add(seed)
	if _, err = s.Recover(context.Background(), "developer", ownerOf(claimed.Reservations[0]), true); err != nil {
		f.Fatal(err)
	}
	seed, _ = os.ReadFile(filepath.Join(s.Dir, "journal.jsonl"))
	f.Add(seed)
	f.Add([]byte("{}\n"))
	f.Add([]byte("null\n"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<20 {
			return
		}
		state, err := replay(b)
		if err == nil && (state.sequence < 1 || !state.config.valid() || len(state.jobs) > maxReservations) {
			t.Fatal("invalid state accepted")
		}
	})
}
