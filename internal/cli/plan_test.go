package cli

import "testing"

func TestPlanStopsBeforeRequestsWhenDesktopHasExited(t *testing.T) {
	t.Setenv("APPRIVET_PLAN_OWNER_PID", "-1")
	ctx, cancel := planContext(t.Context())
	defer cancel()
	if ctx.Err() == nil {
		t.Fatal("plan context remained active after its desktop owner exited")
	}
}
