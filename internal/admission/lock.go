package admission

import (
	"context"
	"os"
	"time"
)

func lockFile(ctx context.Context, f *os.File) error {
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	retry := time.NewTicker(25 * time.Millisecond)
	defer retry.Stop()
	for {
		if err := cancelled(ctx); err != nil {
			return err
		}
		locked, err := tryLockFile(f)
		if err != nil {
			return failure("state_io", "kernel file locking is unavailable")
		}
		if locked {
			return nil
		}
		select {
		case <-ctx.Done():
			return cancelled(ctx)
		case <-deadline.C:
			return failure("state_busy", "reservation store is busy; no stale lock was removed")
		case <-retry.C:
		}
	}
}
