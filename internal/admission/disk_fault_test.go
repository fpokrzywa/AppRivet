package admission

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

type diskFaultFile struct {
	journalFile
	mode string
}

func (f diskFaultFile) Write(b []byte) (int, error) {
	switch f.mode {
	case "zero":
		return 0, syscall.ENOSPC
	case "partial", "short":
		n, err := f.journalFile.Write(b[:len(b)/2])
		if err == nil && f.mode == "partial" {
			err = syscall.ENOSPC
		}
		return n, err
	case "exit":
		if _, err := f.journalFile.Write(b[:len(b)/2]); err != nil || f.journalFile.Sync() != nil {
			os.Exit(85)
		}
		os.Exit(86)
	}
	return f.journalFile.Write(b)
}

func (f diskFaultFile) Sync() error {
	if f.mode == "sync" {
		return syscall.ENOSPC
	}
	return f.journalFile.Sync()
}

func injectStageFault(tx *transaction, match func(string) bool, mode string) {
	tx.openStage = func(name string) (journalFile, error) {
		if match(name) && mode == "create" {
			return nil, syscall.ENOSPC
		}
		f, err := tx.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if err != nil {
			return nil, err
		}
		if match(name) {
			return diskFaultFile{f, mode}, nil
		}
		return f, nil
	}
}

type diskFaultFixture struct {
	store           Store
	request         Request
	policy, archive string
	clock           *time.Time
	owner           Ownership
	before          RecoveryReport
}

