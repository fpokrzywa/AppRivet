package admission

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestDiskFullLinuxTmpfs(t *testing.T) {
	mount, enabled := os.LookupEnv("APPRIVET_ENOSPC_TMPFS")
	if !enabled {
		t.Skip("set APPRIVET_ENOSPC_TMPFS to a dedicated 1 MiB tmpfs mount")
	}
	var fs syscall.Statfs_t
	if err := syscall.Statfs(mount, &fs); err != nil {
		t.Fatal(err)
	}
	// Refuse to fill a regular disk, a shared /dev/shm, or an oversized mount.
	const tmpfsMagic = 0x01021994
	if fs.Type != tmpfsMagic || fs.Blocks == 0 || fs.Bsize <= 0 || fs.Blocks > (1<<20)/uint64(fs.Bsize) {
		t.Fatal("requires an isolated tmpfs of at most 1 MiB")
	}
	var current, parent syscall.Stat_t
	if err := syscall.Stat(mount, &current); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Stat(filepath.Dir(filepath.Clean(mount)), &parent); err != nil {
		t.Fatal(err)
	}
	if current.Dev == parent.Dev {
		t.Fatal("requires the root of a dedicated tmpfs mount")
	}
	entries, err := os.ReadDir(mount)
	if err != nil || len(entries) != 0 {
		t.Fatal("tmpfs must start empty", err)
	}
	t.Setenv("TMPDIR", mount)
	ctx := context.Background()
	t.Run("migration", func(t *testing.T) {
		f := newDiskFaultFixture(t, false)
		free := fillFaultTmpfs(t, f.store.Dir)
		_, err := f.store.Migrate(ctx)
		code(t, err, "state_io")
		f.assertHistory(t)
		free()
		if _, err = f.store.Migrate(ctx); err != nil {
			t.Fatal(err)
		}
		f.assertHistory(t)
	})
	t.Run("repair", func(t *testing.T) {
		f := newDiskFaultFixture(t, true)
		tearPublication(t, f.store, "compact", *f.clock)
		plan, err := f.store.InspectRepair(ctx)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(f.store.Dir, "journal.jsonl")
		control := readFaultFile(t, path)
		free := fillFaultTmpfs(t, f.store.Dir)
		_, err = f.store.Repair(ctx, repairExpectation(plan))
		code(t, err, "state_io")
		if !bytes.Equal(control, readFaultFile(t, path)) {
			t.Fatal("ENOSPC changed torn control")
		}
		_, err = f.store.RecoveryStatus(ctx, "developer")
		code(t, err, "publication_pending")
		free()
		result, err := f.store.Repair(ctx, repairExpectation(plan))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(control, readFaultFile(t, filepath.Join(f.store.Dir, result.BeforeImage))) {
			t.Fatal("before-image changed")
		}
		f.assertHistory(t)
	})
	t.Run("ownership_append", func(t *testing.T) {
		f := newDiskFaultFixture(t, true)
		// Seal the claim so the next recovery must allocate an active block.
		if _, err := f.store.Compact(ctx); err != nil {
			t.Fatal(err)
		}
		free := fillFaultTmpfs(t, f.store.Dir)
		_, err := f.store.Recover(ctx, "developer", f.owner, true)
		code(t, err, "commit_uncertain")
		f.assertHistory(t)
		free()
		receipt, err := f.store.Recover(ctx, "developer", f.owner, true)
		if err != nil || len(receipt.Reservations) != 1 || receipt.Reservations[0].Status != "recovered" || !f.owner.matches(receipt.Reservations[0]) {
			t.Fatalf("failed to recover exact owner: %+v, %v", receipt, err)
		}
		_, err = f.store.Reserve(ctx, "developer", f.policy, f.request, f.archive)
		code(t, err, "replay_detected")
	})
}

func fillFaultTmpfs(t *testing.T, dir string) func() {
	t.Helper()
	f, err := os.CreateTemp(dir, "enospc-filler-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(f.Name()) })
	defer f.Close()
	// Allocate real pages. A sparse truncate would leave tmpfs capacity unused.
	data := make([]byte, 64<<10)
	for written := 0; written <= 1<<20; {
		n, err := f.Write(data)
		if errors.Is(err, syscall.ENOSPC) {
			t.Logf("kernel returned ENOSPC after %d filler bytes", written+n)
			return func() {
				t.Helper()
				if err := os.Remove(f.Name()); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err != nil {
			t.Fatal(err)
		}
		written += n
	}
	t.Fatal("filesystem did not report ENOSPC within the write budget")
	return nil
}
