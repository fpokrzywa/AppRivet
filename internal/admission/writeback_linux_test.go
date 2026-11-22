package admission

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"syscall"
	"testing"
	"time"
)

func TestLinuxJournalWriteback(t *testing.T) {
	dir := os.Getenv("APPRIVET_WB_TEST_DIR")
	if dir == "" {
		t.Skip("run .github/scripts/linux-journal-writeback.sh on Linux")
	}
	checkExt4TestDir(t, dir)
	t.Setenv("TMPDIR", dir)
	t.Run("append_fsync", func(t *testing.T) { testNativeAppendWriteback(t, false) })
	t.Run("append_retry_cached", func(t *testing.T) { testNativeAppendWriteback(t, true) })
	t.Run("repair_fsync", testNativeRepairWriteback)
}

func runWritebackControl(t *testing.T, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", append([]string{os.Getenv("APPRIVET_WB_SCRIPT"), "control"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("writeback control %v: %v\n%s", args, err, out)
	}
}

// filefrag reports the logical and physical offsets in the selected unit.
// Require one initialized 4 KiB extent so no metadata or unrelated data is hit.
var singleDataBlock = regexp.MustCompile(`(?m)^\s*0:\s*0\.\.\s*0:\s*([1-9][0-9]*)\.\.\s*[0-9]+:\s*1:\s+last,eof\s*$`)

func writebackBlock(t *testing.T, path string) string {
	t.Helper()
	cmd := exec.Command("filefrag", "-v", "-b4096", path)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.CombinedOutput()
	match := singleDataBlock.FindSubmatch(out)
	if err != nil || match == nil {
		t.Fatalf("expected one initialized data block: %v\n%s", err, out)
	}
	return string(match[1])
}

func newWritebackFixture(t *testing.T) (diskFaultFixture, string) {
	t.Helper()
	ctx := context.Background()
	f := newDiskFaultFixture(t, true)
	if _, err := f.store.Compact(ctx); err != nil {
		t.Fatal(err)
	}
	// Put the claim and replay keys in sealed history, and an already synced
	// observation in the first active block. The next event fits in that block.
	*f.clock = f.clock.Add(time.Second)
	if _, err := f.store.Status(ctx, "developer", ""); err != nil {
		t.Fatal(err)
	}
	var err error
	f.before, err = f.store.RecoveryStatus(ctx, "developer")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.store.begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.close()
	return f, filepath.Join(f.store.Dir, tx.fileName)
}

func invokeWritebackAppend(t *testing.T, f diskFaultFixture, probe ext4JournalProbe) error {
	t.Helper()
	ctx := context.Background()
	tx, err := f.store.begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.close()
	probe.journalFile = tx.file
	tx.file = probe
	return tx.append(ctx, event{Kind: "recover", At: stamp(*f.clock), ID: f.owner.ReservationID, Owner: &f.owner, Stopped: true})
}

func testNativeAppendWriteback(t *testing.T, retryCached bool) {
	f, path := newWritebackFixture(t)
	ctx := context.Background()
	prefix := readFaultFile(t, path)
	block := writebackBlock(t, path)
	var nativeErr error
	var record []byte
	err := invokeWritebackAppend(t, f, ext4JournalProbe{
		write: func(file journalFile, body []byte) (int, error) {
			if len(prefix)+len(body) > 4096 {
				t.Fatal("append does not fit in the faulted data block")
			}
			record = bytes.Clone(body)
			runWritebackControl(t, "fault", block)
			n, err := file.Write(body)
			if err != nil || n != len(body) {
				runWritebackControl(t, "restore")
				t.Fatalf("buffered write failed before fsync: %d, %v", n, err)
			}
			return n, nil
		},
		sync: func(file journalFile) error {
			defer runWritebackControl(t, "restore")
			nativeErr = file.Sync()
			return nativeErr
		},
	})
	if !errors.Is(nativeErr, syscall.EIO) {
		t.Fatalf("expected native journal fsync EIO, got %v", nativeErr)
	}
	t.Logf("append fsync returned native EIO: %v", nativeErr)
	code(t, err, "commit_uncertain")
	marker := filepath.Join(f.store.Dir, appendPendingName)
	evidence := readFaultFile(t, marker)
	intent, e, err := parseAppendIntent(evidence)
	if err != nil || !bytes.Equal(intent.Record, record) || e.Previous != f.before.JournalSHA256 {
		t.Fatal("native failure lost the exact intended event", err)
	}
	// A reopened descriptor may still hit cached data. Remount for an
	// independent read of what the backing filesystem actually persisted.
	if visible, err := os.ReadFile(path); err == nil {
		t.Logf("before remount exact complete event visible: %t", bytes.Equal(visible, append(bytes.Clone(prefix), record...)))
	}
	if retryCached {
		// On this kernel, the full event may still be visible after failed
		// writeback. Successful repair must make those bytes durable today.
		plan, err := f.store.InspectAppendRepair(ctx)
		if err == nil {
			result, err := f.store.RepairAppend(ctx, AppendRepairExpect{plan.IntentSHA256, plan.ActiveSHA256})
			if err != nil || result.JournalSHA256 != e.Hash || result.BytesWritten+result.BytesReasserted != len(record) {
				t.Fatal("could not sync the verified event after device restoration", result, err)
			}
			runWritebackControl(t, "remount")
			if !bytes.Equal(readFaultFile(t, path), append(bytes.Clone(prefix), record...)) {
				t.Fatal("repair acknowledged a cached event that did not survive remount")
			}
			assertExt4Repaired(t, f, path, e.Hash)
			t.Logf("same-mount retry persisted the event after native EIO: %d identical bytes reasserted", result.BytesReasserted)
			return
		}
		t.Logf("same-mount inspection refused the evidence: %v", err)
	}
	runWritebackControl(t, "remount")
	actual := readFaultFile(t, path)
	if !bytes.HasPrefix(actual, prefix) || !bytes.Equal(evidence, readFaultFile(t, marker)) {
		t.Fatal("writeback fault changed acknowledged history or intent")
	}
	if bytes.HasPrefix(record, actual[len(prefix):]) {
		t.Logf("after remount exact event prefix persisted: %d/%d bytes", len(actual)-len(prefix), len(record))
		result := repairPendingAppend(t, f.store)
		if result.JournalSHA256 != e.Hash || !bytes.Equal(readFaultFile(t, path), append(bytes.Clone(prefix), record...)) {
			t.Fatal("repair did not complete exactly the recorded event")
		}
		runWritebackControl(t, "remount")
		assertExt4Repaired(t, f, path, e.Hash)
		return
	}
	// Storage may persist the new length and lose its data write. Such bytes
	// are outside the exact-prefix constraint and must never be guessed at.
	t.Log("after remount the persisted tail diverged; repair remains blocked")
	_, err = f.store.InspectAppendRepair(ctx)
	code(t, err, "repair_refused")
	_, err = f.store.RepairAppend(ctx, AppendRepairExpect{sum(evidence), sum(actual)})
	code(t, err, "repair_refused")
	_, err = f.store.Reserve(ctx, "developer", f.policy, f.request, f.archive)
	code(t, err, "append_pending")
	_, err = f.store.CheckOwner(ctx, "developer", f.owner, f.policy, f.archive)
	code(t, err, "append_pending")
	if !bytes.Equal(actual, readFaultFile(t, path)) || !bytes.Equal(evidence, readFaultFile(t, marker)) {
		t.Fatal("a refused repair changed evidence")
	}
}