func newDiskFaultFixture(t *testing.T, compact bool) diskFaultFixture {
	t.Helper()
	s, req, policy, archive, now, j := ownershipFixture(t)
	ctx := context.Background()
	if compact {
		if _, err := s.Migrate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	claimed, err := s.Claim(ctx, "developer", j.ID, "worker", policy, archive)
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.RecoveryStatus(ctx, "developer")
	if err != nil {
		t.Fatal(err)
	}
	return diskFaultFixture{s, req, policy, archive, now, ownerOf(claimed.Reservations[0]), before}
}

func (f diskFaultFixture) assertHistory(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	after, err := f.store.RecoveryStatus(ctx, "developer")
	if err != nil || !reflect.DeepEqual(after, f.before) {
		t.Fatalf("journal or ownership changed: %+v, %v", after, err)
	}
	_, err = f.store.Reserve(ctx, "developer", f.policy, f.request, f.archive)
	code(t, err, "replay_detected")
	if _, err = f.store.CheckOwner(ctx, "developer", f.owner, f.policy, f.archive); err != nil {
		t.Fatal(err)
	}
}

func TestDiskFullStagingPreservesAuthority(t *testing.T) {
	for _, action := range []string{"migrate", "compact"} {
		for _, stage := range []string{"segment-", "active-", "manifest-", "publication-source-", "intent", "marker"} {
			for _, mode := range []string{"create", "partial", "short", "sync"} {
				if stage == "active-" && (mode == "partial" || mode == "short") {
					continue // The new active file is empty.
				}
				t.Run(action+"/"+stage+"/"+mode, func(t *testing.T) {
					f := newDiskFaultFixture(t, action == "compact")
					ctx := context.Background()
					tx, err := f.store.begin(ctx)
					if err != nil {
						t.Fatal(err)
					}
					before := storeFileNames(t, f.store.Dir)
					injectStageFault(tx, func(name string) bool {
						switch stage {
						case "intent":
							return strings.HasSuffix(name, ".intent.json")
						case "marker":
							return strings.HasPrefix(name, evidenceStagePrefix)
						default:
							return strings.HasPrefix(name, stage)
						}
					}, mode)
					_, err = tx.maintain(ctx, *f.clock, action)
					tx.close()
					code(t, err, "state_io")
					if got := storeFileNames(t, f.store.Dir); !reflect.DeepEqual(got, before) {
						t.Fatalf("staging leaked changes: got %v, want %v", got, before)
					}
					f.assertHistory(t)
				})
			}
		}
	}
}

func storeFileNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestInterruptedPendingMarkerWritePreservesAuthority(t *testing.T) {
	f := newDiskFaultFixture(t, false)
	path := filepath.Join(f.store.Dir, "journal.jsonl")
	before := readFaultFile(t, path)
	cmd, input, _ := startReservationProcess(t, f.store, f.request, f.policy, f.archive, "crash-marker")
	if _, err := io.WriteString(input, "x"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err == nil || cmd.ProcessState.ExitCode() != 86 {
		t.Fatal("expected interrupted marker write", err)
	}
	if !bytes.Equal(before, readFaultFile(t, path)) {
		t.Fatal("interrupted marker write changed journal")
	}
	_, err := f.store.InspectRepair(context.Background())
	code(t, err, "repair_unavailable")
	f.assertHistory(t)
}

func TestDiskFullBeforeImageRetries(t *testing.T) {
	for _, mode := range []string{"create", "zero", "partial", "short", "sync", "exit"} {
		t.Run(mode, func(t *testing.T) {
			f := newDiskFaultFixture(t, true)
			ctx := context.Background()
			tearPublication(t, f.store, "compact", *f.clock)
			plan, err := f.store.InspectRepair(ctx)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(f.store.Dir, "journal.jsonl")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "exit" {
				cmd, input, _ := startReservationProcess(t, f.store, f.request, f.policy, f.archive, "crash-before-image")
				if _, err = io.WriteString(input, "x"); err != nil {
					t.Fatal(err)
				}
				if err = cmd.Wait(); err == nil || cmd.ProcessState.ExitCode() != 86 {
					t.Fatal("expected interrupted evidence write", err)
				}
			} else {
				tx, err := f.store.openLocked(ctx)
				if err != nil {
					t.Fatal(err)
				}
				proof, err := tx.proveRepair(ctx, *f.clock)
				if err != nil {
					tx.close()
					t.Fatal(err)
				}
				injectStageFault(tx, func(string) bool { return true }, mode)
				_, err = tx.repair(ctx, proof, repairExpectation(plan))
				tx.close()
				code(t, err, "state_io")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(after, before) {
				t.Fatal("failed evidence write changed control", err)
			}
			_, err = os.Lstat(filepath.Join(f.store.Dir, "repair-control-"+plan.ControlSHA256+".bin"))
			if !os.IsNotExist(err) {
				t.Fatalf("final before-image exists after interrupted staging: %v", err)
			}
			_, err = f.store.RecoveryStatus(ctx, "developer")
			code(t, err, "publication_pending")
			result, err := f.store.Repair(ctx, repairExpectation(plan))
			if err != nil {
				t.Fatal(err)
			}
			saved, err := os.ReadFile(filepath.Join(f.store.Dir, result.BeforeImage))
			if err != nil || !bytes.Equal(saved, before) {
				t.Fatal("repair failed to save exact evidence", err)
			}
			f.assertHistory(t)
		})
	}
}

func TestDiskFullOwnershipAppendFailsClosed(t *testing.T) {
	for _, format := range []string{"legacy", "segmented"} {
		for _, mode := range []string{"zero", "partial", "short", "sync"} {
			t.Run(format+"/"+mode, func(t *testing.T) {
				f := newDiskFaultFixture(t, format == "segmented")
				ctx := context.Background()
				tx, err := f.store.begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(f.store.Dir, tx.fileName)
				before := readFaultFile(t, path)
				tx.file = diskFaultFile{tx.file, mode}
				err = tx.append(ctx, event{Kind: "recover", At: stamp(*f.clock), ID: f.owner.ReservationID, Owner: &f.owner, Stopped: true})
				tx.close()
				code(t, err, "commit_uncertain")
				after := readFaultFile(t, path)
				if !bytes.HasPrefix(after, before) {
					t.Fatal("fault changed earlier events")
				}
				if mode == "zero" {
					if !bytes.Equal(after, before) {
						t.Fatal("zero-byte failure changed active history")
					}
					f.assertHistory(t)
					return
				}
				if mode != "sync" {
					if len(after) <= len(before) {
						t.Fatal("expected a torn append")
					}
					_, err = f.store.Reserve(ctx, "developer", f.policy, f.request, f.archive)
					code(t, err, "state_corrupt")
					_, err = f.store.CheckOwner(ctx, "developer", f.owner, f.policy, f.archive)
					code(t, err, "state_corrupt")
					_, err = f.store.InspectRepair(ctx)
					code(t, err, "repair_unavailable")
					if !bytes.Equal(after, readFaultFile(t, path)) {
						t.Fatal("failed checks rewrote a torn event")
					}
					return
				}
				// After a full write with an uncertain sync, reopening must see the
				// exact recovery tuple. Retrying recovery must not append twice.
				view, err := f.store.RecoveryStatus(ctx, "developer")
				if err != nil || view.JournalSequence != f.before.JournalSequence+1 || len(view.Reservations) != 1 || view.Reservations[0].Status != "recovered" || !f.owner.matches(view.Reservations[0]) {
					t.Fatalf("lost complete recovery: %+v, %v", view, err)
				}
				if _, err = f.store.Recover(ctx, "developer", f.owner, true); err != nil {
					t.Fatal(err)
				}
				_, err = f.store.Reserve(ctx, "developer", f.policy, f.request, f.archive)
				code(t, err, "replay_detected")
				if !bytes.Equal(after, readFaultFile(t, path)) {
					t.Fatal("retry appended to recovered history")
				}
			})
		}
	}
}

func TestDiskFullControlWriteRetainsRepairEvidence(t *testing.T) {
	for _, action := range []string{"migrate", "compact"} {
		for _, mode := range []string{"zero", "partial", "short", "sync"} {
			t.Run(action+"/"+mode, func(t *testing.T) {
				f := newDiskFaultFixture(t, action == "compact")
				ctx := context.Background()
				tx, err := f.store.begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if tx.control == nil {
					tx.file = diskFaultFile{tx.file, mode}
				} else {
					tx.control = diskFaultFile{tx.control, mode}
				}
				_, err = tx.maintain(ctx, *f.clock, action)
				tx.close()
				code(t, err, "commit_uncertain")
				_, err = f.store.RecoveryStatus(ctx, "developer")
				code(t, err, "publication_pending")
				plan, err := f.store.InspectRepair(ctx)
				if err != nil {
					t.Fatal(err)
				}
				want := "torn"
				if mode == "sync" {
					want = "published"
				}
				if plan.ControlState != want {
					t.Fatalf("got control state %s, want %s", plan.ControlState, want)
				}
				result, err := f.store.Repair(ctx, repairExpectation(plan))
				if err != nil || result.ControlRewritten != (mode != "sync") {
					t.Fatalf("repair did not finalize the recorded successor: %+v, %v", result, err)
				}
				f.assertHistory(t)
			})
		}
	}
}

func readFaultFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
