package cli

import (
	"context"
	"os"
	"strconv"
	"time"
)

// The desktop app starts a separate process for a plan. Stop further provider
// requests if the app quits while one of those processes is running.
func planContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	owner := os.Getenv("APPRIVET_PLAN_OWNER_PID")
	if owner == "" {
		return ctx, cancel
	}
	if strconv.Itoa(os.Getppid()) != owner {
		cancel()
		return ctx, cancel
	}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if strconv.Itoa(os.Getppid()) != owner {
					cancel()
					return
				}
			}
		}
	}()
	return ctx, cancel
}