func testNativeRepairWriteback(t *testing.T) {
	f, path := newWritebackFixture(t)
	ctx := context.Background()
	// Model a lost acknowledgement after a genuinely durable append. This
	// keeps the exact pending event on disk before we inject the device fault.
	err := invokeWritebackAppend(t, f, ext4JournalProbe{sync: func(file journalFile) error {
		if err := file.Sync(); err != nil {
			t.Fatal(err)
		}
		return syscall.EINTR
	}})
	code(t, err, "commit_uncertain")
	plan, pin := inspectExt4Pending(t, f)
	actual := readFaultFile(t, path)
	marker := filepath.Join(f.store.Dir, appendPendingName)
	evidence := readFaultFile(t, marker)
	block := writebackBlock(t, path)
	var nativeErr error
	func() {
		tx, err := f.store.openLocked(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.close()
		proof, err := tx.proveAppend(ctx, *f.clock)
		if err != nil || proof.report.PresentBytes != proof.report.EventBytes {
			t.Fatalf("expected a fully persisted pending event: %v", err)
		}
		tx.file = ext4JournalProbe{
			journalFile: tx.file,
			write: func(file journalFile, data []byte) (int, error) {
				if !bytes.Equal(data, actual[len(actual)-plan.EventBytes:]) {
					t.Fatal("repair tried to write past the recorded event")
				}
				runWritebackControl(t, "fault", block)
				n, err := file.Write(data)
				if err != nil || n != len(data) {
					runWritebackControl(t, "restore")
					t.Fatalf("buffered repair write failed before fsync: %d, %v", n, err)
				}
				return n, nil
			},
			sync: func(file journalFile) error {
				defer runWritebackControl(t, "restore")
				nativeErr = file.Sync()
				return nativeErr
			},
		}
		_, err = tx.repairAppend(ctx, proof, pin)
		code(t, err, "commit_uncertain")
	}()
	if !errors.Is(nativeErr, syscall.EIO) {
		t.Fatalf("expected native repair fsync EIO, got %v", nativeErr)
	}
	t.Logf("repair fsync returned native EIO: %v", nativeErr)
	runWritebackControl(t, "remount")
	if !bytes.Equal(actual, readFaultFile(t, path)) || !bytes.Equal(evidence, readFaultFile(t, marker)) {
		t.Fatal("failed re-sync changed durable event bytes or intent")
	}
	result := repairPendingAppend(t, f.store)
	if result.BytesWritten != 0 || result.BytesReasserted != plan.EventBytes || result.JournalSHA256 != plan.JournalSHA256 {
		t.Fatal("retry did not finalize the exact complete event", result)
	}
	runWritebackControl(t, "remount")
	assertExt4Repaired(t, f, path, plan.JournalSHA256)
	if !bytes.Equal(actual, readFaultFile(t, path)) {
		t.Fatal("native sync retry rewrote journal history")
	}
}
