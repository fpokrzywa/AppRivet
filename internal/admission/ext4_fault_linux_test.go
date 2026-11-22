package admission

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

const ext4TestBytes = 32 << 20

func TestLinuxExt4JournalFaults(t *testing.T) {
	dir := os.Getenv("APPRIVET_EXT4_TEST_DIR")
	if dir == "" {
		t.Skip("run .github/scripts/linux-journal-faults.sh on Linux")
	}
	checkExt4TestDir(t, dir)
	t.Setenv("TMPDIR", dir)
	switch os.Getenv("APPRIVET_EXT4_TEST_MODE") {
	case "quota":
		t.Run("quota_repair", testExt4QuotaRepair)
	case "full":
		t.Run("native_sync", func(t *testing.T) { testExt4FullSync(t, false) })
		t.Run("injected_sync_error", func(t *testing.T) { testExt4FullSync(t, true) })
	default:
		t.Fatal("APPRIVET_EXT4_TEST_MODE must be quota or full")
	}
}

func checkExt4TestDir(t *testing.T, dir string) {
	t.Helper()
	root := filepath.Dir(dir)
	const ext4Magic = 0xef53
	fs := ext4Space(t, dir)
	if os.Geteuid() == 0 || !filepath.IsAbs(dir) || fs.Type != ext4Magic || fs.Bsize != 4096 || fs.Blocks == 0 || fs.Blocks > ext4TestBytes/4096 {
		t.Fatal("requires an unprivileged user on a dedicated ext4 image up to 32 MiB")
	}
	var work, parent, outer syscall.Stat_t
	for name, info := range map[string]*syscall.Stat_t{dir: &work, root: &parent, filepath.Dir(root): &outer} {
		if err := syscall.Stat(name, info); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(dir)
	if work.Dev != parent.Dev || parent.Dev == outer.Dev || work.Uid != uint32(os.Geteuid()) || err != nil || len(entries) != 0 {
		t.Fatal("requires an empty owned directory immediately under the image mount", err)
	}
}

func ext4Space(t *testing.T, path string) syscall.Statfs_t {
	t.Helper()
	var fs syscall.Statfs_t
	if err := syscall.Statfs(path, &fs); err != nil {
		t.Fatal(err)
	}
	return fs
}

// Reserve real extents instead of extending a sparse file. The retry at one
// block distinguishes actual exhaustion from a request larger than the space.
func exhaustExt4(t *testing.T, dir string, want syscall.Errno) func() {
	t.Helper()
	f, err := os.CreateTemp(dir, "ext4-filler-")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	t.Cleanup(func() { _ = os.Remove(f.Name()) })
	const block = 4096
	step := int64(256 << 10)
	var offset int64
	for offset < ext4TestBytes {
		err = syscall.Fallocate(int(f.Fd()), 0, offset, step)
		switch {
		case err == nil:
			offset += step
		case !errors.Is(err, want):
			t.Fatalf("allocating extents: got %v, want %v", err, want)
		case step != block:
			step = block
		default:
			t.Logf("kernel fallocate returned %v at one block with %d available filesystem blocks", err, ext4Space(t, dir).Bavail)
			return func() {
				if err := os.Remove(f.Name()); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	t.Fatal("out of extent allocation budget without kernel exhaustion")
	return nil
}

type ext4JournalProbe struct {
	journalFile
	write func(journalFile, []byte) (int, error)
	sync  func(journalFile) error
}

func (p ext4JournalProbe) Write(b []byte) (int, error) {
	if p.write != nil {
		return p.write(p.journalFile, b)
	}
	return p.journalFile.Write(b)
}

func (p ext4JournalProbe) Sync() error {
	if p.sync != nil {
		return p.sync(p.journalFile)
	}
	return p.journalFile.Sync()
}

func appendExt4Recovery(t *testing.T, f diskFaultFixture, probe ext4JournalProbe) (string, error) {
	t.Helper()
	ctx := context.Background()
	// Keep the claim in sealed history so its recovery needs a new data block.
	if _, err := f.store.Compact(ctx); err != nil {
		t.Fatal(err)
	}
	tx, err := f.store.begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.close()
	if tx.size != 0 {
		t.Fatal("expected an empty append segment")
	}
	probe.journalFile = tx.file
	tx.file = probe
	err = tx.append(ctx, event{Kind: "recover", At: stamp(*f.clock), ID: f.owner.ReservationID, Owner: &f.owner, Stopped: true})
	return filepath.Join(f.store.Dir, tx.fileName), err
}

func testExt4QuotaRepair(t *testing.T) {
	ctx := context.Background()
	f := newDiskFaultFixture(t, true)
	var free func()
	var writeErr error
	var written int
	path, err := appendExt4Recovery(t, f, ext4JournalProbe{write: func(file journalFile, body []byte) (int, error) {
		// The append intent is already synced at this point.
		free = exhaustExt4(t, f.store.Dir, syscall.EDQUOT)
		if ext4Space(t, f.store.Dir).Bavail < 256 {
			t.Fatal("expected spare global capacity under the user quota")
		}
		written, writeErr = file.Write(body)
		return written, writeErr
	}})
	code(t, err, "commit_uncertain")
	if written != 0 || !errors.Is(writeErr, syscall.EDQUOT) {
		t.Fatalf("journal write returned %d, %v, want zero bytes and EDQUOT", written, writeErr)
	}
	t.Logf("journal write returned kernel %v with free filesystem capacity", writeErr)
	plan, expect := inspectExt4Pending(t, f)
	_, err = f.store.RepairAppend(ctx, expect)
	code(t, err, "commit_uncertain")
	stillPending, sameExpect := inspectExt4Pending(t, f)
	if stillPending.PresentBytes != 0 || sameExpect != expect || len(readFaultFile(t, path)) != 0 {
		t.Fatal("quota failure changed the pending event or active bytes")
	}
	free()
	result, err := f.store.RepairAppend(ctx, expect)
	if err != nil || result.BytesWritten != plan.EventBytes {
		t.Fatal(result, err)
	}
	assertExt4Repaired(t, f, path, plan.JournalSHA256)
}

func testExt4FullSync(t *testing.T, inject bool) {
	ctx := context.Background()
	f := newDiskFaultFixture(t, true)
	var free func()
	var nativeErr error
	var record []byte
	path, err := appendExt4Recovery(t, f, ext4JournalProbe{
		write: func(file journalFile, body []byte) (int, error) {
			record = bytes.Clone(body)
			return file.Write(body)
		},
		sync: func(file journalFile) error {
			// Fill the device after the append write and before its actual fsync.
			free = exhaustExt4(t, f.store.Dir, syscall.ENOSPC)
			nativeErr = file.Sync()
			t.Logf("native journal fsync on full ext4 returned: %v", nativeErr)
			if nativeErr == nil && inject {
				t.Log("injecting ENOSPC at the application sync boundary")
				return syscall.ENOSPC
			}
			return nativeErr
		},
	})
	if free == nil || len(record) == 0 {
		t.Fatal("test did not reach fsync after an append write", err)
	}
	if nativeErr != nil && !errors.Is(nativeErr, syscall.ENOSPC) {
		t.Fatal("unexpected native fsync failure", nativeErr)
	}
	if nativeErr == nil && !inject && err != nil {
		t.Fatal("successful native fsync did not finalize the append", err)
	}
	// Hash of the intended canonical event, independent of the repair receipt.
	var intended event
	if err := json.Unmarshal(record, &intended); err != nil {
		t.Fatal(err)
	}
	if err == nil {
		if inject || nativeErr != nil {
			t.Fatal("sync failure was acknowledged")
		}
		free()
	} else {
		code(t, err, "commit_uncertain")
		plan, expect := inspectExt4Pending(t, f)
		if plan.JournalSHA256 != intended.Hash {
			t.Fatal("sync failure changed the intended event")
		}
		if nativeErr != nil {
			free()
		} else if plan.PresentBytes != plan.EventBytes {
			t.Fatal("injected sync error followed a complete successful native write and sync")
		}
		result, err := f.store.RepairAppend(ctx, expect)
		if err != nil || (nativeErr == nil && result.BytesWritten != 0) {
			t.Fatal(result, err)
		}
		if nativeErr == nil {
			free()
		}
	}
	if !bytes.Equal(record, readFaultFile(t, path)) {
		t.Fatal("a sync failure duplicated or changed the recovery event")
	}
	assertExt4Repaired(t, f, path, intended.Hash)
}

func inspectExt4Pending(t *testing.T, f diskFaultFixture) (AppendRepairReport, AppendRepairExpect) {
	t.Helper()
	ctx := context.Background()
	_, err := f.store.RecoveryStatus(ctx, "developer")
	code(t, err, "append_pending")
	_, err = f.store.Reserve(ctx, "developer", f.policy, f.request, f.archive)
	code(t, err, "append_pending")
	_, err = f.store.CheckOwner(ctx, "developer", f.owner, f.policy, f.archive)
	code(t, err, "append_pending")
	plan, err := f.store.InspectAppendRepair(ctx)
	if err != nil || plan.EventKind != "recover" || plan.JournalSequence != f.before.JournalSequence+1 {
		t.Fatal(plan, err)
	}
	return plan, AppendRepairExpect{plan.IntentSHA256, plan.ActiveSHA256}
}

func assertExt4Repaired(t *testing.T, f diskFaultFixture, active, eventHash string) {
	t.Helper()
	ctx := context.Background()
	after, err := f.store.RecoveryStatus(ctx, "developer")
	if err != nil || after.JournalSequence != f.before.JournalSequence+1 || after.JournalSHA256 != eventHash || after.RecordedAt != f.before.RecordedAt || len(after.Reservations) != 1 || !f.owner.matches(after.Reservations[0]) || after.Reservations[0].Status != "recovered" {
		t.Fatal(after, err)
	}
	beforeRetry := readFaultFile(t, active)
	if _, err := f.store.Recover(ctx, "developer", f.owner, true); err != nil {
		t.Fatal(err)
	}
	_, err = f.store.CheckOwner(ctx, "developer", f.owner, f.policy, f.archive)
	code(t, err, "ownership_conflict")
	_, err = f.store.Reserve(ctx, "developer", f.policy, f.request, f.archive)
	code(t, err, "replay_detected")
	req := f.request
	req.RequestID = "another-request"
	_, err = f.store.Reserve(ctx, "developer", f.policy, req, f.archive)
	code(t, err, "replay_detected")
	if !bytes.Equal(beforeRetry, readFaultFile(t, active)) {
		t.Fatal("retry, stale owner or replay appended another event")
	}
}
